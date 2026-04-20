package repository

import (
	"errors"
	"fmt"
	"math"
	"supply-chain/internal/model"
	"supply-chain/pkg/sqlutil"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BillingRepo struct {
	db *gorm.DB
}

func NewBillingRepo(db *gorm.DB) *BillingRepo {
	return &BillingRepo{db: db}
}

// ---------- Wallet ----------

func (r *BillingRepo) GetWalletByAccountID(accountID uint64) (*model.Wallet, error) {
	var w model.Wallet
	err := r.db.Where("account_id = ?", accountID).First(&w).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// GetWalletByAccountIDForUpdate fetches a wallet within a transaction with a SELECT ... FOR UPDATE
// row lock so concurrent deductions / recharges / refunds serialize on the same row instead of
// reading stale balances and overwriting each other (lost update).
func (r *BillingRepo) GetWalletByAccountIDForUpdate(tx *gorm.DB, accountID uint64) (*model.Wallet, error) {
	var w model.Wallet
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ?", accountID).First(&w).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *BillingRepo) CreateWallet(w *model.Wallet) error {
	return r.db.Create(w).Error
}

// UpdateWalletBalance updates balance within an existing transaction.
func (r *BillingRepo) UpdateWalletBalance(tx *gorm.DB, accountID uint64, newBalance float64) error {
	return tx.Model(&model.Wallet{}).
		Where("account_id = ?", accountID).
		Update("balance", newBalance).Error
}

// GetAllWallets returns all wallets (for monthly discount refresh).
func (r *BillingRepo) GetAllWallets() ([]model.Wallet, error) {
	var wallets []model.Wallet
	err := r.db.Find(&wallets).Error
	return wallets, err
}

// UpdateWalletDiscount refreshes discount info for a wallet.
func (r *BillingRepo) UpdateWalletDiscount(accountID uint64, rate float64, level string, snapshot, lastMonthSpending float64) error {
	return r.db.Model(&model.Wallet{}).Where("account_id = ?", accountID).Updates(map[string]interface{}{
		"discount_rate":       rate,
		"level":               level,
		"balance_snapshot":    snapshot,
		"last_month_spending": lastMonthSpending,
	}).Error
}

// GetLastMonthSpending sums actual_amount of successful deductions in the previous calendar month.
func (r *BillingRepo) GetLastMonthSpending(accountID uint64) (float64, error) {
	now := time.Now()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	firstOfLastMonth := firstOfThisMonth.AddDate(0, -1, 0)

	var total float64
	err := r.db.Model(&model.BillingRecord{}).
		Where("account_id = ? AND type = 'deduct' AND status = 'success' AND created_at >= ? AND created_at < ?",
			accountID, firstOfLastMonth, firstOfThisMonth).
		Select("COALESCE(SUM(actual_amount), 0)").
		Scan(&total).Error
	return total, err
}

// LockDiscountInTx updates discount_rate and level within an existing transaction.
// Used to atomically lock the first-deduction discount alongside the deduction itself.
func (r *BillingRepo) LockDiscountInTx(tx *gorm.DB, accountID uint64, rate float64, level string) error {
	return tx.Model(&model.Wallet{}).Where("account_id = ?", accountID).
		Updates(map[string]interface{}{
			"discount_rate": rate,
			"level":         level,
		}).Error
}

// GetCurrentMonthRechargeTotal sums approved recharges for the current month (new-account discount).
func (r *BillingRepo) GetCurrentMonthRechargeTotal(accountID uint64) (float64, error) {
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var total float64
	err := r.db.Model(&model.RechargeRequest{}).
		Where("account_id = ? AND status = 'approved' AND created_at >= ?", accountID, firstOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error
	return total, err
}

// ---------- RechargeRequest ----------

func (r *BillingRepo) IsTransactionNoExist(txNo string) (bool, error) {
	var count int64
	err := r.db.Model(&model.RechargeRequest{}).Where("transaction_no = ?", txNo).Count(&count).Error
	return count > 0, err
}

func (r *BillingRepo) CreateRechargeRequest(req *model.RechargeRequest) error {
	return r.db.Create(req).Error
}

// ---------- BillingRecord ----------

func (r *BillingRepo) FlowNoExists(flowNo string) (bool, error) {
	var count int64
	err := r.db.Model(&model.BillingRecord{}).Where("flow_no = ?", flowNo).Count(&count).Error
	return count > 0, err
}

// DeleteRetryableRecord removes an error/insufficient billing record so it can be retried.
// Returns true if a record was deleted (i.e., it was retryable).
func (r *BillingRepo) DeleteRetryableRecord(flowNo string) (bool, error) {
	result := r.db.Where("flow_no = ? AND status IN ?", flowNo, []string{"error", "insufficient"}).
		Delete(&model.BillingRecord{})
	return result.RowsAffected > 0, result.Error
}

func (r *BillingRepo) CreateBillingRecord(tx *gorm.DB, rec *model.BillingRecord) error {
	return tx.Create(rec).Error
}

// GetDeductionRecord fetches the successful deduction record by flow_no.
func (r *BillingRepo) GetDeductionRecord(flowNo string) (*model.BillingRecord, error) {
	var rec model.BillingRecord
	err := r.db.Where("flow_no = ? AND status = ?", flowNo, "success").First(&rec).Error
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// ProcessRefund credits the wallet and creates a refund billing record in a single transaction.
// Also updates order billing_status to BillingStatusRefunded(4).
func (r *BillingRepo) ProcessRefund(accountID uint64, tradeUID, tradeNo, platform, shopName, flowNo string, amount float64, markApprovedAt *time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// FOR UPDATE so this refund serializes with concurrent deductions / recharges on the same wallet.
		w, err := r.GetWalletByAccountIDForUpdate(tx, accountID)
		if err != nil {
			return fmt.Errorf("get wallet: %w", err)
		}
		newBalance := math.Round((w.Balance+amount)*100) / 100

		rec := &model.BillingRecord{
			FlowNo:         flowNo,
			AccountID:      accountID,
			TradeNo:        tradeNo,
			TradeUID:       tradeUID,
			Platform:       platform,
			ShopName:       shopName,
			Type:           "refund",
			ActualAmount:   amount,
			Status:         "success",
			BalanceBefore:  w.Balance,
			BalanceAfter:   newBalance,
			MarkApprovedAt: markApprovedAt,
		}

		if err := r.CreateBillingRecord(tx, rec); err != nil {
			return err
		}
		if err := r.UpdateWalletBalance(tx, accountID, newBalance); err != nil {
			return err
		}
		return tx.Model(&model.OrderTrade{}).
			Where("uid = ? AND billing_status = ?", tradeUID, model.BillingStatusSuccess).
			Update("billing_status", model.BillingStatusRefunded).Error
	})
}

func (r *BillingRepo) DB() *gorm.DB { return r.db }

// ---------- Admin: Finance Overview ----------

// GetTotalBalance returns the sum of all wallet balances.
func (r *BillingRepo) GetTotalBalance() (float64, error) {
	var total float64
	err := r.db.Model(&model.Wallet{}).Select("COALESCE(SUM(balance), 0)").Scan(&total).Error
	return total, err
}

// GetTodayApprovedRechargeTotal returns the sum of approved recharges today.
func (r *BillingRepo) GetTodayApprovedRechargeTotal() (float64, error) {
	today := time.Now().Format("2006-01-02")
	var total float64
	err := r.db.Model(&model.RechargeRequest{}).
		Where("status = 'approved' AND DATE(created_at) = ?", today).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error
	return total, err
}

// ---------- Admin: Recharge Requests ----------

// ListRechargeRequests returns paginated recharge requests with optional status filter.
func (r *BillingRepo) ListRechargeRequests(req *model.AdminRechargeListReq) ([]model.RechargeRequest, int64, error) {
	q := r.db.Model(&model.RechargeRequest{})
	if req.Status != "" {
		q = q.Where("status = ?", req.Status)
	}
	if req.StartDate != "" {
		q = q.Where("created_at >= ?", req.StartDate+" 00:00:00")
	}
	if req.EndDate != "" {
		q = q.Where("created_at <= ?", req.EndDate+" 23:59:59")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var records []model.RechargeRequest
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}

// GetRechargeRequestByID returns a single recharge request by ID.
func (r *BillingRepo) GetRechargeRequestByID(id uint64) (*model.RechargeRequest, error) {
	var req model.RechargeRequest
	if err := r.db.First(&req, id).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

// ApproveRecharge approves a pending recharge request in a single transaction.
// It adds the amount to the wallet balance and writes a billing_record.
func (r *BillingRepo) ApproveRecharge(rechargeID uint64, accountID uint64, amount float64, flowNo string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Atomic status check: only update if still pending
		result := tx.Model(&model.RechargeRequest{}).
			Where("id = ? AND status = 'pending'", rechargeID).
			Update("status", "approved")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("该充值申请已处理或不存在")
		}

		// Get or create wallet (FOR UPDATE locks the row so concurrent deduct/refund/recharge serialize)
		var balanceBefore, balanceAfter float64
		var w model.Wallet
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("account_id = ?", accountID).First(&w).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			balanceBefore = 0
			balanceAfter = math.Round(amount*100) / 100
			rate, level := model.CalcDiscount(amount)
			if err := tx.Create(&model.Wallet{
				AccountID:    accountID,
				Balance:      balanceAfter,
				DiscountRate: rate,
				Level:        level,
			}).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			balanceBefore = w.Balance
			balanceAfter = math.Round((w.Balance+amount)*100) / 100
			if err := tx.Model(&model.Wallet{}).
				Where("account_id = ?", accountID).
				Update("balance", balanceAfter).Error; err != nil {
				return err
			}
		}

		// Write billing record
		return tx.Create(&model.BillingRecord{
			FlowNo:        flowNo,
			AccountID:     accountID,
			Type:          "recharge",
			Status:        "success",
			ActualAmount:  amount,
			DiscountRate:  1.0,
			BalanceBefore: balanceBefore,
			BalanceAfter:  balanceAfter,
		}).Error
	})
}

// RejectRecharge rejects a pending recharge request.
func (r *BillingRepo) RejectRecharge(id uint64, remark string) error {
	result := r.db.Model(&model.RechargeRequest{}).
		Where("id = ? AND status = 'pending'", id).
		Updates(map[string]interface{}{"status": "rejected", "remark": remark})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("该充值申请已处理或不存在")
	}
	return nil
}

// ---------- Admin: All Billing Records ----------

// AccountBasic holds minimal account info for display.
type AccountBasic struct {
	ID       uint64
	Username string
	RealName string
}

// GetAccountInfoByIDs fetches username and real_name for a set of account IDs.
func (r *BillingRepo) GetAccountInfoByIDs(ids []uint64) (map[uint64]AccountBasic, error) {
	result := make(map[uint64]AccountBasic)
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := r.db.Table("account").Select("id, username, real_name").Where("id IN ?", ids).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a AccountBasic
		if err := rows.Scan(&a.ID, &a.Username, &a.RealName); err != nil {
			continue
		}
		result[a.ID] = a
	}
	return result, nil
}

// ListAllBillingRecords returns paginated billing records for all accounts (admin view).
// Only successful records are returned (excludes error/insufficient deductions).
func (r *BillingRepo) ListAllBillingRecords(req *model.AdminBillingListReq) ([]model.BillingRecord, int64, error) {
	q := r.db.Model(&model.BillingRecord{}).Where("status = ?", "success")
	if req.StartDate != "" {
		q = q.Where("created_at >= ?", req.StartDate+" 00:00:00")
	}
	if req.EndDate != "" {
		q = q.Where("created_at <= ?", req.EndDate+" 23:59:59")
	}
	if req.Type != "" {
		q = q.Where("type = ?", req.Type)
	}
	if req.AccountID > 0 {
		q = q.Where("account_id = ?", req.AccountID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var records []model.BillingRecord
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}

// GetAllBillingRecordsForExport returns all records matching filters (no pagination, for Excel export).
// Only successful records are exported (excludes error/insufficient deductions).
func (r *BillingRepo) GetAllBillingRecordsForExport(req *model.AdminBillingListReq) ([]model.BillingRecord, error) {
	q := r.db.Model(&model.BillingRecord{}).Where("status = ?", "success")
	if req.StartDate != "" {
		q = q.Where("created_at >= ?", req.StartDate+" 00:00:00")
	}
	if req.EndDate != "" {
		q = q.Where("created_at <= ?", req.EndDate+" 23:59:59")
	}
	if req.Type != "" {
		q = q.Where("type = ?", req.Type)
	}
	if req.AccountID > 0 {
		q = q.Where("account_id = ?", req.AccountID)
	}
	var records []model.BillingRecord
	err := q.Order("created_at DESC").Find(&records).Error
	return records, err
}

// ---------- Role-aware helpers ----------

func (r *BillingRepo) GetAccountShopIDs(accountID uint64) ([]uint64, error) {
	var shopIDs []uint64
	err := r.db.Table("account_shop").
		Where("account_id = ?", accountID).
		Pluck("shop_id", &shopIDs).Error
	return shopIDs, err
}

func (r *BillingRepo) GetEmployeeAccountIDsByShopIDs(shopIDs []uint64) ([]uint64, error) {
	if len(shopIDs) == 0 {
		return nil, nil
	}
	var ids []uint64
	err := r.db.Table("account_shop").
		Select("DISTINCT account_shop.account_id").
		Joins("JOIN account ON account.id = account_shop.account_id").
		Where("account_shop.shop_id IN ? AND account.role = ?", shopIDs, model.RoleEmployee).
		Pluck("account_shop.account_id", &ids).Error
	return ids, err
}

func (r *BillingRepo) GetAllEmployeeAccountIDs() ([]uint64, error) {
	var ids []uint64
	err := r.db.Table("account").
		Where("role = ?", model.RoleEmployee).
		Pluck("id", &ids).Error
	return ids, err
}

func (r *BillingRepo) GetWalletsByAccountIDs(accountIDs []uint64) ([]model.Wallet, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	var wallets []model.Wallet
	err := r.db.Where("account_id IN ?", accountIDs).Find(&wallets).Error
	return wallets, err
}

// ---------- Employee: Own Recharge Records ----------

// ListRechargeRequestsByAccountIDs returns the recharge history for the given accounts.
func (r *BillingRepo) ListRechargeRequestsByAccountIDs(accountIDs []uint64, page, pageSize int) ([]model.RechargeRequest, int64, error) {
	q := r.db.Model(&model.RechargeRequest{}).Where("account_id IN ?", accountIDs)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var records []model.RechargeRequest
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}

// ListBillingRecords queries billing records with filters.
// Insufficient-balance attempts are permanently excluded from the customer view.
func (r *BillingRepo) ListBillingRecords(req *model.BillingListReq, accountIDs []uint64) ([]model.BillingRecord, int64, error) {
	q := r.db.Model(&model.BillingRecord{}).Where("account_id IN ? AND status != 'insufficient' AND NOT (status = 'error' AND actual_amount = 0)", accountIDs)

	// Keyword: order number or flow_no
	if req.Keyword != "" {
		kw := sqlutil.EscapeLike(req.Keyword)
		q = q.Where("trade_no LIKE ? OR flow_no LIKE ?", kw, kw)
	}

	// Time filter — preset days take priority over explicit dates
	if req.Days > 0 {
		since := time.Now().AddDate(0, 0, -req.Days)
		q = q.Where("created_at >= ?", since)
	} else {
		if req.StartDate != "" {
			q = q.Where("created_at >= ?", req.StartDate+" 00:00:00")
		}
		if req.EndDate != "" {
			q = q.Where("created_at <= ?", req.EndDate+" 23:59:59")
		}
	}

	if req.Platform != "" {
		q = q.Where("platform = ?", req.Platform)
	}
	if req.ShopName != "" {
		q = q.Where("shop_name = ?", req.ShopName)
	}
	// Status filter: "success" = deduct success, "refund" = refund success, "error" = price/barcode error.
	// "insufficient" is never shown to customers (excluded at query base level).
	switch req.Status {
	case "success":
		q = q.Where("type = 'deduct' AND status = 'success'")
	case "refund":
		q = q.Where("type = 'refund' AND status = 'success'")
	case "error":
		q = q.Where("status = 'error'")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var records []model.BillingRecord
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}

// GetBillingRecordsForExport returns all records matching the same filters as
// ListBillingRecords but without pagination (for Excel export).
func (r *BillingRepo) GetBillingRecordsForExport(req *model.BillingListReq, accountIDs []uint64) ([]model.BillingRecord, error) {
	q := r.db.Model(&model.BillingRecord{}).Where("account_id IN ? AND status != 'insufficient' AND NOT (status = 'error' AND actual_amount = 0)", accountIDs)

	if req.Keyword != "" {
		kw := sqlutil.EscapeLike(req.Keyword)
		q = q.Where("trade_no LIKE ? OR flow_no LIKE ?", kw, kw)
	}
	if req.Days > 0 {
		since := time.Now().AddDate(0, 0, -req.Days)
		q = q.Where("created_at >= ?", since)
	} else {
		if req.StartDate != "" {
			q = q.Where("created_at >= ?", req.StartDate+" 00:00:00")
		}
		if req.EndDate != "" {
			q = q.Where("created_at <= ?", req.EndDate+" 23:59:59")
		}
	}
	if req.Platform != "" {
		q = q.Where("platform = ?", req.Platform)
	}
	if req.ShopName != "" {
		q = q.Where("shop_name = ?", req.ShopName)
	}
	switch req.Status {
	case "success":
		q = q.Where("type = 'deduct' AND status = 'success'")
	case "refund":
		q = q.Where("type = 'refund' AND status = 'success'")
	case "error":
		q = q.Where("status = 'error'")
	}

	var records []model.BillingRecord
	err := q.Order("created_at DESC").Find(&records).Error
	return records, err
}
