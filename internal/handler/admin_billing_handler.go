package handler

import (
	"fmt"
	"strconv"
	"supply-chain/internal/model"
	"supply-chain/internal/service"
	"supply-chain/pkg/response"
	"time"

	"github.com/gin-gonic/gin"
)

type AdminBillingHandler struct {
	billingSvc *service.BillingService
}

func NewAdminBillingHandler(billingSvc *service.BillingService) *AdminBillingHandler {
	return &AdminBillingHandler{billingSvc: billingSvc}
}

// GET /api/v1/admin/finance/overview
func (h *AdminBillingHandler) GetFinanceOverview(c *gin.Context) {
	overview, err := h.billingSvc.GetFinanceOverview()
	if err != nil {
		response.InternalError(c, "查询失败: "+err.Error())
		return
	}
	response.Success(c, overview)
}

// GET /api/v1/admin/finance/recharge-requests
func (h *AdminBillingHandler) ListRechargeRequests(c *gin.Context) {
	var req model.AdminRechargeListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.billingSvc.ListRechargeRequestsAdmin(&req)
	if err != nil {
		response.InternalError(c, "查询失败: "+err.Error())
		return
	}
	response.Success(c, result)
}

// POST /api/v1/admin/finance/recharge-requests/:id/approve
func (h *AdminBillingHandler) ApproveRecharge(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的ID")
		return
	}
	if err := h.billingSvc.ApproveRecharge(id); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// POST /api/v1/admin/finance/recharge-requests/:id/reject
func (h *AdminBillingHandler) RejectRecharge(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的ID")
		return
	}
	var req model.RejectRechargeReq
	_ = c.ShouldBindJSON(&req) // remark is optional
	if err := h.billingSvc.RejectRecharge(id, req.Remark); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// GET /api/v1/admin/finance/billing-records
func (h *AdminBillingHandler) ListAllBillingRecords(c *gin.Context) {
	var req model.AdminBillingListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.billingSvc.ListAllBillingRecords(&req)
	if err != nil {
		response.InternalError(c, "查询失败: "+err.Error())
		return
	}
	response.Success(c, result)
}

// GET /api/v1/admin/finance/billing-records/export
func (h *AdminBillingHandler) ExportBillingRecords(c *gin.Context) {
	var req model.AdminBillingListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	data, err := h.billingSvc.ExportBillingRecords(&req)
	if err != nil {
		response.InternalError(c, "导出失败: "+err.Error())
		return
	}
	filename := fmt.Sprintf("资金流水_%s.xlsx", time.Now().Format("20060102150405"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}
