// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/token"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRefreshCSRFTokenRetainsConcurrentReplacements(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	suffix := time.Now().Format("20060102150405.000000000")
	user := model.User{
		Username:     "csrf-race-" + suffix,
		Email:        "csrf-race-" + suffix + "@example.test",
		PasswordHash: "test",
		Role:         "user",
		Status:       "active",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	session := model.Session{
		UserID: user.ID, TokenHash: token.SHA256("session-" + suffix),
		TokenType: "session", Platform: "web", LastActiveAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Where("session_id = ?", session.ID).Delete(&model.CSRFToken{})
		db.Delete(&model.Session{}, session.ID)
		db.Delete(&model.User{}, user.ID)
	})

	repo := NewSessionRepo(db, nil)
	currentHash := token.SHA256("expired-cookie")
	firstHash := token.SHA256("replacement-one")
	secondHash := token.SHA256("replacement-two")
	expiresAt := now.Add(24 * time.Hour)
	for _, replacementHash := range []string{firstHash, secondHash} {
		reused, err := repo.RefreshCSRFToken(session.ID, currentHash, replacementHash, expiresAt)
		if err != nil {
			t.Fatal(err)
		}
		if reused {
			t.Fatal("missing current token was unexpectedly reused")
		}
	}

	for _, replacementHash := range []string{firstHash, secondHash} {
		if _, err := repo.FindCSRFBySessionAndHash(session.ID, replacementHash); err != nil {
			t.Fatalf("concurrent replacement %s was invalidated: %v", replacementHash, err)
		}
	}

	for index := 0; index < 10; index++ {
		replacementHash := token.SHA256(fmt.Sprintf("bounded-replacement-%d-%s", index, suffix))
		if _, err := repo.RefreshCSRFToken(session.ID, "", replacementHash, expiresAt); err != nil {
			t.Fatal(err)
		}
	}
	var activeCount int64
	if err := db.Model(&model.CSRFToken{}).
		Where("session_id = ? AND expires_at > ?", session.ID, time.Now()).Count(&activeCount).Error; err != nil {
		t.Fatal(err)
	}
	if activeCount != maxActiveCSRFTokensPerSession {
		t.Fatalf("active CSRF tokens = %d, want bounded count %d", activeCount, maxActiveCSRFTokensPerSession)
	}
}
