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
	accountID, _ := c.Get("account_id")
	wallet, err := h.billingSvc.GetWallet(accountID.(uint64))
	if err != nil {
		response.InternalError(c, "查询钱包失败: "+err.Error())
		return
	}
	response.Success(c, wallet)
}

// POST /api/v1/billing/recharge
func (h *BillingHandler) SubmitRecharge(c *gin.Context) {
	accountID, _ := c.Get("account_id")
	var req model.SubmitRechargeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.billingSvc.SubmitRecharge(accountID.(uint64), &req); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// GET /api/v1/billing
func (h *BillingHandler) ListBillingRecords(c *gin.Context) {
	accountID, _ := c.Get("account_id")
	var req model.BillingListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.billingSvc.ListBillingRecords(accountID.(uint64), &req)
	if err != nil {
		response.InternalError(c, "查询失败: "+err.Error())
		return
	}
	response.Success(c, result)
}
