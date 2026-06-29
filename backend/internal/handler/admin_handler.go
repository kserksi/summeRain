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

type AdminHandler struct {
	adminService *service.AdminService
}

func NewAdminHandler(adminService *service.AdminService) *AdminHandler {
	return &AdminHandler{adminService: adminService}
}

func (h *AdminHandler) RequireAdmin(c *gin.Context) {
	if middleware.GetPlatform(c) != "web" {
		response.Error(c, errcode.ErrAdminWebOnly)
		return
	}
	if middleware.GetRole(c) != "admin" {
		response.Error(c, errcode.New(4030, "权限不足", 403))
		return
	}
	c.Next()
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	result, appErr := h.adminService.ListUsers(page, pageSize)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, result)
}

type setUserStatusReq struct {
	Status string `json:"status" binding:"required,oneof=active suspended pending"`
}

func (h *AdminHandler) SetUserStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3001, "无效的用户ID", 400))
		return
	}

	callerID := middleware.GetUserID(c)
	if callerID == id {
		response.Error(c, errcode.New(1001, "不能操作自己的账号", 400))
		return
	}

	var req setUserStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, errcode.New(3001, "参数验证失败", 400))
		return
	}

	if appErr := h.adminService.SetUserStatus(id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}

func (h *AdminHandler) GetConfigs(c *gin.Context) {
	configs, appErr := h.adminService.GetConfigs()
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, configs)
}

type batchUpdateConfigsReq struct {
	Items []service.ConfigUpdateItem `json:"items" binding:"required,dive"`
}

func (h *AdminHandler) UpdateConfigs(c *gin.Context) {
	var req batchUpdateConfigsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, errcode.New(3001, "参数验证失败", 400))
		return
	}

	result, appErr := h.adminService.UpdateConfigs(req.Items)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, result)
}

func (h *AdminHandler) GetStats(c *gin.Context) {
	stats, appErr := h.adminService.GetStats()
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, stats)
}

func (h *AdminHandler) ListImages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, appErr := h.adminService.ListAllImages(page, pageSize)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, result)
}

type requestDeletionReq struct {
	Username string `json:"username" binding:"required"`
}

func (h *AdminHandler) RequestUserDeletion(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3001, "无效的用户ID", 400))
		return
	}

	if middleware.GetUserID(c) == id {
		response.Error(c, errcode.New(1001, "不能操作自己的账号", 400))
		return
	}

	var req requestDeletionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, errcode.New(3001, "参数验证失败", 400))
		return
	}

	adminUsername := c.Query("admin")
	appErr := h.adminService.RequestUserDeletion(id, adminUsername, req.Username)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}

func (h *AdminHandler) CancelUserDeletion(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3001, "无效的用户ID", 400))
		return
	}

	appErr := h.adminService.CancelUserDeletion(id)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}

type updateQuotaReq struct {
	StorageQuota int64 `json:"storage_quota" binding:"required"`
}

func (h *AdminHandler) DeleteImage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3001, "无效的图片ID", 400))
		return
	}
	result, appErr := h.adminService.AdminDeleteImage(id)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, result)
}

func (h *AdminHandler) UpdateUserQuota(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3001, "无效的用户ID", 400))
		return
	}

	var req updateQuotaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, errcode.New(3001, "参数验证失败", 400))
		return
	}

	appErr := h.adminService.UpdateUserQuota(id, req.StorageQuota)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}
