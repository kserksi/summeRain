// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLimitJSONBodyRejectsKnownOversizeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(LimitJSONBody(8))
	router.POST("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("123456789"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestLimitJSONBodyBoundsChunkedAndStructuredJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, contentType := range []string{"application/json; charset=utf-8", "application/problem+json"} {
		t.Run(contentType, func(t *testing.T) {
			var readErr error
			router := gin.New()
			router.Use(LimitJSONBody(8))
			router.POST("/", func(c *gin.Context) {
				_, readErr = io.ReadAll(c.Request.Body)
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("123456789"))
			request.ContentLength = -1
			request.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			var maximumError *http.MaxBytesError
			if !errors.As(readErr, &maximumError) {
				t.Fatalf("read error = %v, want MaxBytesError", readErr)
			}
		})
	}
}

func TestLimitJSONBodyLeavesStreamingImageBodyAlone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var received int
	router := gin.New()
	router.Use(LimitJSONBody(8))
	router.PUT("/", func(c *gin.Context) {
		data, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Fatal(err)
		}
		received = len(data)
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader("123456789"))
	request.Header.Set("Content-Type", "image/webp")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if received != 9 || response.Code != http.StatusNoContent {
		t.Fatalf("received=%d status=%d, want 9/%d", received, response.Code, http.StatusNoContent)
	}
}
