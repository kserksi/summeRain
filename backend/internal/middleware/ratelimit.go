// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/response"
)

// The client crash-report sink accepts a bounded number of reports per IP.
const (
	clientErrorRateLimit  = 20
	clientErrorRateWindow = time.Minute
)

// rateLimitStore is the Redis surface the limiters need. Keeping it narrow
// makes the limiter unit-testable without a live Redis.
type rateLimitStore interface {
	Incr(ctx context.Context, key string) *redis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
}

type RateLimitMiddleware struct {
	rdb rateLimitStore
}

func NewRateLimitMiddleware(rdb *redis.Client) *RateLimitMiddleware {
	return newRateLimitMiddlewareWithStore(rdb)
}

func newRateLimitMiddlewareWithStore(store rateLimitStore) *RateLimitMiddleware {
	return &RateLimitMiddleware{rdb: store}
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

// ClientErrorLimit bounds the public client-crash-report sink. A browser that
// crashes in a loop must not be able to amplify its own failure into a log or
// Redis flood.
func (m *RateLimitMiddleware) ClientErrorLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("client-error:ip:%s", ip)

		ctx := c.Request.Context()
		current, err := m.rdb.Incr(ctx, key).Result()
		if err != nil {
			response.Error(c, errcode.ErrRedis)
			return
		}
		if current == 1 {
			m.rdb.Expire(ctx, key, clientErrorRateWindow)
		}
		if current > clientErrorRateLimit {
			c.Header("Retry-After", "60")
			response.Error(c, errcode.ErrClientErrorRateLimited)
			return
		}

		c.Next()
	}
}
