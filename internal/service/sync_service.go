package service

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"supply-chain/internal/config"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"
	"time"
)

const (
	syncTypeOrder     = "wanliniu_order"
	wanLiNiuBaseURL   = "https://open.wanliniu.com/erp"
	wanLiNiuTradeAPI  = "/trades/get"
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

// Start begins the periodic sync in background
func (s *SyncService) Start() {
	if s.cfg.AppKey == "" || s.cfg.AppSecret == "" {
		log.Println("[Sync] WanLiNiu config not set, sync disabled")
		return
	}
	log.Println("[Sync] Starting order sync, interval=60s")
	go s.run()
}

// Stop stops the sync
func (s *SyncService) Stop() {
	close(s.stopCh)
}

// ManualSync triggers an immediate sync
func (s *SyncService) ManualSync() error {
	if s.cfg.AppKey == "" || s.cfg.AppSecret == "" {
		return fmt.Errorf("万里牛配置未设置，无法同步")
	}
	return s.syncOnce()
}

func (s *SyncService) run() {
	// Run immediately on start
	if err := s.syncOnce(); err != nil {
		log.Printf("[Sync] Error: %v\n", err)
	}

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.syncOnce(); err != nil {
				log.Printf("[Sync] Error: %v\n", err)
			}
		case <-s.stopCh:
			log.Println("[Sync] Stopped")
			return
		}
	}
}

func (s *SyncService) syncOnce() error {
	// Get last sync time
	state, err := s.orderRepo.GetSyncState(syncTypeOrder)
	var startTime time.Time
	if err != nil {
		// First sync: default to 30 days ago
		startTime = time.Now().Add(-30 * 24 * time.Hour)
	} else {
		startTime = state.LastSyncTime
	}

	endTime := time.Now()
	pageNo := 1
	pageSize := 100
	totalSynced := 0
	latestModifyTime := startTime

	for {
		trades, total, err := s.fetchTrades(startTime, endTime, pageNo, pageSize)
		if err != nil {
			return fmt.Errorf("fetch trades page %d: %w", pageNo, err)
		}

		for _, trade := range trades {
			if err := s.saveTrade(&trade); err != nil {
				log.Printf("[Sync] Failed to save trade uid=%s: %v\n", trade.UID, err)
				continue
			}
			totalSynced++

			// Track latest modify_time
			if trade.ModifyTime != nil && trade.ModifyTime.After(latestModifyTime) {
				latestModifyTime = *trade.ModifyTime
			}
		}

		if pageNo*pageSize >= total {
			break
		}
		pageNo++
	}

	// Update sync state
	if err := s.orderRepo.UpdateSyncState(syncTypeOrder, latestModifyTime); err != nil {
		log.Printf("[Sync] Failed to update sync state: %v\n", err)
	}

	if totalSynced > 0 {
		log.Printf("[Sync] Synced %d orders\n", totalSynced)
	}
	return nil
}

// saveTrade saves a trade and its items, and auto-creates the shop
func (s *SyncService) saveTrade(trade *wlnTrade) error {
	// Auto-create shop
	if trade.ShopName != "" {
		shop := &model.Shop{
			ShopName: trade.ShopName,
			Platform: trade.Platform,
		}
		if err := s.shopRepo.UpsertShop(shop); err != nil {
			log.Printf("[Sync] Failed to upsert shop %s: %v\n", trade.ShopName, err)
		}
	}

	// Convert to model
	orderTrade := trade.toOrderTrade()
	if err := s.orderRepo.UpsertTrade(orderTrade); err != nil {
		return err
	}

	// Save items
	items := trade.toOrderItems()
	if len(items) > 0 {
		if err := s.orderRepo.UpsertItems(trade.UID, items); err != nil {
			return err
		}
	}

	return nil
}

// ---------- WanLiNiu API ----------

type wlnTradeResp struct {
	Success bool   `json:"success"`
	ErrMsg  string `json:"err_msg"`
	Data    struct {
		Total  int        `json:"total"`
		Trades []wlnTrade `json:"trades"`
	} `json:"data"`
}

type wlnTrade struct {
	UID              string     `json:"uid"`
	OrderID          string     `json:"order_id"`
	Platform         string     `json:"platform"`
	ShopName         string     `json:"shop_name"`
	Status           string     `json:"status"`
	TradeStatus      string     `json:"trade_status"`
	BuyerNick        string     `json:"buyer_nick"`
	ReceiverName     string     `json:"receiver_name"`
	ReceiverPhone    string     `json:"receiver_phone"`
	ReceiverProvince string     `json:"receiver_province"`
	ReceiverCity     string     `json:"receiver_city"`
	ReceiverDistrict string     `json:"receiver_district"`
	ReceiverAddress  string     `json:"receiver_address"`
	TotalAmount      float64    `json:"total_amount"`
	PayAmount        float64    `json:"pay_amount"`
	PostFee          float64    `json:"post_fee"`
	DiscountFee      float64    `json:"discount_fee"`
	LogisticsName    string     `json:"logistics_name"`
	LogisticsNo      string     `json:"logistics_no"`
	BuyerMessage     string     `json:"buyer_message"`
	SellerRemark     string     `json:"seller_remark"`
	PayTime          string     `json:"pay_time"`
	SendTime         string     `json:"send_time"`
	TradeTime        string     `json:"trade_time"`
	ModifyTime       string     `json:"modify_time"`
	Items            []wlnItem  `json:"items"`
}

type wlnItem struct {
	ItemID       string  `json:"item_id"`
	SkuID        string  `json:"sku_id"`
	ProductName  string  `json:"product_name"`
	SkuName      string  `json:"sku_name"`
	Quantity     int     `json:"quantity"`
	Price        float64 `json:"price"`
	TotalFee     float64 `json:"total_fee"`
	RefundStatus string  `json:"refund_status"`
	PicURL       string  `json:"pic_url"`
}

func (t *wlnTrade) toOrderTrade() *model.OrderTrade {
	return &model.OrderTrade{
		UID:              t.UID,
		OrderID:          t.OrderID,
		Platform:         t.Platform,
		ShopName:         t.ShopName,
		Status:           t.Status,
		TradeStatus:      t.TradeStatus,
		BuyerNick:        t.BuyerNick,
		ReceiverName:     t.ReceiverName,
		ReceiverPhone:    t.ReceiverPhone,
		ReceiverProvince: t.ReceiverProvince,
		ReceiverCity:     t.ReceiverCity,
		ReceiverDistrict: t.ReceiverDistrict,
		ReceiverAddress:  t.ReceiverAddress,
		TotalAmount:      t.TotalAmount,
		PayAmount:        t.PayAmount,
		PostFee:          t.PostFee,
		DiscountFee:      t.DiscountFee,
		LogisticsName:    t.LogisticsName,
		LogisticsNo:      t.LogisticsNo,
		BuyerMessage:     t.BuyerMessage,
		SellerRemark:     t.SellerRemark,
		PayTime:          parseTime(t.PayTime),
		SendTime:         parseTime(t.SendTime),
		TradeTime:        parseTime(t.TradeTime),
		ModifyTime:       parseTime(t.ModifyTime),
	}
}

func (t *wlnTrade) toOrderItems() []model.OrderItem {
	items := make([]model.OrderItem, 0, len(t.Items))
	for _, item := range t.Items {
		items = append(items, model.OrderItem{
			TradeUID:     t.UID,
			ItemID:       item.ItemID,
			SkuID:        item.SkuID,
			ProductName:  item.ProductName,
			SkuName:      item.SkuName,
			Quantity:     item.Quantity,
			Price:        item.Price,
			TotalFee:     item.TotalFee,
			RefundStatus: item.RefundStatus,
			PicURL:       item.PicURL,
		})
	}
	return items
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		return nil
	}
	return &t
}

func (s *SyncService) fetchTrades(startTime, endTime time.Time, pageNo, pageSize int) ([]wlnTrade, int, error) {
	params := map[string]string{
		"app_key":      s.cfg.AppKey,
		"method":       "trades.get",
		"timestamp":    time.Now().Format("2006-01-02 15:04:05"),
		"v":            "1.0",
		"format":       "json",
		"modify_start": startTime.Format("2006-01-02 15:04:05"),
		"modify_end":   endTime.Format("2006-01-02 15:04:05"),
		"page_no":      fmt.Sprintf("%d", pageNo),
		"page_size":    fmt.Sprintf("%d", pageSize),
	}

	// Sign the request
	params["sign"] = s.sign(params)

	// Build URL
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	apiURL := wanLiNiuBaseURL + wanLiNiuTradeAPI + "?" + values.Encode()

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read response body: %w", err)
	}

	var result wlnTradeResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, 0, fmt.Errorf("parse response: %w", err)
	}

	if !result.Success {
		return nil, 0, fmt.Errorf("API error: %s", result.ErrMsg)
	}

	return result.Data.Trades, result.Data.Total, nil
}

// sign generates the MD5 signature for WanLiNiu API
func (s *SyncService) sign(params map[string]string) string {
	// Sort keys
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build sign string: app_secret + key1value1key2value2... + app_secret
	var buf strings.Builder
	buf.WriteString(s.cfg.AppSecret)
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteString(params[k])
	}
	buf.WriteString(s.cfg.AppSecret)

	hash := md5.Sum([]byte(buf.String()))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}
