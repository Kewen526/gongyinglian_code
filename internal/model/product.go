package model

import "time"

// Product status constants
const (
	ProductStatusPending = 0 // 待上架
	ProductStatusOnSale  = 1 // 正常在售
	ProductStatusOffSale = 2 // 停售
)

// ---------- Database Models ----------

type Product struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ImageURL     string    `json:"image_url" gorm:"column:image_url;type:varchar(512);not null;default:''"`
	Name         string    `json:"name" gorm:"type:varchar(255);not null;default:''"`
	ProductCode  string    `json:"product_code" gorm:"type:varchar(128);not null;default:''"`
	Supplier     string    `json:"supplier" gorm:"type:varchar(255);not null;default:''"`
	Status       uint8     `json:"status" gorm:"type:tinyint unsigned;not null;default:0;index:idx_status"`
	Brand        string    `json:"brand" gorm:"type:varchar(128);not null;default:''"`
	Category     string    `json:"category" gorm:"type:varchar(128);not null;default:''"`
	GroupName    string    `json:"group_name" gorm:"type:varchar(128);not null;default:''"`
	Material     string    `json:"material" gorm:"type:varchar(255);not null;default:''"`
	PatentStatus string    `json:"patent_status" gorm:"type:varchar(128);not null;default:''"`
	FactoryPrice float64   `json:"factory_price" gorm:"type:decimal(12,2);not null;default:0.00"`
	CreatedAt    time.Time `json:"created_at" gorm:"index:idx_created_at;index:idx_created_at_id"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Product) TableName() string { return "product" }

type ProductSpec struct {
	ID         uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProductID  uint64    `json:"product_id" gorm:"not null;index:idx_product_id"`
	SizeModel  string    `json:"size_model" gorm:"type:varchar(128);not null;default:''"`
	Dimension  string    `json:"dimension" gorm:"type:varchar(128);not null;default:''"`
	Weight     float64   `json:"weight" gorm:"type:decimal(10,3);not null;default:0.000"`
	BoxSpec    string    `json:"box_spec" gorm:"type:varchar(128);not null;default:''"`
	PackingQty uint      `json:"packing_qty" gorm:"type:int unsigned;not null;default:0"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (ProductSpec) TableName() string { return "product_spec" }

type ProductPlatformPrice struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProductID    uint64    `json:"product_id" gorm:"not null;index:idx_product_id"`
	PlatformName string    `json:"platform_name" gorm:"type:varchar(64);not null;default:''"`
	ControlPrice float64   `json:"control_price" gorm:"type:decimal(12,2);not null;default:0.00"`
	Currency     string    `json:"currency" gorm:"type:varchar(8);not null;default:'CNY'"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (ProductPlatformPrice) TableName() string { return "product_platform_price" }

type ProductSKU struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProductID uint64    `json:"product_id" gorm:"not null;index:idx_product_id"`
	Model     string    `json:"model" gorm:"type:varchar(128);not null;default:''"`
	Size      string    `json:"size" gorm:"type:varchar(64);not null;default:''"`
	SKUCode   string    `json:"sku_code" gorm:"column:sku_code;type:varchar(128);not null;default:'';index:idx_sku_code"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ProductSKU) TableName() string { return "product_sku" }

type ProductDetailImage struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProductID uint64    `json:"product_id" gorm:"not null;index:idx_product_id"`
	ImageURL  string    `json:"image_url" gorm:"type:varchar(512);not null;default:''"`
	SortOrder uint      `json:"sort_order" gorm:"type:int unsigned;not null;default:0"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ProductDetailImage) TableName() string { return "product_detail_image" }

type ProductVideo struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProductID uint64    `json:"product_id" gorm:"not null;index:idx_product_id"`
	VideoURL  string    `json:"video_url" gorm:"type:varchar(512);not null;default:''"`
	CoverURL  string    `json:"cover_url" gorm:"type:varchar(512);not null;default:''"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ProductVideo) TableName() string { return "product_video" }

// ---------- Request / Response DTOs ----------

type CreateProductReq struct {
	ImageURL     string  `json:"image_url"`
	Name         string  `json:"name" binding:"required"`
	ProductCode  string  `json:"product_code"`
	Supplier     string  `json:"supplier"`
	Status       uint8   `json:"status"`
	Brand        string  `json:"brand"`
	Category     string  `json:"category"`
	GroupName    string  `json:"group_name"`
	Material     string  `json:"material"`
	PatentStatus string  `json:"patent_status"`
	FactoryPrice float64 `json:"factory_price"`
}

type UpdateProductReq struct {
	ImageURL     *string  `json:"image_url"`
	Name         *string  `json:"name"`
	ProductCode  *string  `json:"product_code"`
	Supplier     *string  `json:"supplier"`
	Status       *uint8   `json:"status"`
	Brand        *string  `json:"brand"`
	Category     *string  `json:"category"`
	GroupName    *string  `json:"group_name"`
	Material     *string  `json:"material"`
	PatentStatus *string  `json:"patent_status"`
	FactoryPrice *float64 `json:"factory_price"`
}

type ProductListReq struct {
	ProductCode string `form:"product_code"`
	Name        string `form:"name"`
	Supplier    string `form:"supplier"`
	GroupName   string `form:"group_name"`
	StartDate   string `form:"start_date"`
	EndDate     string `form:"end_date"`
	PageSize    int    `form:"page_size"`
	// search_after fields for ES cursor-based pagination
	SearchAfterTime string `form:"search_after_time"`
	SearchAfterID   string `form:"search_after_id"`
}

type ProductListResp struct {
	List            []Product `json:"list"`
	Total           int64     `json:"total"`
	SearchAfterTime string    `json:"search_after_time,omitempty"`
	SearchAfterID   string    `json:"search_after_id,omitempty"`
}

type ProductDetailResp struct {
	Product        Product                `json:"product"`
	Specs          []ProductSpec          `json:"specs"`
	PlatformPrices []ProductPlatformPrice `json:"platform_prices"`
	SKUs           []ProductSKU           `json:"skus"`
	DetailImages   []ProductDetailImage   `json:"detail_images"`
	Videos         []ProductVideo         `json:"videos"`
}

// Sub-resource request DTOs

type CreateSpecReq struct {
	SizeModel  string  `json:"size_model"`
	Dimension  string  `json:"dimension"`
	Weight     float64 `json:"weight"`
	BoxSpec    string  `json:"box_spec"`
	PackingQty uint    `json:"packing_qty"`
}

type UpdateSpecReq struct {
	SizeModel  *string  `json:"size_model"`
	Dimension  *string  `json:"dimension"`
	Weight     *float64 `json:"weight"`
	BoxSpec    *string  `json:"box_spec"`
	PackingQty *uint    `json:"packing_qty"`
}

type CreatePlatformPriceReq struct {
	PlatformName string  `json:"platform_name" binding:"required"`
	ControlPrice float64 `json:"control_price"`
	Currency     string  `json:"currency" binding:"required,oneof=CNY USD"`
}

type UpdatePlatformPriceReq struct {
	PlatformName *string  `json:"platform_name"`
	ControlPrice *float64 `json:"control_price"`
	Currency     *string  `json:"currency"`
}

type CreateSKUReq struct {
	Model   string `json:"model"`
	Size    string `json:"size"`
	SKUCode string `json:"sku_code"`
}

type UpdateSKUReq struct {
	Model   *string `json:"model"`
	Size    *string `json:"size"`
	SKUCode *string `json:"sku_code"`
}

type CreateDetailImageReq struct {
	ImageURL  string `json:"image_url" binding:"required"`
	SortOrder uint   `json:"sort_order"`
}

type BatchCreateDetailImageReq struct {
	Images []CreateDetailImageReq `json:"images" binding:"required,dive"`
}

type CreateVideoReq struct {
	VideoURL string `json:"video_url" binding:"required"`
	CoverURL string `json:"cover_url"`
}

type BatchCreateVideoReq struct {
	Videos []CreateVideoReq `json:"videos" binding:"required,dive"`
}
