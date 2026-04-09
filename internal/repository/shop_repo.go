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

// IsShopAssignedToOther returns true if the shop is already assigned to any account other than excludeAccountID.
func (r *ShopRepo) IsShopAssignedToOther(shopID, excludeAccountID uint64) (bool, error) {
	var count int64
	err := r.db.Model(&model.AccountShop{}).
		Where("shop_id = ? AND account_id != ?", shopID, excludeAccountID).
		Count(&count).Error
	return count > 0, err
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
