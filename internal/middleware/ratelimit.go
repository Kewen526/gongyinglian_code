package middleware

import (
	"fmt"
	"sync"
	"supply-chain/pkg/response"
	"time"

	"github.com/gin-gonic/gin"
)

type ipRecord struct {
	count   int
	resetAt time.Time
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

// ---------- Global API rate limiter ----------

type rateBucket struct {
	tokens   int
	lastTime time.Time
}

var (
	globalBuckets = make(map[string]*rateBucket)
	globalMu      sync.Mutex
)

func init() {
	go func() {
		for {
			time.Sleep(time.Minute)
			globalMu.Lock()
			now := time.Now()
			for key, b := range globalBuckets {
				if now.Sub(b.lastTime) > 2*time.Minute {
					delete(globalBuckets, key)
				}
			}
			globalMu.Unlock()
		}
	}()
}

func GlobalRateLimit(maxPerSecond int) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if aid := c.GetUint64("account_id"); aid > 0 {
			key = fmt.Sprintf("user:%d", aid)
		}

		globalMu.Lock()
		b, exists := globalBuckets[key]
		now := time.Now()
		if !exists {
			b = &rateBucket{tokens: maxPerSecond, lastTime: now}
			globalBuckets[key] = b
		}

		elapsed := now.Sub(b.lastTime).Seconds()
		b.tokens += int(elapsed * float64(maxPerSecond))
		if b.tokens > maxPerSecond*2 {
			b.tokens = maxPerSecond * 2
		}
		b.lastTime = now

		if b.tokens <= 0 {
			globalMu.Unlock()
			response.Error(c, 429, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		b.tokens--
		globalMu.Unlock()

		c.Next()
	}
}
