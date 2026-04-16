package model

import "time"

// ==================== Status Constants ====================

// OrderStatus — status field from WanLiNiu
const (
	OrderStatusProcessing = 1 // 处理中
	OrderStatusShipped    = 2 // 已发货
	OrderStatusCompleted  = 3 // 已完成
	OrderStatusClosed     = 4 // 已关闭
	OrderStatusOther      = 5 // 其他
)

// Order mark values. Empty string is shown as "待审核" in the frontend.
const (
	MarkApproved     = "已审核"
	MarkDeductFailed = "余额不足扣款失败"
	MarkBarcodeError = "审核失败货号错误"
)

// ProcessStatus —万里牛单据处理状态
const (
	ProcessStatusDistributorAudit = -3 // 分销商审核
	ProcessStatusPaymentMgmt     = -2 // 到账管理
	ProcessStatusUnpaid          = -1 // 未付款
	ProcessStatusAudit           = 0  // 审核
	ProcessStatusPrintPick       = 1  // 打单配货
	ProcessStatusInspect         = 2  // 验货
	ProcessStatusWeigh           = 3  // 称重
	ProcessStatusPendingSend     = 4  // 待发货
	ProcessStatusFinanceAudit    = 5  // 财审
	ProcessStatusSent            = 8  // 已发货
	ProcessStatusSuccess         = 9  // 成功
	ProcessStatusClosed          = 10 // 关闭
	ProcessStatusExceptionEnd    = 11 // 异常结束
	ProcessStatusExceptionHandle = 12 // 异常处理
	ProcessStatusExtPicking      = 13 // 外部系统配货中
	ProcessStatusPreSale         = 14 // 预售
	ProcessStatusPacking         = 15 // 打包
	ProcessStatusPicking         = 19 // 拣货
)

// ==================== Database Models ====================

// OrderTrade is the main order table — one per trade/order from WanLiNiu.
type OrderTrade struct {
	ID                    uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UID                   string     `json:"uid" gorm:"type:varchar(64);uniqueIndex;not null;comment:万里牛订单UID"`
	TradeNo               string     `json:"trade_no" gorm:"type:varchar(64);index;comment:订单编码"`
	ShopName              string     `json:"shop_name" gorm:"type:varchar(128);index;comment:店铺名称"`
	ShopNick              string     `json:"shop_nick" gorm:"type:varchar(128);comment:店铺昵称"`
	SysShop               string     `json:"sys_shop" gorm:"type:varchar(64);index;comment:万里牛店铺ID"`
	SourcePlatform        string     `json:"source_platform" gorm:"type:varchar(64);index;comment:来源平台"`
	ShopType              int        `json:"shop_type" gorm:"type:int;default:0;comment:平台枚举类型"`
	StorageName           string     `json:"storage_name" gorm:"type:varchar(128);comment:仓库名称"`
	StorageCode           string     `json:"storage_code" gorm:"type:varchar(64);comment:仓库编码"`
	BuyerMsg              string     `json:"buyer_msg" gorm:"type:text;comment:买家留言"`
	SellerMsg             string     `json:"seller_msg" gorm:"type:text;comment:卖家留言"`
	OlnStatus             int        `json:"oln_status" gorm:"type:int;default:0;comment:线上状态"`
	BuyerAccount          string     `json:"buyer_account" gorm:"type:varchar(128);comment:买家账号"`
	Buyer                 string     `json:"buyer" gorm:"type:varchar(255);comment:买家"`
	BuyerShow             string     `json:"buyer_show" gorm:"type:varchar(128);comment:买家显示名"`
	Receiver              string     `json:"receiver" gorm:"type:varchar(128);comment:收件人"`
	Phone                 string     `json:"phone" gorm:"type:varchar(64);comment:手机号"`
	Country               string     `json:"country" gorm:"type:varchar(64);comment:国家"`
	Province              string     `json:"province" gorm:"type:varchar(64);comment:省"`
	City                  string     `json:"city" gorm:"type:varchar(64);comment:市"`
	District              string     `json:"district" gorm:"type:varchar(64);comment:区"`
	Town                  string     `json:"town" gorm:"type:varchar(64);comment:街道"`
	Address               string     `json:"address" gorm:"type:text;comment:地址"`
	Zip                   string     `json:"zip" gorm:"type:varchar(32);comment:邮编"`
	CreateTimeMs          int64      `json:"create_time_ms" gorm:"type:bigint;index;comment:创建时间戳ms"`
	ModifyTimeMs          int64      `json:"modify_time_ms" gorm:"type:bigint;index;comment:修改时间戳ms"`
	PayTimeMs             int64      `json:"pay_time_ms" gorm:"type:bigint;comment:付款时间戳ms"`
	SendTimeMs            int64      `json:"send_time_ms" gorm:"type:bigint;comment:发货时间戳ms"`
	PrintTimeMs           int64      `json:"print_time_ms" gorm:"type:bigint;comment:打单时间戳ms"`
	IndexTimeMs           int64      `json:"index_time_ms" gorm:"type:bigint;comment:系统更新时间戳ms"`
	ApproveTimeMs         int64      `json:"approve_time_ms" gorm:"type:bigint;comment:审核时间戳ms"`
	EstimateSendTimeMs    int64      `json:"estimate_send_time_ms" gorm:"type:bigint;comment:预计发货时间戳ms"`
	Status                int        `json:"status" gorm:"type:int;index;comment:状态 1处理中 2发货 3完成 4关闭 5其他"`
	ProcessStatus         int        `json:"process_status" gorm:"type:int;index;index:idx_refund_scan,priority:1;comment:万里牛处理状态"`
	IsPay                 bool       `json:"is_pay" gorm:"comment:是否已付款"`
	TpTid                 string     `json:"tp_tid" gorm:"type:varchar(255);comment:线上单号"`
	ExpressCode           string     `json:"express_code" gorm:"type:varchar(128);index;comment:快递单号"`
	LogisticCode          string     `json:"logistic_code" gorm:"type:varchar(64);comment:快递公司代码"`
	LogisticName          string     `json:"logistic_name" gorm:"type:varchar(128);comment:快递公司名称"`
	ChannelName           string     `json:"channel_name" gorm:"type:varchar(128);comment:渠道昵称"`
	SumSale               float64    `json:"sum_sale" gorm:"type:decimal(12,2);default:0;comment:总金额"`
	PostFee               float64    `json:"post_fee" gorm:"type:decimal(12,2);default:0;comment:邮费"`
	PaidFee               float64    `json:"paid_fee" gorm:"type:decimal(12,2);default:0;comment:应收款"`
	DiscountFee           float64    `json:"discount_fee" gorm:"type:decimal(12,2);default:0;comment:优惠金额"`
	ServiceFee            float64    `json:"service_fee" gorm:"type:decimal(12,2);default:0;comment:服务费"`
	RealPayment           float64    `json:"real_payment" gorm:"type:decimal(12,2);default:0;comment:买家实付"`
	PostCost              float64    `json:"post_cost" gorm:"type:decimal(12,2);default:0;comment:快递成本"`
	HasRefund             int        `json:"has_refund" gorm:"type:int;default:0;comment:是否有退款"`
	IsExceptionTrade      bool       `json:"is_exception_trade" gorm:"comment:是否异常订单"`
	TradeType             int        `json:"trade_type" gorm:"type:int;default:1;comment:订单类型"`
	Mark                  string     `json:"mark" gorm:"type:varchar(255);comment:订单标记"`
	Flag                  int        `json:"flag" gorm:"type:int;default:0;comment:旗子颜色"`
	PayNo                 string     `json:"pay_no" gorm:"type:varchar(128);comment:支付单号"`
	PayType               string     `json:"pay_type" gorm:"type:varchar(64);comment:支付类型"`
	CurrencyCode          string     `json:"currency_code" gorm:"type:varchar(32);comment:货币种类"`
	CurrencySum           float64    `json:"currency_sum" gorm:"type:decimal(12,2);default:0;comment:原始货币金额"`
	Weight                float64    `json:"weight" gorm:"type:decimal(10,3);default:0;comment:重量kg"`
	Volume                float64    `json:"volume" gorm:"type:decimal(10,4);default:0;comment:体积m³"`
	EstimateWeight        float64    `json:"estimate_weight" gorm:"type:decimal(10,3);default:0;comment:估重"`
	TpLogisticsType       int        `json:"tp_logistics_type" gorm:"type:int;default:0;comment:物流方式"`
	OriginalNo            string     `json:"original_no" gorm:"type:varchar(128);comment:原始单号"`
	OriginalShopType      int        `json:"original_shop_type" gorm:"type:int;default:0;comment:原始平台"`
	WaveNo                string     `json:"wave_no" gorm:"type:varchar(64);comment:波次号"`
	BatchSerial           string     `json:"batch_serial" gorm:"type:varchar(64);comment:批次流水号"`
	GxOriginTradeID       string     `json:"gx_origin_trade_id" gorm:"type:varchar(128);comment:分销原订单单号"`
	IdentityNum           string     `json:"identity_num" gorm:"type:varchar(128);comment:身份信息"`
	IdentityName          string     `json:"identity_name" gorm:"type:varchar(128);comment:身份证名称"`
	BuyerMobile           string     `json:"buyer_mobile" gorm:"type:varchar(64);comment:收件人手机号"`
	Tel                   string     `json:"tel" gorm:"type:varchar(64);comment:电话"`
	PostCurrency          float64    `json:"post_currency" gorm:"type:decimal(12,2);default:0;comment:原币运费"`
	ErrorID               int        `json:"error_id" gorm:"type:int;default:0;comment:异常ID"`
	ShippedOutboundType   int        `json:"shipped_outbound_type" gorm:"type:int;default:0;comment:出库状态"`
	OperApprove           string     `json:"oper_approve" gorm:"type:varchar(64);comment:审单员"`
	OperIntimidate        string     `json:"oper_intimidate" gorm:"type:varchar(64);comment:打单员"`
	OperDistribution      string     `json:"oper_distribution" gorm:"type:varchar(64);comment:配货员"`
	OperInspection        string     `json:"oper_inspection" gorm:"type:varchar(64);comment:验货员"`
	OperSend              string     `json:"oper_send" gorm:"type:varchar(64);comment:发货员"`
	Additon               string     `json:"additon" gorm:"type:text;comment:附加信息"`
	SplitTrade            bool       `json:"split_trade" gorm:"comment:是否拆分订单"`
	ExchangeTrade         bool       `json:"exchange_trade" gorm:"comment:是否售后订单"`
	IsSmallTrade          bool       `json:"is_small_trade" gorm:"comment:是否jit小单"`
	OlnOrderListJSON      string     `json:"-" gorm:"type:text;comment:明细线上单号JSON"`
	MergeUidsJSON         string     `json:"-" gorm:"type:text;comment:合并前订单号JSON"`
	PlatformDiscountJSON  string     `json:"-" gorm:"type:text;comment:平台优惠信息JSON"`
	MarkApprovedAt        *time.Time `json:"mark_approved_at" gorm:"type:datetime;index;comment:mark变为已审核的时间"`
	BillingStatus         int8       `json:"billing_status" gorm:"type:tinyint;default:0;index;index:idx_refund_scan,priority:2;comment:0未扣款1成功2余额不足3错误4已退款"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`

	// Non-DB fields for JSON response
	Items []OrderItem `json:"items,omitempty" gorm:"-"`
}

func (OrderTrade) TableName() string { return "order_trade" }

// OrderItem is the order detail/line item — many per OrderTrade.
type OrderItem struct {
	ID                  uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TradeUID            string    `json:"trade_uid" gorm:"type:varchar(64);index;not null;comment:关联订单UID"`
	OrderID             string    `json:"order_id" gorm:"type:varchar(64);uniqueIndex;not null;comment:万里牛明细ID"`
	ItemName            string    `json:"item_name" gorm:"type:varchar(255);comment:商品名称"`
	SkuName             string    `json:"sku_name" gorm:"type:varchar(255);comment:SKU名称"`
	SkuCode             string    `json:"sku_code" gorm:"type:varchar(128);index;comment:SKU编码"`
	Size                int       `json:"size" gorm:"type:int;default:1;comment:数量"`
	Price               float64   `json:"price" gorm:"type:decimal(12,2);default:0;comment:单价"`
	DiscountedUnitPrice float64   `json:"discounted_unit_price" gorm:"type:decimal(12,2);default:0;comment:折后单价"`
	Receivable          float64   `json:"receivable" gorm:"type:decimal(12,2);default:0;comment:应收"`
	OrderTotalDiscount  float64   `json:"order_total_discount" gorm:"type:decimal(12,2);default:0;comment:优惠金额"`
	Payment             float64   `json:"payment" gorm:"type:decimal(12,2);default:0;comment:实付"`
	IsPackage           bool      `json:"is_package" gorm:"comment:是否组合装"`
	TpTid               string    `json:"tp_tid" gorm:"type:varchar(128);comment:线上单号"`
	TpOid               string    `json:"tp_oid" gorm:"type:varchar(128);comment:线上子单号"`
	OlnItemID           string    `json:"oln_item_id" gorm:"type:varchar(128);comment:线上商品ID"`
	OlnItemCode         string    `json:"oln_item_code" gorm:"type:varchar(128);comment:线上商品编码"`
	OlnSkuCode          string    `json:"oln_sku_code" gorm:"type:varchar(128);comment:线上SKU编码"`
	OlnStatus           int       `json:"oln_status" gorm:"type:int;default:0;comment:线上状态"`
	OlnSkuID            string    `json:"oln_sku_id" gorm:"type:varchar(128);comment:线上SKU ID"`
	OlnSkuName          string    `json:"oln_sku_name" gorm:"type:varchar(512);comment:线上SKU名称"`
	OlnItemName         string    `json:"oln_item_name" gorm:"type:varchar(512);comment:线上商品名称"`
	SysGoodsUID         string    `json:"sys_goods_uid" gorm:"type:varchar(64);comment:系统商品UID"`
	SysSpecUID          string    `json:"sys_spec_uid" gorm:"type:varchar(64);comment:系统规格UID"`
	InventoryStatus     string    `json:"inventory_status" gorm:"type:varchar(32);comment:库存状态"`
	Status              int       `json:"status" gorm:"type:int;default:0;comment:状态"`
	HasRefund           int       `json:"has_refund" gorm:"type:int;default:0;comment:是否有退款"`
	Remark              string    `json:"remark" gorm:"type:text;comment:备注"`
	IsGift              int       `json:"is_gift" gorm:"type:int;default:0;comment:是否赠品"`
	CurrencySum         float64   `json:"currency_sum" gorm:"type:decimal(12,2);default:0;comment:原币金额"`
	ItemImageURL        string    `json:"item_image_url" gorm:"type:varchar(512);comment:商品图片"`
	ItemPlatformURL     string    `json:"item_platform_url" gorm:"type:varchar(512);comment:平台链接"`
	TidSnapshot         string    `json:"tid_snapshot" gorm:"type:varchar(128);comment:快照ID"`
	TaxRate             float64   `json:"tax_rate" gorm:"type:decimal(6,4);default:0;comment:税率"`
	TaxPayment          float64   `json:"tax_payment" gorm:"type:decimal(12,2);default:0;comment:税额"`
	BarCode             string    `json:"bar_code" gorm:"type:varchar(128);comment:条码"`
	GxPayment           float64   `json:"gx_payment" gorm:"type:decimal(12,2);default:0;comment:分销实付"`
	GxPrice             float64   `json:"gx_price" gorm:"type:decimal(12,2);default:0;comment:分销单价"`
	EstimateSendTimeMs  int64     `json:"estimate_send_time_ms" gorm:"type:bigint;comment:预计发货时间ms"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (OrderItem) TableName() string { return "order_item" }

// Shop represents a unique shop extracted from order data.
type Shop struct {
	ID             uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SysShop        string    `json:"sys_shop" gorm:"type:varchar(64);uniqueIndex;not null;comment:万里牛店铺ID"`
	ShopName       string    `json:"shop_name" gorm:"type:varchar(128);comment:店铺名称"`
	ShopNick       string    `json:"shop_nick" gorm:"type:varchar(128);comment:店铺昵称"`
	SourcePlatform string    `json:"source_platform" gorm:"type:varchar(64);index;comment:来源平台"`
	ShopType       int       `json:"shop_type" gorm:"type:int;default:0;comment:平台类型"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Shop) TableName() string { return "shop" }

// AccountShop is the many-to-many relation between accounts and shops.
type AccountShop struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID uint64    `json:"account_id" gorm:"not null;uniqueIndex:uk_account_shop"`
	ShopID    uint64    `json:"shop_id" gorm:"not null;uniqueIndex:uk_account_shop;index:idx_shop_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (AccountShop) TableName() string { return "account_shop" }

// SyncState tracks the last sync timestamp for incremental syncing.
type SyncState struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SyncKey      string    `json:"sync_key" gorm:"type:varchar(64);uniqueIndex;not null;comment:同步标识"`
	LastSyncTime int64     `json:"last_sync_time" gorm:"type:bigint;comment:上次同步时间戳ms"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (SyncState) TableName() string { return "sync_state" }

// ==================== Request / Response DTOs ====================

// OrderListReq is the query parameters for listing orders.
type OrderListReq struct {
	Page           int    `form:"page" binding:"omitempty,min=1"`
	PageSize       int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	SourcePlatform string `form:"source_platform"`    // 平台筛选
	ShopName       string `form:"shop_name"`           // 店铺名筛选
	SysShop        string `form:"sys_shop"`            // 店铺ID筛选
	StartTime      string `form:"start_time"`          // 开始时间 YYYY-MM-DD HH:MM:SS
	EndTime        string `form:"end_time"`            // 结束时间
	Status         string `form:"status"`              // 状态筛选
	Keyword        string `form:"keyword"`             // 订单号/快递单号搜索
}

// OrderListResp is the response for listing orders.
type OrderListResp struct {
	Total int64        `json:"total"`
	List  []OrderTrade `json:"list"`
}

// UpdateAccountShopsReq is the request to set shop permissions for an account.
type UpdateAccountShopsReq struct {
	ShopIDs []uint64 `json:"shop_ids" binding:"required"`
}

// UpdateOrderItem represents a single order's updatable fields.
// TradeNo is required; all other fields are optional pointers — only non-nil fields are written.
type UpdateOrderItem struct {
	TradeNo      string   `json:"trade_no" binding:"required"`
	Mark         *string  `json:"mark"`
	Flag         *int     `json:"flag"`
	SellerMsg    *string  `json:"seller_msg"`
	BuyerMsg     *string  `json:"buyer_msg"`
	Receiver     *string  `json:"receiver"`
	Phone        *string  `json:"phone"`
	Province     *string  `json:"province"`
	City         *string  `json:"city"`
	District     *string  `json:"district"`
	Town         *string  `json:"town"`
	Address      *string  `json:"address"`
	Zip          *string  `json:"zip"`
	ExpressCode  *string  `json:"express_code"`
	LogisticCode *string  `json:"logistic_code"`
	LogisticName *string  `json:"logistic_name"`
	ChannelName  *string  `json:"channel_name"`
}

// BatchUpdateOrderReq is a list of order update items.
type BatchUpdateOrderReq []UpdateOrderItem

// MarkItem represents a single mark operation sent to WanLiNiu.
type MarkItem struct {
	BillCode string `json:"bill_code" binding:"required"`
	MarkName string `json:"mark_name"`
	Type     int    `json:"type"` // 0=覆盖 1=追加 2=清除
}

// BatchMarkReq is a list of mark operations.
type BatchMarkReq []MarkItem

// ShopListResp groups shops by platform.
type ShopsByPlatformResp struct {
	Platform string `json:"platform"`
	Shops    []Shop `json:"shops"`
}

// StatusOption for frontend dropdown.
type StatusOption struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

// GetProcessStatusLabel returns a display label for the process_status value.
func GetProcessStatusLabel(ps int) string {
	switch ps {
	case ProcessStatusDistributorAudit:
		return "分销商审核"
	case ProcessStatusPaymentMgmt:
		return "到账管理"
	case ProcessStatusUnpaid:
		return "未付款"
	case ProcessStatusAudit:
		return "待审核"
	case ProcessStatusPrintPick:
		return "打单配货"
	case ProcessStatusInspect:
		return "验货"
	case ProcessStatusWeigh:
		return "称重"
	case ProcessStatusPendingSend:
		return "待发货"
	case ProcessStatusFinanceAudit:
		return "财审"
	case ProcessStatusSent:
		return "已发货"
	case ProcessStatusSuccess:
		return "已完成"
	case ProcessStatusClosed:
		return "已关闭"
	case ProcessStatusExceptionEnd:
		return "异常结束"
	case ProcessStatusExceptionHandle:
		return "异常处理"
	case ProcessStatusExtPicking:
		return "配货中"
	case ProcessStatusPreSale:
		return "预售"
	case ProcessStatusPacking:
		return "打包"
	case ProcessStatusPicking:
		return "拣货"
	default:
		return "未知"
	}
}

// GetAllProcessStatusOptions returns all status options for frontend dropdown.
func GetAllProcessStatusOptions() []StatusOption {
	return []StatusOption{
		{Value: ProcessStatusUnpaid, Label: "未付款"},
		{Value: ProcessStatusAudit, Label: "待审核"},
		{Value: ProcessStatusPrintPick, Label: "打单配货"},
		{Value: ProcessStatusInspect, Label: "验货"},
		{Value: ProcessStatusWeigh, Label: "称重"},
		{Value: ProcessStatusPendingSend, Label: "待发货"},
		{Value: ProcessStatusFinanceAudit, Label: "财审"},
		{Value: ProcessStatusSent, Label: "已发货"},
		{Value: ProcessStatusSuccess, Label: "已完成"},
		{Value: ProcessStatusClosed, Label: "已关闭"},
		{Value: ProcessStatusPacking, Label: "打包"},
		{Value: ProcessStatusPicking, Label: "拣货"},
	}
}
