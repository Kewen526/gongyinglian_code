package model

import "time"

// Role constants
const (
	RoleSuperAdmin = 0 // 超级管理员（拥有所有权限+开账号权限）
	RoleTeamLead   = 1 // 团队负责人
	RoleSupervisor = 2 // 主管
	RoleEmployee   = 3 // 员工
)

type Account struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	Username    string    `json:"username" gorm:"type:varchar(64);uniqueIndex;not null"`
	Password    string    `json:"-" gorm:"type:varchar(255);not null"`
	RealName    string    `json:"real_name" gorm:"type:varchar(64);not null;default:''"`
	Role        uint8     `json:"role" gorm:"type:tinyint unsigned;not null"`
	ParentID    *uint64   `json:"parent_id" gorm:"type:bigint unsigned;index;default:null;comment:直属上级账号ID"`
	AutoReview  bool      `json:"auto_review" gorm:"type:tinyint(1);not null;default:1;index:idx_auto_review;comment:自动审核开关"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Account) TableName() string { return "account" }

type Module struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name      string    `json:"name" gorm:"type:varchar(64);not null"`
	Code      string    `json:"code" gorm:"type:varchar(64);uniqueIndex;not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Module) TableName() string { return "module" }

type AccountPermission struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID uint64    `json:"account_id" gorm:"not null;uniqueIndex:uk_account_module"`
	ModuleID  uint64    `json:"module_id" gorm:"not null;uniqueIndex:uk_account_module;index:idx_module_id"`
	CanView   uint8     `json:"can_view" gorm:"type:tinyint unsigned;not null;default:0"`
	CanEdit   uint8     `json:"can_edit" gorm:"type:tinyint unsigned;not null;default:0"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AccountPermission) TableName() string { return "account_permission" }

// ---------- Request / Response DTOs ----------

type CreateAccountReq struct {
	Username    string           `json:"username" binding:"required"`
	Password    string           `json:"password" binding:"required,min=6"`
	RealName    string           `json:"real_name"`
	Role        uint8            `json:"role" binding:"oneof=0 1 2 3"`
	ParentID    *uint64          `json:"parent_id"`
	Permissions []PermissionItem `json:"permissions"`
}

type PermissionItem struct {
	ModuleID uint64 `json:"module_id" binding:"required"`
	CanView  uint8  `json:"can_view"`
	CanEdit  uint8  `json:"can_edit"`
}

type UpdateAccountReq struct {
	Username *string  `json:"username"`
	Password *string  `json:"password"`
	RealName *string  `json:"real_name"`
	Role     *uint8   `json:"role" binding:"omitempty,oneof=0 1 2 3"`
	ParentID *uint64  `json:"parent_id"`
}

type UpdatePermissionsReq struct {
	Permissions []PermissionItem `json:"permissions" binding:"required"`
}

type AccountDetailResp struct {
	ID          uint64             `json:"id"`
	Username    string             `json:"username"`
	RealName    string             `json:"real_name"`
	Role        uint8              `json:"role"`
	ParentID    *uint64            `json:"parent_id"`
	ParentName  string             `json:"parent_name"`
	Permissions []PermissionDetail `json:"permissions"`
	ShopIDs     []uint64           `json:"shop_ids"`
	CreatedAt   time.Time          `json:"created_at"`
}

type PermissionDetail struct {
	ModuleID   uint64 `json:"module_id"`
	ModuleName string `json:"module_name"`
	ModuleCode string `json:"module_code"`
	CanView    uint8  `json:"can_view"`
	CanEdit    uint8  `json:"can_edit"`
}

type AccountListReq struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=200"`
}

type AccountListResp struct {
	Total int64              `json:"total"`
	List  []AccountDetailResp `json:"list"`
}

// ---------- Account Product Scope ----------

// AccountProductScope stores which suppliers and tags an employee can see.
// Only applies to RoleEmployee accounts that have product module permission.
type AccountProductScope struct {
	ID           uint64      `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID    uint64      `json:"account_id" gorm:"not null;uniqueIndex"`
	Suppliers    StringSlice `json:"suppliers" gorm:"type:json"`
	Tags         StringSlice `json:"tags" gorm:"type:json"`
	HiddenFields StringSlice `json:"hidden_fields" gorm:"column:hidden_fields;type:json"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

func (AccountProductScope) TableName() string { return "account_product_scope" }

type ProductScopeReq struct {
	Suppliers    []string `json:"suppliers"`
	Tags         []string `json:"tags"`
	HiddenFields []string `json:"hidden_fields"`
}

type ProductScopeResp struct {
	Suppliers    []string `json:"suppliers"`
	Tags         []string `json:"tags"`
	HiddenFields []string `json:"hidden_fields"`
}

// TeamLeaderPaymentInfo stores payment collection info for a team leader.
// Employees see this when submitting recharge requests.
type TeamLeaderPaymentInfo struct {
	ID                  uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID           uint64    `json:"account_id" gorm:"not null;uniqueIndex;comment:团队负责人账号ID"`
	CorpBankName        string    `json:"corp_bank_name" gorm:"type:varchar(128);not null;default:''"`
	CorpAccountName     string    `json:"corp_account_name" gorm:"type:varchar(128);not null;default:''"`
	CorpAccountNo       string    `json:"corp_account_no" gorm:"type:varchar(64);not null;default:''"`
	PersonalBankName    string    `json:"personal_bank_name" gorm:"type:varchar(128);not null;default:''"`
	PersonalAccountName string    `json:"personal_account_name" gorm:"type:varchar(128);not null;default:''"`
	PersonalAccountNo   string    `json:"personal_account_no" gorm:"type:varchar(64);not null;default:''"`
	AlipayQR            string    `json:"alipay_qr" gorm:"type:varchar(512);not null;default:''"`
	WechatQR            string    `json:"wechat_qr" gorm:"type:varchar(512);not null;default:''"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (TeamLeaderPaymentInfo) TableName() string { return "team_leader_payment_info" }

type SavePaymentInfoReq struct {
	CorpBankName        string `json:"corp_bank_name"`
	CorpAccountName     string `json:"corp_account_name"`
	CorpAccountNo       string `json:"corp_account_no"`
	PersonalBankName    string `json:"personal_bank_name"`
	PersonalAccountName string `json:"personal_account_name"`
	PersonalAccountNo   string `json:"personal_account_no"`
	AlipayQR            string `json:"alipay_qr"`
	WechatQR            string `json:"wechat_qr"`
}

type LoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResp struct {
	Token     string            `json:"token"`
	Account   AccountDetailResp `json:"account"`
}
