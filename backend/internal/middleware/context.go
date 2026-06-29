// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package middleware

import "github.com/gin-gonic/gin"

const (
	ContextKeyUserID    = "user_id"
	ContextKeySessionID = "session_id"
	ContextKeyPlatform  = "platform"
	ContextKeyRole      = "role"
	ContextKeyTokenType = "token_type"
)

func GetUserID(c *gin.Context) uint64 {
	v, _ := c.Get(ContextKeyUserID)
	if id, ok := v.(uint64); ok {
		return id
	}
	return 0
}

func GetSessionID(c *gin.Context) uint64 {
	v, _ := c.Get(ContextKeySessionID)
	if id, ok := v.(uint64); ok {
		return id
	}
	return 0
}

func GetPlatform(c *gin.Context) string {
	v, _ := c.Get(ContextKeyPlatform)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func GetRole(c *gin.Context) string {
	v, _ := c.Get(ContextKeyRole)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
