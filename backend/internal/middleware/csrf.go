// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/response"
	"github.com/kserksi/summerain/internal/pkg/token"
	"github.com/kserksi/summerain/internal/repository"
)

type CSRFMiddleware struct {
	sessionRepo *repository.SessionRepo
}

// RefreshGuard protects the recovery endpoint without requiring the expired
// CSRF token itself. Origin is mandatory; Fetch Metadata is enforced whenever
// the browser provides it (older WebKit versions may omit it).
func (m *CSRFMiddleware) RefreshGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sameRequestOrigin(c) {
			response.Error(c, errcode.ErrCSRFRefreshRejected)
			return
		}

		fetchSite := strings.ToLower(strings.TrimSpace(c.GetHeader("Sec-Fetch-Site")))
		if fetchSite != "" && fetchSite != "same-origin" {
			response.Error(c, errcode.ErrCSRFRefreshRejected)
			return
		}
		c.Next()
	}
}

func sameRequestOrigin(c *gin.Context) bool {
	origin, err := url.Parse(c.GetHeader("Origin"))
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.User != nil ||
		origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0]); forwarded != "" {
		scheme = strings.ToLower(forwarded)
	}

	return (origin.Scheme == "http" || origin.Scheme == "https") &&
		strings.EqualFold(origin.Scheme, scheme) &&
		strings.EqualFold(origin.Host, c.Request.Host)
}

func NewCSRFMiddleware(sessionRepo *repository.SessionRepo) *CSRFMiddleware {
	return &CSRFMiddleware{sessionRepo: sessionRepo}
}

func (m *CSRFMiddleware) Validate() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}

		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			c.Next()
			return
		}

		sessionID := GetSessionID(c)
		if sessionID == 0 {
			c.Next()
			return
		}

		csrfHeader := c.GetHeader("X-CSRF-Token")
		if csrfHeader == "" {
			response.Error(c, errcode.New(4035, "CSRF token required", 403))
			return
		}

		csrfHash := token.SHA256(csrfHeader)
		csrf, err := m.sessionRepo.FindCSRFBySessionAndHash(sessionID, csrfHash)
		if err != nil {
			response.Error(c, errcode.New(4036, "Invalid CSRF token", 403))
			return
		}

		m.sessionRepo.RenewCSRFExpiry(csrf.ID)
		c.Next()
	}
}
