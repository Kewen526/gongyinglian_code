package model

import "time"

// Role constants
const (
	RoleTeamLead  = 1 // 团队负责人
	RoleSupervisor = 2 // 主管
	RoleEmployee  = 3 // 员工
)

type Account struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	Username  string    `json:"username" gorm:"type:varchar(64);uniqueIndex;not null"`
	Password  string    `json:"-" gorm:"type:varchar(255);not null"`
	RealName  string    `json:"real_name" gorm:"type:varchar(64);not null;default:''"`
	Role      uint8     `json:"role" gorm:"type:tinyint unsigned;not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	Username    string             `json:"username" binding:"required"`
	Password    string             `json:"password" binding:"required,min=6"`
	RealName    string             `json:"real_name"`
	Role        uint8              `json:"role" binding:"required,oneof=1 2 3"`
	Permissions []PermissionItem   `json:"permissions"`
}

type PermissionItem struct {
	ModuleID uint64 `json:"module_id" binding:"required"`
	CanView  uint8  `json:"can_view"`
	CanEdit  uint8  `json:"can_edit"`
}

type UpdatePermissionsReq struct {
	Permissions []PermissionItem `json:"permissions" binding:"required"`
}

type AccountDetailResp struct {
	ID          uint64               `json:"id"`
	Username    string               `json:"username"`
	RealName    string               `json:"real_name"`
	Role        uint8                `json:"role"`
	Permissions []PermissionDetail   `json:"permissions"`
	CreatedAt   time.Time            `json:"created_at"`
}

type PermissionDetail struct {
	ModuleID   uint64 `json:"module_id"`
	ModuleName string `json:"module_name"`
	ModuleCode string `json:"module_code"`
	CanView    uint8  `json:"can_view"`
	CanEdit    uint8  `json:"can_edit"`
}
