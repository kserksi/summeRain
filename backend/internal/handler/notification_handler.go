// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/summerain/image-gallery/internal/middleware"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"github.com/summerain/image-gallery/internal/pkg/response"
	"github.com/summerain/image-gallery/internal/service"
)

type NotificationHandler struct {
	notificationService *service.NotificationService
}

func NewNotificationHandler(notificationService *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{notificationService: notificationService}
}

func (h *NotificationHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var cursor uint64
	if s := c.Query("cursor"); s != "" {
		cursor, _ = strconv.ParseUint(s, 10, 64)
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := h.notificationService.List(userID, cursor, limit)
	if err != nil {
		response.Error(c, errcode.ErrDatabase)
		return
	}
	response.Success(c, result)
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3001, "无效的通知ID", 400))
		return
	}

	if appErr := h.notificationService.MarkRead(userID, id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}

func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if appErr := h.notificationService.MarkAllRead(userID); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}

func (h *NotificationHandler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3001, "无效的通知ID", 400))
		return
	}

	if appErr := h.notificationService.Delete(userID, id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}

func (h *NotificationHandler) ClearAll(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if appErr := h.notificationService.ClearAll(userID); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}
