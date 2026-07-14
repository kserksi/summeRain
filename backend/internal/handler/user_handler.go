// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/middleware"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/response"
	"github.com/kserksi/summerain/internal/service"
)

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	profile, appErr := h.userService.GetProfile(userID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, profile)
}

type changePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, errcode.New(3001, "参数验证失败", 400))
		return
	}

	userID := middleware.GetUserID(c)
	ipAddress := c.ClientIP()

	if appErr := h.userService.ChangePassword(userID, req.OldPassword, req.NewPassword, ipAddress); appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, nil)
}

type updateAvatarReq struct {
	AvatarURL string `json:"avatar_url" binding:"required"`
}

func (h *UserHandler) UpdateAvatar(c *gin.Context) {
	var req updateAvatarReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, errcode.New(3001, "参数验证失败", 400))
		return
	}

	if req.AvatarURL != "" && !strings.HasPrefix(req.AvatarURL, "data:image/") {
		response.Error(c, errcode.New(3000, "头像必须是图片数据", 400))
		return
	}

	if len(req.AvatarURL) > 200*1024 {
		response.Error(c, errcode.New(3002, "头像文件过大", 400))
		return
	}

	userID := middleware.GetUserID(c)
	if appErr := h.userService.UpdateAvatar(userID, req.AvatarURL); appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, gin.H{"avatar_url": req.AvatarURL})
}
