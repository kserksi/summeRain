// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"github.com/summerain/image-gallery/internal/model"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"github.com/summerain/image-gallery/internal/repository"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserService struct {
	db                  *gorm.DB
	auditLogRepo        *repository.AuditLogRepo
	notificationService *NotificationService
}

func NewUserService(db *gorm.DB, auditLogRepo *repository.AuditLogRepo, notificationService *NotificationService) *UserService {
	return &UserService{
		db:                  db,
		auditLogRepo:        auditLogRepo,
		notificationService: notificationService,
	}
}

type UserProfile struct {
	ID             uint64  `json:"id"`
	Username       string  `json:"username"`
	Email          string  `json:"email"`
	Role           string  `json:"role"`
	Status         string  `json:"status"`
	AvatarURL      *string `json:"avatar_url"`
	StorageUsed    int64   `json:"storage_used"`
	StorageQuota   int64   `json:"storage_quota"`
	StoragePercent float64 `json:"storage_percent"`
	ImageCount     int     `json:"image_count"`
	CreatedAt      string  `json:"created_at"`
}

func (s *UserService) GetProfile(userID uint64) (*UserProfile, *errcode.AppError) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errcode.New(4041, "用户不存在", 404)
		}
		return nil, errcode.ErrDatabase
	}
	var pct float64
	if user.StorageQuota > 0 {
		pct = float64(user.StorageUsed) / float64(user.StorageQuota) * 100
	}
	return &UserProfile{
		ID:             user.ID,
		Username:       user.Username,
		Email:          user.Email,
		Role:           user.Role,
		Status:         user.Status,
		AvatarURL:      user.AvatarURL,
		StorageUsed:    user.StorageUsed,
		StorageQuota:   user.StorageQuota,
		StoragePercent: pct,
		ImageCount:     user.ImageCount,
		CreatedAt:      user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

func (s *UserService) ChangePassword(userID uint64, oldPassword, newPassword, ipAddress string) *errcode.AppError {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return errcode.ErrDatabase
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return errcode.ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errcode.ErrInternal
	}

	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("password_hash", string(hash)).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.Session{}).Error; err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		return errcode.ErrDatabase
	}

	_ = s.auditLogRepo.Create(&model.AuditLog{
		UserID:    userID,
		Action:    "password_changed",
		IPAddress: ipAddress,
	})

	_ = s.notificationService.Create(userID, "auth.password_changed", "密码已修改", "您的密码已成功修改，所有会话已被终止")

	return nil
}

func (s *UserService) UpdateAvatar(userID uint64, avatarURL string) *errcode.AppError {
	if err := s.db.Model(&model.User{}).Where("id = ?", userID).Update("avatar_url", avatarURL).Error; err != nil {
		return errcode.ErrDatabase
	}
	return nil
}
