// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/repository"
	"gorm.io/gorm"
)

type NotificationService struct {
	repo *repository.NotificationRepo
}

func NewNotificationService(repo *repository.NotificationRepo) *NotificationService {
	return &NotificationService{repo: repo}
}

type NotificationListResult struct {
	Items      []model.Notification `json:"items"`
	NextCursor string               `json:"next_cursor"`
}

func (s *NotificationService) List(userID uint64, cursor uint64, limit int) (*NotificationListResult, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	items, err := s.repo.FindByUserID(userID, cursor, limit+1)
	if err != nil {
		return nil, err
	}

	var nextCursor string
	if len(items) > limit {
		nextCursor = formatUint64(items[limit-1].ID)
		items = items[:limit]
	}

	return &NotificationListResult{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
}

func (s *NotificationService) MarkRead(userID, id uint64) *errcode.AppError {
	n, err := s.repo.FindByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return errcode.ErrNotificationNotFound
		}
		return errcode.ErrDatabase
	}
	if n.UserID != userID {
		return errcode.ErrNotificationNotFound
	}
	if err := s.repo.MarkRead(id); err != nil {
		return errcode.ErrDatabase
	}
	return nil
}

func (s *NotificationService) MarkAllRead(userID uint64) *errcode.AppError {
	if err := s.repo.MarkAllRead(userID); err != nil {
		return errcode.ErrDatabase
	}
	return nil
}

func (s *NotificationService) Delete(userID, id uint64) *errcode.AppError {
	n, err := s.repo.FindByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return errcode.ErrNotificationNotFound
		}
		return errcode.ErrDatabase
	}
	if n.UserID != userID {
		return errcode.ErrNotificationNotFound
	}
	if err := s.repo.Delete(id); err != nil {
		return errcode.ErrDatabase
	}
	return nil
}

func (s *NotificationService) ClearAll(userID uint64) *errcode.AppError {
	if err := s.repo.DeleteAll(userID); err != nil {
		return errcode.ErrDatabase
	}
	return nil
}

func (s *NotificationService) Create(userID uint64, nType, title, message string) error {
	n := &model.Notification{
		UserID:  userID,
		Type:    nType,
		Title:   title,
		Message: message,
	}
	return s.repo.Create(n)
}

func formatUint64(v uint64) string {
	if v == 0 {
		return ""
	}
	buf := make([]byte, 0, 20)
	for v > 0 {
		buf = append(buf, byte('0'+v%10))
		v /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
