package model

import "time"

// WlnGoodsSpecCache caches WanLiNiu goods spec info (primarily images) to avoid
// repeated API calls. One row per spec variant; spec_code is the primary key
// and corresponds to order_item.bar_code.
type WlnGoodsSpecCache struct {
	SpecCode    string `json:"spec_code"     gorm:"primaryKey;type:varchar(128);not null"`
	GoodsCode   string `json:"goods_code"    gorm:"type:varchar(128);index;not null"`
	GoodsName   string `json:"goods_name"    gorm:"type:varchar(256)"`
	Spec1       string `json:"spec1"         gorm:"type:varchar(128)"`
	Pic         string `json:"pic"           gorm:"type:varchar(512)"`
	SysGoodsUID string `json:"sys_goods_uid" gorm:"type:varchar(64)"`
	SysSpecUID  string `json:"sys_spec_uid"  gorm:"type:varchar(64)"`
	// FetchedAt is a Unix millisecond timestamp; entries older than WlnGoodsCacheTTL are stale.
	FetchedAt int64 `json:"fetched_at" gorm:"type:bigint;not null;index"`
}

func (WlnGoodsSpecCache) TableName() string { return "wln_goods_spec_cache" }

// WlnGoodsCacheTTL is the duration after which a cached entry is considered stale
// and must be re-fetched from the WanLiNiu goods API.
const WlnGoodsCacheTTL = 3 * 24 * time.Hour
