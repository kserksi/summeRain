// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/service"
)

func newClientErrorTestHandler(t *testing.T) *ClientErrorHandler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previous := log.Writer()
	log.SetOutput(&bytes.Buffer{})
	t.Cleanup(func() { log.SetOutput(previous) })
	return NewClientErrorHandler(service.NewClientErrorService())
}

func newClientErrorTestContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("request_id", "req_test")
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/public/client-errors",
		strings.NewReader(body),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("User-Agent", "test-agent")
	return ctx, recorder
}

func decodeClientErrorEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) int {
	t.Helper()
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return body.Code
}

func TestClientErrorHandlerReports(t *testing.T) {
	handler := newClientErrorTestHandler(t)
	ctx, recorder := newClientErrorTestContext(`{"kind":"boundary","message":"boom"}`)

	handler.Report(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if code := decodeClientErrorEnvelope(t, recorder); code != 0 {
		t.Fatalf("envelope code = %d, body = %s", code, recorder.Body.String())
	}
}

func TestClientErrorHandlerRejectsOversizedBody(t *testing.T) {
	handler := newClientErrorTestHandler(t)
	body := `{"kind":"error","message":"` + strings.Repeat("a", 9<<10) + `"}`
	ctx, recorder := newClientErrorTestContext(body)

	handler.Report(ctx)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413, body = %s", recorder.Code, recorder.Body.String())
	}
	if code := decodeClientErrorEnvelope(t, recorder); code != 3002 {
		t.Fatalf("envelope code = %d, want 3002", code)
	}
}

func TestClientErrorHandlerRejectsMalformedBody(t *testing.T) {
	handler := newClientErrorTestHandler(t)
	ctx, recorder := newClientErrorTestContext(`{"kind":`)

	handler.Report(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", recorder.Code, recorder.Body.String())
	}
	if code := decodeClientErrorEnvelope(t, recorder); code != 3000 {
		t.Fatalf("envelope code = %d, want 3000", code)
	}
}

func TestClientErrorHandlerRejectsUnknownKind(t *testing.T) {
	handler := newClientErrorTestHandler(t)
	ctx, recorder := newClientErrorTestContext(`{"kind":"poster","message":"boom"}`)

	handler.Report(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", recorder.Code, recorder.Body.String())
	}
	if code := decodeClientErrorEnvelope(t, recorder); code != 3000 {
		t.Fatalf("envelope code = %d, want 3000", code)
	}
}
