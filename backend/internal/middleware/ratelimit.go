package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"github.com/summerain/image-gallery/internal/pkg/response"
)

type RateLimitMiddleware struct {
	rdb *redis.Client
}

func NewRateLimitMiddleware(rdb *redis.Client) *RateLimitMiddleware {
	return &RateLimitMiddleware{rdb: rdb}
}

func (m *RateLimitMiddleware) LoginLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		ipKey := fmt.Sprintf("login:ip:%s", ip)

		ctx := c.Request.Context()
		current, err := m.rdb.Incr(ctx, ipKey).Result()
		if err != nil {
			response.Error(c, errcode.ErrRedis)
			return
		}
		if current == 1 {
			m.rdb.Expire(ctx, ipKey, 15*time.Minute)
		}
		if current > 5 {
			response.Error(c, errcode.ErrLoginRateLimited)
			return
		}

		c.Next()
	}
}

func (m *RateLimitMiddleware) BootstrapLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("bootstrap:ip:%s", ip)

		ctx := c.Request.Context()
		current, err := m.rdb.Incr(ctx, key).Result()
		if err != nil {
			response.Error(c, errcode.ErrRedis)
			return
		}
		if current == 1 {
			m.rdb.Expire(ctx, key, time.Minute)
		}
		if current > 10 {
			c.Header("Retry-After", "60")
			response.Error(c, errcode.ErrBootstrapRateLimit)
			return
		}

		c.Next()
	}
}
