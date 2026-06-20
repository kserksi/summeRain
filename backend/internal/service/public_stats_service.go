package service

import (
	"github.com/summerain/image-gallery/internal/model"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"gorm.io/gorm"
)

type PublicStatsService struct {
	db *gorm.DB
}

func NewPublicStatsService(db *gorm.DB) *PublicStatsService {
	return &PublicStatsService{db: db}
}

type PublicStats struct {
	Images      int64 `json:"images"`
	Users       int64 `json:"users"`
	Views       int64 `json:"views"`
	StorageUsed int64 `json:"storage_used"`
}

// Get returns aggregate site statistics for the public stats endpoint.
func (s *PublicStatsService) Get() (*PublicStats, *errcode.AppError) {
	var stats PublicStats

	if err := s.db.Table("images").Count(&stats.Images).Error; err != nil {
		return nil, errcode.ErrDatabase
	}
	if err := s.db.Model(&model.User{}).Where("status = ?", "active").
		Count(&stats.Users).Error; err != nil {
		return nil, errcode.ErrDatabase
	}

	type sumResult struct {
		Total int64
	}
	var views sumResult
	if err := s.db.Model(&model.Image{}).
		Select("COALESCE(SUM(view_count), 0) AS total").Scan(&views).Error; err == nil {
		stats.Views = views.Total
	}
	var storage sumResult
	if err := s.db.Model(&model.User{}).
		Select("COALESCE(SUM(storage_used), 0) AS total").Scan(&storage).Error; err == nil {
		stats.StorageUsed = storage.Total
	}

	return &stats, nil
}
