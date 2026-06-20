package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/summerain/image-gallery/internal/config"
	"github.com/summerain/image-gallery/internal/model"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"github.com/summerain/image-gallery/internal/repository"
	"gorm.io/gorm"
)

type AdminService struct {
	db                  *gorm.DB
	configRepo          *repository.SystemConfigRepo
	notificationService *NotificationService
	storageCfg          *config.StorageConfig
	rdb                 *redis.Client
	imageRepo           *repository.ImageRepo
	imageFileRepo       *repository.ImageFileRepo
	imageSvc            *ImageService
}

func NewAdminService(db *gorm.DB, configRepo *repository.SystemConfigRepo, notificationService *NotificationService, storageCfg *config.StorageConfig, rdb *redis.Client, imageRepo *repository.ImageRepo, imageFileRepo *repository.ImageFileRepo, imageSvc *ImageService) *AdminService {
	return &AdminService{
		db:                  db,
		configRepo:          configRepo,
		notificationService: notificationService,
		storageCfg:          storageCfg,
		rdb:                 rdb,
		imageRepo:           imageRepo,
		imageFileRepo:       imageFileRepo,
		imageSvc:            imageSvc,
	}
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
	result := s.db.Model(&model.User{}).Where("id = ?", userID).Update("status", status)
	if result.Error != nil {
		return errcode.ErrDatabase
	}
	if result.RowsAffected == 0 {
		return errcode.New(4041, "用户不存在", 404)
	}

	if status == "suspended" {
		_ = s.notificationService.Create(userID, "admin.user_disabled", "账号已被禁用", "您的账号已被管理员禁用")
		s.db.Where("user_id = ?", userID).Delete(&model.Session{})
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

func (s *AdminService) UpdateConfigs(items []ConfigUpdateItem) (*ConfigUpdateResult, *errcode.AppError) {
	updates := make([]model.SystemConfig, len(items))
	for i, item := range items {
		updates[i] = model.SystemConfig{
			ConfigKey:   item.Key,
			ConfigValue: item.Value,
		}
	}
	if err := s.configRepo.BatchUpdate(updates); err != nil {
		return nil, errcode.ErrDatabase
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

type SystemStats struct {
	TotalUsers   int64 `json:"total_users"`
	TotalImages  int64 `json:"total_images"`
	StorageUsed  int64 `json:"storage_used"`
	ActiveUsers  int64 `json:"active_users"`
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
	DefaultQuota    int64 = 524288000
	MinQuota        int64 = 524288000
	DeletionLockHours       = 24
)

type AdminImageListResult struct {
	Items []*ImageWithUser `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
}

type 	ImageWithUser struct {
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
		return errcode.New(4041, "用户不存在", 404)
	}
	if user.Username != confirmUsername {
		return errcode.New(3000, "用户名不匹配", 400)
	}
	if user.Role == "admin" {
		return errcode.New(4030, "不能注销管理员账户", 403)
	}

	deletionTime := time.Now().Add(DeletionLockHours * time.Hour)
	if err := s.db.Model(&user).Updates(map[string]interface{}{
		"status":               "pending_deletion",
		"deletion_scheduled_at": deletionTime,
		"deleted_by_admin":      adminUsername,
	}).Error; err != nil {
		return errcode.ErrDatabase
	}

	_ = s.notificationService.Create(targetID, "admin.deletion_requested", "账号注销通知",
		"您的账号已被管理员标记注销，24小时后将永久删除所有数据。请在此期间下载您需要的数据。")

	// Kill all sessions to force re-login with restricted status
	s.db.Where("user_id = ?", targetID).Delete(&model.Session{})

	return nil
}

func (s *AdminService) CancelUserDeletion(targetID uint64) *errcode.AppError {
	var user model.User
	if err := s.db.First(&user, targetID).Error; err != nil {
		return errcode.New(4041, "用户不存在", 404)
	}
	if user.Status != "pending_deletion" {
		return errcode.New(3000, "该用户不在注销锁定中", 400)
	}

	if err := s.db.Model(&user).Updates(map[string]interface{}{
		"status":                "active",
		"deletion_scheduled_at": nil,
		"deleted_by_admin":      "",
		"batch_download_count":  0,
	}).Error; err != nil {
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
