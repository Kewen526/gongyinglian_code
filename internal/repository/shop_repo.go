package repository

import (
	"supply-chain/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ShopRepo struct {
	db *gorm.DB
}

func NewShopRepo(db *gorm.DB) *ShopRepo {
	return &ShopRepo{db: db}
}

// Upsert inserts a new shop or updates if sys_shop already exists.
func (r *ShopRepo) Upsert(shop *model.Shop) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "sys_shop"}},
		DoUpdates: clause.AssignmentColumns([]string{"shop_name", "shop_nick", "source_platform", "shop_type", "updated_at"}),
	}).Create(shop).Error
}

// ListAll returns all shops ordered by platform and name.
func (r *ShopRepo) ListAll() ([]model.Shop, error) {
	var shops []model.Shop
	err := r.db.Order("source_platform ASC, shop_name ASC").Find(&shops).Error
	return shops, err
}

// ListByPlatform returns shops filtered by platform.
func (r *ShopRepo) ListByPlatform(platform string) ([]model.Shop, error) {
	var shops []model.Shop
	err := r.db.Where("source_platform = ?", platform).Order("shop_name ASC").Find(&shops).Error
	return shops, err
}

// ListPlatforms returns distinct platforms.
func (r *ShopRepo) ListPlatforms() ([]string, error) {
	var platforms []string
	err := r.db.Model(&model.Shop{}).Distinct("source_platform").Order("source_platform ASC").Pluck("source_platform", &platforms).Error
	return platforms, err
}

// GetByIDs returns shops by IDs.
func (r *ShopRepo) GetByIDs(ids []uint64) ([]model.Shop, error) {
	var shops []model.Shop
	err := r.db.Where("id IN ?", ids).Find(&shops).Error
	return shops, err
}

// GetAccountShopIDs returns the shop IDs that an account has access to.
func (r *ShopRepo) GetAccountShopIDs(accountID uint64) ([]uint64, error) {
	var shopIDs []uint64
	err := r.db.Model(&model.AccountShop{}).
		Where("account_id = ?", accountID).
		Pluck("shop_id", &shopIDs).Error
	return shopIDs, err
}

// IsShopAssignedToOtherEmployee checks if a shop is already assigned to
// another employee (role=3). Only employee-to-employee mutual exclusion exists;
// team leads and supervisors are free to share any shop.
func (r *ShopRepo) IsShopAssignedToOtherEmployee(shopID uint64, excludeAccountID uint64) (bool, uint64, error) {
	var as model.AccountShop
	err := r.db.Table("account_shop").
		Select("account_shop.*").
		Joins("JOIN account ON account.id = account_shop.account_id").
		Where("account_shop.shop_id = ? AND account.role = ? AND account_shop.account_id != ?",
			shopID, model.RoleEmployee, excludeAccountID).
		First(&as).Error
	if err != nil {
		return false, 0, nil // not found or error → treat as available
	}
	return true, as.AccountID, nil
}

// EmployeeShopAssignment holds a shop-to-employee mapping for the occupied-shops API.
type EmployeeShopAssignment struct {
	ShopID    uint64 `json:"shop_id"`
	AccountID uint64 `json:"account_id"`
	Username  string `json:"username"`
	RealName  string `json:"real_name"`
}

// GetEmployeeOccupiedShops returns all shop assignments that belong to employees (role=3).
// excludeAccountID > 0 means "exclude this employee's own assignments" (edit mode).
func (r *ShopRepo) GetEmployeeOccupiedShops(excludeAccountID uint64) ([]EmployeeShopAssignment, error) {
	q := r.db.Table("account_shop").
		Select("account_shop.shop_id, account_shop.account_id, account.username, account.real_name").
		Joins("JOIN account ON account.id = account_shop.account_id").
		Where("account.role = ?", model.RoleEmployee)
	if excludeAccountID > 0 {
		q = q.Where("account_shop.account_id != ?", excludeAccountID)
	}
	var results []EmployeeShopAssignment
	err := q.Scan(&results).Error
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []EmployeeShopAssignment{}
	}
	return results, nil
}

// GetSysShopsByIDs returns the sys_shop strings for the given shop IDs.
// Uses a single Pluck query for minimal overhead.
func (r *ShopRepo) GetSysShopsByIDs(ids []uint64) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var sysShops []string
	err := r.db.Model(&model.Shop{}).Where("id IN ?", ids).Pluck("sys_shop", &sysShops).Error
	return sysShops, err
}

// ReplaceAccountShops replaces all shop permissions for an account.
func (r *ShopRepo) ReplaceAccountShops(accountID uint64, shopIDs []uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ?", accountID).Delete(&model.AccountShop{}).Error; err != nil {
			return err
		}
		if len(shopIDs) == 0 {
			return nil
		}
		records := make([]model.AccountShop, 0, len(shopIDs))
		for _, sid := range shopIDs {
			records = append(records, model.AccountShop{
				AccountID: accountID,
				ShopID:    sid,
			})
		}
		return tx.Create(&records).Error
	})
}
