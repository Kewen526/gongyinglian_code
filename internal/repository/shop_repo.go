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

// GetOccupiedShopIDs returns all shop IDs currently assigned to any account,
// optionally excluding one account (used when editing an existing account's shops).
func (r *ShopRepo) GetOccupiedShopIDs(excludeAccountID uint64) ([]uint64, error) {
	var shopIDs []uint64
	q := r.db.Model(&model.AccountShop{})
	if excludeAccountID > 0 {
		q = q.Where("account_id != ?", excludeAccountID)
	}
	err := q.Pluck("shop_id", &shopIDs).Error
	return shopIDs, err
}

// IsShopAssignedToSibling checks per-layer mutual exclusion.
// "Siblings" = accounts that share the same parent_id AND the same role,
// excluding the target account itself.
// For team leads (parent_id IS NULL), siblings are all other team leads.
func (r *ShopRepo) IsShopAssignedToSibling(shopID uint64, targetAccountID uint64, targetRole uint8, targetParentID *uint64) (bool, uint64, error) {
	// Find sibling account IDs (same role, same parent, excluding self)
	siblingQ := r.db.Model(&model.Account{}).
		Where("role = ? AND id != ?", targetRole, targetAccountID)
	if targetParentID != nil {
		siblingQ = siblingQ.Where("parent_id = ?", *targetParentID)
	} else {
		siblingQ = siblingQ.Where("parent_id IS NULL")
	}

	var siblingIDs []uint64
	if err := siblingQ.Pluck("id", &siblingIDs).Error; err != nil {
		return false, 0, err
	}
	if len(siblingIDs) == 0 {
		return false, 0, nil
	}

	// Check if any sibling has this shop
	var as model.AccountShop
	err := r.db.Where("shop_id = ? AND account_id IN ?", shopID, siblingIDs).First(&as).Error
	if err != nil {
		return false, 0, nil // not found
	}
	return true, as.AccountID, nil
}

// GetOccupiedShopsDetail returns shop assignment details for display.
// Each shop that is assigned to any account is returned with the account info.
type ShopAssignment struct {
	ShopID    uint64 `json:"shop_id"`
	AccountID uint64 `json:"account_id"`
	Username  string `json:"username"`
	RealName  string `json:"real_name"`
	Role      uint8  `json:"role"`
}

func (r *ShopRepo) GetOccupiedShopsDetail(scopeShopIDs []uint64) ([]ShopAssignment, error) {
	if len(scopeShopIDs) == 0 {
		return []ShopAssignment{}, nil
	}
	var results []ShopAssignment
	err := r.db.Table("account_shop").
		Select("account_shop.shop_id, account_shop.account_id, account.username, account.real_name, account.role").
		Joins("JOIN account ON account.id = account_shop.account_id").
		Where("account_shop.shop_id IN ?", scopeShopIDs).
		Scan(&results).Error
	return results, err
}

// GetShopIDsByAccountIDs returns all shop IDs assigned to any of the given account IDs.
func (r *ShopRepo) GetShopIDsByAccountIDs(accountIDs []uint64) ([]uint64, error) {
	if len(accountIDs) == 0 {
		return []uint64{}, nil
	}
	var shopIDs []uint64
	err := r.db.Model(&model.AccountShop{}).
		Where("account_id IN ?", accountIDs).
		Pluck("shop_id", &shopIDs).Error
	return shopIDs, err
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
