// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/response"
)

const DefaultMaximumJSONBodyBytes int64 = 1 << 20

// LimitJSONBody keeps ordinary API JSON decoding bounded. Streaming image
// parts and legacy multipart requests use their own endpoint-specific limits.
func LimitJSONBody(maximum int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maximum < 1 || c.Request.Body == nil || !isJSONContentType(c.GetHeader("Content-Type")) {
			c.Next()
			return
		}
		if c.Request.ContentLength > maximum {
			response.Error(c, errcode.New(3002, "请求体过大", http.StatusRequestEntityTooLarge))
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maximum)
		c.Next()
	}
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}
