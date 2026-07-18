// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/repository"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestUpdateConfigsRejectsR2TargetDriftWithUnclassifiedV1Images(t *testing.T) {
	db := openAdminServiceTestDB(t)
	suffix := fmt.Sprint(time.Now().UnixNano())
	user := model.User{
		Username: "r2-drift-" + suffix, Email: "r2-drift-" + suffix + "@example.test",
		PasswordHash: "test", Role: "user", Status: "active",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(t.Name() + suffix))
	imageFile := model.ImageFile{
		FileHash: hex.EncodeToString(hash[:]), FileSize: 1, MimeType: "image/webp",
		OriginalPath: "original/r2-drift-" + suffix + ".webp",
	}
	if err := db.Create(&imageFile).Error; err != nil {
		t.Fatal(err)
	}
	image := model.Image{
		UserID: user.ID, ImageFileID: imageFile.ID, UniqueLink: "r2-drift-" + suffix,
		Visibility: "private", PipelineVersion: 1, ProcessingStatus: model.ImageProcessingStatusCompleted, FileSize: 1,
	}
	if err := db.Create(&image).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&model.Image{}, image.ID)
		db.Unscoped().Delete(&model.ImageFile{}, imageFile.ID)
		db.Unscoped().Delete(&model.User{}, user.ID)
	})

	configRepo := repository.NewSystemConfigRepo(db)
	svc := &AdminService{db: db, configRepo: configRepo}
	_, appErr := svc.UpdateConfigs([]ConfigUpdateItem{
		{Key: "r2_endpoint", Value: "https://new-target-" + suffix + ".example"},
		{Key: "r2_bucket", Value: "new-bucket"},
	})
	if appErr == nil || appErr.Code != 4094 || !strings.Contains(appErr.Message, "未分类") {
		t.Fatalf("UpdateConfigs() error = %#v, want unclassified-history conflict", appErr)
	}
}

func TestCancelUserDeletionReturnsNotFoundWhenPermanentDeletionWins(t *testing.T) {
	db := openAdminServiceTestDB(t)
	user := createPendingDeletionTestUser(t, db, "delete-wins")

	deleteTx := db.Begin()
	if deleteTx.Error != nil {
		t.Fatal(deleteTx.Error)
	}
	deletionCommitted := false
	defer func() {
		if !deletionCommitted {
			deleteTx.Rollback()
		}
	}()

	var locked model.User
	if err := deleteTx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := deleteTx.Delete(&model.User{}, user.ID).Error; err != nil {
		t.Fatal(err)
	}

	svc := newAdminServiceForDeletionTest(db)
	result := make(chan *errcode.AppError, 1)
	go func() {
		result <- svc.CancelUserDeletion(user.ID)
	}()

	select {
	case appErr := <-result:
		t.Fatalf("CancelUserDeletion() returned before the deleting transaction committed: %v", appErr)
	case <-time.After(150 * time.Millisecond):
	}

	if err := deleteTx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	deletionCommitted = true

	select {
	case appErr := <-result:
		if appErr == nil || appErr.Code != 4041 || appErr.HTTP != 404 {
			t.Fatalf("CancelUserDeletion() error = %#v, want user-not-found", appErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CancelUserDeletion() remained blocked after permanent deletion committed")
	}
}

func TestCancelUserDeletionPreventsLaterPermanentDeletionCheck(t *testing.T) {
	db := openAdminServiceTestDB(t)
	user := createPendingDeletionTestUser(t, db, "cancel-wins")
	t.Cleanup(func() {
		db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.Notification{})
		db.Unscoped().Delete(&model.User{}, user.ID)
	})

	svc := newAdminServiceForDeletionTest(db)
	if appErr := svc.CancelUserDeletion(user.ID); appErr != nil {
		t.Fatalf("CancelUserDeletion() error = %v", appErr)
	}

	workerTx := db.Begin()
	if workerTx.Error != nil {
		t.Fatal(workerTx.Error)
	}
	defer workerTx.Rollback()
	var locked model.User
	if err := workerTx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if locked.Status != "active" || locked.DeletionScheduledAt != nil || locked.DeletedByAdmin != "" || locked.BatchDownloadCount != 0 {
		t.Fatalf("cancelled user state = %#v", locked)
	}

	var notificationCount int64
	if err := workerTx.Model(&model.Notification{}).
		Where("user_id = ? AND type = ?", user.ID, "admin.deletion_cancelled").
		Count(&notificationCount).Error; err != nil {
		t.Fatal(err)
	}
	if notificationCount != 1 {
		t.Fatalf("cancellation notifications = %d, want 1", notificationCount)
	}
}

func TestSetUserStatusTransitionsOnlyActiveAndSuspended(t *testing.T) {
	db := openAdminServiceTestDB(t)
	user := createAdminStatusTestUser(t, db, "status-transition", model.UserStatusActive)
	t.Cleanup(func() {
		db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.Notification{})
		db.Unscoped().Delete(&model.User{}, user.ID)
	})
	svc := newAdminServiceForDeletionTest(db)

	if appErr := svc.SetUserStatus(user.ID, model.UserStatusSuspended); appErr != nil {
		t.Fatalf("SetUserStatus(suspended) error = %v", appErr)
	}
	assertAdminTestUserStatus(t, db, user.ID, model.UserStatusSuspended)

	if appErr := svc.SetUserStatus(user.ID, model.UserStatusActive); appErr != nil {
		t.Fatalf("SetUserStatus(active) error = %v", appErr)
	}
	assertAdminTestUserStatus(t, db, user.ID, model.UserStatusActive)

	var notificationCount int64
	if err := db.Model(&model.Notification{}).
		Where("user_id = ? AND type = ?", user.ID, "admin.user_disabled").
		Count(&notificationCount).Error; err != nil {
		t.Fatal(err)
	}
	if notificationCount != 1 {
		t.Fatalf("disable notifications = %d, want 1", notificationCount)
	}
}

func TestSetUserStatusRejectsDeletionStates(t *testing.T) {
	db := openAdminServiceTestDB(t)
	svc := newAdminServiceForDeletionTest(db)

	for _, status := range []string{model.UserStatusPendingDeletion, model.UserStatusDeleting} {
		t.Run(status, func(t *testing.T) {
			user := createAdminStatusTestUser(t, db, "status-locked-"+status, status)
			t.Cleanup(func() {
				db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.Notification{})
				db.Unscoped().Delete(&model.User{}, user.ID)
			})

			appErr := svc.SetUserStatus(user.ID, model.UserStatusActive)

			if appErr == nil || appErr.Code != 4095 || appErr.HTTP != 409 {
				t.Fatalf("SetUserStatus() error = %#v, want status conflict", appErr)
			}
			assertAdminTestUserStatus(t, db, user.ID, status)
		})
	}
}

func TestSetUserStatusRejectsUnsupportedTarget(t *testing.T) {
	db := openAdminServiceTestDB(t)
	user := createAdminStatusTestUser(t, db, "status-invalid-target", model.UserStatusActive)
	t.Cleanup(func() {
		db.Unscoped().Delete(&model.User{}, user.ID)
	})
	svc := newAdminServiceForDeletionTest(db)

	appErr := svc.SetUserStatus(user.ID, model.UserStatusPendingDeletion)

	if appErr == nil || appErr.Code != 3001 || appErr.HTTP != 400 {
		t.Fatalf("SetUserStatus() error = %#v, want bad request", appErr)
	}
	assertAdminTestUserStatus(t, db, user.ID, model.UserStatusActive)
}

func TestRequestUserDeletionRejectsNonActiveStates(t *testing.T) {
	db := openAdminServiceTestDB(t)
	svc := newAdminServiceForDeletionTest(db)

	for _, status := range []string{
		model.UserStatusSuspended,
		model.UserStatusPendingDeletion,
		model.UserStatusDeleting,
	} {
		t.Run(status, func(t *testing.T) {
			user := createAdminStatusTestUser(t, db, "req-"+status, status)
			t.Cleanup(func() {
				db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.Notification{})
				db.Unscoped().Delete(&model.User{}, user.ID)
			})

			appErr := svc.RequestUserDeletion(user.ID, "integration-admin", user.Username)

			if appErr == nil || appErr.Code != 4095 || appErr.HTTP != 409 {
				t.Fatalf("RequestUserDeletion() error = %#v, want status conflict", appErr)
			}
			assertAdminTestUserStatus(t, db, user.ID, status)
			var notificationCount int64
			if err := db.Model(&model.Notification{}).
				Where("user_id = ? AND type = ?", user.ID, "admin.deletion_requested").
				Count(&notificationCount).Error; err != nil {
				t.Fatal(err)
			}
			if notificationCount != 0 {
				t.Fatalf("deletion notifications = %d, want 0", notificationCount)
			}
		})
	}
}

func openAdminServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	return db
}

func createPendingDeletionTestUser(t *testing.T, db *gorm.DB, prefix string) model.User {
	t.Helper()
	now := time.Now()
	suffix := fmt.Sprint(now.UnixNano())
	scheduledAt := now.Add(-time.Hour)
	user := model.User{
		Username:            prefix + "-" + suffix,
		Email:               prefix + "-" + suffix + "@example.test",
		PasswordHash:        "test",
		Role:                "user",
		Status:              "pending_deletion",
		DeletionScheduledAt: &scheduledAt,
		DeletedByAdmin:      "integration-test",
		BatchDownloadCount:  2,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func createAdminStatusTestUser(t *testing.T, db *gorm.DB, prefix, status string) model.User {
	t.Helper()
	suffix := fmt.Sprint(time.Now().UnixNano())
	user := model.User{
		Username:     prefix + "-" + suffix,
		Email:        prefix + "-" + suffix + "@example.test",
		PasswordHash: "test",
		Role:         "user",
		Status:       status,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func assertAdminTestUserStatus(t *testing.T, db *gorm.DB, userID uint64, want string) {
	t.Helper()
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		t.Fatal(err)
	}
	if user.Status != want {
		t.Fatalf("user status = %q, want %q", user.Status, want)
	}
}

func newAdminServiceForDeletionTest(db *gorm.DB) *AdminService {
	notifications := NewNotificationService(repository.NewNotificationRepo(db))
	return NewAdminService(db, nil, notifications, nil, nil, nil, nil, nil)
}
