package repository

import (
	"supply-chain/internal/model"

	"gorm.io/gorm"
)

type ProductRepo struct {
	db *gorm.DB
}

func NewProductRepo(db *gorm.DB) *ProductRepo {
	return &ProductRepo{db: db}
}

// ---------- Product CRUD ----------

func (r *ProductRepo) Create(p *model.Product) error {
	return r.db.Create(p).Error
}

func (r *ProductRepo) GetByID(id uint64) (*model.Product, error) {
	var p model.Product
	if err := r.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ProductRepo) Update(id uint64, updates map[string]interface{}) error {
	return r.db.Model(&model.Product{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ProductRepo) Delete(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete all sub-resources
		tx.Where("product_id = ?", id).Delete(&model.ProductSpec{})
		tx.Where("product_id = ?", id).Delete(&model.ProductPlatformPrice{})
		tx.Where("product_id = ?", id).Delete(&model.ProductSKU{})
		tx.Where("product_id = ?", id).Delete(&model.ProductDetailImage{})
		tx.Where("product_id = ?", id).Delete(&model.ProductVideo{})
		return tx.Delete(&model.Product{}, id).Error
	})
}

// GetByIDs fetches products by a list of IDs, preserving the order of ids.
func (r *ProductRepo) GetByIDs(ids []uint64) ([]model.Product, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var products []model.Product
	if err := r.db.Where("id IN ?", ids).Find(&products).Error; err != nil {
		return nil, err
	}
	// Build map for ordering
	m := make(map[uint64]model.Product, len(products))
	for _, p := range products {
		m[p.ID] = p
	}
	ordered := make([]model.Product, 0, len(ids))
	for _, id := range ids {
		if p, ok := m[id]; ok {
			ordered = append(ordered, p)
		}
	}
	return ordered, nil
}

// BatchGetForReindex fetches products in batches for full ES reindex.
func (r *ProductRepo) BatchGetForReindex(lastID uint64, batchSize int) ([]model.Product, error) {
	var products []model.Product
	err := r.db.Where("id > ?", lastID).Order("id ASC").Limit(batchSize).Find(&products).Error
	return products, err
}

// ---------- Spec ----------

func (r *ProductRepo) CreateSpec(s *model.ProductSpec) error {
	return r.db.Create(s).Error
}

func (r *ProductRepo) GetSpecsByProductID(productID uint64) ([]model.ProductSpec, error) {
	var specs []model.ProductSpec
	err := r.db.Where("product_id = ?", productID).Order("id ASC").Find(&specs).Error
	return specs, err
}

func (r *ProductRepo) UpdateSpec(id uint64, updates map[string]interface{}) error {
	return r.db.Model(&model.ProductSpec{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ProductRepo) DeleteSpec(id uint64) error {
	return r.db.Delete(&model.ProductSpec{}, id).Error
}

// ---------- Platform Price ----------

func (r *ProductRepo) CreatePlatformPrice(p *model.ProductPlatformPrice) error {
	return r.db.Create(p).Error
}

func (r *ProductRepo) GetPlatformPricesByProductID(productID uint64) ([]model.ProductPlatformPrice, error) {
	var prices []model.ProductPlatformPrice
	err := r.db.Where("product_id = ?", productID).Order("id ASC").Find(&prices).Error
	return prices, err
}

func (r *ProductRepo) UpdatePlatformPrice(id uint64, updates map[string]interface{}) error {
	return r.db.Model(&model.ProductPlatformPrice{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ProductRepo) DeletePlatformPrice(id uint64) error {
	return r.db.Delete(&model.ProductPlatformPrice{}, id).Error
}

// ---------- SKU ----------

func (r *ProductRepo) CreateSKU(s *model.ProductSKU) error {
	return r.db.Create(s).Error
}

func (r *ProductRepo) GetSKUsByProductID(productID uint64) ([]model.ProductSKU, error) {
	var skus []model.ProductSKU
	err := r.db.Where("product_id = ?", productID).Order("id ASC").Find(&skus).Error
	return skus, err
}

func (r *ProductRepo) UpdateSKU(id uint64, updates map[string]interface{}) error {
	return r.db.Model(&model.ProductSKU{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ProductRepo) DeleteSKU(id uint64) error {
	return r.db.Delete(&model.ProductSKU{}, id).Error
}

// ---------- Detail Image ----------

func (r *ProductRepo) BatchCreateDetailImages(images []model.ProductDetailImage) error {
	if len(images) == 0 {
		return nil
	}
	return r.db.Create(&images).Error
}

func (r *ProductRepo) GetDetailImagesByProductID(productID uint64) ([]model.ProductDetailImage, error) {
	var imgs []model.ProductDetailImage
	err := r.db.Where("product_id = ?", productID).Order("sort_order ASC, id ASC").Find(&imgs).Error
	return imgs, err
}

func (r *ProductRepo) DeleteDetailImage(id uint64) error {
	return r.db.Delete(&model.ProductDetailImage{}, id).Error
}

// ---------- Video ----------

func (r *ProductRepo) BatchCreateVideos(videos []model.ProductVideo) error {
	if len(videos) == 0 {
		return nil
	}
	return r.db.Create(&videos).Error
}

func (r *ProductRepo) GetVideosByProductID(productID uint64) ([]model.ProductVideo, error) {
	var vids []model.ProductVideo
	err := r.db.Where("product_id = ?", productID).Order("id ASC").Find(&vids).Error
	return vids, err
}

func (r *ProductRepo) DeleteVideo(id uint64) error {
	return r.db.Delete(&model.ProductVideo{}, id).Error
}
