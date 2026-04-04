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

// POST /api/v1/accounts (requires super admin)
func (h *AccountHandler) CreateAccount(c *gin.Context) {
	var req model.CreateAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	account, err := h.svc.CreateAccount(&req)
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

// PUT /api/v1/accounts/:id/permissions (requires super admin)
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

	if err := h.svc.UpdatePermissions(id, &req); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, nil)
}
