package repository

import (
	"supply-chain/internal/model"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WlnGoodsRepo struct {
	db *gorm.DB
}

func NewWlnGoodsRepo(db *gorm.DB) *WlnGoodsRepo {
	return &WlnGoodsRepo{db: db}
}

// GetCachedSpecs returns a map of spec_code → cache entry for the given spec codes
// that are still within TTL. Expired or absent entries are omitted — treat them as
// cache misses.
func (r *WlnGoodsRepo) GetCachedSpecs(specCodes []string) (map[string]model.WlnGoodsSpecCache, error) {
	if len(specCodes) == 0 {
		return map[string]model.WlnGoodsSpecCache{}, nil
	}
	expiryCutoff := time.Now().Add(-model.WlnGoodsCacheTTL).UnixMilli()
	var rows []model.WlnGoodsSpecCache
	if err := r.db.
		Where("spec_code IN ? AND fetched_at > ?", specCodes, expiryCutoff).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]model.WlnGoodsSpecCache, len(rows))
	for _, row := range rows {
		result[row.SpecCode] = row
	}
	return result, nil
}

// UpsertSpecsCache batch-upserts cache entries, updating all fields on conflict.
func (r *WlnGoodsRepo) UpsertSpecsCache(specs []model.WlnGoodsSpecCache) error {
	if len(specs) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "spec_code"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"goods_code", "goods_name", "spec1", "pic",
			"sys_goods_uid", "sys_spec_uid", "fetched_at",
		}),
	}).Create(&specs).Error
}

// FillMissingOrderItemImages sets item_image_url only where it is currently empty,
// keyed by bar_code. Used during initial sync enrichment.
func (r *WlnGoodsRepo) FillMissingOrderItemImages(barCodeToPic map[string]string) error {
	for barCode, pic := range barCodeToPic {
		if barCode == "" || pic == "" {
			continue
		}
		if err := r.db.Model(&model.OrderItem{}).
			Where("bar_code = ? AND (item_image_url IS NULL OR item_image_url = '')", barCode).
			Update("item_image_url", pic).Error; err != nil {
			return err
		}
	}
	return nil
}

// RefreshOrderItemImages overwrites item_image_url for all order_items matching
// bar_code, regardless of current value. Used during periodic cache refresh.
func (r *WlnGoodsRepo) RefreshOrderItemImages(barCodeToPic map[string]string) error {
	for barCode, pic := range barCodeToPic {
		if barCode == "" || pic == "" {
			continue
		}
		if err := r.db.Model(&model.OrderItem{}).
			Where("bar_code = ?", barCode).
			Update("item_image_url", pic).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetExpiredGoodsCodeDistinct returns distinct goods_codes whose cached entries
// have exceeded WlnGoodsCacheTTL and need to be refreshed.
func (r *WlnGoodsRepo) GetExpiredGoodsCodeDistinct() ([]string, error) {
	expiryCutoff := time.Now().Add(-model.WlnGoodsCacheTTL).UnixMilli()
	var goodsCodes []string
	err := r.db.Model(&model.WlnGoodsSpecCache{}).
		Where("fetched_at <= ?", expiryCutoff).
		Distinct("goods_code").
		Pluck("goods_code", &goodsCodes).Error
	return goodsCodes, err
}
