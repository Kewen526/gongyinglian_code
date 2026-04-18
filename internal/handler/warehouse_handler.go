package handler

import (
	"supply-chain/internal/model"
	"supply-chain/internal/service"
	"supply-chain/pkg/response"

	"github.com/gin-gonic/gin"
)

type WarehouseHandler struct {
	svc *service.WarehouseService
}

func NewWarehouseHandler(svc *service.WarehouseService) *WarehouseHandler {
	return &WarehouseHandler{svc: svc}
}

// GET /api/v1/warehouse/wallet
func (h *WarehouseHandler) GetWallet(c *gin.Context) {
	accountID := c.GetUint64("account_id")
	role := getRole(c)
	wallet, err := h.svc.GetWallet(accountID, role)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, wallet)
}

// GET /api/v1/warehouse/billing
func (h *WarehouseHandler) ListBillingRecords(c *gin.Context) {
	accountID := c.GetUint64("account_id")
	role := getRole(c)
	var req model.WarehouseBillingListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.svc.ListBillingRecords(accountID, role, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// POST /api/v1/warehouse/recharge
func (h *WarehouseHandler) SubmitRecharge(c *gin.Context) {
	role := getRole(c)
	if role != model.RoleEmployee {
		response.Forbidden(c, "仅员工可提交充值申请")
		return
	}
	accountID := c.GetUint64("account_id")
	var req model.WarehouseSubmitRechargeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.svc.SubmitRecharge(accountID, &req); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// GET /api/v1/warehouse/recharge-records
func (h *WarehouseHandler) ListMyRechargeRecords(c *gin.Context) {
	accountID := c.GetUint64("account_id")
	role := getRole(c)
	var req model.WarehouseMyRechargeListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	result, err := h.svc.ListMyRechargeRecords(accountID, role, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}
