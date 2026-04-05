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

// UpsertShop inserts or updates a shop by shop_name
func (r *ShopRepo) UpsertShop(shop *model.Shop) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "shop_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"platform", "updated_at"}),
	}).Create(shop).Error
}

// GetAllShops returns all shops ordered by platform and name
func (r *ShopRepo) GetAllShops() ([]model.Shop, error) {
	var shops []model.Shop
	err := r.db.Order("platform ASC, shop_name ASC").Find(&shops).Error
	return shops, err
}

// GetShopsByIDs returns shops by IDs
func (r *ShopRepo) GetShopsByIDs(ids []uint64) ([]model.Shop, error) {
	var shops []model.Shop
	err := r.db.Where("id IN ?", ids).Find(&shops).Error
	return shops, err
}

// GetShopByName returns a shop by name
func (r *ShopRepo) GetShopByName(name string) (*model.Shop, error) {
	var shop model.Shop
	err := r.db.Where("shop_name = ?", name).First(&shop).Error
	if err != nil {
		return nil, err
	}
	return &shop, nil
}

// GetAccountShops returns all shops assigned to an account
func (r *ShopRepo) GetAccountShops(accountID uint64) ([]model.Shop, error) {
	var shops []model.Shop
	err := r.db.Raw(`
		SELECT s.* FROM shop s
		INNER JOIN account_shop as2 ON as2.shop_id = s.id
		WHERE as2.account_id = ?
		ORDER BY s.platform ASC, s.shop_name ASC
	`, accountID).Scan(&shops).Error
	return shops, err
}

// GetAccountShopNames returns all shop names for an account
func (r *ShopRepo) GetAccountShopNames(accountID uint64) ([]string, error) {
	var names []string
	err := r.db.Raw(`
		SELECT s.shop_name FROM shop s
		INNER JOIN account_shop as2 ON as2.shop_id = s.id
		WHERE as2.account_id = ?
	`, accountID).Scan(&names).Error
	return names, err
}

// ReplaceAccountShops replaces all shop assignments for an account
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

// GetShopsGroupedByPlatform returns shops grouped by platform
func (r *ShopRepo) GetShopsGroupedByPlatform() ([]model.ShopGroupedResp, error) {
	shops, err := r.GetAllShops()
	if err != nil {
		return nil, err
	}

	groupMap := make(map[string][]model.Shop)
	var platforms []string
	for _, s := range shops {
		if _, exists := groupMap[s.Platform]; !exists {
			platforms = append(platforms, s.Platform)
		}
		groupMap[s.Platform] = append(groupMap[s.Platform], s)
	}

	result := make([]model.ShopGroupedResp, 0, len(platforms))
	for _, p := range platforms {
		result = append(result, model.ShopGroupedResp{
			Platform: p,
			Shops:    groupMap[p],
		})
	}
	return result, nil
}
