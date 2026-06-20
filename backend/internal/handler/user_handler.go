package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/summerain/image-gallery/internal/middleware"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"github.com/summerain/image-gallery/internal/pkg/response"
	"github.com/summerain/image-gallery/internal/service"
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
