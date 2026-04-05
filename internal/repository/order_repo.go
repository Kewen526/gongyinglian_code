package repository

import (
	"supply-chain/internal/model"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OrderRepo struct {
	db *gorm.DB
}

func NewOrderRepo(db *gorm.DB) *OrderRepo {
	return &OrderRepo{db: db}
}

// UpsertTrade inserts or updates an order trade by uid
func (r *OrderRepo) UpsertTrade(trade *model.OrderTrade) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "uid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"order_id", "platform", "shop_name", "status", "trade_status",
			"buyer_nick", "receiver_name", "receiver_phone",
			"receiver_province", "receiver_city", "receiver_district", "receiver_address",
			"total_amount", "pay_amount", "post_fee", "discount_fee",
			"logistics_name", "logistics_no",
			"buyer_message", "seller_remark",
			"pay_time", "send_time", "trade_time", "modify_time",
			"updated_at",
		}),
	}).Create(trade).Error
}

// UpsertItems replaces all items for a given trade_uid
func (r *OrderRepo) UpsertItems(tradeUID string, items []model.OrderItem) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("trade_uid = ?", tradeUID).Delete(&model.OrderItem{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Create(&items).Error
	})
}

// ListOrders returns paginated orders filtered by conditions and shop permissions
func (r *OrderRepo) ListOrders(req *model.OrderListReq, shopNames []string) ([]model.OrderTrade, int64, error) {
	query := r.db.Model(&model.OrderTrade{})

	// Shop permission filter
	if len(shopNames) > 0 {
		query = query.Where("shop_name IN ?", shopNames)
	}

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}
	if req.ShopName != "" {
		query = query.Where("shop_name = ?", req.ShopName)
	}
	if req.Platform != "" {
		query = query.Where("platform = ?", req.Platform)
	}
	if req.StartDate != "" {
		query = query.Where("trade_time >= ?", req.StartDate)
	}
	if req.EndDate != "" {
		query = query.Where("trade_time <= ?", req.EndDate+" 23:59:59")
	}
	if req.Keyword != "" {
		kw := "%" + req.Keyword + "%"
		query = query.Where("order_id LIKE ? OR buyer_nick LIKE ? OR receiver_name LIKE ? OR logistics_no LIKE ?",
			kw, kw, kw, kw)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	var list []model.OrderTrade
	err := query.Order("trade_time DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&list).Error

	return list, total, err
}

// GetTradeByID returns a single trade by ID
func (r *OrderRepo) GetTradeByID(id uint64) (*model.OrderTrade, error) {
	var trade model.OrderTrade
	err := r.db.First(&trade, id).Error
	if err != nil {
		return nil, err
	}
	return &trade, nil
}

// GetItemsByTradeUID returns all items for a trade
func (r *OrderRepo) GetItemsByTradeUID(tradeUID string) ([]model.OrderItem, error) {
	var items []model.OrderItem
	err := r.db.Where("trade_uid = ?", tradeUID).Find(&items).Error
	return items, err
}

// GetSyncState returns the last sync time for a given sync type
func (r *OrderRepo) GetSyncState(syncType string) (*model.SyncState, error) {
	var state model.SyncState
	err := r.db.Where("sync_type = ?", syncType).First(&state).Error
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// UpdateSyncState upserts the sync state
func (r *OrderRepo) UpdateSyncState(syncType string, lastSyncTime time.Time) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "sync_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_sync_time", "updated_at"}),
	}).Create(&model.SyncState{
		SyncType:     syncType,
		LastSyncTime: lastSyncTime,
		UpdatedAt:    time.Now(),
	}).Error
}

// GetDistinctPlatforms returns all distinct platforms from orders
func (r *OrderRepo) GetDistinctPlatforms() ([]string, error) {
	var platforms []string
	err := r.db.Model(&model.OrderTrade{}).Distinct("platform").Where("platform != ''").Pluck("platform", &platforms).Error
	return platforms, err
}
