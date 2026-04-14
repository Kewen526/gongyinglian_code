package middleware

import (
	"fmt"
	"strconv"
	"strings"
	"supply-chain/internal/config"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"
	"supply-chain/pkg/response"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims is the custom claims embedded in the JWT token.
type JWTClaims struct {
	AccountID uint64 `json:"account_id"`
	Username  string `json:"username"`
	Role      uint8  `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT token for the given account.
func GenerateToken(account *model.Account) (string, error) {
	cfg := config.GlobalConfig.JWT
	claims := JWTClaims{
		AccountID: account.ID,
		Username:  account.Username,
		Role:      account.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(cfg.ExpireHour) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   strconv.FormatUint(account.ID, 10),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.Secret))
}

// JWTAuth validates the JWT token from the Authorization header.
// Sets "account_id", "username", "role" into gin.Context.
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Error(c, 401, "未登录，请提供token")
			c.Abort()
			return
		}

		// Expect "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Error(c, 401, "token格式错误，应为: Bearer <token>")
			c.Abort()
			return
		}

		tokenStr := parts[1]
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(config.GlobalConfig.JWT.Secret), nil
		})

		if err != nil || !token.Valid {
			response.Error(c, 401, "token无效或已过期")
			c.Abort()
			return
		}

		c.Set("account_id", claims.AccountID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// RequireSuperAdmin ensures the current user is a super admin (role=0).
// Used to protect account management endpoints.
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Error(c, 401, "未登录")
			c.Abort()
			return
		}
		if role.(uint8) != model.RoleSuperAdmin {
			response.Forbidden(c, "仅超级管理员可执行此操作")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAccountManager ensures the current user can manage accounts.
// Allowed: super admin (0), team lead (1), supervisor (2).
func RequireAccountManager() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Error(c, 401, "未登录")
			c.Abort()
			return
		}
		r := role.(uint8)
		if r != model.RoleSuperAdmin && r != model.RoleTeamLead && r != model.RoleSupervisor {
			response.Forbidden(c, "无账号管理权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireModulePermission checks that the current user has the specified
// permission (view or edit) on the given module code.
// Super admin bypasses all checks.
func RequireModulePermission(accountRepo *repository.AccountRepo, moduleCode string, needEdit bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		// Super admin bypasses all permission checks
		if role.(uint8) == model.RoleSuperAdmin {
			c.Next()
			return
		}

		accountID, _ := c.Get("account_id")

		// Get account permissions
		perms, err := accountRepo.GetPermissionsByAccountID(accountID.(uint64))
		if err != nil {
			response.InternalError(c, "权限查询失败")
			c.Abort()
			return
		}

		// Get all modules to find the target module ID
		modules, err := accountRepo.GetAllModules()
		if err != nil {
			response.InternalError(c, "模块查询失败")
			c.Abort()
			return
		}

		var targetModuleID uint64
		for _, m := range modules {
			if m.Code == moduleCode {
				targetModuleID = m.ID
				break
			}
		}

		if targetModuleID == 0 {
			response.Forbidden(c, "模块不存在")
			c.Abort()
			return
		}

		// Check permission
		for _, p := range perms {
			if p.ModuleID == targetModuleID {
				if needEdit && p.CanEdit == 1 {
					c.Next()
					return
				}
				if !needEdit && (p.CanView == 1 || p.CanEdit == 1) {
					c.Next()
					return
				}
			}
		}

		if needEdit {
			response.Forbidden(c, "无该模块的编辑权限")
		} else {
			response.Forbidden(c, "无该模块的查看权限")
		}
		c.Abort()
	}
}
