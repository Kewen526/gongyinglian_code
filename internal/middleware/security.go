package middleware

import (
	"net/http"
	"supply-chain/internal/config"
	"supply-chain/pkg/response"

	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		allowed := false
		origins := config.GlobalConfig.Security.AllowedOrigins
		for _, o := range origins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if !allowed {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-App-Token")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func AppTokenCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := config.GlobalConfig.Security.AppToken
		if token == "" {
			c.Next()
			return
		}

		if c.GetHeader("X-App-Token") != token {
			response.Error(c, 403, "禁止访问")
			c.Abort()
			return
		}

		c.Next()
	}
}
