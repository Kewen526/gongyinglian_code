package service

import (
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"
	"time"

	"gorm.io/gorm"
)

// makeFlowNo produces a short, deterministic flow number.
// Format: {prefix}{12 hex chars of MD5(sysShop:tradeNo)} — always 13 chars, collision-free in practice.
// Prefix: "D" = deduct, "R" = refund.
func makeFlowNo(prefix, sysShop, tradeNo string) string {
	h := md5.Sum([]byte(sysShop + ":" + tradeNo))
	return fmt.Sprintf("%s%x", prefix, h[:6])
}

// platformFallback maps order platform names to their price-table equivalents.
var platformFallback = map[string]string{}

// DeductCheckResult reports why an order can or cannot be审核'd right now.
type DeductCheckResult int

const (
	DeductOK           DeductCheckResult = iota // balance sufficient
	DeductInsufficient                          // wallet exists but balance too low
	DeductSkip                                  // no account, no wallet, etc.
	DeductBarcodeError                          // barcode not found or no control price
)

// MarkPusher pushes a batch of mark updates to the ERP (WanLiNiu).
// Wired from SyncService.BatchMarkOrders at startup.
type MarkPusher func(items []model.MarkItem) error

type BillingService struct {
	billingRepo *repository.BillingRepo
	orderRepo   *repository.OrderRepo
	productRepo *repository.ProductRepo
	markPusher  MarkPusher
	stopCh      chan struct{}
}

func NewBillingService(billingRepo *repository.BillingRepo, orderRepo *repository.OrderRepo, productRepo *repository.ProductRepo) *BillingService {
	return &BillingService{
		billingRepo: billingRepo,
		orderRepo:   orderRepo,
		productRepo: productRepo,
		stopCh:      make(chan struct{}),
	}
}

// SetMarkPusher injects the ERP mark-push function (breaks circular wiring with SyncService).
func (s *BillingService) SetMarkPusher(p MarkPusher) { s.markPusher = p }

func (s *BillingService) Stop() { close(s.stopCh) }

// ---------- Scheduled tasks ----------

// StartAutoDeduct scans every 5 minutes for unprocessed approved orders.
func (s *BillingService) StartAutoDeduct() {
	go func() {
		log.Println("[Billing] Auto-deduct task started (interval=5m)")
		s.autoDeductOnce()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.autoDeductOnce()
			case <-s.stopCh:
				log.Println("[Billing] Auto-deduct task stopped")
				return
			}
		}
	}()
}

func (s *BillingService) autoDeductOnce() {
	trades, err := s.orderRepo.ListPendingBillingOrders()
	if err != nil {
		log.Printf("[Billing] ListPendingBillingOrders error: %v\n", err)
		return
	}
	for _, trade := range trades {
		if err := s.ProcessDeduction(&trade); err != nil {
			log.Printf("[Billing] ProcessDeduction trade=%s error: %v\n", trade.TradeNo, err)
		}
	}
}

// StartAutoRefund scans every 5 minutes for orders needing a refund (process_status=99 + billing_status=1).
func (s *BillingService) StartAutoRefund() {
	go func() {
		log.Println("[Billing] Auto-refund task started (interval=5m)")
		s.autoRefundOnce()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.autoRefundOnce()
			case <-s.stopCh:
				log.Println("[Billing] Auto-refund task stopped")
				return
			}
		}
	}()
}

func (s *BillingService) autoRefundOnce() {
	trades, err := s.orderRepo.ListPendingRefundOrders()
	if err != nil {
		log.Printf("[Billing] ListPendingRefundOrders error: %v\n", err)
		return
	}
	for _, trade := range trades {
		if err := s.processRefund(&trade); err != nil {
			log.Printf("[Billing] ProcessRefund trade=%s error: %v\n", trade.TradeNo, err)
		}
	}
}

// processRefund handles the full refund for one order.
// Idempotent: skips if refund flow_no already exists or billing_status is already 4.
func (s *BillingService) processRefund(trade *model.OrderTrade) error {
	accountID, err := s.resolveAccountID(trade.SysShop)
	if err != nil || accountID == 0 {
		return nil
	}

	refundFlowNo := makeFlowNo("R", trade.SysShop, trade.TradeNo)
	if exists, _ := s.billingRepo.FlowNoExists(refundFlowNo); exists {
		return nil
	}

	deductFlowNo := makeFlowNo("D", trade.SysShop, trade.TradeNo)
	deductRec, err := s.billingRepo.GetDeductionRecord(deductFlowNo)
	if err != nil {
		return fmt.Errorf("deduction record not found for %s: %w", trade.TradeNo, err)
	}

	if err := s.billingRepo.ProcessRefund(
		accountID,
		trade.UID,
		trade.TradeNo,
		trade.SourcePlatform,
		trade.ShopName,
		refundFlowNo,
		deductRec.ActualAmount,
		trade.MarkApprovedAt,
	); err != nil {
		return err
	}

	log.Printf("[Billing] Refunded trade=%s amount=%.2f accountID=%d\n", trade.TradeNo, deductRec.ActualAmount, accountID)
	return nil
}

// StartMonthlyDiscountRefresh fires on the 1st of every month at 00:00.
func (s *BillingService) StartMonthlyDiscountRefresh() {
	go func() {
		log.Println("[Billing] Monthly discount refresh task started")
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
			timer := time.NewTimer(time.Until(next))
			select {
			case <-timer.C:
				s.refreshDiscounts()
				timer.Stop()
			case <-s.stopCh:
				timer.Stop()
				log.Println("[Billing] Monthly discount refresh task stopped")
				return
			}
		}
	}()
}

func (s *BillingService) refreshDiscounts() {
	log.Println("[Billing] Refreshing monthly discounts...")
	wallets, err := s.billingRepo.GetAllWallets()
	if err != nil {
		log.Printf("[Billing] GetAllWallets error: %v\n", err)
		return
	}
	for _, w := range wallets {
		spending, _ := s.billingRepo.GetLastMonthSpending(w.AccountID)
		var rate float64
		var level string
		if spending > 0 {
			// Last month had consumption: lock discount based on spending tier.
			rate, level = model.CalcDiscount(spending)
		} else {
			// No consumption last month: reset to no-discount; will be re-calculated
			// from current month recharge total on the first deduction.
			rate, level = 1.0, ""
		}
		_ = s.billingRepo.UpdateWalletDiscount(w.AccountID, rate, level, w.Balance, spending)
		log.Printf("[Billing] AccountID=%d level=%s rate=%.2f\n", w.AccountID, level, rate)
	}
}

// ---------- Core deduction logic ----------

// TriggerDeductionAsync triggers deduction in a goroutine (non-blocking).
func (s *BillingService) TriggerDeductionAsync(trade *model.OrderTrade) {
	go func() {
		if err := s.ProcessDeduction(trade); err != nil {
			log.Printf("[Billing] async deduction trade=%s: %v\n", trade.TradeNo, err)
		}
	}()
}

// ProcessDeduction performs the full deduction for one order.
// Already-succeeded orders are skipped. Error/insufficient orders are retried (old record overwritten).
//
// Mark state flow:
//   - insufficient → sets mark="余额不足扣款失败" + billing_status=2 (no WanLiNiu push)
//   - recovery (previous mark was "余额不足扣款失败", now sufficient) → flips mark back
//     to "已审核" in DB and pushes the mark to WanLiNiu (best-effort)
func (s *BillingService) ProcessDeduction(trade *model.OrderTrade) error {
	// Only skip if already successfully deducted
	if trade.BillingStatus == model.BillingStatusSuccess {
		return nil
	}

	// Identify the owning account via shop → account_shop
	accountID, err := s.resolveAccountID(trade.SysShop)
	if err != nil || accountID == 0 {
		// No account assigned to this shop, skip silently
		return nil
	}

	// Get or create wallet
	wallet, err := s.getOrSkipWallet(accountID)
	if err != nil || wallet == nil {
		return nil
	}

	// Compute order amount
	originalAmount, calcErr := s.calculateOrderAmount(trade.UID)
	flowNo := makeFlowNo("D", trade.SysShop, trade.TradeNo)

	// Idempotency: if flow_no exists, try to delete retryable (error/insufficient) record.
	// If it can't be deleted (means it's a success record), skip entirely.
	if exists, _ := s.billingRepo.FlowNoExists(flowNo); exists {
		deleted, err := s.billingRepo.DeleteRetryableRecord(flowNo)
		if err != nil {
			return err
		}
		if !deleted {
			// Record exists but is not retryable (already succeeded), skip
			return nil
		}
	}

	// Load the authoritative mark from DB so we can detect the recovery case
	// (mark was "余额不足扣款失败" before this retry).
	prevMark := trade.Mark
	if fresh, err := s.orderRepo.GetTradeByUID(trade.UID); err == nil && fresh != nil {
		prevMark = fresh.Mark
	}

	rec := &model.BillingRecord{
		FlowNo:         flowNo,
		AccountID:      accountID,
		TradeNo:        trade.TradeNo,
		TradeUID:       trade.UID,
		Platform:       trade.SourcePlatform,
		ShopName:       trade.ShopName,
		Type:           "deduct",
		MarkApprovedAt: trade.MarkApprovedAt,
	}

	if calcErr != nil {
		// Price/barcode error — record but do not deduct. Keep mark as-is;
		// these are investigated manually.
		rec.Status = "error"
		rec.ErrorMsg = calcErr.Error()
		rec.BalanceBefore = wallet.Balance
		rec.BalanceAfter = wallet.Balance
		_ = s.billingRepo.DB().Create(rec).Error
		_ = s.orderRepo.UpdateBillingStatus(trade.UID, model.BillingStatusError)
		return nil
	}

	// Resolve effective discount rate. If wallet has no locked discount yet (rate >= 1.0),
	// calculate from current month's total approved recharge. This gives a preview for
	// the fast-path balance check; the actual locking happens inside the transaction.
	previewRate := wallet.DiscountRate
	if previewRate <= 0 || previewRate >= 1.0 {
		monthRecharge, _ := s.billingRepo.GetCurrentMonthRechargeTotal(accountID)
		if monthRecharge > 0 {
			previewRate, _ = model.CalcDiscount(monthRecharge)
		} else {
			previewRate = 1.0
		}
	}
	previewActual := math.Round(originalAmount*previewRate*100) / 100

	rec.OriginalAmount = originalAmount

	if wallet.Balance < previewActual {
		// Fast-path insufficient check (uses preview rate — conservative but safe).
		_ = s.orderRepo.SetMarkDeductFailed(trade.UID)
		return nil
	}

	// Deduct in transaction with row lock so concurrent deductions / refunds / recharges
	// on the same wallet serialize. Without the lock, multiple goroutines read the same
	// stale balance and overwrite each other (lost update).
	rec.Status = "success"

	var insufficientAfterLock bool
	txErr := s.billingRepo.DB().Transaction(func(tx *gorm.DB) error {
		w, err := s.billingRepo.GetWalletByAccountIDForUpdate(tx, accountID)
		if err != nil {
			return err
		}

		// Re-resolve and lock discount inside the row lock so the first deduction
		// atomically writes the discount together with the balance update.
		effectiveRate := w.DiscountRate
		if effectiveRate <= 0 || effectiveRate >= 1.0 {
			monthRecharge, _ := s.billingRepo.GetCurrentMonthRechargeTotal(accountID)
			if monthRecharge > 0 {
				var level string
				effectiveRate, level = model.CalcDiscount(monthRecharge)
				if err := s.billingRepo.LockDiscountInTx(tx, accountID, effectiveRate, level); err != nil {
					return fmt.Errorf("lock discount: %w", err)
				}
			} else {
				effectiveRate = 1.0
			}
		}

		actual := math.Round(originalAmount*effectiveRate*100) / 100
		if w.Balance < actual {
			insufficientAfterLock = true
			return nil
		}
		newBalance := math.Round((w.Balance-actual)*100) / 100
		rec.DiscountRate = effectiveRate
		rec.DiscountAmount = math.Round((originalAmount-actual)*100) / 100
		rec.ActualAmount = actual
		rec.BalanceBefore = w.Balance
		rec.BalanceAfter = newBalance
		if err := s.billingRepo.CreateBillingRecord(tx, rec); err != nil {
			return err
		}
		return s.billingRepo.UpdateWalletBalance(tx, accountID, newBalance)
	})
	if txErr != nil {
		return txErr
	}
	if insufficientAfterLock {
		_ = s.orderRepo.SetMarkDeductFailed(trade.UID)
		return nil
	}
	if err := s.orderRepo.UpdateBillingStatus(trade.UID, model.BillingStatusSuccess); err != nil {
		return err
	}

	// Recovery: if this retry succeeded after a prior insufficient-balance state,
	// flip local mark back to "已审核" and push the mark to WanLiNiu so the ERP
	// resumes its shipping flow.
	if prevMark == model.MarkDeductFailed {
		now := time.Now()
		if err := s.orderRepo.RecoverMarkToApproved(trade.UID, now); err != nil {
			log.Printf("[Billing] Recover mark trade=%s: %v\n", trade.TradeNo, err)
		}
		if s.markPusher != nil {
			items := []model.MarkItem{{BillCode: trade.TradeNo, MarkName: model.MarkApproved, Type: 0}}
			if err := s.markPusher(items); err != nil {
				log.Printf("[Billing] Recovery push to WanLiNiu trade=%s: %v\n", trade.TradeNo, err)
			}
		}
	}
	return nil
}

// CheckAutoReviewEligible checks if an order can be auto-reviewed.
// Checks: shop assigned to employee → product exists → 1688 price found → balance sufficient.
func (s *BillingService) CheckAutoReviewEligible(sysShop, tradeUID string) DeductCheckResult {
	t0 := time.Now()
	accountID, err := s.resolveAccountID(sysShop)
	if d := time.Since(t0); d > 2*time.Second {
		log.Printf("[AutoReview][SLOW] resolveAccountID sysShop=%s took %v\n", sysShop, d)
	}
	if err != nil || accountID == 0 {
		return DeductSkip
	}
	t1 := time.Now()
	cost, err := s.calculateOrderAmount(tradeUID)
	if d := time.Since(t1); d > 2*time.Second {
		log.Printf("[AutoReview][SLOW] calculateOrderAmount uid=%s took %v\n", tradeUID, d)
	}
	if err != nil {
		return DeductBarcodeError
	}
	t2 := time.Now()
	wallet, err := s.getOrSkipWallet(accountID)
	if d := time.Since(t2); d > 2*time.Second {
		log.Printf("[AutoReview][SLOW] getOrSkipWallet account=%d took %v\n", accountID, d)
	}
	if err != nil || wallet == nil {
		return DeductInsufficient
	}
	discountRate := wallet.DiscountRate
	if discountRate <= 0 || discountRate >= 1.0 {
		monthRecharge, _ := s.billingRepo.GetCurrentMonthRechargeTotal(accountID)
		if monthRecharge > 0 {
			discountRate, _ = model.CalcDiscount(monthRecharge)
		} else {
			discountRate = 1.0
		}
	}
	actual := math.Round(cost*discountRate*100) / 100
	if wallet.Balance < actual {
		return DeductInsufficient
	}
	return DeductOK
}

// BatchCheckAutoReviewEligible checks all candidates in bulk using only 3 SQL queries
// (wallet + items + prices), then computes eligibility in memory.
func (s *BillingService) BatchCheckAutoReviewEligible(accountID uint64, candidates []model.OrderTrade) map[string]DeductCheckResult {
	results := make(map[string]DeductCheckResult, len(candidates))

	wallet, err := s.getOrSkipWallet(accountID)
	if err != nil || wallet == nil {
		for _, c := range candidates {
			results[c.UID] = DeductInsufficient
		}
		return results
	}
	discountRate := wallet.DiscountRate
	if discountRate <= 0 || discountRate >= 1.0 {
		monthRecharge, _ := s.billingRepo.GetCurrentMonthRechargeTotal(accountID)
		if monthRecharge > 0 {
			discountRate, _ = model.CalcDiscount(monthRecharge)
		} else {
			discountRate = 1.0
		}
	}

	tradeUIDs := make([]string, len(candidates))
	for i, c := range candidates {
		tradeUIDs[i] = c.UID
	}

	itemsMap, err := s.orderRepo.BatchGetItemsByTradeUIDs(tradeUIDs)
	if err != nil {
		log.Printf("[AutoReview] BatchGetItemsByTradeUIDs error: %v\n", err)
		for _, c := range candidates {
			results[c.UID] = DeductSkip
		}
		return results
	}

	productCodeSet := make(map[string]struct{})
	for _, items := range itemsMap {
		for _, item := range items {
			if item.BarCode == "" {
				continue
			}
			parts := strings.SplitN(item.BarCode, "-", 2)
			if code := strings.TrimSpace(parts[0]); code != "" {
				productCodeSet[code] = struct{}{}
			}
		}
	}
	productCodes := make([]string, 0, len(productCodeSet))
	for code := range productCodeSet {
		productCodes = append(productCodes, code)
	}

	priceMap, err := s.productRepo.BatchGetControlPrices(productCodes, "1688")
	if err != nil {
		log.Printf("[AutoReview] BatchGetControlPrices error: %v\n", err)
		for _, c := range candidates {
			results[c.UID] = DeductSkip
		}
		return results
	}

	for _, c := range candidates {
		items := itemsMap[c.UID]
		if len(items) == 0 {
			results[c.UID] = DeductBarcodeError
			continue
		}

		var total float64
		barcodeErr := false
		for _, item := range items {
			if item.BarCode == "" {
				barcodeErr = true
				break
			}
			parts := strings.SplitN(item.BarCode, "-", 2)
			productCode := strings.TrimSpace(parts[0])
			if productCode == "" {
				barcodeErr = true
				break
			}
			price, found := priceMap[productCode]
			if !found {
				barcodeErr = true
				break
			}
			total += price * float64(item.Size)
		}
		if barcodeErr {
			results[c.UID] = DeductBarcodeError
			continue
		}

		actual := math.Round(total*discountRate*100) / 100
		if wallet.Balance < actual {
			results[c.UID] = DeductInsufficient
		} else {
			results[c.UID] = DeductOK
		}
	}

	return results
}

// calculateOrderAmount computes the total 1688 control-price amount for an order's items.
// Always uses the 1688 platform price regardless of the order's source platform.
func (s *BillingService) calculateOrderAmount(tradeUID string) (float64, error) {
	items, err := s.orderRepo.GetItemsByTradeUID(tradeUID)
	if err != nil {
		return 0, fmt.Errorf("获取订单商品失败: %w", err)
	}
	if len(items) == 0 {
		return 0, errors.New("订单无商品明细")
	}

	var total float64

	for _, item := range items {
		// Parse bar_code — split by "-", take first segment
		if item.BarCode == "" {
			return 0, fmt.Errorf("商品 [%s / %s] 条码为空，无法解析货号", item.ItemName, item.SkuName)
		}
		parts := strings.SplitN(item.BarCode, "-", 2)
		productCode := strings.TrimSpace(parts[0])
		if productCode == "" {
			return 0, fmt.Errorf("商品 [%s / %s] 解析货号失败", item.ItemName, item.SkuName)
		}

		price, found, err := s.productRepo.GetControlPrice(productCode, "1688", "")
		if err != nil {
			return 0, fmt.Errorf("查询货号 %s 管控价格失败: %w", productCode, err)
		}
		if !found {
			return 0, fmt.Errorf("货号 %s 未找到货号或1688管控价格", productCode)
		}

		total += price * float64(item.Size)
	}

	return math.Round(total*100) / 100, nil
}

// resolveAccountID finds the employee account (role=3) assigned to the given sys_shop.
// A shop may belong to multiple accounts (team leads, supervisors share freely),
// but billing deductions always target the employee. Employee mutual exclusion
// guarantees at most one employee per shop.
func (s *BillingService) resolveAccountID(sysShop string) (uint64, error) {
	if sysShop == "" {
		return 0, nil
	}
	var shop model.Shop
	if err := s.billingRepo.DB().Where("sys_shop = ?", sysShop).First(&shop).Error; err != nil {
		return 0, nil
	}
	var as model.AccountShop
	err := s.billingRepo.DB().Table("account_shop").
		Select("account_shop.*").
		Joins("JOIN account ON account.id = account_shop.account_id").
		Where("account_shop.shop_id = ? AND account.role = ?", shop.ID, model.RoleEmployee).
		First(&as).Error
	if err != nil {
		return 0, nil
	}
	return as.AccountID, nil
}

// getOrSkipWallet returns the wallet for an account; returns nil if none exists (no wallet = not yet recharged).
func (s *BillingService) getOrSkipWallet(accountID uint64) (*model.Wallet, error) {
	wallet, err := s.billingRepo.GetWalletByAccountID(accountID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil // no wallet yet, skip
	}
	return wallet, err
}

// ---------- Public API methods ----------

// resolveEffectiveAccountIDs determines which employee account IDs the caller can see.
func (s *BillingService) resolveEffectiveAccountIDs(callerID uint64, role uint8) ([]uint64, error) {
	switch role {
	case model.RoleEmployee:
		return []uint64{callerID}, nil
	case model.RoleSupervisor, model.RoleTeamLead:
		shopIDs, err := s.billingRepo.GetAccountShopIDs(callerID)
		if err != nil {
			return nil, err
		}
		return s.billingRepo.GetEmployeeAccountIDsByShopIDs(shopIDs)
	case model.RoleSuperAdmin:
		return s.billingRepo.GetAllEmployeeAccountIDs()
	default:
		return []uint64{callerID}, nil
	}
}

// GetWallet returns wallet info. Managers see aggregated balance without level/discount.
func (s *BillingService) GetWallet(accountID uint64, role uint8) (*model.WalletResp, error) {
	if role == model.RoleEmployee {
		return s.getEmployeeWallet(accountID)
	}
	accountIDs, err := s.resolveEffectiveAccountIDs(accountID, role)
	if err != nil {
		return nil, err
	}
	if len(accountIDs) == 0 {
		return &model.WalletResp{Balance: 0}, nil
	}
	wallets, err := s.billingRepo.GetWalletsByAccountIDs(accountIDs)
	if err != nil {
		return nil, err
	}
	var total float64
	for _, w := range wallets {
		total += w.Balance
	}
	return &model.WalletResp{
		Balance: math.Round(total*100) / 100,
	}, nil
}

func (s *BillingService) getEmployeeWallet(accountID uint64) (*model.WalletResp, error) {
	wallet, err := s.billingRepo.GetWalletByAccountID(accountID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &model.WalletResp{
			Balance:         0,
			DiscountRate:    1.0,
			Level:           "",
			DiscountDisplay: "无折扣",
		}, nil
	}
	if err != nil {
		return nil, err
	}
	display := "无折扣"
	if wallet.DiscountRate < 1.0 {
		pct := int(math.Round(wallet.DiscountRate * 100))
		display = fmt.Sprintf("%d折", pct)
	}
	return &model.WalletResp{
		Balance:         wallet.Balance,
		DiscountRate:    wallet.DiscountRate,
		Level:           wallet.Level,
		DiscountDisplay: display,
	}, nil
}

// SubmitRecharge creates a pending recharge request, initialising the wallet if first time.
func (s *BillingService) SubmitRecharge(accountID uint64, req *model.SubmitRechargeReq) error {
	// Check transaction_no uniqueness
	exists, err := s.billingRepo.IsTransactionNoExist(req.TransactionNo)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("该交易流水号已提交过，请勿重复提交")
	}

	// Create wallet on first recharge (no discount yet — will be locked on first deduction).
	_, walletErr := s.billingRepo.GetWalletByAccountID(accountID)
	if errors.Is(walletErr, gorm.ErrRecordNotFound) {
		if err := s.billingRepo.CreateWallet(&model.Wallet{
			AccountID:    accountID,
			Balance:      0,
			DiscountRate: 1.0,
			Level:        "",
		}); err != nil {
			return err
		}
	}

	return s.billingRepo.CreateRechargeRequest(&model.RechargeRequest{
		AccountID:     accountID,
		Amount:        req.Amount,
		PaymentMethod: req.PaymentMethod,
		TransactionNo: req.TransactionNo,
		VoucherURL:    req.VoucherURL,
		Status:        "pending",
	})
}

// ---------- Admin API methods ----------

// GetFinanceOverview returns total wallet balance and today's approved recharge total.
func (s *BillingService) GetFinanceOverview() (*model.FinanceOverviewResp, error) {
	total, err := s.billingRepo.GetTotalBalance()
	if err != nil {
		return nil, err
	}
	todayRecharge, err := s.billingRepo.GetTodayApprovedRechargeTotal()
	if err != nil {
		return nil, err
	}
	return &model.FinanceOverviewResp{
		TotalBalance:       total,
		TodayRechargeTotal: todayRecharge,
	}, nil
}

// ListRechargeRequestsAdmin returns paginated recharge requests with account info.
func (s *BillingService) ListRechargeRequestsAdmin(req *model.AdminRechargeListReq) (*model.AdminRechargeListResp, error) {
	records, total, err := s.billingRepo.ListRechargeRequests(req)
	if err != nil {
		return nil, err
	}
	// Batch fetch account info
	accountIDs := make([]uint64, 0, len(records))
	for _, r := range records {
		accountIDs = append(accountIDs, r.AccountID)
	}
	accountMap, _ := s.billingRepo.GetAccountInfoByIDs(accountIDs)

	list := make([]model.RechargeRecordResp, 0, len(records))
	for _, r := range records {
		info := accountMap[r.AccountID]
		list = append(list, model.RechargeRecordResp{
			ID:            r.ID,
			AccountID:     r.AccountID,
			Username:      info.Username,
			RealName:      info.RealName,
			Amount:        r.Amount,
			PaymentMethod: r.PaymentMethod,
			TransactionNo: r.TransactionNo,
			VoucherURL:    r.VoucherURL,
			Status:        r.Status,
			Remark:        r.Remark,
			CreatedAt:     r.CreatedAt,
		})
	}
	return &model.AdminRechargeListResp{Total: total, List: list}, nil
}

// ApproveRecharge approves a recharge request and credits the wallet.
func (s *BillingService) ApproveRecharge(rechargeID uint64) error {
	req, err := s.billingRepo.GetRechargeRequestByID(rechargeID)
	if err != nil {
		return fmt.Errorf("充值申请不存在")
	}
	flowNo := fmt.Sprintf("RECHARGE-%d", rechargeID)
	return s.billingRepo.ApproveRecharge(rechargeID, req.AccountID, req.Amount, flowNo)
}

// RejectRecharge rejects a pending recharge request.
func (s *BillingService) RejectRecharge(rechargeID uint64, remark string) error {
	return s.billingRepo.RejectRecharge(rechargeID, remark)
}

// ListAllBillingRecords returns all billing records with account info (admin view).
func (s *BillingService) ListAllBillingRecords(req *model.AdminBillingListReq) (*model.AdminBillingListResp, error) {
	records, total, err := s.billingRepo.ListAllBillingRecords(req)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]uint64, 0, len(records))
	for _, r := range records {
		accountIDs = append(accountIDs, r.AccountID)
	}
	accountMap, _ := s.billingRepo.GetAccountInfoByIDs(accountIDs)

	list := make([]model.BillingRecordWithUser, 0, len(records))
	for _, r := range records {
		info := accountMap[r.AccountID]
		list = append(list, model.BillingRecordWithUser{
			BillingRecord: r,
			Username:      info.Username,
			RealName:      info.RealName,
		})
	}
	return &model.AdminBillingListResp{Total: total, List: list}, nil
}

// ExportBillingRecords generates an Excel file for all matching billing records.
func (s *BillingService) ExportBillingRecords(req *model.AdminBillingListReq) ([]byte, error) {
	records, err := s.billingRepo.GetAllBillingRecordsForExport(req)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]uint64, 0, len(records))
	for _, r := range records {
		accountIDs = append(accountIDs, r.AccountID)
	}
	accountMap, _ := s.billingRepo.GetAccountInfoByIDs(accountIDs)

	return buildBillingExcel(records, accountMap)
}

// ---------- Employee: own recharge records ----------

// ListMyRechargeRecords returns recharge history. Managers see records for all employees under their shops.
func (s *BillingService) ListMyRechargeRecords(accountID uint64, role uint8, req *model.MyRechargeListReq) (*model.MyRechargeListResp, error) {
	accountIDs, err := s.resolveEffectiveAccountIDs(accountID, role)
	if err != nil {
		return nil, err
	}
	records, total, err := s.billingRepo.ListRechargeRequestsByAccountIDs(accountIDs, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}
	return &model.MyRechargeListResp{Total: total, List: records}, nil
}

// ExportMyBillingRecords generates an Excel file. Managers get multi-account export with user info.
func (s *BillingService) ExportMyBillingRecords(accountID uint64, role uint8, req *model.BillingListReq) ([]byte, error) {
	accountIDs, err := s.resolveEffectiveAccountIDs(accountID, role)
	if err != nil {
		return nil, err
	}
	records, err := s.billingRepo.GetBillingRecordsForExport(req, accountIDs)
	if err != nil {
		return nil, err
	}
	if role != model.RoleEmployee {
		ids := make([]uint64, 0, len(records))
		for _, r := range records {
			ids = append(ids, r.AccountID)
		}
		accountMap, _ := s.billingRepo.GetAccountInfoByIDs(ids)
		return buildBillingExcel(records, accountMap)
	}
	return buildMyBillingExcel(records)
}

// ListBillingRecords returns filtered billing records plus wallet summary.
func (s *BillingService) ListBillingRecords(accountID uint64, role uint8, req *model.BillingListReq) (*model.BillingListResp, error) {
	accountIDs, err := s.resolveEffectiveAccountIDs(accountID, role)
	if err != nil {
		return nil, err
	}
	records, total, err := s.billingRepo.ListBillingRecords(req, accountIDs)
	if err != nil {
		return nil, err
	}
	wallet, _ := s.GetWallet(accountID, role)
	if wallet == nil {
		wallet = &model.WalletResp{DiscountDisplay: "85折", Level: "V1", DiscountRate: 0.85}
	}
	return &model.BillingListResp{
		Total:  total,
		List:   records,
		Wallet: *wallet,
	}, nil
}
