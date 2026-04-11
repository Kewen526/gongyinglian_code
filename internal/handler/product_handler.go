package handler

import (
	"strconv"
	"supply-chain/internal/model"
	"supply-chain/internal/service"
	"supply-chain/pkg/response"

	"github.com/gin-gonic/gin"
)

type ProductHandler struct {
	svc *service.ProductService
}

func NewProductHandler(svc *service.ProductService) *ProductHandler {
	return &ProductHandler{svc: svc}
}

func parseIDParam(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的ID参数: "+name)
		return 0, false
	}
	return id, true
}

// ==================== Product CRUD ====================

// POST /api/v1/products
func (h *ProductHandler) CreateProduct(c *gin.Context) {
	var req model.CreateProductReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	p, err := h.svc.CreateProduct(&req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, p)
}

// GET /api/v1/products/:id
func (h *ProductHandler) GetProductDetail(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	detail, err := h.svc.GetProductDetail(id)
	if err != nil {
		response.NotFound(c, "产品不存在")
		return
	}
	response.Success(c, detail)
}

// PUT /api/v1/products/:id
func (h *ProductHandler) UpdateProduct(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req model.UpdateProductReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdateProduct(id, &req); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// DELETE /api/v1/products/:id
func (h *ProductHandler) DeleteProduct(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteProduct(id); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// GET /api/v1/products
func (h *ProductHandler) ListProducts(c *gin.Context) {
	var req model.ProductListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	// Extract account info from JWT context for scope filtering
	accountID, _ := c.Get("account_id")
	role, _ := c.Get("role")
	var aid uint64
	var r uint8
	if v, ok := accountID.(uint64); ok {
		aid = v
	}
	if v, ok := role.(uint8); ok {
		r = v
	}
	result, err := h.svc.ListProducts(&req, aid, r)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// GET /api/v1/products/suppliers
func (h *ProductHandler) GetSuppliers(c *gin.Context) {
	suppliers, err := h.svc.GetDistinctSuppliers()
	if err != nil {
		response.InternalError(c, "查询供应商失败: "+err.Error())
		return
	}
	response.Success(c, suppliers)
}

// ==================== Spec ====================

// POST /api/v1/products/:id/specs
func (h *ProductHandler) CreateSpec(c *gin.Context) {
	productID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req model.CreateSpecReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	spec, err := h.svc.CreateSpec(productID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, spec)
}

// PUT /api/v1/products/:id/specs/:specId
func (h *ProductHandler) UpdateSpec(c *gin.Context) {
	specID, ok := parseIDParam(c, "specId")
	if !ok {
		return
	}
	var req model.UpdateSpecReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdateSpec(specID, &req); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// DELETE /api/v1/products/:id/specs/:specId
func (h *ProductHandler) DeleteSpec(c *gin.Context) {
	specID, ok := parseIDParam(c, "specId")
	if !ok {
		return
	}
	if err := h.svc.DeleteSpec(specID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// ==================== Platform Price ====================

// POST /api/v1/products/:id/platform-prices
func (h *ProductHandler) CreatePlatformPrice(c *gin.Context) {
	productID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req model.CreatePlatformPriceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	price, err := h.svc.CreatePlatformPrice(productID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, price)
}

// PUT /api/v1/products/:id/platform-prices/:priceId
func (h *ProductHandler) UpdatePlatformPrice(c *gin.Context) {
	priceID, ok := parseIDParam(c, "priceId")
	if !ok {
		return
	}
	var req model.UpdatePlatformPriceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdatePlatformPrice(priceID, &req); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// DELETE /api/v1/products/:id/platform-prices/:priceId
func (h *ProductHandler) DeletePlatformPrice(c *gin.Context) {
	priceID, ok := parseIDParam(c, "priceId")
	if !ok {
		return
	}
	if err := h.svc.DeletePlatformPrice(priceID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// ==================== SKU ====================

// POST /api/v1/products/:id/skus
func (h *ProductHandler) CreateSKU(c *gin.Context) {
	productID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req model.CreateSKUReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	sku, err := h.svc.CreateSKU(productID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, sku)
}

// PUT /api/v1/products/:id/skus/:skuId
func (h *ProductHandler) UpdateSKU(c *gin.Context) {
	skuID, ok := parseIDParam(c, "skuId")
	if !ok {
		return
	}
	var req model.UpdateSKUReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdateSKU(skuID, &req); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// DELETE /api/v1/products/:id/skus/:skuId
func (h *ProductHandler) DeleteSKU(c *gin.Context) {
	skuID, ok := parseIDParam(c, "skuId")
	if !ok {
		return
	}
	if err := h.svc.DeleteSKU(skuID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// ==================== Detail Images ====================

// POST /api/v1/products/:id/detail-images
func (h *ProductHandler) BatchCreateDetailImages(c *gin.Context) {
	productID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req model.BatchCreateDetailImageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	images, err := h.svc.BatchCreateDetailImages(productID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, images)
}

// DELETE /api/v1/products/:id/detail-images/:imageId
func (h *ProductHandler) DeleteDetailImage(c *gin.Context) {
	imageID, ok := parseIDParam(c, "imageId")
	if !ok {
		return
	}
	if err := h.svc.DeleteDetailImage(imageID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// ==================== Videos ====================

// POST /api/v1/products/:id/videos
func (h *ProductHandler) BatchCreateVideos(c *gin.Context) {
	productID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req model.BatchCreateVideoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	videos, err := h.svc.BatchCreateVideos(productID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, videos)
}

// DELETE /api/v1/products/:id/videos/:videoId
func (h *ProductHandler) DeleteVideo(c *gin.Context) {
	videoID, ok := parseIDParam(c, "videoId")
	if !ok {
		return
	}
	if err := h.svc.DeleteVideo(videoID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// ==================== Full Reindex ====================

// POST /api/v1/products/reindex
func (h *ProductHandler) FullReindex(c *gin.Context) {
	if err := h.svc.FullReindex(); err != nil {
		response.InternalError(c, "重建索引失败: "+err.Error())
		return
	}
	response.Success(c, "重建索引完成")
}
