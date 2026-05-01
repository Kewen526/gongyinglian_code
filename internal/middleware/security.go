package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"supply-chain/internal/config"
	"supply-chain/pkg/response"
	"time"

	"github.com/gin-gonic/gin"
)

// originMatches checks if an origin matches a pattern.
// Supports exact match, "*" wildcard, and "*.suffix" wildcard (e.g. "*.run.app").
func originMatches(pattern, origin string) bool {
	if pattern == "*" || pattern == origin {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".run.app"
		// Strip protocol from origin to get host (e.g. "https://foo.run.app" → "foo.run.app")
		host := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
		// Remove port if present
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		return strings.HasSuffix(host, suffix)
	}
	return false
}

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
			if originMatches(o, origin) {
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

		// Accept previous, current, and next bucket to handle client/server clock skew
		if clientToken == generateHMACToken(secret, currentBucket) ||
			clientToken == generateHMACToken(secret, currentBucket-1) ||
			clientToken == generateHMACToken(secret, currentBucket+1) {
			c.Next()
			return
		}

		response.Error(c, 403, "令牌无效或已过期")
		c.Abort()
	}
}
