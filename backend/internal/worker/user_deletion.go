// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kserksi/summerain/internal/model"
)

func (m *Manager) runUserDeletion(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[user-deletion] panic recovered: %v", r)
					}
				}()
				m.processPendingDeletions()
			}()
		}
	}
}

func (m *Manager) processPendingDeletions() {
	var users []model.User
	if err := m.DB.Where("status = ? AND deletion_scheduled_at < ?", "pending_deletion", time.Now()).Find(&users).Error; err != nil {
		log.Printf("[user-deletion] query error: %v", err)
		return
	}

	for _, user := range users {
		log.Printf("[user-deletion] permanently deleting user %s (id=%d)", user.Username, user.ID)
		m.permanentlyDeleteUser(user.ID)
	}
}

func (m *Manager) permanentlyDeleteUser(userID uint64) {
	// 1. Delete image files from disk
	var images []model.Image
	m.DB.Where("user_id = ?", userID).Find(&images)
	for _, img := range images {
		var imgFile model.ImageFile
		if err := m.DB.First(&imgFile, img.ImageFileID).Error; err == nil {
			originalPath := filepath.Join(m.Config.Storage.BasePath, imgFile.OriginalPath)
			os.Remove(originalPath)
			thumbPath := filepath.Join(m.Config.Storage.BasePath, "thumbnail", filepath.Base(imgFile.OriginalPath))
			os.Remove(thumbPath)
			processedPath := filepath.Join(m.Config.Storage.BasePath, "processed", filepath.Base(imgFile.OriginalPath))
			os.Remove(processedPath)
		}
	}

	// 2. Delete DB records (order matters for FK constraints)
	m.DB.Where("user_id = ?", userID).Delete(&model.ImageAccessToken{})
	m.DB.Where("user_id = ?", userID).Delete(&model.Image{})
	m.DB.Where("user_id = ?", userID).Delete(&model.Notification{})
	m.DB.Where("user_id = ?", userID).Delete(&model.AuditLog{})
	m.DB.Where("user_id = ?", userID).Delete(&model.UploadQueue{})

	// Delete image_files owned by this user's images
	imageFileIDs := make([]uint64, 0, len(images))
	for _, img := range images {
		imageFileIDs = append(imageFileIDs, img.ImageFileID)
	}
	if len(imageFileIDs) > 0 {
		m.DB.Where("id IN ?", imageFileIDs).Delete(&model.ImageFile{})
	}

	// Delete sessions
	m.DB.Where("user_id = ?", userID).Delete(&model.Session{})

	// Finally delete the user
	result := m.DB.Delete(&model.User{}, userID)
	if result.Error != nil {
		log.Printf("[user-deletion] failed to delete user %d: %v", userID, result.Error)
		return
	}

	log.Printf("[user-deletion] user %d permanently deleted (%d images removed)", userID, len(images))
}
