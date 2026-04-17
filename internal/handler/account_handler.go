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

// GET /api/v1/orders/auto-review — returns the calling account's auto-review status
func (h *AccountHandler) GetAutoReviewStatus(c *gin.Context) {
	aid := c.GetUint64("account_id")

	enabled, err := h.svc.GetAutoReview(aid)
	if err != nil {
		response.InternalError(c, "获取自动审核状态失败")
		return
	}
	response.Success(c, model.AutoReviewResp{Enabled: enabled})
}

// PUT /api/v1/orders/auto-review — toggles the calling account's auto-review switch
func (h *AccountHandler) SetAutoReviewStatus(c *gin.Context) {
	aid := c.GetUint64("account_id")

	var req model.AutoReviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.svc.SetAutoReview(aid, req.Enabled); err != nil {
		response.InternalError(c, "更新自动审核状态失败")
		return
	}
	response.Success(c, model.AutoReviewResp{Enabled: req.Enabled})
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
