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

// GET /api/v1/shops/occupied — shop assignment details visible to the caller.
// Returns both a simple ID list (occupied_shop_ids) and detailed assignments.
func (h *OrderHandler) GetOccupiedShopIDs(c *gin.Context) {
	accountID, _ := c.Get("account_id")
	role, _ := c.Get("role")

	details, err := h.orderSvc.GetOccupiedShopsDetail(accountID.(uint64), role.(uint8))
	if err != nil {
		response.InternalError(c, "查询失败: "+err.Error())
		return
	}

	// Build simple ID set for backward compatibility
	idSet := make(map[uint64]bool)
	for _, d := range details {
		idSet[d.ShopID] = true
	}
	ids := make([]uint64, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}

	response.Success(c, gin.H{
		"occupied_shop_ids": ids,
		"assignments":       details,
	})
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

// PATCH /api/v1/orders/batch-update — batch update local order fields
func (h *OrderHandler) BatchUpdateOrders(c *gin.Context) {
	var req model.BatchUpdateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if len(req) == 0 {
		response.BadRequest(c, "请求列表不能为空")
		return
	}
	if err := h.orderSvc.BatchUpdateOrders(req); err != nil {
		response.InternalError(c, "批量更新失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

// POST /api/v1/orders/mark — batch mark orders in WanLiNiu.
// "已审核" marks are balance-checked first: insufficient-balance orders are
// marked "余额不足扣款失败" locally without hitting WanLiNiu. Other mark types
// are forwarded to WanLiNiu unchanged.
func (h *OrderHandler) BatchMarkOrders(c *gin.Context) {
	var req model.BatchMarkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if len(req) == 0 {
		response.BadRequest(c, "请求列表不能为空")
		return
	}
	result, err := h.syncSvc.BatchMarkOrdersWithBalanceCheck(req)
	if err != nil {
		response.InternalError(c, "标记失败: "+err.Error())
		return
	}
	response.Success(c, result)
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

	// Determine caller: super admin passes 0 (no subset check), others pass their own ID.
	role, _ := c.Get("role")
	var callerID uint64
	if role.(uint8) != model.RoleSuperAdmin {
		aid, _ := c.Get("account_id")
		callerID = aid.(uint64)
	}

	if err := h.orderSvc.UpdateAccountShops(id, req.ShopIDs, callerID); err != nil {
		response.InternalError(c, "更新失败: "+err.Error())
		return
	}

	response.Success(c, nil)
}
