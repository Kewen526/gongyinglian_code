package repository

import (
	"supply-chain/internal/model"
	"time"

	"gorm.io/gorm"
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

func (r *BillingRepo) CreateBillingRecord(tx *gorm.DB, rec *model.BillingRecord) error {
	return tx.Create(rec).Error
}

func (r *BillingRepo) DB() *gorm.DB { return r.db }

// ListBillingRecords queries billing records with filters.
func (r *BillingRepo) ListBillingRecords(req *model.BillingListReq, accountID uint64) ([]model.BillingRecord, int64, error) {
	q := r.db.Model(&model.BillingRecord{}).Where("account_id = ?", accountID)

	// Keyword: order number or flow_no
	if req.Keyword != "" {
		q = q.Where("trade_no LIKE ? OR flow_no LIKE ?", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
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
	// Status filter: "success" means deduct success, "refund" means refund success
	switch req.Status {
	case "success":
		q = q.Where("type = 'deduct' AND status = 'success'")
	case "refund":
		q = q.Where("type = 'refund' AND status = 'success'")
	case "insufficient":
		q = q.Where("status = 'insufficient'")
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
