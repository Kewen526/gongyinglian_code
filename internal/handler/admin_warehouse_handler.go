package handler

import (
	"strconv"
	"supply-chain/internal/model"
	"supply-chain/internal/service"
	"supply-chain/pkg/response"

	"github.com/gin-gonic/gin"
)

type AdminWarehouseHandler struct {
	svc *service.WarehouseService
}

func NewAdminWarehouseHandler(svc *service.WarehouseService) *AdminWarehouseHandler {
	return &AdminWarehouseHandler{svc: svc}
}

// GET /api/v1/admin/warehouse/overview
func (h *AdminWarehouseHandler) GetOverview(c *gin.Context) {
	overview, err := h.svc.GetOverview()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, overview)
}

// GET /api/v1/admin/warehouse/recharge-requests
func (h *AdminWarehouseHandler) ListRechargeRequests(c *gin.Context) {
	var req model.WarehouseAdminRechargeListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.svc.ListRechargeRequestsAdmin(&req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// POST /api/v1/admin/warehouse/recharge-requests/:id/approve
func (h *AdminWarehouseHandler) ApproveRecharge(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的ID")
		return
	}
	if err := h.svc.ApproveRecharge(id); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// POST /api/v1/admin/warehouse/recharge-requests/:id/reject
func (h *AdminWarehouseHandler) RejectRecharge(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的ID")
		return
	}
	var req model.WarehouseRejectRechargeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.svc.RejectRecharge(id, req.Remark); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// GET /api/v1/admin/warehouse/billing-records
func (h *AdminWarehouseHandler) ListBillingRecords(c *gin.Context) {
	var req model.WarehouseAdminBillingListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.svc.ListAllBillingRecords(&req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}
