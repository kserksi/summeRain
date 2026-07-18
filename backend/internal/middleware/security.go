// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import "github.com/gin-gonic/gin"

func SecurityHeaders(crossOriginIsolation ...bool) gin.HandlerFunc {
	isolate := len(crossOriginIsolation) > 0 && crossOriginIsolation[0]
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if isolate {
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Embedder-Policy", "require-corp")
		}
		c.Next()
	}
}
