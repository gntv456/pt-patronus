package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter 按 key（通常是客户端 IP）维护独立令牌桶，用于防暴力破解/滥用。
// 空闲超过 bucketIdleTTL 的桶会被定期回收，避免海量伪造 key（如伪造 X-Forwarded-For）
// 被当作内存放大武器（配合 router 的 SetTrustedProxies 从根上消除伪造）。
type RateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucketEntry
	rps       rate.Limit
	burst     int
	lastSweep time.Time
}

type bucketEntry struct {
	limiter *rate.Limiter
	lastUse time.Time
}

const (
	bucketIdleTTL = 2 * time.Minute // 桶空闲超过此时长即回收
	sweepInterval = 1 * time.Minute // 每隔此时长执行一次回收
)

// NewRateLimiter 构造。rps=每秒令牌，burst=突发上限。
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	if rps <= 0 {
		rps = 1
	}
	if burst <= 0 {
		burst = 1
	}
	return &RateLimiter{buckets: map[string]*bucketEntry{}, rps: rate.Limit(rps), burst: burst}
}

func (l *RateLimiter) limiter(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Sub(l.lastSweep) >= sweepInterval {
		l.sweepLocked(now)
	}
	e, ok := l.buckets[key]
	if !ok {
		e = &bucketEntry{limiter: rate.NewLimiter(l.rps, l.burst), lastUse: now}
		l.buckets[key] = e
	}
	e.lastUse = now
	return e.limiter
}

// sweepLocked 删除空闲超时的桶。调用方需持锁。
func (l *RateLimiter) sweepLocked(now time.Time) {
	cutoff := now.Add(-bucketIdleTTL)
	for k, e := range l.buckets {
		if e.lastUse.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
	l.lastSweep = now
}

// Allow 报告 key 是否在限流内。
func (l *RateLimiter) Allow(key string) bool {
	return l.limiter(key).Allow()
}

// Middleware 返回按 ClientIP 限流的 Gin 中间件。仅作用于 /api/ 路径：
// 静态资源（前端 SPA 的 /assets/*.js、*.css、logo 等）走 go:embed 直出，浏览器
// 并发加载 ~30 个 chunk 是正常行为且近乎零成本，不应计入令牌桶——否则刷新时
// 并发 chunk 瞬间打爆 burst，后续 chunk 返 429 → Vue 拿不到 JS 崩成整页白板
// （「第一次进去正常、再进去/刷新就白板」即此症状）。登录端点路径均为
// /api/v1/auth/*，同样受此前缀判断覆盖，限流行为不变。
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Next()
			return
		}
		if !l.Allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}
