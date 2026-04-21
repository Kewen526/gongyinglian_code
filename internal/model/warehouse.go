package model

import "time"

// Warehouse billing status constants for order_trade.warehouse_status
const (
	WarehouseStatusPending      int8 = 0 // 未扣款
	WarehouseStatusSuccess      int8 = 1 // 扣款成功
	WarehouseStatusInsufficient int8 = 2 // 余额不足
)

// ---------- Database Models ----------

type WarehouseWallet struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID uint64    `json:"account_id" gorm:"not null;uniqueIndex;comment:账号ID"`
	Balance   float64   `json:"balance" gorm:"type:decimal(12,2);default:0;comment:可用余额"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (WarehouseWallet) TableName() string { return "warehouse_wallet" }

type WarehouseRechargeRequest struct {
	ID            uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID     uint64    `json:"account_id" gorm:"not null;index;comment:账号ID"`
	Amount        float64   `json:"amount" gorm:"type:decimal(12,2);not null;comment:充值金额"`
	PaymentMethod string    `json:"payment_method" gorm:"type:varchar(32);not null;comment:支付方式 wechat/alipay/bank"`
	TransactionNo string    `json:"transaction_no" gorm:"type:varchar(128);not null;uniqueIndex;comment:交易流水号"`
	VoucherURL    string    `json:"voucher_url" gorm:"type:varchar(512);not null;comment:凭证截图URL"`
	Status        string    `json:"status" gorm:"type:varchar(16);default:'pending';comment:状态 pending/approved/rejected"`
	Remark        string    `json:"remark" gorm:"type:text;comment:备注"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (WarehouseRechargeRequest) TableName() string { return "warehouse_recharge_request" }

type WarehouseBillingRecord struct {
	ID            uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	FlowNo        string    `json:"flow_no" gorm:"type:varchar(32);not null;uniqueIndex;comment:云仓流水单号 CW-YYYYMMDD-NNN"`
	AccountID     uint64    `json:"account_id" gorm:"not null;index;comment:账号ID"`
	TradeNo       string    `json:"trade_no" gorm:"type:varchar(64);index;comment:关联订单号"`
	TradeUID      string    `json:"trade_uid" gorm:"type:varchar(64);uniqueIndex;comment:订单UID"`
	Platform      string    `json:"platform" gorm:"type:varchar(64);default:'';comment:平台"`
	ShopName      string    `json:"shop_name" gorm:"type:varchar(128);comment:店铺名"`
	BusinessType  string    `json:"business_type" gorm:"type:varchar(32);default:'订单发货';comment:业务类型"`
	Type          string    `json:"type" gorm:"type:varchar(16);default:'deduct';comment:deduct/recharge"`
	ShippingFee   float64   `json:"shipping_fee" gorm:"type:decimal(12,2);default:0;comment:运费"`
	PackingFee    float64   `json:"packing_fee" gorm:"type:decimal(12,2);default:0;comment:打包费"`
	TotalAmount   float64   `json:"total_amount" gorm:"type:decimal(12,2);default:0;comment:总扣款金额"`
	ItemCount     int       `json:"item_count" gorm:"type:int;default:0;comment:总件数"`
	BalanceBefore float64   `json:"balance_before" gorm:"type:decimal(12,2);default:0;comment:扣前余额"`
	BalanceAfter  float64   `json:"balance_after" gorm:"type:decimal(12,2);default:0;comment:扣后余额"`
	Status        string    `json:"status" gorm:"type:varchar(16);comment:success/insufficient"`
	TradeTime     *time.Time `json:"trade_time" gorm:"comment:交易时间(发货时间)"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (WarehouseBillingRecord) TableName() string { return "warehouse_billing_record" }

// WarehouseFlowCounter is an atomic per-day sequence table for CW flow numbers.
type WarehouseFlowCounter struct {
	Date string `gorm:"primaryKey;type:varchar(8);comment:YYYYMMDD"`
	Seq  int    `gorm:"not null;default:0"`
}

func (WarehouseFlowCounter) TableName() string { return "warehouse_flow_counter" }

// ---------- Request / Response DTOs ----------

type WarehouseWalletResp struct {
	Balance float64 `json:"balance"`
}

type WarehouseSubmitRechargeReq struct {
	Amount        float64 `json:"amount" binding:"required,gt=0"`
	PaymentMethod string  `json:"payment_method" binding:"required,oneof=wechat alipay bank"`
	TransactionNo string  `json:"transaction_no" binding:"required"`
	VoucherURL    string  `json:"voucher_url" binding:"required"`
}

type WarehouseBillingListReq struct {
	Keyword   string `form:"keyword"`
	ShopName  string `form:"shop_name"`
	Type      string `form:"type"`
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size"`
}

type WarehouseBillingListResp struct {
	Total  int64                    `json:"total"`
	List   []WarehouseBillingRecord `json:"list"`
	Wallet WarehouseWalletResp      `json:"wallet"`
}

type WarehouseMyRechargeListReq struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

type WarehouseMyRechargeListResp struct {
	Total int64                      `json:"total"`
	List  []WarehouseRechargeRequest `json:"list"`
}

// ---------- Admin DTOs ----------

type WarehouseOverviewResp struct {
	TotalBalance       float64 `json:"total_balance"`
	TodayRechargeTotal float64 `json:"today_recharge_total"`
}

type WarehouseAdminRechargeListReq struct {
	Status    string `form:"status"`
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size"`
}

type WarehouseRechargeRecordResp struct {
	ID            uint64    `json:"id"`
	AccountID     uint64    `json:"account_id"`
	Username      string    `json:"username"`
	RealName      string    `json:"real_name"`
	Amount        float64   `json:"amount"`
	PaymentMethod string    `json:"payment_method"`
	TransactionNo string    `json:"transaction_no"`
	VoucherURL    string    `json:"voucher_url"`
	Status        string    `json:"status"`
	Remark        string    `json:"remark"`
	CreatedAt     time.Time `json:"created_at"`
}

type WarehouseAdminRechargeListResp struct {
	Total int64                         `json:"total"`
	List  []WarehouseRechargeRecordResp `json:"list"`
}

type WarehouseRejectRechargeReq struct {
	Remark string `json:"remark"`
}

type WarehouseAdminBillingListReq struct {
	Keyword   string `form:"keyword"`
	Type      string `form:"type"`
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	AccountID uint64 `form:"account_id"`
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size"`
}

type WarehouseBillingRecordWithUser struct {
	WarehouseBillingRecord
	Username string `json:"username"`
	RealName string `json:"real_name"`
}

type WarehouseAdminBillingListResp struct {
	Total int64                            `json:"total"`
	List  []WarehouseBillingRecordWithUser  `json:"list"`
}
