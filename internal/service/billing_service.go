package service

import (
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
// It is idempotent — if billing_status != 0 it exits immediately.
func (s *BillingService) ProcessDeduction(trade *model.OrderTrade) error {
	if trade.BillingStatus != model.BillingStatusPending {
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
	flowNo := fmt.Sprintf("%s-%s-D", trade.SysShop, trade.TradeNo)

	// Check if flow_no already exists (idempotency guard)
	if exists, _ := s.billingRepo.FlowNoExists(flowNo); exists {
		return nil
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
		rec.Status = "insufficient"
		rec.BalanceAfter = wallet.Balance
		_ = s.billingRepo.DB().Create(rec).Error
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
