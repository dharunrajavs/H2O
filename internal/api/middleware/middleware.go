package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// RequestID injects a unique X-Request-ID header into every request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// Logger logs each request with structured fields
func Logger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		log.Info("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
			zap.String("request_id", c.GetString("request_id")),
			zap.String("tenant", c.GetString("tenant_id")),
		)
	}
}

// Recovery recovers from panics and returns 500
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return gin.RecoveryWithWriter(nil, func(c *gin.Context, err any) {
		log.Error("panic recovered",
			zap.Any("error", err),
			zap.String("path", c.Request.URL.Path))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	})
}

// CORS handles Cross-Origin Resource Sharing headers
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// ipLimiter manages per-IP rate limiters
type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func newIPLimiter(r rate.Limit, b int) *ipLimiter {
	return &ipLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
	}
}

func (il *ipLimiter) get(ip string) *rate.Limiter {
	il.mu.Lock()
	defer il.mu.Unlock()

	if lim, ok := il.limiters[ip]; ok {
		return lim
	}

	lim := rate.NewLimiter(il.rate, il.burst)
	il.limiters[ip] = lim
	return lim
}

// RateLimit limits requests per IP to n requests per interval
func RateLimit(n int, interval time.Duration) gin.HandlerFunc {
	r := rate.Limit(float64(n) / interval.Seconds())
	limiter := newIPLimiter(r, n)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.get(ip).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": fmt.Sprintf("%v", interval),
			})
			return
		}
		c.Next()
	}
}
