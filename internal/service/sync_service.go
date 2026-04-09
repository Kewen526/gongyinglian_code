package service

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"supply-chain/internal/config"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"
	"time"
)

const (
	syncKey        = "order_sync"
	tradeListPath  = "/erp/opentrade/list/trades"
	batchMarkPath  = "/erp/opentrade/modify/batch/mark"
	maxPageSize    = 200
	initialSyncDays = 7 // first sync pulls last 7 days
)

type SyncService struct {
	orderRepo *repository.OrderRepo
	shopRepo  *repository.ShopRepo
	cfg       *config.WanLiNiuConfig
	stopCh    chan struct{}
}

func NewSyncService(orderRepo *repository.OrderRepo, shopRepo *repository.ShopRepo, cfg *config.WanLiNiuConfig) *SyncService {
	return &SyncService{
		orderRepo: orderRepo,
		shopRepo:  shopRepo,
		cfg:       cfg,
		stopCh:    make(chan struct{}),
	}
}

// StartAutoSync starts a background goroutine that syncs orders periodically.
func (s *SyncService) StartAutoSync() {
	interval := time.Duration(s.cfg.SyncInterval) * time.Second
	if interval < 30*time.Second {
		interval = 60 * time.Second
	}

	go func() {
		log.Printf("[Sync] Auto sync started, interval=%v\n", interval)
		// Run immediately on startup
		s.syncOnce()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.syncOnce()
			case <-s.stopCh:
				log.Println("[Sync] Auto sync stopped")
				return
			}
		}
	}()
}

// Stop gracefully stops the auto sync.
func (s *SyncService) Stop() {
	close(s.stopCh)
}

// SyncNow triggers an immediate sync and returns the count of orders synced.
func (s *SyncService) SyncNow() (int, error) {
	return s.syncOnceWithResult()
}

func (s *SyncService) syncOnce() {
	count, err := s.syncOnceWithResult()
	if err != nil {
		log.Printf("[Sync] Error: %v\n", err)
	} else {
		log.Printf("[Sync] Completed, synced %d orders\n", count)
	}
}

func (s *SyncService) syncOnceWithResult() (int, error) {
	// Get last sync time
	state, err := s.orderRepo.GetSyncState(syncKey)
	if err != nil {
		return 0, fmt.Errorf("get sync state: %w", err)
	}

	var startTime time.Time
	now := time.Now()

	if state == nil || state.LastSyncTime == 0 {
		// First sync: pull last N days
		startTime = now.AddDate(0, 0, -initialSyncDays)
	} else {
		// Incremental: from last sync time (subtract 5 minutes for safety overlap)
		startTime = time.UnixMilli(state.LastSyncTime).Add(-5 * time.Minute)
	}

	startStr := startTime.Format("2006-01-02 15:04:05")
	endStr := now.Format("2006-01-02 15:04:05")

	log.Printf("[Sync] Fetching orders: modify_time %s ~ %s\n", startStr, endStr)

	// Fetch all pages
	allOrders, err := s.fetchAllOrders(startStr, endStr)
	if err != nil {
		return 0, fmt.Errorf("fetch orders: %w", err)
	}

	if len(allOrders) == 0 {
		// Update sync state even if no orders
		s.orderRepo.UpsertSyncState(syncKey, now.UnixMilli())
		return 0, nil
	}

	// Save orders to DB
	savedCount, err := s.saveOrders(allOrders)
	if err != nil {
		return savedCount, fmt.Errorf("save orders: %w", err)
	}

	// Update sync state
	s.orderRepo.UpsertSyncState(syncKey, now.UnixMilli())

	return savedCount, nil
}

func (s *SyncService) fetchAllOrders(startTime, endTime string) ([]map[string]interface{}, error) {
	var allOrders []map[string]interface{}
	page := 1

	for {
		result, err := s.fetchPage(page, maxPageSize, startTime, endTime)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}

		code, _ := result["code"].(float64)
		if int(code) != 0 {
			return nil, fmt.Errorf("API error on page %d: %v", page, result)
		}

		dataRaw, ok := result["data"]
		if !ok || dataRaw == nil {
			break
		}

		orders, ok := dataRaw.([]interface{})
		if !ok || len(orders) == 0 {
			break
		}

		for _, o := range orders {
			if m, ok := o.(map[string]interface{}); ok {
				allOrders = append(allOrders, m)
			}
		}

		if len(orders) < maxPageSize {
			break
		}

		page++
		time.Sleep(300 * time.Millisecond) // rate limit
	}

	return allOrders, nil
}

func (s *SyncService) fetchPage(page, limit int, startTime, endTime string) (map[string]interface{}, error) {
	bizParams := map[string]string{
		"page":        strconv.Itoa(page),
		"limit":       strconv.Itoa(limit),
		"modify_time": startTime,
		"end_time":    endTime,
	}

	// Add query_extend for package info
	queryExtend, _ := json.Marshal(map[string]interface{}{
		"query_package_info": true,
	})
	bizParams["query_extend"] = string(queryExtend)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	allParams := make(map[string]string)
	for k, v := range bizParams {
		allParams[k] = v
	}
	allParams["_app"] = s.cfg.AppKey
	allParams["_t"] = timestamp
	allParams["_sign"] = buildSign(allParams, s.cfg.Secret)

	// Build form data
	form := url.Values{}
	for k, v := range allParams {
		form.Set(k, v)
	}

	resp, err := http.Post(
		s.cfg.BaseURL+tradeListPath,
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return result, nil
}

func buildSign(params map[string]string, secret string) string {
	// Filter out _sign
	filtered := make(map[string]string)
	for k, v := range params {
		if k != "_sign" {
			filtered[k] = v
		}
	}

	// Sort keys
	keys := make([]string, 0, len(filtered))
	for k := range filtered {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build param string
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+url.QueryEscape(filtered[k]))
	}
	paramStr := strings.Join(parts, "&")

	raw := secret + paramStr + secret
	hash := md5.Sum([]byte(raw))
	return strings.ToUpper(fmt.Sprintf("%x", hash))
}

func (s *SyncService) saveOrders(rawOrders []map[string]interface{}) (int, error) {
	saved := 0
	for _, raw := range rawOrders {
		trade := mapToOrderTrade(raw)
		items := mapToOrderItems(raw)

		// Upsert shop
		if trade.SysShop != "" {
			shop := model.Shop{
				SysShop:        trade.SysShop,
				ShopName:       trade.ShopName,
				ShopNick:       trade.ShopNick,
				SourcePlatform: trade.SourcePlatform,
				ShopType:       trade.ShopType,
			}
			if err := s.shopRepo.Upsert(&shop); err != nil {
				log.Printf("[Sync] Warning: upsert shop %s: %v\n", trade.SysShop, err)
			}
		}

		// Upsert order + items
		if err := s.orderRepo.UpsertTradeWithItems(&trade, items); err != nil {
			log.Printf("[Sync] Warning: upsert trade %s: %v\n", trade.UID, err)
			continue
		}
		saved++
	}
	return saved, nil
}

// ==================== Mapping Helpers ====================

func mapToOrderTrade(m map[string]interface{}) model.OrderTrade {
	t := model.OrderTrade{
		UID:                 getString(m, "uid"),
		TradeNo:             getString(m, "trade_no"),
		ShopName:            getString(m, "shop_name"),
		ShopNick:            getString(m, "shop_nick"),
		SysShop:             getString(m, "sys_shop"),
		SourcePlatform:      getString(m, "source_platform"),
		ShopType:            getInt(m, "shop_type"),
		StorageName:         getString(m, "storage_name"),
		StorageCode:         getString(m, "storage_code"),
		BuyerMsg:            getString(m, "buyer_msg"),
		SellerMsg:           getString(m, "seller_msg"),
		OlnStatus:           getInt(m, "oln_status"),
		BuyerAccount:        getString(m, "buyer_account"),
		Buyer:               getString(m, "buyer"),
		BuyerShow:           getString(m, "buyer_show"),
		Receiver:            getString(m, "receiver"),
		Phone:               getString(m, "phone"),
		Country:             getString(m, "country"),
		Province:            getString(m, "province"),
		City:                getString(m, "city"),
		District:            getString(m, "district"),
		Town:                getString(m, "town"),
		Address:             getString(m, "address"),
		Zip:                 getString(m, "zip"),
		CreateTimeMs:        getInt64(m, "create_time"),
		ModifyTimeMs:        getInt64(m, "modify_time"),
		PayTimeMs:           getInt64(m, "pay_time"),
		SendTimeMs:          getInt64(m, "send_time"),
		PrintTimeMs:         getInt64(m, "print_time"),
		IndexTimeMs:         getInt64(m, "index_time"),
		ApproveTimeMs:       getInt64(m, "approve_time"),
		EstimateSendTimeMs:  getInt64(m, "estimate_send_time"),
		Status:              getInt(m, "status"),
		ProcessStatus:       getInt(m, "process_status"),
		IsPay:               getBool(m, "is_pay"),
		TpTid:               getString(m, "tp_tid"),
		ExpressCode:         getString(m, "express_code"),
		LogisticCode:        getString(m, "logistic_code"),
		LogisticName:        getString(m, "logistic_name"),
		ChannelName:         getString(m, "channel_name"),
		SumSale:             getFloat(m, "sum_sale"),
		PostFee:             getFloat(m, "post_fee"),
		PaidFee:             getFloat(m, "paid_fee"),
		DiscountFee:         getFloat(m, "discount_fee"),
		ServiceFee:          getFloat(m, "service_fee"),
		RealPayment:         getFloat(m, "real_payment"),
		PostCost:            getFloat(m, "post_cost"),
		HasRefund:           getInt(m, "has_refund"),
		IsExceptionTrade:    getBool(m, "is_exception_trade"),
		TradeType:           getInt(m, "trade_type"),
		Mark:                getString(m, "mark"),
		Flag:                getInt(m, "flag"),
		PayNo:               getString(m, "pay_no"),
		PayType:             getString(m, "pay_type"),
		CurrencyCode:        getString(m, "currency_code"),
		CurrencySum:         getFloat(m, "currency_sum"),
		Weight:              getFloat(m, "weight"),
		Volume:              getFloat(m, "volume"),
		EstimateWeight:      getFloat(m, "estimate_weight"),
		TpLogisticsType:     getInt(m, "tp_logistics_type"),
		OriginalNo:          getString(m, "original_no"),
		OriginalShopType:    getInt(m, "original_shop_type"),
		WaveNo:              getString(m, "wave_no"),
		BatchSerial:         getString(m, "batch_serial"),
		GxOriginTradeID:     getString(m, "gx_origin_trade_id"),
		IdentityNum:         getString(m, "identity_num"),
		IdentityName:        getString(m, "identity_name"),
		BuyerMobile:         getString(m, "buyer_mobile"),
		Tel:                 getString(m, "tel"),
		PostCurrency:        getFloat(m, "post_currency"),
		ErrorID:             getInt(m, "error_id"),
		ShippedOutboundType: getInt(m, "shipped_outbound_type"),
		OperApprove:         getString(m, "oper_apppove"), // note: API typo
		OperIntimidate:      getString(m, "oper_intimidate"),
		OperDistribution:    getString(m, "oper_distribution"),
		OperInspection:      getString(m, "oper_inspection"),
		OperSend:            getString(m, "oper_send"),
		Additon:             getString(m, "additon"),
		SplitTrade:          getBool(m, "split_trade"),
		ExchangeTrade:       getBool(m, "exchange_trade"),
		IsSmallTrade:        getBool(m, "is_small_trade"),
	}

	// Serialize array/object fields to JSON
	if v, ok := m["oln_order_list"]; ok {
		b, _ := json.Marshal(v)
		t.OlnOrderListJSON = string(b)
	}
	if v, ok := m["merge_uids"]; ok {
		b, _ := json.Marshal(v)
		t.MergeUidsJSON = string(b)
	}
	if v, ok := m["platform_origin_discount"]; ok {
		b, _ := json.Marshal(v)
		t.PlatformDiscountJSON = string(b)
	}

	return t
}

func mapToOrderItems(m map[string]interface{}) []model.OrderItem {
	ordersRaw, ok := m["orders"]
	if !ok {
		return nil
	}
	ordersList, ok := ordersRaw.([]interface{})
	if !ok {
		return nil
	}

	tradeUID := getString(m, "uid")
	items := make([]model.OrderItem, 0, len(ordersList))

	for _, raw := range ordersList {
		om, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		item := model.OrderItem{
			TradeUID:            tradeUID,
			OrderID:             getString(om, "order_id"),
			ItemName:            getString(om, "item_name"),
			SkuName:             getString(om, "sku_name"),
			SkuCode:             getString(om, "sku_code"),
			Size:                getInt(om, "size"),
			Price:               getFloat(om, "price"),
			DiscountedUnitPrice: getFloat(om, "discounted_unit_price"),
			Receivable:          getFloat(om, "receivable"),
			OrderTotalDiscount:  getFloat(om, "order_total_discount"),
			Payment:             getFloat(om, "payment"),
			IsPackage:           getBool(om, "is_package"),
			TpTid:               getString(om, "tp_tid"),
			TpOid:               getString(om, "tp_oid"),
			OlnItemID:           getString(om, "oln_item_id"),
			OlnItemCode:         getString(om, "oln_item_code"),
			OlnSkuCode:          getString(om, "oln_sku_code"),
			OlnStatus:           getInt(om, "oln_status"),
			OlnSkuID:            getString(om, "oln_sku_id"),
			OlnSkuName:          getString(om, "oln_sku_name"),
			OlnItemName:         getString(om, "oln_item_name"),
			SysGoodsUID:         getString(om, "sys_goods_uid"),
			SysSpecUID:          getString(om, "sys_spec_uid"),
			InventoryStatus:     getString(om, "inventory_status"),
			Status:              getInt(om, "status"),
			HasRefund:           getInt(om, "has_refund"),
			Remark:              getString(om, "remark"),
			IsGift:              getInt(om, "is_gift"),
			CurrencySum:         getFloat(om, "currency_sum"),
			ItemImageURL:        getString(om, "item_image_url"),
			ItemPlatformURL:     getString(om, "item_platform_url"),
			TidSnapshot:         getString(om, "tid_snapshot"),
			TaxRate:             getFloat(om, "tax_rate"),
			TaxPayment:          getFloat(om, "tax_payment"),
			BarCode:             getString(om, "bar_code"),
			GxPayment:           getFloat(om, "gx_payment"),
			GxPrice:             getFloat(om, "gx_price"),
			EstimateSendTimeMs:  getInt64(om, "estimate_send_time"),
		}
		items = append(items, item)
	}
	return items
}

// ==================== Type conversion helpers ====================

func getString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

func getFloat(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	default:
		return 0
	}
}

func getInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case json.Number:
		i, _ := val.Int64()
		return int(i)
	default:
		return 0
	}
}

func getInt64(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	case json.Number:
		i, _ := val.Int64()
		return i
	default:
		return 0
	}
}

func getBool(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	default:
		return false
	}
}

// BatchMarkOrders sends a batch mark request to WanLiNiu.
func (s *SyncService) BatchMarkOrders(items []model.MarkItem) error {
	type markReqItem struct {
		BillCode string `json:"bill_code"`
		MarkName string `json:"mark_name"`
		Type     int    `json:"type"`
	}

	reqItems := make([]markReqItem, len(items))
	for i, item := range items {
		reqItems[i] = markReqItem{
			BillCode: item.BillCode,
			MarkName: item.MarkName,
			Type:     item.Type,
		}
	}

	batchJSON, err := json.Marshal(reqItems)
	if err != nil {
		return fmt.Errorf("marshal batch_mark_requests: %w", err)
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	allParams := map[string]string{
		"_app":                s.cfg.AppKey,
		"_t":                  timestamp,
		"batch_mark_requests": string(batchJSON),
	}
	allParams["_sign"] = buildSign(allParams, s.cfg.Secret)

	form := url.Values{}
	for k, v := range allParams {
		form.Set(k, v)
	}

	resp, err := http.Post(
		s.cfg.BaseURL+batchMarkPath,
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("json unmarshal: %w", err)
	}

	code, ok := result["code"]
	if !ok {
		return fmt.Errorf("unexpected response: %s", string(body))
	}
	codeFloat, _ := code.(float64)
	if int(codeFloat) != 0 {
		msg, _ := result["message"].(string)
		return fmt.Errorf("wanliniu error %d: %s", int(codeFloat), msg)
	}

	return nil
}
