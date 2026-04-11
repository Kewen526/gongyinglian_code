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

// platformFallback maps platform names to their fallback aliases for price lookup.
var platformFallback = map[string]string{
	"阿里巴巴": "1688",
}

type BillingService struct {
	billingRepo *repository.BillingRepo
	orderRepo   *repository.OrderRepo
	productRepo *repository.ProductRepo
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
		spendRate, spendLevel := model.CalcDiscount(spending)
		balRate, balLevel := model.CalcDiscount(w.Balance)

		// Take the better discount (lower rate number = more discount)
		rate, level := spendRate, spendLevel
		if balRate < spendRate {
			rate, level = balRate, balLevel
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
	originalAmount, calcErr := s.calculateOrderAmount(trade.UID, trade.SourcePlatform)
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

	now := time.Now()
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
		// Price/barcode error — record but do not deduct
		rec.Status = "error"
		rec.ErrorMsg = calcErr.Error()
		rec.BalanceBefore = wallet.Balance
		rec.BalanceAfter = wallet.Balance
		_ = s.billingRepo.DB().Create(rec).Error
		_ = s.orderRepo.UpdateBillingStatus(trade.UID, model.BillingStatusError)
		return nil
	}

	// Apply discount
	discountRate := wallet.DiscountRate
	if discountRate == 0 {
		discountRate = 0.85
	}
	actual := math.Round(originalAmount*discountRate*100) / 100
	discount := math.Round((originalAmount-actual)*100) / 100

	rec.OriginalAmount = originalAmount
	rec.DiscountRate = discountRate
	rec.DiscountAmount = discount
	rec.ActualAmount = actual
	rec.BalanceBefore = wallet.Balance

	if wallet.Balance < actual {
		// Balance insufficient: update order status for retry tracking, but do NOT
		// create a billing record — keeps the customer billing view clean.
		_ = s.orderRepo.UpdateBillingStatus(trade.UID, model.BillingStatusInsufficient)
		return nil
	}

	// Deduct in transaction
	newBalance := math.Round((wallet.Balance-actual)*100) / 100
	rec.Status = "success"
	rec.BalanceAfter = newBalance
	_ = now // suppress unused warning

	txErr := s.billingRepo.DB().Transaction(func(tx *gorm.DB) error {
		if err := s.billingRepo.CreateBillingRecord(tx, rec); err != nil {
			return err
		}
		return s.billingRepo.UpdateWalletBalance(tx, accountID, newBalance)
	})
	if txErr != nil {
		return txErr
	}
	return s.orderRepo.UpdateBillingStatus(trade.UID, model.BillingStatusSuccess)
}

// calculateOrderAmount computes the total control-price amount for an order's items.
func (s *BillingService) calculateOrderAmount(tradeUID, platform string) (float64, error) {
	items, err := s.orderRepo.GetItemsByTradeUID(tradeUID)
	if err != nil {
		return 0, fmt.Errorf("获取订单商品失败: %w", err)
	}
	if len(items) == 0 {
		return 0, errors.New("订单无商品明细")
	}

	fallback := platformFallback[platform]
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

		price, found, err := s.productRepo.GetControlPrice(productCode, platform, fallback)
		if err != nil {
			return 0, fmt.Errorf("查询货号 %s 管控价格失败: %w", productCode, err)
		}
		if !found {
			return 0, fmt.Errorf("货号 %s 在平台 %s 未找到管控价格", productCode, platform)
		}

		total += price * float64(item.Size)
	}

	return math.Round(total*100) / 100, nil
}

// resolveAccountID finds the employee account assigned to the given sys_shop.
func (s *BillingService) resolveAccountID(sysShop string) (uint64, error) {
	if sysShop == "" {
		return 0, nil
	}
	var shop model.Shop
	if err := s.billingRepo.DB().Where("sys_shop = ?", sysShop).First(&shop).Error; err != nil {
		return 0, nil
	}
	var as model.AccountShop
	if err := s.billingRepo.DB().Where("shop_id = ?", shop.ID).First(&as).Error; err != nil {
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

// CanAutoReview checks whether the employee owning sysShop can afford the given order.
// Returns (canAfford, ownerAccountID). Returns (false, 0) on any lookup or price error.
// Called by the auto-review task before marking an order.
func (s *BillingService) CanAutoReview(sysShop, tradeUID, platform string) (bool, uint64) {
	accountID, err := s.resolveAccountID(sysShop)
	if err != nil || accountID == 0 {
		return false, 0
	}
	wallet, err := s.getOrSkipWallet(accountID)
	if err != nil || wallet == nil {
		return false, 0
	}
	cost, err := s.calculateOrderAmount(tradeUID, platform)
	if err != nil {
		// Price/barcode error — skip auto-review for this order; manual review required.
		return false, 0
	}
	discountRate := wallet.DiscountRate
	if discountRate == 0 {
		discountRate = 0.85
	}
	actual := math.Round(cost*discountRate*100) / 100
	return wallet.Balance >= actual, accountID
}

// ---------- Public API methods ----------

// GetWallet returns wallet info for an account; creates it on first recharge, not here.
func (s *BillingService) GetWallet(accountID uint64) (*model.WalletResp, error) {
	wallet, err := s.billingRepo.GetWalletByAccountID(accountID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &model.WalletResp{
			Balance:         0,
			DiscountRate:    0.85,
			Level:           "V1",
			DiscountDisplay: "85折",
		}, nil
	}
	if err != nil {
		return nil, err
	}
	pct := int(math.Round(wallet.DiscountRate * 100))
	return &model.WalletResp{
		Balance:         wallet.Balance,
		DiscountRate:    wallet.DiscountRate,
		Level:           wallet.Level,
		DiscountDisplay: fmt.Sprintf("%d折", pct),
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

	// Create wallet on first recharge
	_, walletErr := s.billingRepo.GetWalletByAccountID(accountID)
	if errors.Is(walletErr, gorm.ErrRecordNotFound) {
		// Determine initial discount from this recharge amount
		rate, level := model.CalcDiscount(req.Amount)
		if err := s.billingRepo.CreateWallet(&model.Wallet{
			AccountID:    accountID,
			Balance:      0,
			DiscountRate: rate,
			Level:        level,
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

// ListMyRechargeRecords returns the recharge history for the logged-in user.
func (s *BillingService) ListMyRechargeRecords(accountID uint64, req *model.MyRechargeListReq) (*model.MyRechargeListResp, error) {
	records, total, err := s.billingRepo.ListRechargeRequestsByAccountID(accountID, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}
	return &model.MyRechargeListResp{Total: total, List: records}, nil
}

// ListBillingRecords returns filtered billing records plus wallet summary.
func (s *BillingService) ListBillingRecords(accountID uint64, req *model.BillingListReq) (*model.BillingListResp, error) {
	records, total, err := s.billingRepo.ListBillingRecords(req, accountID)
	if err != nil {
		return nil, err
	}
	wallet, _ := s.GetWallet(accountID)
	if wallet == nil {
		wallet = &model.WalletResp{DiscountDisplay: "85折", Level: "V1", DiscountRate: 0.85}
	}
	return &model.BillingListResp{
		Total:  total,
		List:   records,
		Wallet: *wallet,
	}, nil
}
