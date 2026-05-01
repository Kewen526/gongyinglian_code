package model

import "time"

// BillingStatus constants for order_trade.billing_status
const (
	BillingStatusPending      int8 = 0 // 未扣款
	BillingStatusSuccess      int8 = 1 // 扣款成功
	BillingStatusInsufficient int8 = 2 // 余额不足
	BillingStatusError        int8 = 3 // 货号/价格错误
	BillingStatusRefunded     int8 = 4 // 已退款
)

// Discount level thresholds (applied to both last-month spending and balance snapshot)
type DiscountTier struct {
	MaxAmount    float64
	DiscountRate float64
	Level        string
}

var DiscountTiers = []DiscountTier{
	{500, 0.85, "V1"},
	{1000, 0.80, "V2"},
	{10000, 0.75, "V3"},
	{50000, 0.70, "V4"},
	{200000, 0.65, "V5"},
}

// CalcDiscount returns the discount rate and level for a given amount.
func CalcDiscount(amount float64) (float64, string) {
	for _, t := range DiscountTiers {
		if amount <= t.MaxAmount {
			return t.DiscountRate, t.Level
		}
	}
	return 0.65, "V5"
}

// ---------- Database Models ----------

// Wallet holds the balance and discount info for an employee account.
type Wallet struct {
	ID                uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID         uint64    `json:"account_id" gorm:"not null;uniqueIndex;comment:账号ID"`
	Balance           float64   `json:"balance" gorm:"type:decimal(12,3);default:0;comment:可用余额"`
	DiscountRate      float64   `json:"discount_rate" gorm:"type:decimal(4,2);default:1;comment:当前折扣率，1.0=无折扣"`
	Level             string    `json:"level" gorm:"type:varchar(8);default:'';comment:等级"`
	LastMonthSpending float64   `json:"last_month_spending" gorm:"type:decimal(12,3);default:0;comment:上月消费总额"`
	BalanceSnapshot   float64   `json:"balance_snapshot" gorm:"type:decimal(12,3);default:0;comment:本月1号0点余额快照"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (Wallet) TableName() string { return "wallet" }

// RechargeRequest is a top-up application submitted by an employee.
type RechargeRequest struct {
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

func (RechargeRequest) TableName() string { return "recharge_request" }

// BillingRecord is one financial transaction (deduction or refund).
type BillingRecord struct {
	ID             uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	FlowNo         string     `json:"flow_no" gorm:"type:varchar(128);not null;uniqueIndex;comment:资金流水号"`
	AccountID      uint64     `json:"account_id" gorm:"not null;index;comment:账号ID"`
	TradeNo        string     `json:"trade_no" gorm:"type:varchar(64);index;comment:订单号"`
	TradeUID       string     `json:"trade_uid" gorm:"type:varchar(64);index;comment:订单UID"`
	Platform       string     `json:"platform" gorm:"type:varchar(64);comment:平台"`
	ShopName       string     `json:"shop_name" gorm:"type:varchar(128);comment:店铺名"`
	OriginalAmount Float3     `json:"original_amount" gorm:"type:decimal(12,3);default:0;comment:原价"`
	DiscountRate   float64    `json:"discount_rate" gorm:"type:decimal(4,2);default:1;comment:折扣率"`
	DiscountAmount Float3     `json:"discount_amount" gorm:"type:decimal(12,3);default:0;comment:优惠金额"`
	ActualAmount   Float3     `json:"actual_amount" gorm:"type:decimal(12,3);default:0;comment:实际扣款"`
	Type           string     `json:"type" gorm:"type:varchar(16);comment:deduct/refund"`
	Status         string     `json:"status" gorm:"type:varchar(16);index;comment:success/insufficient/error"`
	BalanceBefore  Float3     `json:"balance_before" gorm:"type:decimal(12,3);default:0;comment:交易前余额"`
	BalanceAfter   Float3     `json:"balance_after" gorm:"type:decimal(12,3);default:0;comment:交易后余额"`
	MarkApprovedAt *time.Time `json:"mark_approved_at" gorm:"comment:发货时间(已审核时刻)"`
	ErrorMsg       string     `json:"error_msg" gorm:"type:text;comment:错误原因"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (BillingRecord) TableName() string { return "billing_record" }

// ---------- Request / Response DTOs ----------

type SubmitRechargeReq struct {
	Amount        float64 `json:"amount" binding:"required,gt=0"`
	PaymentMethod string  `json:"payment_method" binding:"required,oneof=wechat alipay bank"`
	TransactionNo string  `json:"transaction_no" binding:"required"`
	VoucherURL    string  `json:"voucher_url" binding:"required"`
}

type BillingListReq struct {
	Keyword   string `form:"keyword"`    // 订单号或流水号
	Days      int    `form:"days"`       // 7/15/30，与start_date互斥
	StartDate string `form:"start_date"` // YYYY-MM-DD
	EndDate   string `form:"end_date"`
	Platform  string `form:"platform"`
	ShopName  string `form:"shop_name"`
	Status    string `form:"status"` // success/refund/insufficient/error
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size" binding:"omitempty,min=1,max=200"`
}

type WalletResp struct {
	Balance        float64 `json:"balance"`
	DiscountRate   float64 `json:"discount_rate"`
	Level          string  `json:"level"`
	DiscountDisplay string `json:"discount_display"` // e.g. "85折"
}

type BillingListResp struct {
	Total  int64           `json:"total"`
	List   []BillingRecord `json:"list"`
	Wallet WalletResp      `json:"wallet"`
}

// ---------- Admin DTOs ----------

type FinanceOverviewResp struct {
	TotalBalance       float64 `json:"total_balance"`
	TodayRechargeTotal float64 `json:"today_recharge_total"`
}

type RechargeRecordResp struct {
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

type AdminRechargeListReq struct {
	Status    string `form:"status"`     // pending/approved/rejected（空=全部）
	StartDate string `form:"start_date"` // YYYY-MM-DD
	EndDate   string `form:"end_date"`
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size" binding:"omitempty,min=1,max=200"`
}

type AdminRechargeListResp struct {
	Total int64                `json:"total"`
	List  []RechargeRecordResp `json:"list"`
}

type RejectRechargeReq struct {
	Remark string `json:"remark"`
}

type AdminBillingListReq struct {
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	Type      string `form:"type"`       // recharge/deduct/refund（空=全部）
	AccountID uint64 `form:"account_id"` // 按账号筛选（可选）
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size" binding:"omitempty,min=1,max=200"`
}

type BillingRecordWithUser struct {
	BillingRecord
	Username string `json:"username"`
	RealName string `json:"real_name"`
}

type AdminBillingListResp struct {
	Total int64                   `json:"total"`
	List  []BillingRecordWithUser `json:"list"`
}

// ---------- Employee recharge records DTOs ----------

type MyRechargeListReq struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=200"`
}

type MyRechargeListResp struct {
	Total int64              `json:"total"`
	List  []RechargeRequest  `json:"list"`
}
