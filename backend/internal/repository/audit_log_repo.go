// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"github.com/kserksi/summerain/internal/model"
	"gorm.io/gorm"
)

type AuditLogRepo struct {
	db *gorm.DB
}

func NewAuditLogRepo(db *gorm.DB) *AuditLogRepo {
	return &AuditLogRepo{db: db}
}

func (r *AuditLogRepo) Create(log *model.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *AuditLogRepo) FindByUserID(userID uint64, offset, limit int) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64

	r.db.Model(&model.AuditLog{}).Where("user_id = ?", userID).Count(&total)
	err := r.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&logs).Error
	return logs, total, err
}
