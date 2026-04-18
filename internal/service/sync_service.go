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
	syncKey          = "order_sync"
	tradeListPath    = "/erp/opentrade/list/trades"
	batchMarkPath    = "/erp/opentrade/modify/batch/mark"
	returnOrderPath  = "/erp/open/return/order/list"
	maxPageSize      = 200
	initialSyncDays  = 7  // first sync pulls last 7 days
	afterSaleSyncDays = 15 // after-sale sync pulls last 15 days
)

// autoReviewBatchSize is the max orders per WanLiNiu batch-mark API call.
// WanLiNiu's /erp/opentrade/modify/batch/mark endpoint caps at 10 orders per call
// (error 1500: 最多只支持10笔订单).
const autoReviewBatchSize = 10

// autoReviewBatchDelay is the pause between consecutive WanLiNiu batch-mark calls
// within a single cycle, to avoid hitting ERP rate limits on large backlogs.
// With batch size 10 and ~5-minute cycle, this caps throughput at ~100 orders/sec,
// leaving headroom for 30k+ orders per cycle.
const autoReviewBatchDelay = 100 * time.Millisecond

var wanLiNiuClient = &http.Client{Timeout: 30 * time.Second}

type SyncService struct {
	orderRepo      *repository.OrderRepo
	shopRepo       *repository.ShopRepo
	accountRepo    *repository.AccountRepo
	cfg            *config.WanLiNiuConfig
	billingService *BillingService
	stopCh         chan struct{}
}

func NewSyncService(orderRepo *repository.OrderRepo, shopRepo *repository.ShopRepo, accountRepo *repository.AccountRepo, cfg *config.WanLiNiuConfig, billingService *BillingService) *SyncService {
	return &SyncService{
		orderRepo:      orderRepo,
		shopRepo:       shopRepo,
		accountRepo:    accountRepo,
		cfg:            cfg,
		billingService: billingService,
		stopCh:         make(chan struct{}),
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

	resp, err := wanLiNiuClient.Post(
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
		if trade.OlnStatus == 1 {
			continue
		}
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

		// Trigger billing deduction when order is marked "已审核"
		if trade.Mark == model.MarkApproved && s.billingService != nil {
			_ = s.orderRepo.SetMarkApprovedAtIfNull(trade.UID, time.Now())
			s.billingService.TriggerDeductionAsync(&trade)
		}
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
		// Mark 不从万里牛导入 —— 本字段由我们系统单向写入。
		// 万里牛侧的自定义 mark（例如 "打包费"）不应污染审核状态。
		// 新订单以空 mark 入库，走我们的审核流程。
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

// StartAfterSaleSync starts a background goroutine that marks after-sale orders every 5 minutes.
func (s *SyncService) StartAfterSaleSync() {
	go func() {
		log.Println("[AfterSale] After-sale sync started (interval=5m)")
		s.afterSaleOnce()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.afterSaleOnce()
			case <-s.stopCh:
				log.Println("[AfterSale] After-sale sync stopped")
				return
			}
		}
	}()
}

func (s *SyncService) afterSaleOnce() {
	now := time.Now()
	start := now.AddDate(0, 0, -afterSaleSyncDays).Format("2006-01-02 00:00:00")
	end := now.Format("2006-01-02 15:04:05")

	count, err := s.syncAfterSaleOrders(start, end)
	if err != nil {
		log.Printf("[AfterSale] Error: %v\n", err)
	} else if count > 0 {
		log.Printf("[AfterSale] Marked %d orders as after-sale complete\n", count)
	}
}

func (s *SyncService) syncAfterSaleOrders(startTime, endTime string) (int, error) {
	var allOrders []map[string]interface{}
	page := 1

	for {
		result, err := s.fetchReturnOrderPage(page, maxPageSize, startTime, endTime)
		if err != nil {
			return 0, err
		}

		code, _ := result["code"].(float64)
		if int(code) != 0 {
			return 0, fmt.Errorf("API error: %v", result)
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
		time.Sleep(300 * time.Millisecond)
	}

	count := 0
	for _, order := range allOrders {
		// Filter: type=0 (退货) AND describe="退货退款"
		if getInt(order, "type") != 0 || getString(order, "describe") != "退货退款" {
			continue
		}
		tradeNo := getString(order, "oln_trade_code")
		if tradeNo == "" {
			continue
		}
		updated, err := s.orderRepo.MarkAfterSaleComplete(tradeNo)
		if err != nil {
			log.Printf("[AfterSale] Failed to mark trade_no=%s: %v\n", tradeNo, err)
			continue
		}
		if updated {
			count++
		}
	}
	return count, nil
}

func (s *SyncService) fetchReturnOrderPage(page, limit int, startTime, endTime string) (map[string]interface{}, error) {
	allParams := map[string]string{
		"page":       strconv.Itoa(page),
		"limit":      strconv.Itoa(limit),
		"time_type":  "1",
		"start_time": startTime,
		"end_time":   endTime,
		"_app":       s.cfg.AppKey,
		"_t":         strconv.FormatInt(time.Now().Unix(), 10),
	}
	allParams["_sign"] = buildSign(allParams, s.cfg.Secret)

	form := url.Values{}
	for k, v := range allParams {
		form.Set(k, v)
	}

	resp, err := wanLiNiuClient.Post(
		s.cfg.BaseURL+returnOrderPath,
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

	resp, err := wanLiNiuClient.Post(
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

// MarkBatchResult summarises the outcome of a balance-checked mark batch.
type MarkBatchResult struct {
	Pushed               int      `json:"pushed"`                 // successfully pushed to WanLiNiu
	MarkedDeductFailed   int      `json:"marked_deduct_failed"`   // locally marked "余额不足扣款失败"
	Skipped              int      `json:"skipped"`                // no wallet / price error / unknown trade
	InsufficientTradeNos []string `json:"insufficient_trade_nos"` // trade_nos that hit insufficient balance
}

// BatchMarkOrdersWithBalanceCheck is the审核-aware wrapper around BatchMarkOrders.
// For every MarkItem whose MarkName == "已审核" (Type=0 overwrite), it runs a
// balance check on the corresponding order before pushing to WanLiNiu:
//   - Sufficient          → included in the WanLiNiu batch
//   - Insufficient balance → locally marked "余额不足扣款失败" (not pushed)
//   - Price error / no wallet / unknown trade → skipped silently
// MarkItems with other MarkName values (custom marks, clear operations) are passed
// through to WanLiNiu unchanged.
func (s *SyncService) BatchMarkOrdersWithBalanceCheck(items []model.MarkItem) (*MarkBatchResult, error) {
	result := &MarkBatchResult{}
	toPush := make([]model.MarkItem, 0, len(items))
	approvedUIDs := make([]string, 0)
	insufficientUIDs := make([]string, 0)

	for _, m := range items {
		// Only apply balance pre-check to "已审核" overwrite operations.
		if !(m.MarkName == model.MarkApproved && m.Type == 0) {
			toPush = append(toPush, m)
			continue
		}
		trade, err := s.orderRepo.GetTradeByTradeNo(m.BillCode)
		if err != nil || trade == nil {
			// Unknown trade_no — skip silently (frontend may have stale data).
			result.Skipped++
			continue
		}
		switch s.billingService.CheckAutoReviewEligible(trade.SysShop, trade.UID) {
		case DeductOK:
			toPush = append(toPush, m)
			approvedUIDs = append(approvedUIDs, trade.UID)
		case DeductInsufficient:
			insufficientUIDs = append(insufficientUIDs, trade.UID)
			result.InsufficientTradeNos = append(result.InsufficientTradeNos, trade.TradeNo)
		case DeductSkip:
			result.Skipped++
		}
	}

	// Apply insufficient-mark transition locally (no WanLiNiu push).
	if len(insufficientUIDs) > 0 {
		if err := s.orderRepo.BatchSetMarkDeductFailed(insufficientUIDs); err != nil {
			log.Printf("[Mark] BatchSetMarkDeductFailed: %v\n", err)
		}
		result.MarkedDeductFailed = len(insufficientUIDs)
	}

	if len(toPush) == 0 {
		return result, nil
	}

	// Push to WanLiNiu.
	if err := s.BatchMarkOrders(toPush); err != nil {
		return result, err
	}
	result.Pushed = len(toPush)

	// For the "已审核" entries we pushed, update local mark + trigger async deduction.
	if len(approvedUIDs) > 0 {
		now := time.Now()
		if err := s.orderRepo.BatchMarkApproved(approvedUIDs, now); err != nil {
			log.Printf("[Mark] BatchMarkApproved DB error: %v\n", err)
		}
		for _, uid := range approvedUIDs {
			t, err := s.orderRepo.GetTradeByUID(uid)
			if err != nil || t == nil {
				continue
			}
			t.Mark = model.MarkApproved
			t.MarkApprovedAt = &now
			s.billingService.TriggerDeductionAsync(t)
		}
	}

	return result, nil
}

// ==================== Auto Review ====================

// StartAutoReview starts the background auto-review task (5-minute interval).
// It scans accounts with auto_review=true and marks qualifying orders on WanLiNiu.
func (s *SyncService) StartAutoReview() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[AutoReview] PANIC recovered: %v\n", r)
			}
		}()
		log.Println("[AutoReview] Task started (interval=5m)")
		s.autoReviewOnce()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.autoReviewOnce()
			case <-s.stopCh:
				log.Println("[AutoReview] Task stopped")
				return
			}
		}
	}()
}

func (s *SyncService) autoReviewOnce() {
	log.Println("[AutoReview] Cycle started")
	accounts, err := s.accountRepo.ListEmployees()
	if err != nil {
		log.Printf("[AutoReview] ListEmployees error: %v\n", err)
		return
	}
	if len(accounts) == 0 {
		log.Println("[AutoReview] No employee accounts found, skipping")
		return
	}
	log.Printf("[AutoReview] Processing %d accounts\n", len(accounts))
	for i := range accounts {
		s.processAccountAutoReview(&accounts[i])
	}
	log.Println("[AutoReview] Cycle finished")
}

// processAccountAutoReview handles the full auto-review cycle for one account.
func (s *SyncService) processAccountAutoReview(account *model.Account) {
	// Step 1: resolve this account's visible shop IDs
	shopIDs, err := s.getAutoReviewShopIDs(account)
	if err != nil {
		log.Printf("[AutoReview] getAutoReviewShopIDs account=%d: %v\n", account.ID, err)
		return
	}
	if len(shopIDs) == 0 {
		log.Printf("[AutoReview] Account=%d has no shops, skipping\n", account.ID)
		return
	}

	// Step 2: convert shop IDs to sys_shop strings (single Pluck query)
	sysShops, err := s.shopRepo.GetSysShopsByIDs(shopIDs)
	if err != nil {
		log.Printf("[AutoReview] GetSysShopsByIDs account=%d: %v\n", account.ID, err)
		return
	}
	if len(sysShops) == 0 {
		log.Printf("[AutoReview] Account=%d shopIDs=%v resolved to 0 sysShops, skipping\n", account.ID, shopIDs)
		return
	}

	// Step 3: fetch paid, un-reviewed candidates (capped at 500)
	candidates, err := s.orderRepo.ListAutoReviewCandidates(sysShops)
	if err != nil {
		log.Printf("[AutoReview] ListAutoReviewCandidates account=%d: %v\n", account.ID, err)
		return
	}
	if len(candidates) == 0 {
		log.Printf("[AutoReview] Account=%d no candidates found\n", account.ID)
		return
	}

	// Step 4: batch eligibility check (3 SQL queries total: wallet + items + prices)
	type approvedItem struct {
		mark  model.MarkItem
		trade *model.OrderTrade
	}

	checkStart := time.Now()
	checkResults := s.billingService.BatchCheckAutoReviewEligible(account.ID, candidates)
	log.Printf("[AutoReview] Account=%d checked %d candidates in %v\n",
		account.ID, len(candidates), time.Since(checkStart).Round(time.Millisecond))

	approved := make([]approvedItem, 0, len(candidates))
	insufficientUIDs := make([]string, 0)
	barcodeErrorUIDs := make([]string, 0)
	for i := range candidates {
		t := &candidates[i]
		switch checkResults[t.UID] {
		case DeductOK:
			approved = append(approved, approvedItem{
				mark:  model.MarkItem{BillCode: t.TradeNo, MarkName: model.MarkApproved, Type: 0},
				trade: t,
			})
		case DeductInsufficient:
			insufficientUIDs = append(insufficientUIDs, t.UID)
		case DeductBarcodeError:
			barcodeErrorUIDs = append(barcodeErrorUIDs, t.UID)
		}
	}
	if len(insufficientUIDs) > 0 {
		if err := s.orderRepo.BatchSetMarkDeductFailed(insufficientUIDs); err != nil {
			log.Printf("[AutoReview] BatchSetMarkDeductFailed account=%d: %v\n", account.ID, err)
		} else {
			log.Printf("[AutoReview] Account=%d marked %d orders as 余额不足扣款失败\n", account.ID, len(insufficientUIDs))
		}
	}
	if len(barcodeErrorUIDs) > 0 {
		if err := s.orderRepo.BatchSetMarkBarcodeError(barcodeErrorUIDs); err != nil {
			log.Printf("[AutoReview] BatchSetMarkBarcodeError account=%d: %v\n", account.ID, err)
		} else {
			log.Printf("[AutoReview] Account=%d marked %d orders as 审核失败货号错误\n", account.ID, len(barcodeErrorUIDs))
		}
	}
	log.Printf("[AutoReview] Account=%d candidates=%d approved=%d insufficient=%d barcodeErr=%d\n",
		account.ID, len(candidates), len(approved), len(insufficientUIDs), len(barcodeErrorUIDs))
	if len(approved) == 0 {
		return
	}

	// Step 5: call WanLiNiu in batches of autoReviewBatchSize, then update DB
	now := time.Now()
	for start := 0; start < len(approved); start += autoReviewBatchSize {
		if start > 0 {
			// Rate-limit WanLiNiu push between consecutive batches.
			time.Sleep(autoReviewBatchDelay)
		}
		end := start + autoReviewBatchSize
		if end > len(approved) {
			end = len(approved)
		}
		batch := approved[start:end]

		markItems := make([]model.MarkItem, len(batch))
		uids := make([]string, len(batch))
		for i, a := range batch {
			markItems[i] = a.mark
			uids[i] = a.trade.UID
		}

		// Call WanLiNiu; skip DB update if API fails (will retry next cycle)
		if err := s.BatchMarkOrders(markItems); err != nil {
			log.Printf("[AutoReview] BatchMarkOrders account=%d batch=%d-%d: %v\n",
				account.ID, start, end, err)
			continue
		}

		// Bulk DB update (single UPDATE...WHERE uid IN ?)
		if err := s.orderRepo.BatchMarkApproved(uids, now); err != nil {
			log.Printf("[AutoReview] BatchMarkApproved DB error: %v\n", err)
		}

		// Trigger async billing deduction for each marked trade
		for _, a := range batch {
			a.trade.Mark = model.MarkApproved
			a.trade.MarkApprovedAt = &now
			s.billingService.TriggerDeductionAsync(a.trade)
		}

		log.Printf("[AutoReview] Account=%d marked %d orders\n", account.ID, len(batch))
	}
}

// getAutoReviewShopIDs returns the shop IDs for the given account.
// SuperAdmin is excluded — auto-review is only relevant for non-admin accounts.
func (s *SyncService) getAutoReviewShopIDs(account *model.Account) ([]uint64, error) {
	if account.Role == model.RoleSuperAdmin {
		return nil, nil
	}
	return s.shopRepo.GetAccountShopIDs(account.ID)
}
