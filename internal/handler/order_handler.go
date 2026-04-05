package handler

import (
	"errors"
	"net/http"
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
		response.InternalError(c, err.Error())
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

	detail, err := h.orderSvc.GetOrderDetail(id, accountID.(uint64), role.(uint8))
	if err != nil {
		if errors.Is(err, service.ErrNoShopPermission) {
			response.Forbidden(c, err.Error())
			return
		}
		response.NotFound(c, "订单不存在")
		return
	}
	response.Success(c, detail)
}

// POST /api/v1/orders/sync
func (h *OrderHandler) ManualSync(c *gin.Context) {
	if err := h.syncSvc.ManualSync(); err != nil {
		response.InternalError(c, "同步失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "同步完成"})
}

// GET /api/v1/shops
func (h *OrderHandler) GetAllShops(c *gin.Context) {
	shops, err := h.orderSvc.GetAllShops()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, shops)
}

// GET /api/v1/shops/grouped
func (h *OrderHandler) GetShopsGrouped(c *gin.Context) {
	grouped, err := h.orderSvc.GetShopsGrouped()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, grouped)
}

// GET /api/v1/platforms
func (h *OrderHandler) GetPlatforms(c *gin.Context) {
	platforms, err := h.orderSvc.GetPlatforms()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, platforms)
}

// GET /api/v1/accounts/:id/shops
func (h *OrderHandler) GetAccountShops(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的账号ID")
		return
	}

	result, err := h.orderSvc.GetAccountShops(id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// PUT /api/v1/accounts/:id/shops
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
		response.InternalError(c, err.Error())
		return
	}

	// Return updated shops
	result, err := h.orderSvc.GetAccountShops(id)
	if err != nil {
		response.Error(c, http.StatusOK, "更新成功，但获取最新数据失败")
		return
	}
	response.Success(c, result)
}
