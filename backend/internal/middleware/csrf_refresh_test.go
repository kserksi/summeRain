// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCSRFRefreshGuardAllowsSameOrigin(t *testing.T) {
	status, code := runCSRFRefreshGuard(t, "https://images.example", "same-origin", "https")
	if status != http.StatusNoContent || code != 0 {
		t.Fatalf("status=%d code=%d, want 204/0", status, code)
	}
}

func TestCSRFRefreshGuardAllowsMissingFetchMetadataForOlderWebKit(t *testing.T) {
	status, code := runCSRFRefreshGuard(t, "https://images.example", "", "https")
	if status != http.StatusNoContent || code != 0 {
		t.Fatalf("status=%d code=%d, want 204/0", status, code)
	}
}

func TestCSRFRefreshGuardRejectsMissingOrCrossSiteOrigin(t *testing.T) {
	tests := []struct {
		name      string
		origin    string
		fetchSite string
		proto     string
	}{
		{name: "missing origin", origin: "", fetchSite: "same-origin", proto: "https"},
		{name: "different host", origin: "https://attacker.example", fetchSite: "cross-site", proto: "https"},
		{name: "different scheme", origin: "http://images.example", fetchSite: "same-origin", proto: "https"},
		{name: "hostile fetch metadata", origin: "https://images.example", fetchSite: "cross-site", proto: "https"},
		{name: "opaque origin", origin: "null", fetchSite: "same-origin", proto: "https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code := runCSRFRefreshGuard(t, tt.origin, tt.fetchSite, tt.proto)
			if status != http.StatusForbidden || code != 4036 {
				t.Fatalf("status=%d code=%d, want 403/4036", status, code)
			}
		})
	}
}

func runCSRFRefreshGuard(t *testing.T, origin, fetchSite, forwardedProto string) (int, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	csrf := NewCSRFMiddleware(nil)
	router.POST("/api/v1/auth/csrf/refresh", csrf.RefreshGuard(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "http://images.example/api/v1/auth/csrf/refresh", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if fetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	if forwardedProto != "" {
		req.Header.Set("X-Forwarded-Proto", forwardedProto)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNoContent {
		return recorder.Code, 0
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return recorder.Code, body.Code
}
