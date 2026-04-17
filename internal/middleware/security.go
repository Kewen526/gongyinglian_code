package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"supply-chain/internal/config"
	"supply-chain/pkg/response"
	"time"

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
		reqHeaders := c.GetHeader("Access-Control-Request-Headers")
		if reqHeaders == "" {
			reqHeaders = "Authorization, Content-Type, X-App-Token"
		}
		c.Header("Access-Control-Allow-Headers", reqHeaders)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

const defaultTokenInterval = 300 // 5 minutes

func generateHMACToken(secret string, bucket int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d", bucket)))
	return hex.EncodeToString(mac.Sum(nil))
}

func AppTokenCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		secret := config.GlobalConfig.Security.AppToken
		if secret == "" {
			c.Next()
			return
		}

		interval := int64(config.GlobalConfig.Security.TokenInterval)
		if interval <= 0 {
			interval = defaultTokenInterval
		}

		clientToken := c.GetHeader("X-App-Token")
		if clientToken == "" {
			response.Error(c, 403, "禁止访问")
			c.Abort()
			return
		}

		now := time.Now().Unix()
		currentBucket := now / interval

		// Accept current and previous bucket to handle boundary clock skew
		if clientToken == generateHMACToken(secret, currentBucket) ||
			clientToken == generateHMACToken(secret, currentBucket-1) {
			c.Next()
			return
		}

		response.Error(c, 403, "令牌无效或已过期")
		c.Abort()
	}
}
