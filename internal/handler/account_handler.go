package handler

import (
	"net/http"
	"strconv"
	"supply-chain/internal/middleware"
	"supply-chain/internal/model"
	"supply-chain/internal/service"
	"supply-chain/pkg/response"

	"github.com/gin-gonic/gin"
)

type AccountHandler struct {
	svc *service.AccountService
}

func NewAccountHandler(svc *service.AccountService) *AccountHandler {
	return &AccountHandler{svc: svc}
}

// getCallerInfo extracts caller account_id and role from gin context.
func getCallerInfo(c *gin.Context) (uint64, uint8) {
	aid := c.GetUint64("account_id")
	r, _ := c.Get("role")
	role, _ := r.(uint8)
	return aid, role
}

// POST /api/v1/login
func (h *AccountHandler) Login(c *gin.Context) {
	var req model.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	account, err := h.svc.Login(&req)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, err.Error())
		return
	}

	// Generate JWT token
	token, err := middleware.GenerateToken(account)
	if err != nil {
		response.InternalError(c, "生成token失败")
		return
	}

	// Get account detail with permissions
	detail, err := h.svc.GetAccountDetail(account.ID)
	if err != nil {
		response.InternalError(c, "获取账号信息失败")
		return
	}

	response.Success(c, model.LoginResp{
		Token:   token,
		Account: *detail,
	})
}

// GET /api/v1/accounts
func (h *AccountHandler) ListAccounts(c *gin.Context) {
	var req model.AccountListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	callerID, callerRole := getCallerInfo(c)
	result, err := h.svc.ListAccounts(req.Page, req.PageSize, callerID, callerRole)
	if err != nil {
		response.InternalError(c, "查询账号列表失败: "+err.Error())
		return
	}
	response.Success(c, result)
}

// POST /api/v1/accounts
func (h *AccountHandler) CreateAccount(c *gin.Context) {
	var req model.CreateAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	callerID, callerRole := getCallerInfo(c)
	account, err := h.svc.CreateAccount(&req, callerID, callerRole)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, account)
}

// GET /api/v1/modules
func (h *AccountHandler) GetAllModules(c *gin.Context) {
	modules, err := h.svc.GetAllModules()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, modules)
}

// GET /api/v1/accounts/:id
func (h *AccountHandler) GetAccountDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的账号ID")
		return
	}

	detail, err := h.svc.GetAccountDetail(id)
	if err != nil {
		response.NotFound(c, "账号不存在")
		return
	}

	response.Success(c, detail)
}

// PUT /api/v1/accounts/:id
func (h *AccountHandler) UpdateAccount(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的账号ID")
		return
	}

	var req model.UpdateAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	callerID, callerRole := getCallerInfo(c)
	if err := h.svc.UpdateAccount(id, &req, callerID, callerRole); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// DELETE /api/v1/accounts/:id
func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的账号ID")
		return
	}

	callerID, callerRole := getCallerInfo(c)
	if err := h.svc.DeleteAccount(id, callerID, callerRole); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// GET /api/v1/accounts/:id/product-scope
func (h *AccountHandler) GetProductScope(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的账号ID")
		return
	}
	scope, err := h.svc.GetProductScope(id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, scope)
}

// PUT /api/v1/accounts/:id/product-scope
func (h *AccountHandler) SaveProductScope(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的账号ID")
		return
	}
	var req model.ProductScopeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	callerID, callerRole := getCallerInfo(c)
	if err := h.svc.SaveProductScope(id, &req, callerID, callerRole); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// PUT /api/v1/accounts/:id/permissions
func (h *AccountHandler) UpdatePermissions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的账号ID")
		return
	}

	var req model.UpdatePermissionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	callerID, callerRole := getCallerInfo(c)
	if err := h.svc.UpdatePermissions(id, &req, callerID, callerRole); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// ---------- Payment Info ----------

// GET /api/v1/payment-info
// 仅团队负责人可调用，返回自己配置的收款信息（未配置时返回空结构体）。
func (h *AccountHandler) GetMyPaymentInfo(c *gin.Context) {
	_, role := getCallerInfo(c)
	if role != model.RoleTeamLead {
		response.Forbidden(c, "仅团队负责人可查看收款信息")
		return
	}
	accountID := c.GetUint64("account_id")
	info, err := h.svc.GetMyPaymentInfo(accountID)
	if err != nil {
		response.InternalError(c, "查询收款信息失败")
		return
	}
	response.Success(c, info)
}

// PUT /api/v1/payment-info
// 仅团队负责人可调用，保存收款信息。主管不可操作。
func (h *AccountHandler) SaveMyPaymentInfo(c *gin.Context) {
	_, role := getCallerInfo(c)
	if role != model.RoleTeamLead {
		response.Forbidden(c, "仅团队负责人可配置收款信息")
		return
	}
	accountID := c.GetUint64("account_id")
	var req model.SavePaymentInfoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.SavePaymentInfo(accountID, &req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, nil)
}

// GET /api/v1/payment-info/leader
// 仅员工可调用，返回所属团队负责人的收款信息。
func (h *AccountHandler) GetLeaderPaymentInfo(c *gin.Context) {
	_, role := getCallerInfo(c)
	if role != model.RoleEmployee {
		response.Forbidden(c, "仅员工可查看团队负责人收款信息")
		return
	}
	accountID := c.GetUint64("account_id")
	info, err := h.svc.GetLeaderPaymentInfo(accountID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, info)
}
