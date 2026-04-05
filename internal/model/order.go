package model

import "time"

// Order status constants (from 万里牛 ERP)
const (
	OrderStatusWaitPay     = "WAIT_PAY"     // 待付款
	OrderStatusPaid        = "PAID"         // 已付款
	OrderStatusWaitSend    = "WAIT_SEND"    // 待发货
	OrderStatusSent        = "SENT"         // 已发货
	OrderStatusSuccess     = "SUCCESS"      // 交易成功
	OrderStatusClosed      = "CLOSED"       // 交易关闭
	OrderStatusRefunding   = "REFUNDING"    // 退款中
	OrderStatusRefunded    = "REFUNDED"     // 已退款
)

// ---------- Database Models ----------

// OrderTrade represents a trade order synced from 万里牛 ERP
type OrderTrade struct {
	ID              uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UID             string     `json:"uid" gorm:"type:varchar(64);uniqueIndex:uk_uid;not null"`
	OrderID         string     `json:"order_id" gorm:"type:varchar(64);not null;index:idx_order_id"`
	Platform        string     `json:"platform" gorm:"type:varchar(64);not null;default:''"`
	ShopName        string     `json:"shop_name" gorm:"type:varchar(128);not null;default:'';index:idx_shop_name"`
	Status          string     `json:"status" gorm:"type:varchar(32);not null;default:'';index:idx_status"`
	TradeStatus     string     `json:"trade_status" gorm:"type:varchar(64);not null;default:''"`
	BuyerNick       string     `json:"buyer_nick" gorm:"type:varchar(128);not null;default:''"`
	ReceiverName    string     `json:"receiver_name" gorm:"type:varchar(128);not null;default:''"`
	ReceiverPhone   string     `json:"receiver_phone" gorm:"type:varchar(32);not null;default:''"`
	ReceiverProvince string    `json:"receiver_province" gorm:"type:varchar(64);not null;default:''"`
	ReceiverCity    string     `json:"receiver_city" gorm:"type:varchar(64);not null;default:''"`
	ReceiverDistrict string   `json:"receiver_district" gorm:"type:varchar(64);not null;default:''"`
	ReceiverAddress string     `json:"receiver_address" gorm:"type:varchar(512);not null;default:''"`
	TotalAmount     float64    `json:"total_amount" gorm:"type:decimal(12,2);not null;default:0.00"`
	PayAmount       float64    `json:"pay_amount" gorm:"type:decimal(12,2);not null;default:0.00"`
	PostFee         float64    `json:"post_fee" gorm:"type:decimal(12,2);not null;default:0.00"`
	DiscountFee     float64    `json:"discount_fee" gorm:"type:decimal(12,2);not null;default:0.00"`
	LogisticsName   string     `json:"logistics_name" gorm:"type:varchar(64);not null;default:''"`
	LogisticsNo     string     `json:"logistics_no" gorm:"type:varchar(128);not null;default:'';index:idx_logistics_no"`
	BuyerMessage    string     `json:"buyer_message" gorm:"type:text"`
	SellerRemark    string     `json:"seller_remark" gorm:"type:text"`
	PayTime         *time.Time `json:"pay_time" gorm:"type:datetime"`
	SendTime        *time.Time `json:"send_time" gorm:"type:datetime"`
	TradeTime       *time.Time `json:"trade_time" gorm:"type:datetime;index:idx_trade_time"`
	ModifyTime      *time.Time `json:"modify_time" gorm:"type:datetime;index:idx_modify_time"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (OrderTrade) TableName() string { return "order_trade" }

// OrderItem represents a line item in an order
type OrderItem struct {
	ID          uint64  `json:"id" gorm:"primaryKey;autoIncrement"`
	TradeUID    string  `json:"trade_uid" gorm:"type:varchar(64);not null;index:idx_trade_uid"`
	ItemID      string  `json:"item_id" gorm:"type:varchar(64);not null;default:''"`
	SkuID       string  `json:"sku_id" gorm:"type:varchar(64);not null;default:''"`
	ProductName string  `json:"product_name" gorm:"type:varchar(255);not null;default:''"`
	SkuName     string  `json:"sku_name" gorm:"type:varchar(255);not null;default:''"`
	Quantity    int     `json:"quantity" gorm:"type:int;not null;default:0"`
	Price       float64 `json:"price" gorm:"type:decimal(12,2);not null;default:0.00"`
	TotalFee    float64 `json:"total_fee" gorm:"type:decimal(12,2);not null;default:0.00"`
	RefundStatus string `json:"refund_status" gorm:"type:varchar(32);not null;default:''"`
	PicURL      string  `json:"pic_url" gorm:"type:varchar(512);not null;default:''"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (OrderItem) TableName() string { return "order_item" }

// Shop represents a store/shop extracted from orders
type Shop struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ShopName  string    `json:"shop_name" gorm:"type:varchar(128);uniqueIndex:uk_shop_name;not null"`
	Platform  string    `json:"platform" gorm:"type:varchar(64);not null;default:''"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Shop) TableName() string { return "shop" }

// AccountShop represents the many-to-many relationship between accounts and shops
type AccountShop struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID uint64    `json:"account_id" gorm:"not null;uniqueIndex:uk_account_shop"`
	ShopID    uint64    `json:"shop_id" gorm:"not null;uniqueIndex:uk_account_shop;index:idx_shop_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (AccountShop) TableName() string { return "account_shop" }

// SyncState tracks the last sync timestamp for incremental syncing
type SyncState struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SyncType    string    `json:"sync_type" gorm:"type:varchar(64);uniqueIndex:uk_sync_type;not null"`
	LastSyncTime time.Time `json:"last_sync_time" gorm:"type:datetime;not null"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (SyncState) TableName() string { return "sync_state" }

// ---------- Request / Response DTOs ----------

type OrderListReq struct {
	Keyword   string `form:"keyword"`
	Status    string `form:"status"`
	ShopName  string `form:"shop_name"`
	Platform  string `form:"platform"`
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size"`
}

type OrderListResp struct {
	List  []OrderTrade `json:"list"`
	Total int64        `json:"total"`
	Page  int          `json:"page"`
	PageSize int       `json:"page_size"`
}

type OrderDetailResp struct {
	Trade OrderTrade  `json:"trade"`
	Items []OrderItem `json:"items"`
}

type ShopGroupedResp struct {
	Platform string `json:"platform"`
	Shops    []Shop `json:"shops"`
}

type UpdateAccountShopsReq struct {
	ShopIDs []uint64 `json:"shop_ids" binding:"required"`
}

type AccountShopResp struct {
	AccountID uint64 `json:"account_id"`
	Shops     []Shop `json:"shops"`
}
