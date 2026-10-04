// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/pkg/response"
	"github.com/kserksi/summerain/internal/service"
)

// clientErrorMaximumBodyBytes bounds the public crash-report document. The
// global JSON body limit is far larger; this endpoint needs much less.
const clientErrorMaximumBodyBytes = 8 << 10

type ClientErrorHandler struct {
	service *service.ClientErrorService
}

func NewClientErrorHandler(clientErrorService *service.ClientErrorService) *ClientErrorHandler {
	return &ClientErrorHandler{service: clientErrorService}
}

func (h *ClientErrorHandler) Report(c *gin.Context) {
	var report service.ClientErrorReport
	if appErr := bindBoundedJSON(c, &report, clientErrorMaximumBodyBytes, 3000, "无效的客户端错误上报"); appErr != nil {
		response.Error(c, appErr)
		return
	}
	if appErr := h.service.Report(report, service.ClientErrorMeta{
		RequestID: c.GetString("request_id"),
		UserAgent: c.GetHeader("User-Agent"),
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}
