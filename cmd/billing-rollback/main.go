// Billing Rollback Tool - 零风险修复并发扣款"丢失更新"产生的超额扣款记录。
//
// 工作原理（时间序回放）：
//   把一个账号的所有 billing_record / warehouse_billing_record 按 created_at 排序，
//   从 0 开始重新计算余额：
//     - recharge/refund 累加
//     - deduct 如果当前余额 >= 扣款额 -> KEEP
//     - deduct 如果当前余额 <  扣款额 -> REVERSE (就是被 race 放进来的超额扣款)
//   回放结束后，balance 就是正确的钱包余额；REVERSE 集合就是要回滚的记录。
//
// 四种模式：
//   -mode=detect  仅读：打印 race 分组统计（探测规模）
//   -mode=plan    仅读+写计划表：生成 billing_rollback_plan，告诉你每笔记录 KEEP/REVERSE
//   -mode=verify  仅读：对比计划表 vs 当前 wallet，输出每个账号的修复后余额
//   -mode=apply   写入：按计划执行 —— DELETE reversed 的 billing_record、重置订单状态、修正 wallet.balance（全部放一个事务 + FOR UPDATE 锁）
//
// 全部操作有事务保护，apply 需要显式 --confirm。计划表保留全部历史，可审计可追溯。
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"time"

	"supply-chain/internal/config"
	"supply-chain/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// RollbackPlan 记录每一笔 billing_record / warehouse_billing_record 的决策。
// 一个账号一次 plan 用同一个 Batch 标识。apply 会按 Batch 读取并执行。
type RollbackPlan struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	Batch         string    `gorm:"type:varchar(32);not null;index;comment:计划批次号"`
	System        string    `gorm:"type:varchar(16);not null;index;comment:billing|warehouse"`
	RecordID      uint64    `gorm:"not null;index;comment:原记录ID"`
	AccountID     uint64    `gorm:"not null;index;comment:账号ID"`
	TradeUID      string    `gorm:"type:varchar(64);index;comment:订单UID"`
	TradeNo       string    `gorm:"type:varchar(64);comment:订单号"`
	FlowNo        string    `gorm:"type:varchar(128);comment:流水号"`
	Type          string    `gorm:"type:varchar(16);comment:deduct/refund/recharge"`
	Status        string    `gorm:"type:varchar(16);comment:原status"`
	ActualAmount  float64   `gorm:"type:decimal(12,2);comment:金额"`
	RecordTime    time.Time `gorm:"comment:原记录created_at"`
	BalanceBefore float64   `gorm:"type:decimal(12,2);comment:原balance_before"`
	BalanceAfter  float64   `gorm:"type:decimal(12,2);comment:原balance_after"`
	SimulBalance  float64   `gorm:"type:decimal(12,2);comment:回放后应有余额"`
	Decision      string    `gorm:"type:varchar(16);index;comment:KEEP|REVERSE"`
	Reason        string    `gorm:"type:varchar(255);comment:决策原因"`
	Executed      bool      `gorm:"default:false;index"`
	ExecutedAt    *time.Time
	CreatedAt     time.Time
}

func (RollbackPlan) TableName() string { return "billing_rollback_plan" }

// AccountSummary 每个账号回放后的结果汇总（写 plan 时顺便落盘）。
type AccountSummary struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement"`
	Batch             string    `gorm:"type:varchar(32);not null;index"`
	System            string    `gorm:"type:varchar(16);not null;index"`
	AccountID         uint64    `gorm:"not null;index"`
	CurrentBalance    float64   `gorm:"type:decimal(12,2);comment:当前钱包余额"`
	CorrectBalance    float64   `gorm:"type:decimal(12,2);comment:回放后应有余额"`
	BalanceDelta      float64   `gorm:"type:decimal(12,2);comment:修正金额(+表示加回)"`
	KeepCount         int       `gorm:"comment:保留记录数"`
	ReverseCount      int       `gorm:"comment:回滚记录数"`
	ReverseAmount     float64   `gorm:"type:decimal(12,2);comment:回滚总金额"`
	Executed          bool      `gorm:"default:false"`
	ExecutedAt        *time.Time
	CreatedAt         time.Time
}

func (AccountSummary) TableName() string { return "billing_rollback_summary" }

// Event 是一个账号在时间序回放时的统一事件结构。
// billing 和 warehouse 两套数据都映射到这里。
type Event struct {
	RecordID     uint64
	System       string // "billing" | "warehouse"
	Type         string // "deduct" | "refund" | "recharge"
	Status       string
	Amount       float64
	CreatedAt    time.Time
	TradeUID     string
	TradeNo      string
	FlowNo       string
	BalBefore    float64
	BalAfter     float64
}

func main() {
	mode := flag.String("mode", "", "detect | plan | verify | apply")
	system := flag.String("system", "billing", "billing | warehouse | both")
	account := flag.Uint64("account", 0, "限定单个 account_id (0=全部)")
	confirm := flag.Bool("confirm", false, "apply 模式必须加此参数")
	batch := flag.String("batch", "", "apply/verify 模式使用的 batch 号；plan 模式自动生成")
	configPath := flag.String("config", "configs/config.json", "config.json 路径")
	flag.Parse()

	if *mode == "" {
		fmt.Println("usage: billing-rollback -mode=<detect|plan|verify|apply> [-system=billing|warehouse|both] [-account=<id>] [-batch=<batch>] [-confirm]")
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("[Config] %s not found, using defaults: %v", *configPath, err)
		cfg = config.DefaultConfig()
	}

	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}

	if err := db.AutoMigrate(&RollbackPlan{}, &AccountSummary{}); err != nil {
		log.Fatalf("migrate plan table: %v", err)
	}

	systems := resolveSystems(*system)

	switch *mode {
	case "detect":
		for _, s := range systems {
			runDetect(db, s, *account)
		}
	case "plan":
		b := *batch
		if b == "" {
			b = fmt.Sprintf("B%s", time.Now().Format("20060102-150405"))
		}
		for _, s := range systems {
			runPlan(db, s, *account, b)
		}
		fmt.Printf("\n>>> plan written with batch=%s\n", b)
		fmt.Printf(">>> next step: billing-rollback -mode=verify -batch=%s\n", b)
	case "verify":
		if *batch == "" {
			log.Fatal("verify 模式需要 -batch 参数")
		}
		for _, s := range systems {
			runVerify(db, s, *batch, *account)
		}
	case "apply":
		if !*confirm {
			log.Fatal("apply 模式必须加 -confirm 参数")
		}
		if *batch == "" {
			log.Fatal("apply 模式必须指定 -batch（先用 plan 生成）")
		}
		for _, s := range systems {
			runApply(db, s, *batch, *account)
		}
	case "rebalance":
		if !*confirm {
			log.Fatal("rebalance 模式必须加 -confirm 参数")
		}
		for _, s := range systems {
			runRebalance(db, s, *account)
		}
	default:
		log.Fatalf("unknown mode: %s", *mode)
	}
}

func resolveSystems(s string) []string {
	switch s {
	case "both":
		return []string{"billing", "warehouse"}
	case "billing", "warehouse":
		return []string{s}
	default:
		log.Fatalf("unknown system: %s", s)
		return nil
	}
}

// ---------- detect ----------

func runDetect(db *gorm.DB, system string, accountID uint64) {
	fmt.Printf("\n========== DETECT [%s] ==========\n", system)
	var rows []struct {
		AccountID     uint64
		BalanceBefore float64
		RaceCount     int
		SumAmount     float64
		FirstAt       time.Time
		LastAt        time.Time
	}

	table, typeCol, amountCol := tableColumns(system)
	q := db.Table(table).
		Select(fmt.Sprintf("account_id, balance_before, COUNT(*) AS race_count, SUM(%s) AS sum_amount, MIN(created_at) AS first_at, MAX(created_at) AS last_at", amountCol)).
		Where(fmt.Sprintf("%s = ? AND status = ?", typeCol), "deduct", "success").
		Group("account_id, balance_before").
		Having("COUNT(*) > 1").
		Order("account_id, first_at")
	if accountID > 0 {
		q = q.Where("account_id = ?", accountID)
	}
	if err := q.Scan(&rows).Error; err != nil {
		log.Fatalf("detect query: %v", err)
	}

	if len(rows) == 0 {
		fmt.Println("No race groups detected.")
		return
	}
	fmt.Printf("%-12s %-14s %-8s %-12s %-20s %-20s\n", "account_id", "balance_before", "count", "sum_amount", "first_at", "last_at")
	var totalRecs int
	var totalAmt float64
	accountSet := map[uint64]struct{}{}
	for _, r := range rows {
		fmt.Printf("%-12d %-14.2f %-8d %-12.2f %-20s %-20s\n",
			r.AccountID, r.BalanceBefore, r.RaceCount, r.SumAmount,
			r.FirstAt.Format("2006-01-02 15:04:05"), r.LastAt.Format("2006-01-02 15:04:05"))
		totalRecs += r.RaceCount
		totalAmt += r.SumAmount
		accountSet[r.AccountID] = struct{}{}
	}
	fmt.Printf("\nTOTAL: %d race groups, %d records, %d accounts affected, sum=%.2f\n",
		len(rows), totalRecs, len(accountSet), totalAmt)
}

// ---------- plan ----------

func runPlan(db *gorm.DB, system string, accountID uint64, batch string) {
	fmt.Printf("\n========== PLAN [%s] batch=%s ==========\n", system, batch)

	accounts := affectedAccounts(db, system, accountID)
	if len(accounts) == 0 {
		fmt.Println("No affected accounts.")
		return
	}
	fmt.Printf("affected accounts: %d\n", len(accounts))

	for _, acc := range accounts {
		planAccount(db, system, acc, batch)
	}
}

// affectedAccounts 返回所有存在成功扣款记录的账号（实际存在"race"模式的账号集合）。
// plan 对每个账号独立回放，不管有没有 race。这样也能捕捉到其他形式的超额扣款。
func affectedAccounts(db *gorm.DB, system string, accountID uint64) []uint64 {
	table, _, _ := tableColumns(system)
	var ids []uint64
	q := db.Table(table).Distinct("account_id").Where("status = ?", "success")
	if accountID > 0 {
		q = q.Where("account_id = ?", accountID)
	}
	if err := q.Pluck("account_id", &ids).Error; err != nil {
		log.Fatalf("list accounts: %v", err)
	}
	return ids
}

func planAccount(db *gorm.DB, system string, accountID uint64, batch string) {
	events := loadEvents(db, system, accountID)
	if len(events) == 0 {
		return
	}

	// 时间序排序（精确到微秒；同一时间再按 record_id 排稳定）
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].CreatedAt.Before(events[j].CreatedAt)
		}
		return events[i].RecordID < events[j].RecordID
	})

	balance := 0.0
	var plans []RollbackPlan
	var keepCnt, revCnt int
	var revAmt float64

	for _, e := range events {
		var decision, reason string
		switch e.Type {
		case "recharge", "refund":
			balance = round2(balance + e.Amount)
			decision = "KEEP"
			reason = fmt.Sprintf("credit %s", e.Type)
		case "deduct":
			if balance+1e-9 >= e.Amount {
				balance = round2(balance - e.Amount)
				decision = "KEEP"
				reason = "balance sufficient at replay"
			} else {
				decision = "REVERSE"
				reason = fmt.Sprintf("race over-deduct: need=%.2f, have=%.2f", e.Amount, balance)
				revCnt++
				revAmt = round2(revAmt + e.Amount)
			}
		default:
			decision = "KEEP"
			reason = "unknown type, preserved"
		}
		if decision == "KEEP" {
			keepCnt++
		}

		plans = append(plans, RollbackPlan{
			Batch:         batch,
			System:        system,
			RecordID:      e.RecordID,
			AccountID:     accountID,
			TradeUID:      e.TradeUID,
			TradeNo:       e.TradeNo,
			FlowNo:        e.FlowNo,
			Type:          e.Type,
			Status:        e.Status,
			ActualAmount:  e.Amount,
			RecordTime:    e.CreatedAt,
			BalanceBefore: e.BalBefore,
			BalanceAfter:  e.BalAfter,
			SimulBalance:  balance,
			Decision:      decision,
			Reason:        reason,
		})
	}

	curBal := currentWalletBalance(db, system, accountID)
	summary := AccountSummary{
		Batch:          batch,
		System:         system,
		AccountID:      accountID,
		CurrentBalance: curBal,
		CorrectBalance: round2(balance),
		BalanceDelta:   round2(balance - curBal),
		KeepCount:      keepCnt,
		ReverseCount:   revCnt,
		ReverseAmount:  revAmt,
	}

	// 批量插入计划 + 汇总。即便 REVERSE=0 也写入，供审计；只有真有 REVERSE 才有操作。
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.CreateInBatches(plans, 500).Error; err != nil {
			return err
		}
		return tx.Create(&summary).Error
	}); err != nil {
		log.Fatalf("save plan for account %d: %v", accountID, err)
	}
	if revCnt > 0 {
		fmt.Printf("  [%s] account=%d: current=%.2f correct=%.2f delta=%+.2f keep=%d reverse=%d reverse_amt=%.2f\n",
			system, accountID, curBal, balance, balance-curBal, keepCnt, revCnt, revAmt)
	}
}

// loadEvents 加载一个账号所有的 success 状态记录（deduct/refund/recharge）。
// warehouse 只有 deduct/recharge，没有 refund。
func loadEvents(db *gorm.DB, system string, accountID uint64) []Event {
	var events []Event
	switch system {
	case "billing":
		var recs []model.BillingRecord
		err := db.Where("account_id = ? AND status = ?", accountID, "success").Find(&recs).Error
		if err != nil {
			log.Fatalf("load billing for account %d: %v", accountID, err)
		}
		for _, r := range recs {
			events = append(events, Event{
				RecordID: r.ID, System: "billing", Type: r.Type, Status: r.Status,
				Amount: r.ActualAmount, CreatedAt: r.CreatedAt,
				TradeUID: r.TradeUID, TradeNo: r.TradeNo, FlowNo: r.FlowNo,
				BalBefore: r.BalanceBefore, BalAfter: r.BalanceAfter,
			})
		}
	case "warehouse":
		var recs []model.WarehouseBillingRecord
		err := db.Where("account_id = ? AND status = ?", accountID, "success").Find(&recs).Error
		if err != nil {
			log.Fatalf("load warehouse for account %d: %v", accountID, err)
		}
		for _, r := range recs {
			events = append(events, Event{
				RecordID: r.ID, System: "warehouse", Type: r.Type, Status: r.Status,
				Amount: r.TotalAmount, CreatedAt: r.CreatedAt,
				TradeUID: r.TradeUID, TradeNo: r.TradeNo, FlowNo: r.FlowNo,
				BalBefore: r.BalanceBefore, BalAfter: r.BalanceAfter,
			})
		}
	}
	return events
}

func currentWalletBalance(db *gorm.DB, system string, accountID uint64) float64 {
	switch system {
	case "billing":
		var w model.Wallet
		if err := db.Where("account_id = ?", accountID).First(&w).Error; err != nil {
			return 0
		}
		return w.Balance
	case "warehouse":
		var w model.WarehouseWallet
		if err := db.Where("account_id = ?", accountID).First(&w).Error; err != nil {
			return 0
		}
		return w.Balance
	}
	return 0
}

// ---------- verify ----------

func runVerify(db *gorm.DB, system, batch string, accountID uint64) {
	fmt.Printf("\n========== VERIFY [%s] batch=%s ==========\n", system, batch)
	var sums []AccountSummary
	q := db.Where("batch = ? AND `system` = ?", batch, system)
	if accountID > 0 {
		q = q.Where("account_id = ?", accountID)
	}
	if err := q.Order("account_id").Find(&sums).Error; err != nil {
		log.Fatalf("load summary: %v", err)
	}
	if len(sums) == 0 {
		fmt.Println("No plan found for this batch.")
		return
	}

	fmt.Printf("%-12s %-14s %-14s %-10s %-8s %-10s %-14s %s\n",
		"account_id", "current", "correct", "delta", "keep", "reverse", "reverse_amt", "executed")
	var totalRev int
	var totalRevAmt float64
	for _, s := range sums {
		exe := "no"
		if s.Executed {
			exe = s.ExecutedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%-12d %-14.2f %-14.2f %-+10.2f %-8d %-10d %-14.2f %s\n",
			s.AccountID, s.CurrentBalance, s.CorrectBalance, s.BalanceDelta,
			s.KeepCount, s.ReverseCount, s.ReverseAmount, exe)
		totalRev += s.ReverseCount
		totalRevAmt += s.ReverseAmount
	}
	fmt.Printf("\nTOTAL: %d accounts, %d records to reverse, amount=%.2f\n",
		len(sums), totalRev, totalRevAmt)
}

// ---------- apply ----------

func runApply(db *gorm.DB, system, batch string, accountID uint64) {
	fmt.Printf("\n========== APPLY [%s] batch=%s ==========\n", system, batch)

	var sums []AccountSummary
	q := db.Where("batch = ? AND `system` = ? AND reverse_count > 0 AND executed = ?", batch, system, false)
	if accountID > 0 {
		q = q.Where("account_id = ?", accountID)
	}
	if err := q.Order("account_id").Find(&sums).Error; err != nil {
		log.Fatalf("load pending summaries: %v", err)
	}
	if len(sums) == 0 {
		fmt.Println("Nothing to apply (no pending reverses for this batch/account).")
		return
	}

	for _, s := range sums {
		applyAccount(db, s)
	}
}

func applyAccount(db *gorm.DB, s AccountSummary) {
	var plans []RollbackPlan
	if err := db.Where("batch = ? AND `system` = ? AND account_id = ? AND decision = ? AND executed = ?",
		s.Batch, s.System, s.AccountID, "REVERSE", false).
		Order("record_time, record_id").Find(&plans).Error; err != nil {
		log.Fatalf("load plans for account %d: %v", s.AccountID, err)
	}
	if len(plans) == 0 {
		return
	}

	// 整个账号一个事务：FOR UPDATE 锁钱包 -> 删 billing_record -> 重置订单 -> 修正钱包 -> 标记计划已执行
	err := db.Transaction(func(tx *gorm.DB) error {
		// 锁钱包行（保证不会和在线扣款/充值抢锁）
		var curBal float64
		if s.System == "billing" {
			var w model.Wallet
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("account_id = ?", s.AccountID).First(&w).Error; err != nil {
				return fmt.Errorf("lock wallet: %w", err)
			}
			curBal = w.Balance
		} else {
			var w model.WarehouseWallet
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("account_id = ?", s.AccountID).First(&w).Error; err != nil {
				return fmt.Errorf("lock warehouse wallet: %w", err)
			}
			curBal = w.Balance
		}

		// 安全校验：apply 前钱包余额必须等于计划生成时的 CurrentBalance，
		// 否则说明期间有并发交易进来（比如充值），要求重新跑 plan。
		if math.Abs(curBal-s.CurrentBalance) > 0.01 {
			return fmt.Errorf("wallet balance drifted: plan_current=%.2f live_current=%.2f (请重新跑 plan)",
				s.CurrentBalance, curBal)
		}

		// 逐条回滚
		table, _, _ := tableColumns(s.System)
		for _, p := range plans {
			// 1) 删 billing_record（释放 flow_no，让订单能重新被扣款流程处理）
			if err := tx.Table(table).Where("id = ?", p.RecordID).Delete(nil).Error; err != nil {
				return fmt.Errorf("delete record %d: %w", p.RecordID, err)
			}
			// 2) 重置订单状态：billing_status=0 且 mark='已审核' / warehouse_status=0
			if p.TradeUID != "" {
				if s.System == "billing" {
					if err := tx.Table("order_trade").Where("uid = ?", p.TradeUID).
						Updates(map[string]interface{}{
							"billing_status": 0,
							"mark":           "已审核",
						}).Error; err != nil {
						return fmt.Errorf("reset order billing %s: %w", p.TradeUID, err)
					}
				} else {
					if err := tx.Table("order_trade").Where("uid = ?", p.TradeUID).
						Update("warehouse_status", 0).Error; err != nil {
						return fmt.Errorf("reset order warehouse %s: %w", p.TradeUID, err)
					}
				}
			}
			// 3) 标记计划为已执行
			now := time.Now()
			if err := tx.Model(&RollbackPlan{}).Where("id = ?", p.ID).
				Updates(map[string]interface{}{"executed": true, "executed_at": &now}).Error; err != nil {
				return fmt.Errorf("mark plan %d executed: %w", p.ID, err)
			}
		}

		// 4) 修正钱包余额 -> 回放后的正确值
		if s.System == "billing" {
			if err := tx.Model(&model.Wallet{}).Where("account_id = ?", s.AccountID).
				Update("balance", s.CorrectBalance).Error; err != nil {
				return fmt.Errorf("update wallet: %w", err)
			}
		} else {
			if err := tx.Model(&model.WarehouseWallet{}).Where("account_id = ?", s.AccountID).
				Update("balance", s.CorrectBalance).Error; err != nil {
				return fmt.Errorf("update warehouse wallet: %w", err)
			}
		}

		// 5) 标记 summary 已执行
		now := time.Now()
		if err := tx.Model(&AccountSummary{}).Where("id = ?", s.ID).
			Updates(map[string]interface{}{"executed": true, "executed_at": &now}).Error; err != nil {
			return fmt.Errorf("mark summary executed: %w", err)
		}

		return nil
	})

	if err != nil {
		fmt.Printf("  [%s] account=%d FAILED: %v\n", s.System, s.AccountID, err)
		return
	}
	fmt.Printf("  [%s] account=%d OK: balance %.2f -> %.2f, reversed %d records (%.2f)\n",
		s.System, s.AccountID, s.CurrentBalance, s.CorrectBalance, s.ReverseCount, s.ReverseAmount)
}

// ---------- rebalance ----------

// runRebalance 重新按时间序计算每笔 billing_record / warehouse_billing_record 的
// balance_before 和 balance_after，使账单页面显示的"交易后余额"和当前钱包余额保持一致。
func runRebalance(db *gorm.DB, system string, accountID uint64) {
	fmt.Printf("\n========== REBALANCE [%s] ==========\n", system)
	accounts := affectedAccounts(db, system, accountID)
	if len(accounts) == 0 {
		fmt.Println("No accounts found.")
		return
	}
	var totalUpdated int
	for _, acc := range accounts {
		n := rebalanceAccount(db, system, acc)
		totalUpdated += n
	}
	fmt.Printf("\nTOTAL: updated %d records across %d accounts\n", totalUpdated, len(accounts))
}

func rebalanceAccount(db *gorm.DB, system string, accountID uint64) int {
	events := loadEvents(db, system, accountID)
	if len(events) == 0 {
		return 0
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].CreatedAt.Before(events[j].CreatedAt)
		}
		return events[i].RecordID < events[j].RecordID
	})

	table, _, _ := tableColumns(system)
	balance := 0.0
	updated := 0

	err := db.Transaction(func(tx *gorm.DB) error {
		// 锁钱包行，防止和在线扣款并发
		if system == "billing" {
			var w model.Wallet
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("account_id = ?", accountID).First(&w).Error; err != nil {
				return fmt.Errorf("lock wallet: %w", err)
			}
		} else {
			var w model.WarehouseWallet
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("account_id = ?", accountID).First(&w).Error; err != nil {
				return fmt.Errorf("lock warehouse wallet: %w", err)
			}
		}

		for _, e := range events {
			before := balance
			switch e.Type {
			case "recharge", "refund":
				balance = round2(balance + e.Amount)
			case "deduct":
				balance = round2(balance - e.Amount)
			}
			after := balance

			if math.Abs(e.BalBefore-before) > 0.001 || math.Abs(e.BalAfter-after) > 0.001 {
				if err := tx.Table(table).Where("id = ?", e.RecordID).
					Updates(map[string]interface{}{
						"balance_before": before,
						"balance_after":  after,
					}).Error; err != nil {
					return fmt.Errorf("update record %d: %w", e.RecordID, err)
				}
				updated++
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("  [%s] account=%d FAILED: %v\n", system, accountID, err)
		return 0
	}
	fmt.Printf("  [%s] account=%d: updated %d records\n", system, accountID, updated)
	return updated
}

// ---------- helpers ----------

func tableColumns(system string) (table, typeCol, amountCol string) {
	switch system {
	case "billing":
		return "billing_record", "type", "actual_amount"
	case "warehouse":
		return "warehouse_billing_record", "type", "total_amount"
	}
	return "", "", ""
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
