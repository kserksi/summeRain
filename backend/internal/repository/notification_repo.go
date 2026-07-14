// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"github.com/kserksi/summerain/internal/model"
	"gorm.io/gorm"
)

type NotificationRepo struct {
	db *gorm.DB
}

func NewNotificationRepo(db *gorm.DB) *NotificationRepo {
	return &NotificationRepo{db: db}
}

func (r *NotificationRepo) Create(n *model.Notification) error {
	return r.db.Create(n).Error
}

func (r *NotificationRepo) FindByUserID(userID uint64, cursor uint64, limit int) ([]model.Notification, error) {
	var notifications []model.Notification
	q := r.db.Where("user_id = ?", userID)
	if cursor > 0 {
		q = q.Where("id < ?", cursor)
	}
	err := q.Order("id DESC").Limit(limit).Find(&notifications).Error
	return notifications, err
}

func (r *NotificationRepo) FindByID(id uint64) (*model.Notification, error) {
	var n model.Notification
	err := r.db.First(&n, id).Error
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *NotificationRepo) MarkRead(id uint64) error {
	return r.db.Model(&model.Notification{}).Where("id = ?", id).Update("is_read", true).Error
}

func (r *NotificationRepo) MarkAllRead(userID uint64) error {
	return r.db.Model(&model.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Update("is_read", true).Error
}

func (r *NotificationRepo) Delete(id uint64) error {
	return r.db.Delete(&model.Notification{}, id).Error
}

func (r *NotificationRepo) DeleteAll(userID uint64) error {
	return r.db.Where("user_id = ?", userID).Delete(&model.Notification{}).Error
}

func (r *NotificationRepo) CountUnread(userID uint64) (int64, error) {
	var count int64
	err := r.db.Model(&model.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&count).Error
	return count, err
}
