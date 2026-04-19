package repository

import (
	"errors"
	"fmt"
	"math"
	"supply-chain/internal/model"
	"supply-chain/pkg/sqlutil"
	"time"

	"gorm.io/gorm"
)

type WarehouseRepo struct {
	db *gorm.DB
}

func NewWarehouseRepo(db *gorm.DB) *WarehouseRepo {
	return &WarehouseRepo{db: db}
}

func (r *WarehouseRepo) DB() *gorm.DB { return r.db }

// ---------- Wallet ----------

func (r *WarehouseRepo) GetWalletByAccountID(accountID uint64) (*model.WarehouseWallet, error) {
	var w model.WarehouseWallet
	err := r.db.Where("account_id = ?", accountID).First(&w).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *WarehouseRepo) UpdateWalletBalance(tx *gorm.DB, accountID uint64, newBalance float64) error {
	return tx.Model(&model.WarehouseWallet{}).
		Where("account_id = ?", accountID).
		Update("balance", newBalance).Error
}

// ---------- Recharge ----------

func (r *WarehouseRepo) IsTransactionNoExist(txNo string) (bool, error) {
	var count int64
	err := r.db.Model(&model.WarehouseRechargeRequest{}).Where("transaction_no = ?", txNo).Count(&count).Error
	return count > 0, err
}

func (r *WarehouseRepo) CreateRechargeRequest(req *model.WarehouseRechargeRequest) error {
	return r.db.Create(req).Error
}

func (r *WarehouseRepo) GetRechargeRequestByID(id uint64) (*model.WarehouseRechargeRequest, error) {
	var req model.WarehouseRechargeRequest
	if err := r.db.First(&req, id).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *WarehouseRepo) ApproveRecharge(rechargeID uint64, accountID uint64, amount float64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.WarehouseRechargeRequest{}).
			Where("id = ? AND status = 'pending'", rechargeID).
			Update("status", "approved")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("该充值申请已处理或不存在")
		}

		var balanceBefore, balanceAfter float64
		var w model.WarehouseWallet
		err := tx.Where("account_id = ?", accountID).First(&w).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			balanceBefore = 0
			balanceAfter = math.Round(amount*100) / 100
			if err := tx.Create(&model.WarehouseWallet{
				AccountID: accountID,
				Balance:   balanceAfter,
			}).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			balanceBefore = w.Balance
			balanceAfter = math.Round((w.Balance+amount)*100) / 100
			if err := tx.Model(&model.WarehouseWallet{}).
				Where("account_id = ?", accountID).
				Update("balance", balanceAfter).Error; err != nil {
				return err
			}
		}

		flowNo, err := r.GenerateFlowNo(tx)
		if err != nil {
			return fmt.Errorf("生成流水号失败: %w", err)
		}

		return tx.Create(&model.WarehouseBillingRecord{
			FlowNo:        flowNo,
			AccountID:     accountID,
			TradeUID:      fmt.Sprintf("RECHARGE-%d", rechargeID),
			Type:          "recharge",
			BusinessType:  "充值",
			TotalAmount:   amount,
			Status:        "success",
			BalanceBefore: balanceBefore,
			BalanceAfter:  balanceAfter,
		}).Error
	})
}

func (r *WarehouseRepo) RejectRecharge(id uint64, remark string) error {
	result := r.db.Model(&model.WarehouseRechargeRequest{}).
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

// ---------- Billing Record ----------

func (r *WarehouseRepo) CreateBillingRecord(tx *gorm.DB, rec *model.WarehouseBillingRecord) error {
	return tx.Create(rec).Error
}

// GenerateFlowNo generates the next flow number for today: CW-YYYYMMDD-NNN.
// Must be called within a transaction to avoid duplicates.
func (r *WarehouseRepo) GenerateFlowNo(tx *gorm.DB) (string, error) {
	today := time.Now().Format("20060102")
	prefix := "CW-" + today + "-"

	var maxFlowNo *string
	err := tx.Model(&model.WarehouseBillingRecord{}).
		Where("flow_no LIKE ?", prefix+"%").
		Select("MAX(flow_no)").
		Scan(&maxFlowNo).Error
	if err != nil {
		return "", err
	}

	seq := 1
	if maxFlowNo != nil && len(*maxFlowNo) >= len(prefix)+3 {
		suffix := (*maxFlowNo)[len(prefix):]
		if n, err := fmt.Sscanf(suffix, "%d", &seq); n == 1 && err == nil {
			seq++
		}
	}
	return fmt.Sprintf("%s%03d", prefix, seq), nil
}

// ---------- Queries ----------

func (r *WarehouseRepo) GetAccountShopIDs(accountID uint64) ([]uint64, error) {
	var shopIDs []uint64
	err := r.db.Table("account_shop").
		Where("account_id = ?", accountID).
		Pluck("shop_id", &shopIDs).Error
	return shopIDs, err
}

func (r *WarehouseRepo) GetEmployeeAccountIDsByShopIDs(shopIDs []uint64) ([]uint64, error) {
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

func (r *WarehouseRepo) GetAllEmployeeAccountIDs() ([]uint64, error) {
	var ids []uint64
	err := r.db.Table("account").
		Where("role = ?", model.RoleEmployee).
		Pluck("id", &ids).Error
	return ids, err
}

func (r *WarehouseRepo) GetWalletsByAccountIDs(accountIDs []uint64) ([]model.WarehouseWallet, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	var wallets []model.WarehouseWallet
	err := r.db.Where("account_id IN ?", accountIDs).Find(&wallets).Error
	return wallets, err
}

func (r *WarehouseRepo) ListBillingRecords(req *model.WarehouseBillingListReq, accountIDs []uint64) ([]model.WarehouseBillingRecord, int64, error) {
	q := r.db.Model(&model.WarehouseBillingRecord{}).Where("account_id IN ?", accountIDs)
	if req.Keyword != "" {
		kw := sqlutil.EscapeLike(req.Keyword)
		q = q.Where("trade_no LIKE ? OR flow_no LIKE ?", kw, kw)
	}
	if req.ShopName != "" {
		q = q.Where("shop_name = ?", req.ShopName)
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
	var records []model.WarehouseBillingRecord
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}

func (r *WarehouseRepo) GetBillingRecordsForExport(req *model.WarehouseBillingListReq, accountIDs []uint64) ([]model.WarehouseBillingRecord, error) {
	q := r.db.Model(&model.WarehouseBillingRecord{}).Where("account_id IN ?", accountIDs)
	if req.Keyword != "" {
		kw := sqlutil.EscapeLike(req.Keyword)
		q = q.Where("trade_no LIKE ? OR flow_no LIKE ?", kw, kw)
	}
	if req.ShopName != "" {
		q = q.Where("shop_name = ?", req.ShopName)
	}
	if req.StartDate != "" {
		q = q.Where("created_at >= ?", req.StartDate+" 00:00:00")
	}
	if req.EndDate != "" {
		q = q.Where("created_at <= ?", req.EndDate+" 23:59:59")
	}
	var records []model.WarehouseBillingRecord
	err := q.Order("created_at DESC").Find(&records).Error
	return records, err
}

func (r *WarehouseRepo) ListRechargeRequestsByAccountIDs(accountIDs []uint64, page, pageSize int) ([]model.WarehouseRechargeRequest, int64, error) {
	q := r.db.Model(&model.WarehouseRechargeRequest{}).Where("account_id IN ?", accountIDs)
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
	var records []model.WarehouseRechargeRequest
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}

// ---------- Admin Queries ----------

func (r *WarehouseRepo) GetTotalBalance() (float64, error) {
	var total float64
	err := r.db.Model(&model.WarehouseWallet{}).Select("COALESCE(SUM(balance), 0)").Scan(&total).Error
	return total, err
}

func (r *WarehouseRepo) GetTodayApprovedRechargeTotal() (float64, error) {
	today := time.Now().Format("2006-01-02")
	var total float64
	err := r.db.Model(&model.WarehouseRechargeRequest{}).
		Where("status = 'approved' AND DATE(created_at) = ?", today).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error
	return total, err
}

func (r *WarehouseRepo) ListRechargeRequests(req *model.WarehouseAdminRechargeListReq) ([]model.WarehouseRechargeRequest, int64, error) {
	q := r.db.Model(&model.WarehouseRechargeRequest{})
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
	var records []model.WarehouseRechargeRequest
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}

func (r *WarehouseRepo) ListAllBillingRecords(req *model.WarehouseAdminBillingListReq) ([]model.WarehouseBillingRecord, int64, error) {
	q := r.db.Model(&model.WarehouseBillingRecord{})
	if req.Keyword != "" {
		kw := sqlutil.EscapeLike(req.Keyword)
		q = q.Where("trade_no LIKE ? OR flow_no LIKE ?", kw, kw)
	}
	if req.StartDate != "" {
		q = q.Where("created_at >= ?", req.StartDate+" 00:00:00")
	}
	if req.EndDate != "" {
		q = q.Where("created_at <= ?", req.EndDate+" 23:59:59")
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
	var records []model.WarehouseBillingRecord
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}

// GetAccountInfoByIDs reuses the same pattern as BillingRepo.
func (r *WarehouseRepo) GetAccountInfoByIDs(ids []uint64) (map[uint64]AccountBasic, error) {
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

// ---------- Order queries for warehouse deduction ----------

// ListPendingWarehouseOrders returns orders that need warehouse deduction.
func (r *WarehouseRepo) ListPendingWarehouseOrders() ([]model.OrderTrade, error) {
	var trades []model.OrderTrade
	err := r.db.Where(
		"process_status = ? AND warehouse_status = ?",
		8, model.WarehouseStatusPending,
	).Limit(500).Find(&trades).Error
	return trades, err
}

// ListInsufficientWarehouseOrders returns orders with warehouse_status=2 for retry.
func (r *WarehouseRepo) ListInsufficientWarehouseOrders() ([]model.OrderTrade, error) {
	var trades []model.OrderTrade
	err := r.db.Where("warehouse_status = ?", model.WarehouseStatusInsufficient).
		Limit(500).Find(&trades).Error
	return trades, err
}

func (r *WarehouseRepo) UpdateWarehouseStatus(uid string, status int8) error {
	return r.db.Model(&model.OrderTrade{}).
		Where("uid = ?", uid).
		Update("warehouse_status", status).Error
}

func (r *WarehouseRepo) GetItemsByTradeUID(tradeUID string) ([]model.OrderItem, error) {
	var items []model.OrderItem
	err := r.db.Where("trade_uid = ?", tradeUID).Find(&items).Error
	return items, err
}

// ResolveEmployeeAccountID finds the employee account assigned to a shop.
func (r *WarehouseRepo) ResolveEmployeeAccountID(sysShop string) (uint64, error) {
	if sysShop == "" {
		return 0, nil
	}
	var shop model.Shop
	if err := r.db.Where("sys_shop = ?", sysShop).First(&shop).Error; err != nil {
		return 0, nil
	}
	var as model.AccountShop
	err := r.db.Table("account_shop").
		Select("account_shop.*").
		Joins("JOIN account ON account.id = account_shop.account_id").
		Where("account_shop.shop_id = ? AND account.role = ?", shop.ID, model.RoleEmployee).
		First(&as).Error
	if err != nil {
		return 0, nil
	}
	return as.AccountID, nil
}
