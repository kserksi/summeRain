// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetCSRFTokenCookieUsesHostCookieSecurityAttributes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	setCSRFTokenCookie(c, "csrf-value")

	cookies := recorder.Header().Values("Set-Cookie")
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	for _, attribute := range []string{
		"__Host-csrf_token=csrf-value",
		"Path=/",
		"Max-Age=2592000",
		"Secure",
		"SameSite=Strict",
	} {
		if !strings.Contains(cookie, attribute) {
			t.Errorf("cookie %q is missing %q", cookie, attribute)
		}
	}
	if strings.Contains(cookie, "HttpOnly") {
		t.Errorf("CSRF double-submit cookie must remain readable by the browser client: %q", cookie)
	}
	if strings.Contains(cookie, "Domain=") {
		t.Errorf("__Host- cookie must not set Domain: %q", cookie)
	}
}
