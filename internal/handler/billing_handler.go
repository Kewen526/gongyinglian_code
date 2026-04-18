package handler

import (
	"supply-chain/internal/model"
	"supply-chain/internal/service"
	"supply-chain/pkg/response"

	"github.com/gin-gonic/gin"
)

type BillingHandler struct {
	billingSvc *service.BillingService
}

func NewBillingHandler(billingSvc *service.BillingService) *BillingHandler {
	return &BillingHandler{billingSvc: billingSvc}
}

// GET /api/v1/billing/wallet
func (h *BillingHandler) GetWallet(c *gin.Context) {
	accountID := c.GetUint64("account_id")
	wallet, err := h.billingSvc.GetWallet(accountID)
	if err != nil {
		response.InternalError(c, "查询钱包失败")
		return
	}
	response.Success(c, wallet)
}

// POST /api/v1/billing/recharge
func (h *BillingHandler) SubmitRecharge(c *gin.Context) {
	accountID := c.GetUint64("account_id")
	var req model.SubmitRechargeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.billingSvc.SubmitRecharge(accountID, &req); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// GET /api/v1/billing
func (h *BillingHandler) ListBillingRecords(c *gin.Context) {
	accountID := c.GetUint64("account_id")
	var req model.BillingListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	result, err := h.billingSvc.ListBillingRecords(accountID, &req)
	if err != nil {
		response.InternalError(c, "查询失败")
		return
	}
	response.Success(c, result)
}

// GET /api/v1/billing/export
func (h *BillingHandler) ExportBillingRecords(c *gin.Context) {
	accountID := c.GetUint64("account_id")
	var req model.BillingListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	data, err := h.billingSvc.ExportMyBillingRecords(accountID, &req)
	if err != nil {
		response.InternalError(c, "导出失败")
		return
	}
	c.Header("Content-Disposition", "attachment; filename=billing.xlsx")
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// GET /api/v1/billing/recharge-records
func (h *BillingHandler) ListMyRechargeRecords(c *gin.Context) {
	accountID := c.GetUint64("account_id")
	var req model.MyRechargeListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	result, err := h.billingSvc.ListMyRechargeRecords(accountID, &req)
	if err != nil {
		response.InternalError(c, "查询失败")
		return
	}
	response.Success(c, result)
}
