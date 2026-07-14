// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/response"
	"github.com/kserksi/summerain/internal/pkg/token"
	"github.com/kserksi/summerain/internal/repository"
)

const (
	MinAndroidVersion = 100
	MinWindowsVersion = 100
)

type AuthMiddleware struct {
	sessionRepo *repository.SessionRepo
	userRepo    *repository.UserRepo
}

func NewAuthMiddleware(sessionRepo *repository.SessionRepo, userRepo *repository.UserRepo) *AuthMiddleware {
	return &AuthMiddleware{
		sessionRepo: sessionRepo,
		userRepo:    userRepo,
	}
}

func (m *AuthMiddleware) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		if m.tryBearer(c) {
			return
		}
		if m.tryCookie(c) {
			return
		}
		response.Error(c, errcode.New(4010, "未认证", 401))
	}
}

func (m *AuthMiddleware) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		if m.tryBearer(c) {
			return
		}
		if m.tryCookie(c) {
			return
		}
		c.Next()
	}
}

func (m *AuthMiddleware) tryBearer(c *gin.Context) bool {
	auth := c.GetHeader("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}

	plainToken := strings.TrimPrefix(auth, "Bearer ")
	tokenHash := token.SHA256(plainToken)

	session, err := m.sessionRepo.FindByTokenHash(tokenHash)
	if err != nil {
		response.Error(c, errcode.New(4010, "无效的令牌", 401))
		return true
	}

	if session.TokenType == "identity" {
		response.Error(c, errcode.ErrIdentityNotForAPI)
		return true
	}

	if session.ExpiresAt.Before(time.Now()) {
		response.Error(c, errcode.ErrSessionExpired)
		return true
	}

	user, err := m.userRepo.FindByID(session.UserID)
	if err != nil || user.Status == "suspended" {
		response.Error(c, errcode.New(4030, "账户已被禁用", 403))
		return true
	}

	declaredPlatform := c.GetHeader("X-Platform")
	if declaredPlatform != "" && session.Platform != declaredPlatform {
		m.sessionRepo.Delete(session.ID)
		metadata, _ := json.Marshal(map[string]string{
			"db_platform":       session.Platform,
			"declared_platform": declaredPlatform,
			"device_id":         session.DeviceID,
			"ip":                c.ClientIP(),
		})
		m.sessionRepo.CreateAuditLog(&model.AuditLog{
			UserID:    session.UserID,
			Action:    "auth.platform_mismatch",
			IPAddress: c.ClientIP(),
			Metadata:  string(metadata),
		})
		response.Error(c, errcode.New(4010, "Invalid device token", 401))
		return true
	}

	clientVersion := c.GetHeader("X-Client-Version")
	if clientVersion != "" {
		ver, parseErr := strconv.Atoi(clientVersion)
		if parseErr == nil {
			minVer := m.getMinVersion(session.Platform)
			if ver < minVer {
				m.sessionRepo.CreateAuditLog(&model.AuditLog{
					UserID:    session.UserID,
					Action:    "auth.version_blocked",
					IPAddress: c.ClientIP(),
				})
				c.Header("X-Min-Version", strconv.Itoa(minVer))
				response.Error(c, errcode.ErrVersionTooLow)
				return true
			}
		}
	}

	m.sessionRepo.UpdateExpiry(session.ID, time.Now().Add(15*time.Minute))

	c.Set(ContextKeyUserID, session.UserID)
	c.Set(ContextKeySessionID, session.ID)
	c.Set(ContextKeyPlatform, session.Platform)
	c.Set(ContextKeyRole, user.Role)
	c.Set(ContextKeyTokenType, session.TokenType)
	c.Next()
	return true
}

func (m *AuthMiddleware) tryCookie(c *gin.Context) bool {
	cookie, err := c.Cookie("__Host-session_token")
	if err != nil || cookie == "" {
		return false
	}

	tokenHash := token.SHA256(cookie)
	session, err := m.sessionRepo.FindByTokenHash(tokenHash)
	if err != nil || session.Platform != "web" {
		response.Error(c, errcode.New(4010, "无效的会话", 401))
		return true
	}

	if session.ExpiresAt.Before(time.Now()) {
		response.Error(c, errcode.ErrSessionExpired)
		return true
	}

	user, err := m.userRepo.FindByID(session.UserID)
	if err != nil || user.Status == "suspended" {
		response.Error(c, errcode.New(4030, "账户已被禁用", 403))
		return true
	}
	if user.Status == "pending_deletion" {
		c.Set("pendingDeletion", true)
	}

	c.Set(ContextKeyUserID, session.UserID)
	c.Set(ContextKeySessionID, session.ID)
	c.Set(ContextKeyPlatform, "web")
	c.Set(ContextKeyRole, user.Role)
	c.Set(ContextKeyTokenType, "session")
	c.Next()
	return true
}

func (m *AuthMiddleware) getMinVersion(platform string) int {
	switch platform {
	case "android":
		return MinAndroidVersion
	case "windows":
		return MinWindowsVersion
	default:
		return 0
	}
}

func (m *AuthMiddleware) BootstrapAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			response.Error(c, errcode.New(4010, "未认证", 401))
			return
		}

		plainToken := strings.TrimPrefix(auth, "Bearer ")
		tokenHash := token.SHA256(plainToken)

		session, err := m.sessionRepo.FindByTokenHashAndType(tokenHash, "identity")
		if err != nil {
			response.Error(c, errcode.New(4010, "无效的身份令牌", 401))
			return
		}

		if session.ExpiresAt.Before(time.Now()) {
			response.Error(c, errcode.ErrSessionExpired)
			return
		}

	user, err := m.userRepo.FindByID(session.UserID)
	if err != nil || user.Status == "suspended" {
		response.Error(c, errcode.New(4030, "账户已被禁用", 403))
		return
	}
	if user.Status == "pending_deletion" {
		c.Set("pendingDeletion", true)
	}

		clientVersion := c.GetHeader("X-Client-Version")
		if clientVersion != "" {
			ver, parseErr := strconv.Atoi(clientVersion)
			if parseErr == nil {
				minVer := m.getMinVersion(session.Platform)
				if ver < minVer {
					c.Header("X-Min-Version", strconv.Itoa(minVer))
					response.Error(c, errcode.ErrVersionTooLow)
					return
				}
			}
		}

		c.Set(ContextKeyUserID, session.UserID)
		c.Set(ContextKeySessionID, session.ID)
		c.Set(ContextKeyPlatform, session.Platform)
		c.Set(ContextKeyRole, user.Role)
		c.Set(ContextKeyTokenType, "identity")
		c.Set("identity_token_plain", plainToken)
		c.Next()
	}
}

// Resolve is a NON-aborting optional session resolver used by public routes
// (e.g. /i/:link) to grant owner/admin bypass without forcing auth. It never
// writes a response. A third-party image-access token sent as Bearer will fail
// session lookup here and fall through to image-token validation.
func (m *AuthMiddleware) Resolve(c *gin.Context) (userID uint64, role string, ok bool) {
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		plain := strings.TrimPrefix(auth, "Bearer ")
		if uid, r, found := m.lookupSession(token.SHA256(plain), ""); found {
			return uid, r, true
		}
	}
	if cookie, err := c.Cookie("__Host-session_token"); err == nil && cookie != "" {
		if uid, r, found := m.lookupSession(token.SHA256(cookie), "web"); found {
			return uid, r, true
		}
	}
	return 0, "", false
}

func (m *AuthMiddleware) lookupSession(tokenHash, requirePlatform string) (uint64, string, bool) {
	session, err := m.sessionRepo.FindByTokenHash(tokenHash)
	if err != nil || session.TokenType == "identity" {
		return 0, "", false
	}
	if session.ExpiresAt.Before(time.Now()) {
		return 0, "", false
	}
	if requirePlatform != "" && session.Platform != requirePlatform {
		return 0, "", false
	}
	user, err := m.userRepo.FindByID(session.UserID)
	if err != nil || user.Status == "suspended" {
		return 0, "", false
	}
	return user.ID, user.Role, true
}
