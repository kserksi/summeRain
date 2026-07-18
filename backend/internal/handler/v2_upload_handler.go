// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/middleware"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/response"
	"github.com/kserksi/summerain/internal/service"
)

const (
	v2InitMaximumJSONBytes        = 64 << 10
	v2BatchStatusMaximumJSONBytes = 16 << 10
)

type v2UploadService interface {
	Recipe() service.V2RecipeResponse
	Init(context.Context, uint64, string, *service.V2InitUploadRequest) (*service.V2UploadResponse, *errcode.AppError)
	Status(context.Context, uint64, string) (*service.V2UploadResponse, *errcode.AppError)
	BatchStatus(context.Context, uint64, *service.V2BatchStatusRequest) (*service.V2BatchStatusResponse, *errcode.AppError)
	PutPart(context.Context, uint64, string, string, string, int64, io.Reader) (*service.V2UploadPartResponse, *errcode.AppError)
	Complete(context.Context, uint64, string) (*service.V2UploadResponse, *errcode.AppError)
	Cancel(context.Context, uint64, string) (*service.V2UploadResponse, *errcode.AppError)
}

type V2UploadHandler struct {
	service v2UploadService
}

func NewV2UploadHandler(uploadService *service.V2UploadService) *V2UploadHandler {
	return &V2UploadHandler{service: uploadService}
}

func (h *V2UploadHandler) Recipe(c *gin.Context) {
	response.Success(c, h.service.Recipe())
}

func (h *V2UploadHandler) Init(c *gin.Context) {
	userID, ok := uploadUser(c)
	if !ok {
		return
	}
	var req service.V2InitUploadRequest
	if appErr := bindBoundedV2JSON(c, &req, v2InitMaximumJSONBytes, "无效的 V2 上传清单"); appErr != nil {
		response.Error(c, appErr)
		return
	}
	result, appErr := h.service.Init(c.Request.Context(), userID, c.GetHeader("Idempotency-Key"), &req)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Created(c, result)
}

func (h *V2UploadHandler) Status(c *gin.Context) {
	userID, ok := uploadUser(c)
	if !ok {
		return
	}
	result, appErr := h.service.Status(c.Request.Context(), userID, c.Param("uploadID"))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, result)
}

func (h *V2UploadHandler) BatchStatus(c *gin.Context) {
	userID, ok := uploadUser(c)
	if !ok {
		return
	}
	var req service.V2BatchStatusRequest
	if appErr := bindBoundedV2JSON(c, &req, v2BatchStatusMaximumJSONBytes, "无效的批量状态查询"); appErr != nil {
		response.Error(c, appErr)
		return
	}
	result, appErr := h.service.BatchStatus(c.Request.Context(), userID, &req)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, result)
}

func (h *V2UploadHandler) PutPart(c *gin.Context) {
	userID, ok := uploadUser(c)
	if !ok {
		return
	}
	result, appErr := h.service.PutPart(
		c.Request.Context(),
		userID,
		c.Param("uploadID"),
		c.Param("kind"),
		c.GetHeader("Content-Type"),
		c.Request.ContentLength,
		c.Request.Body,
	)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, result)
}

func (h *V2UploadHandler) Complete(c *gin.Context) {
	userID, ok := uploadUser(c)
	if !ok {
		return
	}
	result, appErr := h.service.Complete(c.Request.Context(), userID, c.Param("uploadID"))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, result)
}

func (h *V2UploadHandler) Cancel(c *gin.Context) {
	userID, ok := uploadUser(c)
	if !ok {
		return
	}
	result, appErr := h.service.Cancel(c.Request.Context(), userID, c.Param("uploadID"))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, result)
}

func uploadUser(c *gin.Context) (uint64, bool) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return 0, false
	}
	if c.GetBool("pendingDeletion") {
		response.Error(c, errcode.New(4038, "账号已进入注销锁定期，无法上传", http.StatusForbidden))
		return 0, false
	}
	return userID, true
}

func bindBoundedV2JSON(c *gin.Context, destination interface{}, maximum int64, invalidMessage string) *errcode.AppError {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maximum)
	if err := c.ShouldBindJSON(destination); err != nil {
		var maximumError *http.MaxBytesError
		if errors.As(err, &maximumError) {
			return errcode.New(3002, "请求体过大", http.StatusRequestEntityTooLarge)
		}
		return errcode.New(3005, invalidMessage, http.StatusBadRequest)
	}
	return nil
}
