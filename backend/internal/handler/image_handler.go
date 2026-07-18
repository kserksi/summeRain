// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/middleware"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/response"
	"github.com/kserksi/summerain/internal/service"
)

type ImageHandler struct {
	imageSvc *service.ImageService
}

func NewImageHandler(imageSvc *service.ImageService) *ImageHandler {
	return &ImageHandler{imageSvc: imageSvc}
}

func (h *ImageHandler) Upload(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return
	}
	if c.GetBool("pendingDeletion") {
		response.Error(c, errcode.New(4038, "账号已进入注销锁定期，无法上传", 403))
		return
	}

	// fix: 之前直接用 c.Request.MultipartForm 偶发拿不到文件,改成 c.MultipartForm() 后稳定
	form, err := c.MultipartForm()
	if err != nil {
		response.Error(c, errcode.ErrInternal)
		return
	}

	files := form.File["images"]
	if len(files) == 0 {
		response.Error(c, errcode.New(3001, "未提供图片文件", 400))
		return
	}

	visibility := c.PostForm("visibility")

	result, appErr := h.imageSvc.Upload(userID, files, visibility)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, result)
}

func (h *ImageHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return
	}

	cursor := c.Query("cursor")
	// limit 没传就用默认 20,这里 _ 掉 err 是因为 atoi 失败 fallback 0,后面 service 会兜底
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)
	sort := c.DefaultQuery("sort", "-created_at")
	visibility := c.Query("visibility")
	search := c.Query("search")

	images, nextCursor, appErr := h.imageSvc.ListByUser(userID, cursor, limit, sort, visibility, search)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	hasMore := nextCursor != ""
	response.Success(c, gin.H{
		"images":      images,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

func (h *ImageHandler) Get(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3000, "无效的图片 ID", 400))
		return
	}

	image, appErr := h.imageSvc.GetByID(id)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	isAdmin := middleware.GetRole(c) == "admin"
	if image.UserID != userID && !isAdmin {
		response.Error(c, errcode.New(4031, "无权访问此图片", 403))
		return
	}

	resp := &imageDetailResponse{Image: image}
	if image.Visibility == "private" {
		if tok, _ := h.imageSvc.ActiveAccessToken(userID, id, isAdmin); tok != nil {
			tokenValue := tok.Token
			expires := tok.ExpiresAt
			resp.AccessToken = &tokenValue
			resp.TokenExpiresAt = &expires
		}
	}
	response.Success(c, resp)
}

type imageDetailResponse struct {
	*model.Image
	AccessToken    *string    `json:"access_token,omitempty"`
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
}

func (h *ImageHandler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return
	}
	if c.GetBool("pendingDeletion") {
		response.Error(c, errcode.New(4038, "账号已进入注销锁定期，无法删除", 403))
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3000, "无效的图片 ID", 400))
		return
	}

	result, appErr := h.imageSvc.Delete(userID, id)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, result)
}

func (h *ImageHandler) ToggleVisibility(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return
	}
	if c.GetBool("pendingDeletion") {
		response.Error(c, errcode.New(4038, "账号已进入注销锁定期，无法修改", 403))
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3000, "无效的图片 ID", 400))
		return
	}

	var req struct {
		Visibility string `json:"visibility" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, errcode.New(3000, "visibility 字段必需", 400))
		return
	}

	if req.Visibility != "public" && req.Visibility != "private" {
		response.Error(c, errcode.New(3000, "visibility 必须为 public 或 private", 400))
		return
	}

	result, appErr := h.imageSvc.ToggleVisibility(userID, id, req.Visibility)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, result)
}

func (h *ImageHandler) IssueToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3000, "无效的图片 ID", 400))
		return
	}

	var req struct {
		TTLms int64 `json:"ttl_ms"`
	}
	// 没传 body 也行,后面 service 会用默认 TTL,_ 掉 err 是故意的
	_ = c.ShouldBindJSON(&req)

	isAdmin := middleware.GetRole(c) == "admin"
	result, appErr := h.imageSvc.IssueAccessToken(userID, id, isAdmin, req.TTLms)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, result)
}

func (h *ImageHandler) RevokeToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3000, "无效的图片 ID", 400))
		return
	}

	isAdmin := middleware.GetRole(c) == "admin"
	result, appErr := h.imageSvc.RevokeAccessToken(userID, id, isAdmin)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, result)
}

func (h *ImageHandler) GetUploadQueue(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3000, "无效的队列 ID", 400))
		return
	}

	queue, appErr := h.imageSvc.GetUploadQueue(id)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	if queue.UserID != userID {
		response.Error(c, errcode.New(4031, "无权访问此记录", 403))
		return
	}

	response.Success(c, queue)
}

func (h *ImageHandler) BatchDownload(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.ErrSessionExpired)
		return
	}
	if !c.GetBool("pendingDeletion") {
		response.Error(c, errcode.New(4030, "仅注销锁定期用户可使用批量下载", 403))
		return
	}

	appErr := h.imageSvc.BatchDownloadOriginals(c.Request.Context(), userID, c.Writer, func(filename string) {
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	})
	if appErr != nil {
		if !c.Writer.Written() {
			response.Error(c, appErr)
		} else {
			_ = c.Error(appErr)
			c.Abort()
		}
		return
	}
}
