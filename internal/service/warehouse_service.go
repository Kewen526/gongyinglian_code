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

type WarehouseService struct {
	repo *repository.WarehouseRepo
	stop chan struct{}
}

func NewWarehouseService(repo *repository.WarehouseRepo) *WarehouseService {
	return &WarehouseService{repo: repo, stop: make(chan struct{})}
}

func (s *WarehouseService) Stop() { close(s.stop) }

// ---------- Auto deduct task ----------

func (s *WarehouseService) StartAutoDeduct() {
	go func() {
		log.Println("[Warehouse] Auto-deduct task started (interval=5m)")
		s.autoDeductOnce()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.autoDeductOnce()
			case <-s.stop:
				log.Println("[Warehouse] Auto-deduct task stopped")
				return
			}
		}
	}()
}

func (s *WarehouseService) autoDeductOnce() {
	// Process pending orders (process_status=8, warehouse_status=0)
	pending, err := s.repo.ListPendingWarehouseOrders()
	if err != nil {
		log.Printf("[Warehouse] ListPendingWarehouseOrders error: %v\n", err)
	} else {
		log.Printf("[Warehouse] Found %d pending orders to process\n", len(pending))
		for i := range pending {
			if err := s.processDeduction(&pending[i]); err != nil {
				log.Printf("[Warehouse] Deduction trade=%s error: %v\n", pending[i].TradeNo, err)
			}
		}
	}

	// Retry insufficient orders
	insufficient, err := s.repo.ListInsufficientWarehouseOrders()
	if err != nil {
		log.Printf("[Warehouse] ListInsufficientWarehouseOrders error: %v\n", err)
		return
	}
	for i := range insufficient {
		if err := s.processDeduction(&insufficient[i]); err != nil {
			log.Printf("[Warehouse] Retry trade=%s error: %v\n", insufficient[i].TradeNo, err)
		}
	}
}

// ---------- Core deduction ----------

func (s *WarehouseService) processDeduction(trade *model.OrderTrade) error {
	if trade.WarehouseStatus == model.WarehouseStatusSuccess {
		return nil
	}

	accountID, err := s.repo.ResolveEmployeeAccountID(trade.SysShop)
	if err != nil || accountID == 0 {
		log.Printf("[Warehouse] Skip trade=%s sysShop=%s: no employee found (accountID=%d, err=%v)\n", trade.TradeNo, trade.SysShop, accountID, err)
		return nil
	}

	wallet, err := s.repo.GetWalletByAccountID(accountID)
	if errors.Is(err, gorm.ErrRecordNotFound) || wallet == nil {
		log.Printf("[Warehouse] Skip trade=%s: no wallet for account=%d\n", trade.TradeNo, accountID)
		return nil
	}
	if err != nil {
		return err
	}

	// Calculate fees
	items, err := s.repo.GetItemsByTradeUID(trade.UID)
	if err != nil {
		return fmt.Errorf("获取订单商品失败: %w", err)
	}

	totalItems := 0
	for _, item := range items {
		totalItems += item.Size
	}

	shippingFee := trade.PostCost
	packingFee := calcPackingFee(totalItems)
	totalAmount := math.Round((shippingFee+packingFee)*1000) / 1000

	// Check existing record (idempotency via trade_uid unique index)
	var existing model.WarehouseBillingRecord
	if err := s.repo.DB().Where("trade_uid = ?", trade.UID).First(&existing).Error; err == nil {
		if existing.Status == "success" {
			_ = s.repo.UpdateWarehouseStatus(trade.UID, model.WarehouseStatusSuccess)
			return nil
		}
		// Delete retryable record
		s.repo.DB().Delete(&existing)
	}

	log.Printf("[Warehouse] Processing trade=%s account=%d items=%d shipping=%.3f packing=%.3f total=%.3f balance=%.3f\n",
		trade.TradeNo, accountID, totalItems, shippingFee, packingFee, totalAmount, wallet.Balance)

	if wallet.Balance < totalAmount {
		log.Printf("[Warehouse] Insufficient balance for trade=%s: need=%.3f have=%.3f\n", trade.TradeNo, totalAmount, wallet.Balance)
		_ = s.repo.UpdateWarehouseStatus(trade.UID, model.WarehouseStatusInsufficient)
		return nil
	}

	tradeTime := time.Now()
	if trade.SendTimeMs > 0 {
		tradeTime = time.UnixMilli(trade.SendTimeMs)
	}

	// Deduct in transaction with row lock so concurrent deductions / recharges on the same
	// wallet serialize. Without the lock, multiple goroutines read the same stale balance
	// and overwrite each other (lost update).
	var insufficientAfterLock bool
	var txErr error
	for attempt := 0; attempt < 5; attempt++ {
		insufficientAfterLock = false
		txErr = s.repo.DB().Transaction(func(tx *gorm.DB) error {
			w, err := s.repo.GetWalletByAccountIDForUpdate(tx, accountID)
			if err != nil {
				return err
			}
			if w.Balance < totalAmount {
				insufficientAfterLock = true
				return nil
			}
			newBalance := math.Round((w.Balance-totalAmount)*1000) / 1000

			flowNo, err := s.repo.GenerateFlowNo(tx)
			if err != nil {
				return fmt.Errorf("生成流水号失败: %w", err)
			}

			rec := &model.WarehouseBillingRecord{
				FlowNo:        flowNo,
				AccountID:     accountID,
				TradeNo:       trade.TradeNo,
				TradeUID:      trade.UID,
				Platform:      trade.SourcePlatform,
				ShopName:      trade.ShopName,
				BusinessType:  "订单发货",
				Type:          "deduct",
				ShippingFee:   shippingFee,
				PackingFee:    packingFee,
				TotalAmount:   totalAmount,
				ItemCount:     totalItems,
				BalanceBefore: w.Balance,
				BalanceAfter:  newBalance,
				Status:        "success",
				TradeTime:     &tradeTime,
			}

			if err := s.repo.CreateBillingRecord(tx, rec); err != nil {
				return err
			}
			return s.repo.UpdateWalletBalance(tx, accountID, newBalance)
		})
		if txErr == nil {
			break
		}
		if !strings.Contains(txErr.Error(), "Duplicate") {
			break
		}
		log.Printf("[Warehouse] flow_no collision for trade=%s, retry %d/5\n", trade.TradeNo, attempt+1)
	}
	if txErr != nil {
		return txErr
	}
	if insufficientAfterLock {
		log.Printf("[Warehouse] Insufficient balance after lock for trade=%s: need=%.3f\n", trade.TradeNo, totalAmount)
		_ = s.repo.UpdateWarehouseStatus(trade.UID, model.WarehouseStatusInsufficient)
		return nil
	}

	_ = s.repo.UpdateWarehouseStatus(trade.UID, model.WarehouseStatusSuccess)
	return nil
}

func calcPackingFee(itemCount int) float64 {
	if itemCount <= 0 {
		return 0
	}
	fee := 0.80 + 0.15*float64(itemCount-1)
	return math.Round(fee*1000) / 1000
}

// ---------- Employee API ----------

func (s *WarehouseService) resolveEffectiveAccountIDs(callerID uint64, role uint8) ([]uint64, error) {
	switch role {
	case model.RoleEmployee:
		return []uint64{callerID}, nil
	case model.RoleSupervisor, model.RoleTeamLead:
		shopIDs, err := s.repo.GetAccountShopIDs(callerID)
		if err != nil {
			return nil, err
		}
		return s.repo.GetEmployeeAccountIDsByShopIDs(shopIDs)
	case model.RoleSuperAdmin:
		return s.repo.GetAllEmployeeAccountIDs()
	default:
		return []uint64{callerID}, nil
	}
}

func (s *WarehouseService) GetWallet(accountID uint64, role uint8) (*model.WarehouseWalletResp, error) {
	if role == model.RoleEmployee {
		wallet, err := s.repo.GetWalletByAccountID(accountID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.WarehouseWalletResp{Balance: 0}, nil
		}
		if err != nil {
			return nil, err
		}
		return &model.WarehouseWalletResp{Balance: wallet.Balance}, nil
	}
	accountIDs, err := s.resolveEffectiveAccountIDs(accountID, role)
	if err != nil {
		return nil, err
	}
	if len(accountIDs) == 0 {
		return &model.WarehouseWalletResp{Balance: 0}, nil
	}
	wallets, err := s.repo.GetWalletsByAccountIDs(accountIDs)
	if err != nil {
		return nil, err
	}
	var total float64
	for _, w := range wallets {
		total += w.Balance
	}
	return &model.WarehouseWalletResp{
		Balance: math.Round(total*1000) / 1000,
	}, nil
}

func (s *WarehouseService) SubmitRecharge(accountID uint64, req *model.WarehouseSubmitRechargeReq) error {
	exists, err := s.repo.IsTransactionNoExist(req.TransactionNo)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("该交易流水号已提交过，请勿重复提交")
	}

	_, walletErr := s.repo.GetWalletByAccountID(accountID)
	if errors.Is(walletErr, gorm.ErrRecordNotFound) {
		if err := s.repo.DB().Create(&model.WarehouseWallet{
			AccountID: accountID,
			Balance:   0,
		}).Error; err != nil {
			return err
		}
	}

	return s.repo.CreateRechargeRequest(&model.WarehouseRechargeRequest{
		AccountID:     accountID,
		Amount:        req.Amount,
		PaymentMethod: req.PaymentMethod,
		TransactionNo: req.TransactionNo,
		VoucherURL:    req.VoucherURL,
		Status:        "pending",
	})
}

func (s *WarehouseService) ExportBillingRecords(accountID uint64, role uint8, req *model.WarehouseBillingListReq) ([]byte, error) {
	accountIDs, err := s.resolveEffectiveAccountIDs(accountID, role)
	if err != nil {
		return nil, err
	}
	records, err := s.repo.GetBillingRecordsForExport(req, accountIDs)
	if err != nil {
		return nil, err
	}
	if role != model.RoleEmployee {
		ids := make([]uint64, 0, len(records))
		for _, r := range records {
			ids = append(ids, r.AccountID)
		}
		accountMap, _ := s.repo.GetAccountInfoByIDs(ids)
		return buildWarehouseExcel(records, accountMap)
	}
	return buildMyWarehouseExcel(records)
}

func (s *WarehouseService) ListBillingRecords(accountID uint64, role uint8, req *model.WarehouseBillingListReq) (*model.WarehouseBillingListResp, error) {
	accountIDs, err := s.resolveEffectiveAccountIDs(accountID, role)
	if err != nil {
		return nil, err
	}
	records, total, err := s.repo.ListBillingRecords(req, accountIDs)
	if err != nil {
		return nil, err
	}
	wallet, _ := s.GetWallet(accountID, role)
	if wallet == nil {
		wallet = &model.WarehouseWalletResp{Balance: 0}
	}
	return &model.WarehouseBillingListResp{
		Total:  total,
		List:   records,
		Wallet: *wallet,
	}, nil
}

func (s *WarehouseService) ListMyRechargeRecords(accountID uint64, role uint8, req *model.WarehouseMyRechargeListReq) (*model.WarehouseMyRechargeListResp, error) {
	accountIDs, err := s.resolveEffectiveAccountIDs(accountID, role)
	if err != nil {
		return nil, err
	}
	records, total, err := s.repo.ListRechargeRequestsByAccountIDs(accountIDs, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}
	return &model.WarehouseMyRechargeListResp{Total: total, List: records}, nil
}

// ---------- Admin API ----------

func (s *WarehouseService) GetOverview() (*model.WarehouseOverviewResp, error) {
	total, err := s.repo.GetTotalBalance()
	if err != nil {
		return nil, err
	}
	todayRecharge, err := s.repo.GetTodayApprovedRechargeTotal()
	if err != nil {
		return nil, err
	}
	return &model.WarehouseOverviewResp{
		TotalBalance:       total,
		TodayRechargeTotal: todayRecharge,
	}, nil
}

func (s *WarehouseService) ListRechargeRequestsAdmin(req *model.WarehouseAdminRechargeListReq) (*model.WarehouseAdminRechargeListResp, error) {
	records, total, err := s.repo.ListRechargeRequests(req)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]uint64, 0, len(records))
	for _, r := range records {
		accountIDs = append(accountIDs, r.AccountID)
	}
	accountMap, _ := s.repo.GetAccountInfoByIDs(accountIDs)

	list := make([]model.WarehouseRechargeRecordResp, 0, len(records))
	for _, r := range records {
		info := accountMap[r.AccountID]
		list = append(list, model.WarehouseRechargeRecordResp{
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
	return &model.WarehouseAdminRechargeListResp{Total: total, List: list}, nil
}

func (s *WarehouseService) ApproveRecharge(rechargeID uint64) error {
	req, err := s.repo.GetRechargeRequestByID(rechargeID)
	if err != nil {
		return fmt.Errorf("充值申请不存在")
	}
	return s.repo.ApproveRecharge(rechargeID, req.AccountID, req.Amount)
}

func (s *WarehouseService) RejectRecharge(rechargeID uint64, remark string) error {
	return s.repo.RejectRecharge(rechargeID, remark)
}

func (s *WarehouseService) ListAllBillingRecords(req *model.WarehouseAdminBillingListReq) (*model.WarehouseAdminBillingListResp, error) {
	records, total, err := s.repo.ListAllBillingRecords(req)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]uint64, 0, len(records))
	for _, r := range records {
		accountIDs = append(accountIDs, r.AccountID)
	}
	accountMap, _ := s.repo.GetAccountInfoByIDs(accountIDs)

	list := make([]model.WarehouseBillingRecordWithUser, 0, len(records))
	for _, r := range records {
		info := accountMap[r.AccountID]
		list = append(list, model.WarehouseBillingRecordWithUser{
			WarehouseBillingRecord: r,
			Username:               info.Username,
			RealName:               info.RealName,
		})
	}
	return &model.WarehouseAdminBillingListResp{Total: total, List: list}, nil
}
