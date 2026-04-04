package middleware

import (
	"supply-chain/pkg/response"

	"github.com/gin-gonic/gin"
)

// SimpleAuth is a placeholder middleware that extracts account_id from header.
// In production, replace with JWT token validation.
func SimpleAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		accountID := c.GetHeader("X-Account-ID")
		if accountID == "" {
			response.Error(c, 401, "未登录")
			c.Abort()
			return
		}
		c.Set("account_id", accountID)
		c.Next()
	}
}

// Recovery is Gin's built-in recovery middleware wrapper.
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
