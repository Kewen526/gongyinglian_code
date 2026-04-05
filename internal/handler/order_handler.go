package handler

import (
	"strconv"
	"supply-chain/internal/model"
	"supply-chain/internal/service"
	"supply-chain/pkg/response"

	"github.com/gin-gonic/gin"
)

type OrderHandler struct {
	orderSvc *service.OrderService
	syncSvc  *service.SyncService
}

func NewOrderHandler(orderSvc *service.OrderService, syncSvc *service.SyncService) *OrderHandler {
	return &OrderHandler{orderSvc: orderSvc, syncSvc: syncSvc}
}

// GET /api/v1/orders
func (h *OrderHandler) ListOrders(c *gin.Context) {
	var req model.OrderListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	accountID, _ := c.Get("account_id")
	role, _ := c.Get("role")

	result, err := h.orderSvc.ListOrders(&req, accountID.(uint64), role.(uint8))
	if err != nil {
		response.InternalError(c, "查询订单失败: "+err.Error())
		return
	}

	response.Success(c, result)
}

// GET /api/v1/orders/:id
func (h *OrderHandler) GetOrderDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的订单ID")
		return
	}

	accountID, _ := c.Get("account_id")
	role, _ := c.Get("role")

	trade, err := h.orderSvc.GetOrderDetail(id, accountID.(uint64), role.(uint8))
	if err != nil {
		response.NotFound(c, "订单不存在")
		return
	}
	if trade == nil {
		response.NotFound(c, "订单不存在或无权限查看")
		return
	}

	response.Success(c, trade)
}

// POST /api/v1/orders/sync — manual sync trigger
func (h *OrderHandler) SyncOrders(c *gin.Context) {
	count, err := h.syncSvc.SyncNow()
	if err != nil {
		response.InternalError(c, "同步失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"synced_count": count,
		"message":      "同步完成",
	})
}

// GET /api/v1/orders/status-options
func (h *OrderHandler) GetStatusOptions(c *gin.Context) {
	options := h.orderSvc.GetStatusOptions()
	response.Success(c, options)
}

// GET /api/v1/shops — list shops, optionally filtered by platform
func (h *OrderHandler) ListShops(c *gin.Context) {
	platform := c.Query("platform")
	shops, err := h.orderSvc.ListShops(platform)
	if err != nil {
		response.InternalError(c, "查询店铺失败")
		return
	}
	response.Success(c, shops)
}

// GET /api/v1/shops/grouped — list shops grouped by platform
func (h *OrderHandler) ListShopsGrouped(c *gin.Context) {
	grouped, err := h.orderSvc.ListShopsGrouped()
	if err != nil {
		response.InternalError(c, "查询店铺失败")
		return
	}
	response.Success(c, grouped)
}

// GET /api/v1/platforms — list distinct platforms
func (h *OrderHandler) ListPlatforms(c *gin.Context) {
	platforms, err := h.orderSvc.ListPlatforms()
	if err != nil {
		response.InternalError(c, "查询平台失败")
		return
	}
	response.Success(c, platforms)
}

// GET /api/v1/accounts/:id/shops — get account's shop permissions
func (h *OrderHandler) GetAccountShops(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的账号ID")
		return
	}

	shopIDs, err := h.orderSvc.GetAccountShops(id)
	if err != nil {
		response.InternalError(c, "查询失败")
		return
	}
	response.Success(c, gin.H{"shop_ids": shopIDs})
}

// PUT /api/v1/accounts/:id/shops — set account's shop permissions
func (h *OrderHandler) UpdateAccountShops(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的账号ID")
		return
	}

	var req model.UpdateAccountShopsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	if err := h.orderSvc.UpdateAccountShops(id, req.ShopIDs); err != nil {
		response.InternalError(c, "更新失败: "+err.Error())
		return
	}

	response.Success(c, nil)
}
