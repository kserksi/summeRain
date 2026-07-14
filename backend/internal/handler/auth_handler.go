// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/middleware"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/response"
	"github.com/kserksi/summerain/internal/service"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(c *gin.Context) {
	platform := c.GetHeader("X-Platform")
	if platform != "" && platform != "web" {
		response.Error(c, errcode.ErrRegistrationWebOnly)
		return
	}

	var input service.RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, errcode.New(3000, "请求参数无效", 400))
		return
	}

	user, appErr := h.authService.Register(c.Request.Context(), &input, c.ClientIP(), c.Request.Host)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Created(c, gin.H{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var input service.LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, errcode.New(3000, "请求参数无效", 400))
		return
	}

	if appErr := h.authService.CheckLoginRateLimit(c.Request.Context(), c.ClientIP(), input.Username); appErr != nil {
		response.Error(c, appErr)
		return
	}

	result, appErr := h.authService.Login(c.Request.Context(), &input, c.ClientIP(), c.Request.UserAgent(), c.Request.Host)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("__Host-session_token", result.SessionToken, 2592000, "/", "", true, true)
	c.SetCookie("__Host-csrf_token", result.CSRFToken, 2592000, "/", "", true, false)

	response.Success(c, gin.H{
		"user": result.User,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	sessionID := middleware.GetSessionID(c)
	if sessionID == 0 {
		response.Error(c, errcode.New(4010, "未认证", 401))
		return
	}

	appErr := h.authService.Logout(sessionID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	c.SetCookie("__Host-session_token", "", -1, "/", "", true, true)
	c.SetCookie("__Host-csrf_token", "", -1, "/", "", true, false)

	response.Success(c, nil)
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Error(c, errcode.New(4010, "未认证", 401))
		return
	}

	user, appErr := h.authService.GetMe(userID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, user)
}

func (h *AuthHandler) DeviceLogin(c *gin.Context) {
	var input service.DeviceLoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, errcode.New(3000, "请求参数无效", 400))
		return
	}

	platform := c.GetHeader("X-Platform")
	if platform == "" || platform == "web" {
		response.Error(c, errcode.New(3000, "X-Platform header required (android/windows)", 400))
		return
	}

	if appErr := h.authService.CheckLoginRateLimit(c.Request.Context(), c.ClientIP(), input.Username); appErr != nil {
		response.Error(c, appErr)
		return
	}

	clientVersion := c.GetHeader("X-Client-Version")
	if clientVersion != "" {
		ver, err := strconv.Atoi(clientVersion)
		if err == nil {
			minVer := getMinVersionForPlatform(platform)
			if ver < minVer {
				c.Header("X-Min-Version", strconv.Itoa(minVer))
				response.Error(c, errcode.ErrVersionTooLow)
				return
			}
		}
	}

	result, appErr := h.authService.DeviceLogin(&input, platform, c.ClientIP(), c.Request.UserAgent())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, result)
}

func (h *AuthHandler) DeviceBootstrap(c *gin.Context) {
	var input service.DeviceBootstrapInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, errcode.New(3000, "请求参数无效", 400))
		return
	}

	if appErr := h.authService.CheckBootstrapRateLimit(c.Request.Context(), c.ClientIP()); appErr != nil {
		c.Header("Retry-After", "60")
		response.Error(c, appErr)
		return
	}

	identityTokenPlain, _ := c.Get("identity_token_plain")
	platform := middleware.GetPlatform(c)

	result, appErr := h.authService.DeviceBootstrap(
		identityTokenPlain.(string),
		&input,
		platform,
		c.ClientIP(),
	)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.Success(c, result)
}

func (h *AuthHandler) DeviceHeartbeat(c *gin.Context) {
	sessionID := middleware.GetSessionID(c)
	appErr := h.authService.DeviceHeartbeat(sessionID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}

func (h *AuthHandler) DeviceShutdown(c *gin.Context) {
	sessionID := middleware.GetSessionID(c)
	userID := middleware.GetUserID(c)
	appErr := h.authService.DeviceShutdown(sessionID, userID, c.ClientIP())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, gin.H{"message": "会话已终止"})
}

func (h *AuthHandler) ListDeviceIdentities(c *gin.Context) {
	userID := middleware.GetUserID(c)
	identities, appErr := h.authService.ListDeviceIdentities(userID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, gin.H{"identities": identities})
}

func (h *AuthHandler) RevokeIdentity(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3000, "无效的 ID", 400))
		return
	}

	userID := middleware.GetUserID(c)
	appErr := h.authService.RevokeIdentity(id, userID, c.ClientIP())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}

func (h *AuthHandler) ListSessions(c *gin.Context) {
	userID := middleware.GetUserID(c)
	sessions, appErr := h.authService.ListSessions(userID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, gin.H{"sessions": sessions})
}

func (h *AuthHandler) RevokeSession(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, errcode.New(3000, "无效的 ID", 400))
		return
	}

	userID := middleware.GetUserID(c)
	appErr := h.authService.RevokeSession(id, userID, c.ClientIP())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, nil)
}

func getMinVersionForPlatform(platform string) int {
	switch platform {
	case "android":
		return 100
	case "windows":
		return 100
	default:
		return 0
	}
}
