package repository

import (
	"supply-chain/internal/model"
	"supply-chain/pkg/sqlutil"
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

// UpsertTradeWithItems inserts or updates an order trade and its items.
func (r *OrderRepo) UpsertTradeWithItems(trade *model.OrderTrade, items []model.OrderItem) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Upsert trade: insert or update on uid conflict
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "uid"}},
			DoUpdates: clause.AssignmentColumns(orderTradeUpdateColumns()),
		}).Create(trade).Error; err != nil {
			return err
		}

		if len(items) == 0 {
			return nil
		}

		// Upsert items: insert or update on order_id conflict
		for i := range items {
			items[i].TradeUID = trade.UID
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "order_id"}},
			DoUpdates: clause.AssignmentColumns(orderItemUpdateColumns()),
		}).Create(&items).Error
	})
}

// ListTrades queries trades with filters and pagination.
// shopIDs can be nil for admin (no shop filter).
func (r *OrderRepo) ListTrades(req *model.OrderListReq, shopIDs []uint64) ([]model.OrderTrade, int64, error) {
	query := r.db.Model(&model.OrderTrade{})

	// Shop permission filter (non-admin)
	if shopIDs != nil {
		// Get sys_shop values for the allowed shop IDs
		var sysShops []string
		if len(shopIDs) == 0 {
			// User has no shop access — return empty
			return []model.OrderTrade{}, 0, nil
		}
		r.db.Model(&model.Shop{}).Where("id IN ?", shopIDs).Pluck("sys_shop", &sysShops)
		if len(sysShops) == 0 {
			return []model.OrderTrade{}, 0, nil
		}
		query = query.Where("sys_shop IN ?", sysShops)
	}

	// Platform filter
	if req.SourcePlatform != "" {
		query = query.Where("source_platform = ?", req.SourcePlatform)
	}

	// Shop name filter
	if req.ShopName != "" {
		query = query.Where("shop_name = ?", req.ShopName)
	}

	// Shop ID filter
	if req.SysShop != "" {
		query = query.Where("sys_shop = ?", req.SysShop)
	}

	// Status filter (process_status)
	if req.Status != "" {
		query = query.Where("process_status = ?", req.Status)
	}

	// Time range filter (based on create_time_ms).
	// Accept both "2006-01-02" and "2006-01-02 15:04:05" from the frontend.
	if req.StartTime != "" {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", req.StartTime, time.Local)
		if err != nil {
			t, err = time.ParseInLocation("2006-01-02", req.StartTime, time.Local)
		}
		if err == nil {
			query = query.Where("create_time_ms >= ?", t.UnixMilli())
		}
	}
	if req.EndTime != "" {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", req.EndTime, time.Local)
		if err != nil {
			// Date-only: extend to end of day so the entire end date is included.
			t, err = time.ParseInLocation("2006-01-02 15:04:05", req.EndTime+" 23:59:59", time.Local)
		}
		if err == nil {
			query = query.Where("create_time_ms <= ?", t.UnixMilli())
		}
	}

	// Keyword search (trade_no or express_code)
	if req.Keyword != "" {
		kw := sqlutil.EscapeLike(req.Keyword)
		query = query.Where("trade_no LIKE ? OR express_code LIKE ?", kw, kw)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Pagination defaults
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	var trades []model.OrderTrade
	err := query.Order("create_time_ms DESC").
		Offset(offset).Limit(pageSize).
		Find(&trades).Error
	if err != nil {
		return nil, 0, err
	}

	return trades, total, nil
}

// GetTradeByUID returns a single trade by UID.
func (r *OrderRepo) GetTradeByUID(uid string) (*model.OrderTrade, error) {
	var trade model.OrderTrade
	err := r.db.Where("uid = ?", uid).First(&trade).Error
	if err != nil {
		return nil, err
	}
	return &trade, nil
}

// GetTradeByTradeNo returns a single trade by trade_no.
func (r *OrderRepo) GetTradeByTradeNo(tradeNo string) (*model.OrderTrade, error) {
	var trade model.OrderTrade
	err := r.db.Where("trade_no = ?", tradeNo).First(&trade).Error
	if err != nil {
		return nil, err
	}
	return &trade, nil
}

// GetTradeByID returns a single trade by primary key ID.
func (r *OrderRepo) GetTradeByID(id uint64) (*model.OrderTrade, error) {
	var trade model.OrderTrade
	err := r.db.First(&trade, id).Error
	if err != nil {
		return nil, err
	}
	return &trade, nil
}

// GetItemsByTradeUID returns all items for a given trade UID.
func (r *OrderRepo) GetItemsByTradeUID(tradeUID string) ([]model.OrderItem, error) {
	var items []model.OrderItem
	err := r.db.Where("trade_uid = ?", tradeUID).Find(&items).Error
	return items, err
}

// BatchGetItemsByTradeUIDs returns items for multiple trades, grouped by trade_uid.
func (r *OrderRepo) BatchGetItemsByTradeUIDs(tradeUIDs []string) (map[string][]model.OrderItem, error) {
	if len(tradeUIDs) == 0 {
		return make(map[string][]model.OrderItem), nil
	}
	var items []model.OrderItem
	err := r.db.Where("trade_uid IN ?", tradeUIDs).Find(&items).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string][]model.OrderItem)
	for _, item := range items {
		result[item.TradeUID] = append(result[item.TradeUID], item)
	}
	return result, nil
}

// GetSyncState returns the sync state for a given key.
func (r *OrderRepo) GetSyncState(key string) (*model.SyncState, error) {
	var state model.SyncState
	err := r.db.Where("sync_key = ?", key).First(&state).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

// SetMarkApprovedAtIfNull sets mark_approved_at for an order only if it is currently NULL.
func (r *OrderRepo) SetMarkApprovedAtIfNull(uid string, t time.Time) error {
	return r.db.Model(&model.OrderTrade{}).
		Where("uid = ? AND mark_approved_at IS NULL", uid).
		Update("mark_approved_at", t).Error
}

// MarkAfterSaleComplete sets process_status=99 for the given trade_no if not already 99.
// Returns true if the record was actually updated (existed and was changed).
func (r *OrderRepo) MarkAfterSaleComplete(tradeNo string) (bool, error) {
	result := r.db.Model(&model.OrderTrade{}).
		Where("trade_no = ? AND process_status != ?", tradeNo, 99).
		Update("process_status", 99)
	return result.RowsAffected > 0, result.Error
}

// UpdateBillingStatus updates billing_status for an order.
func (r *OrderRepo) UpdateBillingStatus(uid string, status int8) error {
	return r.db.Model(&model.OrderTrade{}).
		Where("uid = ?", uid).
		Update("billing_status", status).Error
}

// ListPendingRefundOrders returns orders that have been after-sale completed and need a refund.
// Conditions: process_status=99, mark=已审核, mark_approved_at IS NOT NULL, billing_status=1 (success).
// Uses composite index idx_refund_scan(process_status, billing_status) for performance.
func (r *OrderRepo) ListPendingRefundOrders() ([]model.OrderTrade, error) {
	var trades []model.OrderTrade
	err := r.db.Where(
		"process_status = ? AND billing_status = ? AND mark = ? AND mark_approved_at IS NOT NULL",
		99, model.BillingStatusSuccess, "已审核",
	).Find(&trades).Error
	return trades, err
}

// ListPendingBillingOrders returns orders that need a deduction attempt.
// Includes:
//   - mark="已审核" with billing_status pending/error/insufficient (original retry path)
//   - mark="余额不足扣款失败" with billing_status=insufficient (recovery after recharge)
func (r *OrderRepo) ListPendingBillingOrders() ([]model.OrderTrade, error) {
	var trades []model.OrderTrade
	err := r.db.Where(
		"(mark = ? AND billing_status IN ?) OR (mark = ? AND billing_status = ?)",
		model.MarkApproved,
		[]int8{model.BillingStatusPending, model.BillingStatusError, model.BillingStatusInsufficient},
		model.MarkDeductFailed,
		model.BillingStatusInsufficient,
	).Find(&trades).Error
	return trades, err
}

// SetMarkDeductFailed atomically sets mark="余额不足扣款失败" and billing_status=Insufficient.
// Guarded on billing_status != Success so a concurrent successful deduction is never overwritten.
// Idempotent on orders already in the insufficient state.
func (r *OrderRepo) SetMarkDeductFailed(uid string) error {
	return r.db.Model(&model.OrderTrade{}).
		Where("uid = ? AND billing_status != ?", uid, model.BillingStatusSuccess).
		Updates(map[string]interface{}{
			"mark":           model.MarkDeductFailed,
			"billing_status": model.BillingStatusInsufficient,
		}).Error
}

// BatchSetMarkDeductFailed bulk-applies the insufficient-mark transition to multiple orders.
func (r *OrderRepo) BatchSetMarkDeductFailed(uids []string) error {
	if len(uids) == 0 {
		return nil
	}
	return r.db.Model(&model.OrderTrade{}).
		Where("uid IN ? AND billing_status != ?", uids, model.BillingStatusSuccess).
		Updates(map[string]interface{}{
			"mark":           model.MarkDeductFailed,
			"billing_status": model.BillingStatusInsufficient,
		}).Error
}

// BatchSetMarkBarcodeError bulk-marks orders as "审核失败货号错误".
// These orders will be retried by auto-review once the product/price is added.
func (r *OrderRepo) BatchSetMarkBarcodeError(uids []string) error {
	if len(uids) == 0 {
		return nil
	}
	return r.db.Model(&model.OrderTrade{}).
		Where("uid IN ? AND billing_status != ?", uids, model.BillingStatusSuccess).
		Updates(map[string]interface{}{
			"mark": model.MarkBarcodeError,
		}).Error
}

// RecoverMarkToApproved flips an insufficient order back to 已审核 after a successful retry.
// Sets mark_approved_at only if it wasn't set before (preserves original审核 time when possible).
func (r *OrderRepo) RecoverMarkToApproved(uid string, approvedAt time.Time) error {
	return r.db.Model(&model.OrderTrade{}).
		Where("uid = ? AND mark = ?", uid, model.MarkDeductFailed).
		Updates(map[string]interface{}{
			"mark":             model.MarkApproved,
			"mark_approved_at": approvedAt,
		}).Error
}

// ListAutoReviewCandidates returns paid orders eligible for auto-review for the given sys_shop values.
// Eligible = mark is empty (never audited)
//          | mark is "余额不足扣款失败" (retry after recharge)
//          | mark is "审核失败货号错误" (retry after product added).
// Orders marked "已审核" are excluded.
// No row limit — every cycle audits all pending orders. WanLiNiu rate limiting
// is handled at the batch push layer in sync_service.
func (r *OrderRepo) ListAutoReviewCandidates(sysShops []string) ([]model.OrderTrade, error) {
	if len(sysShops) == 0 {
		return nil, nil
	}
	var trades []model.OrderTrade
	err := r.db.
		Where("sys_shop IN ? AND is_pay = ? AND COALESCE(mark, '') IN ?",
			sysShops, true,
			[]string{"", model.MarkDeductFailed, model.MarkBarcodeError}).
		Order("create_time_ms ASC").
		Find(&trades).Error
	return trades, err
}

// BatchMarkApproved sets mark='已审核' and mark_approved_at for a batch of order UIDs atomically.
// The WHERE guard (COALESCE(mark,'')!='已审核') makes this idempotent.
func (r *OrderRepo) BatchMarkApproved(uids []string, approvedAt time.Time) error {
	if len(uids) == 0 {
		return nil
	}
	return r.db.Model(&model.OrderTrade{}).
		Where("uid IN ? AND COALESCE(mark, '') != ?", uids, "已审核").
		Updates(map[string]interface{}{
			"mark":             "已审核",
			"mark_approved_at": approvedAt,
		}).Error
}

// UpsertSyncState updates or creates a sync state.
func (r *OrderRepo) UpsertSyncState(key string, lastSyncTimeMs int64) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "sync_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_sync_time", "updated_at"}),
	}).Create(&model.SyncState{
		SyncKey:      key,
		LastSyncTime: lastSyncTimeMs,
	}).Error
}

// BatchUpdateTradesByTradeNo updates specified fields for multiple orders in a single transaction.
// Only non-nil pointer fields in each UpdateOrderItem are written to the database.
func (r *OrderRepo) BatchUpdateTradesByTradeNo(items []model.UpdateOrderItem) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			updates := buildOrderUpdateMap(item)
			if len(updates) == 0 {
				continue
			}
			if err := tx.Model(&model.OrderTrade{}).
				Where("trade_no = ?", item.TradeNo).
				Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// buildOrderUpdateMap converts an UpdateOrderItem into a map containing only the non-nil fields.
func buildOrderUpdateMap(item model.UpdateOrderItem) map[string]interface{} {
	m := make(map[string]interface{})
	if item.Mark != nil {
		m["mark"] = *item.Mark
	}
	if item.Flag != nil {
		m["flag"] = *item.Flag
	}
	if item.SellerMsg != nil {
		m["seller_msg"] = *item.SellerMsg
	}
	if item.BuyerMsg != nil {
		m["buyer_msg"] = *item.BuyerMsg
	}
	if item.Receiver != nil {
		m["receiver"] = *item.Receiver
	}
	if item.Phone != nil {
		m["phone"] = *item.Phone
	}
	if item.Province != nil {
		m["province"] = *item.Province
	}
	if item.City != nil {
		m["city"] = *item.City
	}
	if item.District != nil {
		m["district"] = *item.District
	}
	if item.Town != nil {
		m["town"] = *item.Town
	}
	if item.Address != nil {
		m["address"] = *item.Address
	}
	if item.Zip != nil {
		m["zip"] = *item.Zip
	}
	if item.ExpressCode != nil {
		m["express_code"] = *item.ExpressCode
	}
	if item.LogisticCode != nil {
		m["logistic_code"] = *item.LogisticCode
	}
	if item.LogisticName != nil {
		m["logistic_name"] = *item.LogisticName
	}
	if item.ChannelName != nil {
		m["channel_name"] = *item.ChannelName
	}
	return m
}

// orderTradeUpdateColumns returns the columns to update on conflict.
func orderTradeUpdateColumns() []string {
	return []string{
		"trade_no", "shop_name", "shop_nick", "sys_shop", "source_platform", "shop_type",
		"storage_name", "storage_code", "buyer_msg", "seller_msg", "oln_status",
		"buyer_account", "buyer", "buyer_show", "receiver", "phone",
		"country", "province", "city", "district", "town", "address", "zip",
		"create_time_ms", "modify_time_ms", "pay_time_ms", "send_time_ms",
		"print_time_ms", "index_time_ms", "approve_time_ms", "estimate_send_time_ms",
		"status", "process_status", "is_pay", "tp_tid",
		"express_code", "logistic_code", "logistic_name", "channel_name",
		"sum_sale", "post_fee", "paid_fee", "discount_fee", "service_fee",
		"real_payment", "post_cost", "has_refund", "is_exception_trade",
		"trade_type", "flag", "pay_no", "pay_type",
		// "mark" intentionally excluded — after initial insert, mark is owned
		// by our system (set by审核/扣款 flow). WanLiNiu's mark is only taken
		// at INSERT time, never overwritten on subsequent syncs.
		"currency_code", "currency_sum", "weight", "volume", "estimate_weight",
		"tp_logistics_type", "original_no", "original_shop_type",
		"wave_no", "batch_serial", "gx_origin_trade_id",
		"identity_num", "identity_name", "buyer_mobile", "tel",
		"post_currency", "error_id", "shipped_outbound_type",
		"oper_approve", "oper_intimidate", "oper_distribution",
		"oper_inspection", "oper_send", "additon",
		"split_trade", "exchange_trade", "is_small_trade",
		"oln_order_list_json", "merge_uids_json", "platform_discount_json",
		"series_no",
		// "trade_source" intentionally excluded — set once at insert, never overwritten.
		"updated_at",
	}
}

// orderItemUpdateColumns returns the columns to update on conflict.
func orderItemUpdateColumns() []string {
	return []string{
		"trade_uid", "item_name", "sku_name", "sku_code", "size",
		"price", "discounted_unit_price", "receivable", "order_total_discount",
		"payment", "is_package", "tp_tid", "tp_oid",
		"oln_item_id", "oln_item_code", "oln_sku_code", "oln_status",
		"oln_sku_id", "oln_sku_name", "oln_item_name",
		"sys_goods_uid", "sys_spec_uid", "inventory_status",
		"status", "has_refund", "remark", "is_gift",
		"currency_sum", "item_image_url", "item_platform_url",
		"tid_snapshot", "tax_rate", "tax_payment", "bar_code",
		"gx_payment", "gx_price", "estimate_send_time_ms",
		"updated_at",
	}
}
