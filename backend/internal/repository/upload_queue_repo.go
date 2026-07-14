// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"github.com/kserksi/summerain/internal/model"
	"gorm.io/gorm"
)

type UploadQueueRepo struct {
	db *gorm.DB
}

func NewUploadQueueRepo(db *gorm.DB) *UploadQueueRepo {
	return &UploadQueueRepo{db: db}
}

func (r *UploadQueueRepo) Create(queue *model.UploadQueue) error {
	return r.db.Create(queue).Error
}

func (r *UploadQueueRepo) UpdateStatus(id uint64, status string, errorMsg string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if errorMsg != "" {
		updates["error_message"] = errorMsg
	}
	return r.db.Model(&model.UploadQueue{}).Where("id = ?", id).Updates(updates).Error
}

func (r *UploadQueueRepo) UpdateStatusAndFileInfo(id uint64, status string, errorMsg string, fileInfo string) error {
	updates := map[string]interface{}{
		"status":    status,
		"file_info": fileInfo,
	}
	if errorMsg != "" {
		updates["error_message"] = errorMsg
	}
	return r.db.Model(&model.UploadQueue{}).Where("id = ?", id).Updates(updates).Error
}

func (r *UploadQueueRepo) FindByID(id uint64) (*model.UploadQueue, error) {
	var queue model.UploadQueue
	if err := r.db.First(&queue, id).Error; err != nil {
		return nil, err
	}
	return &queue, nil
}
