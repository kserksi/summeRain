// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdminService struct {
	db                   *gorm.DB
	configRepo           *repository.SystemConfigRepo
	notificationService  *NotificationService
	storageCfg           *config.StorageConfig
	rdb                  *redis.Client
	imageRepo            *repository.ImageRepo
	imageFileRepo        *repository.ImageFileRepo
	imageSvc             *ImageService
	crossOriginIsolation bool
}

func NewAdminService(db *gorm.DB, configRepo *repository.SystemConfigRepo, notificationService *NotificationService, storageCfg *config.StorageConfig, rdb *redis.Client, imageRepo *repository.ImageRepo, imageFileRepo *repository.ImageFileRepo, imageSvc *ImageService, crossOriginIsolation ...bool) *AdminService {
	service := &AdminService{
		db:                  db,
		configRepo:          configRepo,
		notificationService: notificationService,
		storageCfg:          storageCfg,
		rdb:                 rdb,
		imageRepo:           imageRepo,
		imageFileRepo:       imageFileRepo,
		imageSvc:            imageSvc,
	}
	if len(crossOriginIsolation) > 0 {
		service.crossOriginIsolation = crossOriginIsolation[0]
	}
	return service
}

type UserListResult struct {
	Items []model.User `json:"items"`
	Total int64        `json:"total"`
	Page  int          `json:"page"`
}

func (s *AdminService) ListUsers(page, pageSize int) (*UserListResult, *errcode.AppError) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var users []model.User
	var total int64

	s.db.Model(&model.User{}).Count(&total)
	if err := s.db.Order("id ASC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, errcode.ErrDatabase
	}

	return &UserListResult{
		Items: users,
		Total: total,
		Page:  page,
	}, nil
}

func (s *AdminService) SetUserStatus(userID uint64, status string) *errcode.AppError {
	var sourceStatus string
	switch status {
	case model.UserStatusActive:
		sourceStatus = model.UserStatusSuspended
	case model.UserStatusSuspended:
		sourceStatus = model.UserStatusActive
	default:
		return errcode.New(3001, "用户状态参数无效", http.StatusBadRequest)
	}

	result := s.db.Model(&model.User{}).
		Where("id = ? AND status = ?", userID, sourceStatus).
		Update("status", status)
	if result.Error != nil {
		return errcode.ErrDatabase
	}
	if result.RowsAffected == 0 {
		var user model.User
		if err := s.db.Select("id", "status").First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.New(4041, "用户不存在", http.StatusNotFound)
			}
			return errcode.ErrDatabase
		}
		if user.Status == status {
			return nil
		}
		return errcode.New(4095, "用户当前状态不允许该操作", http.StatusConflict)
	}

	if status == model.UserStatusSuspended {
		_ = s.notificationService.Create(userID, "admin.user_disabled", "账号已被禁用", "您的账号已被管理员禁用")
	}

	return nil
}

func (s *AdminService) GetConfigs() ([]model.SystemConfig, *errcode.AppError) {
	configs, err := s.configRepo.FindAll()
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	return configs, nil
}

type ConfigUpdateItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ConfigUpdateResult struct {
	RestartNeeded bool `json:"restart_needed"`
}

type r2StorageLineage struct {
	RemoteEndpoint string `gorm:"column:remote_endpoint"`
	RemoteBucket   string `gorm:"column:remote_bucket"`
}

func (s *AdminService) UpdateConfigs(items []ConfigUpdateItem) (*ConfigUpdateResult, *errcode.AppError) {
	if validationErr := validateCaptchaConfigUpdate(items, s.crossOriginIsolation); validationErr != nil {
		return nil, validationErr
	}

	updates := make([]model.SystemConfig, len(items))
	hasR2Update := false
	for i, item := range items {
		updates[i] = model.SystemConfig{
			ConfigKey:   item.Key,
			ConfigValue: item.Value,
		}
		hasR2Update = hasR2Update || strings.HasPrefix(item.Key, "r2_")
	}

	var validationErr *errcode.AppError
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if hasR2Update {
			if err := lockV2Storage(tx); err != nil {
				return err
			}

			var currentConfigs []model.SystemConfig
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Find(&currentConfigs).Error; err != nil {
				return err
			}

			var lineages []r2StorageLineage
			if err := tx.Model(&model.ImageFile{}).
				Distinct("remote_endpoint", "remote_bucket").
				Where("remote_backend = ?", "r2").
				Find(&lineages).Error; err != nil {
				return err
			}
			var unclassifiedHistory int64
			if err := tx.Model(&model.ImageFile{}).
				Joins("JOIN images ON images.image_file_id = image_files.id").
				Where("images.pipeline_version < ? AND TRIM(image_files.remote_backend) = ''", model.ImagePipelineVersionV2).
				Distinct("image_files.id").
				Count(&unclassifiedHistory).Error; err != nil {
				return err
			}
			var pendingRemoteDeletes int64
			if err := tx.Model(&model.OutboxEvent{}).
				Where("event_type = ? AND status <> ? AND COALESCE(JSON_LENGTH(JSON_EXTRACT(payload, '$.remote_objects')), 0) > 0",
					model.OutboxEventTypeStorageDelete, model.OutboxEventStatusPublished).
				Count(&pendingRemoteDeletes).Error; err != nil {
				return err
			}
			if validationErr = validateR2ConfigUpdate(currentConfigs, items, lineages, unclassifiedHistory, pendingRemoteDeletes); validationErr != nil {
				return validationErr
			}
		}

		return repository.NewSystemConfigRepo(tx).BatchUpdate(updates)
	})
	if validationErr != nil {
		return nil, validationErr
	}
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	if hasR2Update && s.imageSvc != nil {
		s.imageSvc.ReloadR2()
	}

	result := &ConfigUpdateResult{}
	for _, item := range items {
		if strings.HasPrefix(item.Key, "watermark_") {
			configs, _ := s.configRepo.FindAll()
			cfgMap := make(map[string]string)
			for _, c := range configs {
				cfgMap[c.ConfigKey] = c.ConfigValue
			}
			RegenerateWatermark(cfgMap, s.storageCfg.BasePath)
			if s.rdb != nil {
				s.rdb.Del(context.Background(), "wm:config")
			}
			result.RestartNeeded = true
			break
		}
	}
	return result, nil
}

func validateCaptchaConfigUpdate(items []ConfigUpdateItem, crossOriginIsolation bool) *errcode.AppError {
	for _, item := range items {
		if item.Key != "captcha_provider" {
			continue
		}
		if err := config.ValidateCaptchaCrossOriginIsolation(item.Value, crossOriginIsolation); err != nil {
			return errcode.New(3006, "启用跨源隔离时不能使用 geetest_v4 验证码", http.StatusBadRequest)
		}
	}
	return nil
}

func validateR2ConfigUpdate(currentConfigs []model.SystemConfig, items []ConfigUpdateItem, lineages []r2StorageLineage, unclassifiedHistory, pendingRemoteDeletes int64) *errcode.AppError {
	current := make(map[string]string, len(currentConfigs))
	for _, item := range currentConfigs {
		current[item.ConfigKey] = item.ConfigValue
	}
	proposed := make(map[string]string, len(current)+len(items))
	for key, value := range current {
		proposed[key] = value
	}
	updated := make(map[string]bool, len(items))
	for _, item := range items {
		proposed[item.Key] = item.Value
		updated[item.Key] = true
	}

	if updated["r2_endpoint"] && strings.TrimSpace(proposed["r2_endpoint"]) != "" {
		if _, err := normalizeR2BaseURL(proposed["r2_endpoint"]); err != nil {
			return errcode.New(3006, "r2_endpoint 必须是无凭据、查询参数和片段的 HTTP(S) URL", http.StatusBadRequest)
		}
	}
	if updated["r2_public_url"] && strings.TrimSpace(proposed["r2_public_url"]) != "" {
		if _, err := normalizeR2BaseURL(proposed["r2_public_url"]); err != nil {
			return errcode.New(3006, "r2_public_url 必须是无凭据、查询参数和片段的 HTTP(S) URL", http.StatusBadRequest)
		}
	}

	protectedHistory := len(lineages) > 0 || unclassifiedHistory > 0 || pendingRemoteDeletes > 0
	for _, key := range []string{"r2_access_key", "r2_secret_key"} {
		if protectedHistory && updated[key] && strings.TrimSpace(proposed[key]) == "" {
			return errcode.New(3006, "已有 R2 图片，不能清空 "+key+"；停用 R2 时请保留凭据", http.StatusBadRequest)
		}
	}

	currentEndpoint := normalizeR2Endpoint(current["r2_endpoint"])
	currentBucket := strings.TrimSpace(current["r2_bucket"])
	proposedEndpoint := normalizeR2Endpoint(proposed["r2_endpoint"])
	proposedBucket := strings.TrimSpace(proposed["r2_bucket"])
	if currentEndpoint == proposedEndpoint && currentBucket == proposedBucket {
		return nil
	}
	if pendingRemoteDeletes > 0 {
		return errcode.New(4094, "仍有未完成的 R2 对象清理，不能切换 endpoint/bucket", http.StatusConflict)
	}
	if unclassifiedHistory > 0 {
		return errcode.New(4094, "仍有未分类的历史图片，不能切换 R2 endpoint/bucket", http.StatusConflict)
	}
	if len(lineages) == 0 {
		return nil
	}

	for _, lineage := range lineages {
		if normalizeR2Endpoint(lineage.RemoteEndpoint) != proposedEndpoint || strings.TrimSpace(lineage.RemoteBucket) != proposedBucket {
			return errcode.New(4094, "已有 R2 图片引用不同的 endpoint/bucket，不能切换存储目标", http.StatusConflict)
		}
	}
	return nil
}

func normalizeR2Endpoint(endpoint string) string {
	return strings.TrimRight(strings.TrimSpace(endpoint), "/")
}

type SystemStats struct {
	TotalUsers    int64 `json:"total_users"`
	TotalImages   int64 `json:"total_images"`
	StorageUsed   int64 `json:"storage_used"`
	ActiveUsers   int64 `json:"active_users"`
	TotalSessions int64 `json:"total_sessions"`
}

func (s *AdminService) GetStats() (*SystemStats, *errcode.AppError) {
	var stats SystemStats

	if err := s.db.Model(&model.User{}).Count(&stats.TotalUsers).Error; err != nil {
		return nil, errcode.ErrDatabase
	}

	s.db.Model(&model.User{}).Where("status = ?", "active").Count(&stats.ActiveUsers)

	type StorageSum struct {
		Total int64
	}
	var storageSum StorageSum
	s.db.Model(&model.User{}).Select("COALESCE(SUM(storage_used), 0) as total").Scan(&storageSum)
	stats.StorageUsed = storageSum.Total

	s.db.Table("images").Count(&stats.TotalImages)
	s.db.Model(&model.Session{}).Count(&stats.TotalSessions)

	return &stats, nil
}

const (
	DefaultQuota      int64 = 524288000
	MinQuota          int64 = 524288000
	DeletionLockHours       = 24
)

type AdminImageListResult struct {
	Items []*ImageWithUser `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
}

type ImageWithUser struct {
	model.Image
	OwnerUsername string `json:"owner_username"`
}

func (s *AdminService) AdminDeleteImage(imageID uint64) (*DeleteResult, *errcode.AppError) {
	return s.imageSvc.AdminDelete(imageID)
}

func (s *AdminService) ListAllImages(page, pageSize int) (*AdminImageListResult, *errcode.AppError) {
	images, total, err := s.imageRepo.FindAll(page, pageSize)
	if err != nil {
		return nil, errcode.ErrDatabase
	}

	items := make([]*ImageWithUser, 0, len(images))
	for _, img := range images {
		var user model.User
		s.db.Select("username").First(&user, img.UserID)
		items = append(items, &ImageWithUser{
			Image:         *img,
			OwnerUsername: user.Username,
		})
	}

	return &AdminImageListResult{
		Items: items,
		Total: total,
		Page:  page,
	}, nil
}

func (s *AdminService) RequestUserDeletion(targetID uint64, adminUsername, confirmUsername string) *errcode.AppError {
	var user model.User
	if err := s.db.First(&user, targetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(4041, "用户不存在", http.StatusNotFound)
		}
		return errcode.ErrDatabase
	}
	if user.Username != confirmUsername {
		return errcode.New(3000, "用户名不匹配", 400)
	}
	if user.Role == "admin" {
		return errcode.New(4030, "不能注销管理员账户", 403)
	}
	if user.Status != model.UserStatusActive {
		return errcode.New(4095, "用户当前状态不允许该操作", http.StatusConflict)
	}

	deletionTime := time.Now().Add(DeletionLockHours * time.Hour)
	result := s.db.Model(&model.User{}).
		Where("id = ? AND status = ?", targetID, model.UserStatusActive).
		Updates(map[string]interface{}{
			"status":                model.UserStatusPendingDeletion,
			"deletion_scheduled_at": deletionTime,
			"deleted_by_admin":      adminUsername,
		})
	if result.Error != nil {
		return errcode.ErrDatabase
	}
	if result.RowsAffected == 0 {
		var current model.User
		if err := s.db.Select("id", "status").First(&current, targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.New(4041, "用户不存在", http.StatusNotFound)
			}
			return errcode.ErrDatabase
		}
		return errcode.New(4095, "用户当前状态不允许该操作", http.StatusConflict)
	}

	_ = s.notificationService.Create(targetID, "admin.deletion_requested", "账号注销通知",
		"您的账号已被管理员标记注销，24小时后将永久删除所有数据。请在此期间下载您需要的数据。")

	// Kill all sessions to force re-login with restricted status
	s.db.Where("user_id = ?", targetID).Delete(&model.Session{})

	return nil
}

func (s *AdminService) CancelUserDeletion(targetID uint64) *errcode.AppError {
	var appErr *errcode.AppError
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				appErr = errcode.New(4041, "用户不存在", 404)
				return appErr
			}
			return err
		}
		if user.Status != model.UserStatusPendingDeletion {
			appErr = errcode.New(3000, "该用户不在注销锁定中", 400)
			return appErr
		}

		result := tx.Model(&model.User{}).
			Where("id = ? AND status = ?", targetID, model.UserStatusPendingDeletion).
			Updates(map[string]interface{}{
				"status":                model.UserStatusActive,
				"deletion_scheduled_at": nil,
				"deleted_by_admin":      "",
				"batch_download_count":  0,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("cancel user deletion updated %d rows", result.RowsAffected)
		}
		return nil
	})
	if appErr != nil {
		return appErr
	}
	if err != nil {
		return errcode.ErrDatabase
	}

	_ = s.notificationService.Create(targetID, "admin.deletion_cancelled", "注销已撤销",
		"您的账号注销请求已被管理员撤销，账号恢复正常使用。")

	return nil
}

func (s *AdminService) UpdateUserQuota(targetID uint64, quotaBytes int64) *errcode.AppError {
	if quotaBytes < MinQuota {
		return errcode.New(3000, "配额不能小于 500MB", 400)
	}

	result := s.db.Model(&model.User{}).Where("id = ?", targetID).Update("storage_quota", quotaBytes)
	if result.Error != nil {
		return errcode.ErrDatabase
	}
	if result.RowsAffected == 0 {
		return errcode.New(4041, "用户不存在", 404)
	}

	quotaDisplay := fmt.Sprintf("%.1f GB", float64(quotaBytes)/float64(1073741824))
	if quotaBytes < 1073741824 {
		quotaDisplay = fmt.Sprintf("%.0f MB", float64(quotaBytes)/float64(1048576))
	}
	_ = s.notificationService.Create(targetID, "admin.quota_updated", "存储配额已调整",
		"管理员已将您的存储配额调整为 "+quotaDisplay+"。")

	return nil
}
