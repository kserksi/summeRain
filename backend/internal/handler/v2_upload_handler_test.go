// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/middleware"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/service"
)

type batchStatusServiceStub struct {
	*service.V2UploadService
	result *service.V2BatchStatusResponse
	err    *errcode.AppError
	userID uint64
	req    *service.V2BatchStatusRequest
}

func (s *batchStatusServiceStub) BatchStatus(_ context.Context, userID uint64, req *service.V2BatchStatusRequest) (*service.V2BatchStatusResponse, *errcode.AppError) {
	s.userID = userID
	s.req = req
	return s.result, s.err
}

func TestV2UploadHandlerBatchStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	uploadID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	stub := &batchStatusServiceStub{result: &service.V2BatchStatusResponse{
		Uploads: []service.V2UploadResponse{{UploadID: uploadID, Status: "processing"}},
	}}
	handler := &V2UploadHandler{service: stub}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(middleware.ContextKeyUserID, uint64(42))
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/uploads/status", bytes.NewBufferString(`{"upload_ids":["`+uploadID+`"]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.BatchStatus(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if stub.userID != 42 || stub.req == nil || len(stub.req.UploadIDs) != 1 || stub.req.UploadIDs[0] != uploadID {
		t.Fatalf("service call = user %d, request %#v", stub.userID, stub.req)
	}
	var body struct {
		Data service.V2BatchStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Uploads) != 1 || body.Data.Uploads[0].UploadID != uploadID {
		t.Fatalf("response = %#v", body.Data)
	}
}

func TestV2UploadHandlerBatchStatusDoesNotExposePartialResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &batchStatusServiceStub{err: errcode.ErrUploadSessionMissing}
	handler := &V2UploadHandler{service: stub}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(middleware.ContextKeyUserID, uint64(42))
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/uploads/status", bytes.NewBufferString(`{"upload_ids":["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.BatchStatus(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, exists := body["data"]; exists {
		t.Fatalf("error response leaked partial data: %s", recorder.Body.String())
	}
}

func TestV2UploadHandlerBatchStatusRejectsOversizedJSONBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &batchStatusServiceStub{}
	handler := &V2UploadHandler{service: stub}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(middleware.ContextKeyUserID, uint64(42))
	body := `{"upload_ids":["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"],"padding":"` +
		strings.Repeat("x", v2BatchStatusMaximumJSONBytes) + `"}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/uploads/status", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.BatchStatus(ctx)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if stub.req != nil {
		t.Fatalf("service received oversized request: %#v", stub.req)
	}
}
