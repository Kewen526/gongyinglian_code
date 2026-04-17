package middleware

import (
	"sync"
	"supply-chain/pkg/response"
	"time"

	"github.com/gin-gonic/gin"
)

type ipRecord struct {
	count    int
	resetAt  time.Time
}

var (
	loginAttempts = make(map[string]*ipRecord)
	loginMu       sync.Mutex
)

const (
	maxLoginAttempts = 10
	loginWindow      = 5 * time.Minute
)

func LoginRateLimit() gin.HandlerFunc {
	go func() {
		for {
			time.Sleep(loginWindow)
			loginMu.Lock()
			now := time.Now()
			for ip, rec := range loginAttempts {
				if now.After(rec.resetAt) {
					delete(loginAttempts, ip)
				}
			}
			loginMu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()

		loginMu.Lock()
		rec, exists := loginAttempts[ip]
		if !exists {
			rec = &ipRecord{resetAt: time.Now().Add(loginWindow)}
			loginAttempts[ip] = rec
		}
		if time.Now().After(rec.resetAt) {
			rec.count = 0
			rec.resetAt = time.Now().Add(loginWindow)
		}
		rec.count++
		count := rec.count
		loginMu.Unlock()

		if count > maxLoginAttempts {
			response.Error(c, 429, "登录尝试次数过多，请5分钟后再试")
			c.Abort()
			return
		}

		c.Next()
	}
}
