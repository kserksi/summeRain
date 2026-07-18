// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/repository"
)

func TestIssueAccessTokenConcurrentReplacementLeavesOneActiveToken(t *testing.T) {
	db := openImageCompensationTestDB(t)
	suffix := fmt.Sprint(time.Now().UnixNano())
	user := model.User{
		Username:     "token-race-" + suffix,
		Email:        "token-race-" + suffix + "@example.test",
		PasswordHash: "test",
		Role:         "user",
		Status:       "active",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	imageFile := model.ImageFile{
		FileHash:     fmt.Sprintf("%064x", time.Now().UnixNano()),
		FileSize:     123,
		MimeType:     "image/webp",
		OriginalPath: "original/token-race-" + suffix + ".webp",
	}
	if err := db.Create(&imageFile).Error; err != nil {
		t.Fatal(err)
	}
	image := model.Image{
		UserID: user.ID, ImageFileID: imageFile.ID,
		UniqueLink: "token-race-" + suffix,
		Title:      "token race", Visibility: "private", FileSize: imageFile.FileSize,
	}
	if err := db.Create(&image).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Where("image_id = ?", image.ID).Delete(&model.ImageAccessToken{})
		db.Delete(&model.Image{}, image.ID)
		db.Delete(&model.ImageFile{}, imageFile.ID)
		db.Delete(&model.User{}, user.ID)
	})

	const issuers = 12
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(issuers + 2)
	}
	tokenRepo := repository.NewImageAccessTokenRepo(db)
	svc := &ImageService{tokenRepo: tokenRepo}
	start := make(chan struct{})
	results := make(chan *AccessTokenResult, issuers)
	errors := make(chan error, issuers)
	var wg sync.WaitGroup
	for range issuers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, appErr := svc.IssueAccessToken(user.ID, image.ID, false, PrivateTokenMinTTLms)
			if appErr != nil {
				errors <- appErr
				return
			}
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errors)

	for err := range errors {
		t.Fatalf("concurrent IssueAccessToken() error = %v", err)
	}
	issuedTokens := make(map[string]bool, issuers)
	for result := range results {
		if result == nil || result.Token == "" {
			t.Fatal("concurrent IssueAccessToken() returned an empty token")
		}
		issuedTokens[result.Token] = true
	}
	if len(issuedTokens) != issuers {
		t.Fatalf("unique issued tokens = %d, want %d", len(issuedTokens), issuers)
	}

	var tokens []model.ImageAccessToken
	if err := db.Where("image_id = ?", image.ID).Find(&tokens).Error; err != nil {
		t.Fatal(err)
	}
	if len(tokens) != issuers {
		t.Fatalf("persisted tokens = %d, want %d", len(tokens), issuers)
	}
	active := 0
	var activeToken model.ImageAccessToken
	for _, stored := range tokens {
		if stored.RevokedAt == nil && stored.ExpiresAt.After(time.Now()) {
			active++
			activeToken = stored
			if !issuedTokens[stored.Token] {
				t.Fatalf("active token %q was not returned by an issuer", stored.Token)
			}
		}
	}
	if active != 1 {
		t.Fatalf("active tokens = %d, want exactly 1", active)
	}

	// Force the create step to fail after the current token has been revoked.
	// The unique token value makes Create fail; the transaction must restore the
	// revocation update rather than leaving the image with no active token.
	duplicate := &model.ImageAccessToken{Token: activeToken.Token, ExpiresAt: time.Now().Add(time.Hour)}
	if err := tokenRepo.ReplaceActiveForImage(user.ID, image.ID, false, duplicate, time.Now()); err == nil {
		t.Fatal("duplicate token replacement unexpectedly succeeded")
	}
	var afterFailure model.ImageAccessToken
	if err := db.First(&afterFailure, activeToken.ID).Error; err != nil {
		t.Fatal(err)
	}
	if afterFailure.RevokedAt != nil {
		t.Fatal("failed replacement did not roll back the previous token revocation")
	}
}
