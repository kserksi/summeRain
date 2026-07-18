// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/repository"
)

func TestClampTTLms(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want int64
	}{
		{"zero falls back to default", 0, PrivateTokenDefaultTTLms},
		{"negative falls back to default", -5, PrivateTokenDefaultTTLms},
		{"below min clamps to min", 1, PrivateTokenMinTTLms},
		{"above max clamps to max", PrivateTokenMaxTTLms + 1, PrivateTokenMaxTTLms},
		{"in range unchanged", 1200000, 1200000},
		{"exactly min unchanged", PrivateTokenMinTTLms, PrivateTokenMinTTLms},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampTTLms(tt.in); got != tt.want {
				t.Fatalf("clampTTLms(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateAccessTokenClassification(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name   string
		token  *model.ImageAccessToken
		findOK bool
		want   AccessTokenValidation
		incr   uint64 // expected IncrementUsage id, 0 = not called
	}{
		{"valid", &model.ImageAccessToken{ID: 1, ImageID: 7, ExpiresAt: future}, true, TokenValid, 1},
		{"expired", &model.ImageAccessToken{ID: 2, ImageID: 7, ExpiresAt: past}, true, TokenExpired, 0},
		{"revoked", &model.ImageAccessToken{ID: 3, ImageID: 7, ExpiresAt: future, RevokedAt: &past}, true, TokenRevoked, 0},
		{"wrong image is not found", &model.ImageAccessToken{ID: 4, ImageID: 99, ExpiresAt: future}, true, TokenNotFound, 0},
		{"missing token is not found", nil, false, TokenNotFound, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeImageAccessTokenRepo{}
			if !tt.findOK {
				repo.tokenErr = errors.New("not found")
			}
			repo.tokenByString = tt.token
			svc := &ImageService{tokenRepo: repo}

			got := svc.ValidateAccessToken(7, "some-token")

			if got != tt.want {
				t.Fatalf("ValidateAccessToken = %v, want %v", got, tt.want)
			}
			if repo.incrementedID != tt.incr {
				t.Fatalf("IncrementUsage id = %d, want %d", repo.incrementedID, tt.incr)
			}
		})
	}
}

func TestIssueAccessTokenReplacesActiveTokenAtomically(t *testing.T) {
	repo := &fakeImageAccessTokenRepo{}
	svc := &ImageService{
		imageRepo: &fakeImageRepo{foundImage: &model.Image{ID: 7, UserID: 3}},
		tokenRepo: repo,
	}

	result, appErr := svc.IssueAccessToken(3, 7, false, 0)

	if appErr != nil {
		t.Fatalf("IssueAccessToken error: %v", appErr)
	}
	if result.Token == "" {
		t.Fatalf("IssueAccessToken returned empty token")
	}
	if len(repo.created) != 1 {
		t.Fatalf("created %d tokens, want 1", len(repo.created))
	}
	if repo.created[0].Token != result.Token {
		t.Fatalf("stored token does not match returned token")
	}
	if repo.revokeCalledImageID != 7 {
		t.Fatalf("ReplaceActiveForImage called for image %d, want 7", repo.revokeCalledImageID)
	}
	if repo.replaceActorID != 3 {
		t.Fatalf("ReplaceActiveForImage actor = %d, want 3", repo.replaceActorID)
	}
	if repo.created[0].ExpiresAt.Sub(time.Now()) < time.Duration(PrivateTokenDefaultTTLms-500)*time.Millisecond {
		t.Fatalf("expiry not clamped to default TTL; got %v", repo.created[0].ExpiresAt)
	}
}

func TestIssueAccessTokenRejectsNonOwner(t *testing.T) {
	svc := &ImageService{
		tokenRepo: &fakeImageAccessTokenRepo{replaceErr: repository.ErrAccessTokenForbidden},
	}

	_, appErr := svc.IssueAccessToken(99, 7, false, 0)

	if appErr == nil || appErr.HTTP != 403 {
		t.Fatalf("IssueAccessToken non-owner error = %v, want 403", appErr)
	}
}

func TestIssueAccessTokenReturnsNotFound(t *testing.T) {
	svc := &ImageService{
		tokenRepo: &fakeImageAccessTokenRepo{replaceErr: repository.ErrAccessTokenImageNotFound},
	}

	_, appErr := svc.IssueAccessToken(3, 999, false, 0)

	if appErr == nil || appErr.HTTP != 404 {
		t.Fatalf("IssueAccessToken missing image error = %v, want 404", appErr)
	}
}

func TestIssueAccessTokenAllowsAdmin(t *testing.T) {
	repo := &fakeImageAccessTokenRepo{}
	svc := &ImageService{
		imageRepo: &fakeImageRepo{foundImage: &model.Image{ID: 7, UserID: 3}},
		tokenRepo: repo,
	}

	_, appErr := svc.IssueAccessToken(99, 7, true, 0)

	if appErr != nil {
		t.Fatalf("IssueAccessToken admin error: %v", appErr)
	}
	if len(repo.created) != 1 {
		t.Fatalf("admin should be able to issue; created %d", len(repo.created))
	}
	if !repo.replaceIsAdmin {
		t.Fatal("admin authority was not passed to the atomic replacement")
	}
}

func TestIssueAccessTokenStrictlyHandlesReplacementError(t *testing.T) {
	repo := &fakeImageAccessTokenRepo{replaceErr: errors.New("revoke failed")}
	svc := &ImageService{tokenRepo: repo}

	result, appErr := svc.IssueAccessToken(3, 7, false, 0)

	if result != nil || appErr != errcode.ErrDatabase {
		t.Fatalf("IssueAccessToken() = %#v, %#v; want database error", result, appErr)
	}
}

func TestRevokeAccessTokenReturnsRevokedState(t *testing.T) {
	repo := &fakeImageAccessTokenRepo{revokeCount: 1}
	svc := &ImageService{
		imageRepo: &fakeImageRepo{foundImage: &model.Image{ID: 7, UserID: 3}},
		tokenRepo: repo,
	}

	result, appErr := svc.RevokeAccessToken(3, 7, false)

	if appErr != nil {
		t.Fatalf("RevokeAccessToken error: %v", appErr)
	}
	if !result.Revoked {
		t.Fatalf("Revoked = false, want true")
	}
}

func TestRevokeAccessTokenRejectsNonOwner(t *testing.T) {
	svc := &ImageService{
		imageRepo: &fakeImageRepo{foundImage: &model.Image{ID: 7, UserID: 3}},
		tokenRepo: &fakeImageAccessTokenRepo{},
	}

	_, appErr := svc.RevokeAccessToken(99, 7, false)

	if appErr == nil || appErr.HTTP != 403 {
		t.Fatalf("RevokeAccessToken non-owner error = %v, want 403", appErr)
	}
}

func TestActiveAccessTokenReturnsNilWhenNone(t *testing.T) {
	repo := &fakeImageAccessTokenRepo{activeErr: errors.New("not found")}
	svc := &ImageService{
		imageRepo: &fakeImageRepo{foundImage: &model.Image{ID: 7, UserID: 3}},
		tokenRepo: repo,
	}

	tok, appErr := svc.ActiveAccessToken(3, 7, false)

	if appErr != nil {
		t.Fatalf("ActiveAccessToken error: %v", appErr)
	}
	if tok != nil {
		t.Fatalf("ActiveAccessToken = %+v, want nil when none active", tok)
	}
}

func TestAllowedImageMIMERejectsSVG(t *testing.T) {
	if allowedImageMIME("image/svg+xml") {
		t.Fatalf("allowedImageMIME accepted SVG")
	}
}

func TestAllowedImageMIMEAcceptsSupportedTypes(t *testing.T) {
	for _, mimeType := range []string{"image/png", "image/jpeg", "image/webp", "image/gif"} {
		if !allowedImageMIME(mimeType) {
			t.Fatalf("allowedImageMIME(%q) = false, want true", mimeType)
		}
	}
}

func TestAllowedImageMIMERejectsAVIFUntilSniffingIsSupported(t *testing.T) {
	if allowedImageMIME("image/avif") {
		t.Fatalf("allowedImageMIME accepted AVIF without reliable content sniffing")
	}
}

type fakeImageAccessTokenRepo struct {
	tokenByString       *model.ImageAccessToken
	tokenErr            error
	activeToken         *model.ImageAccessToken
	activeErr           error
	revokeCount         int64
	revokeErr           error
	revokeCalledImageID uint64
	replaceActorID      uint64
	replaceIsAdmin      bool
	replaceErr          error
	created             []*model.ImageAccessToken
	incrementedID       uint64
	incrementErr        error
}

func (f *fakeImageAccessTokenRepo) ReplaceActiveForImage(actorID, imageID uint64, isAdmin bool, t *model.ImageAccessToken, revokedAt time.Time) error {
	f.replaceActorID = actorID
	f.replaceIsAdmin = isAdmin
	f.revokeCalledImageID = imageID
	if f.replaceErr != nil {
		return f.replaceErr
	}
	f.created = append(f.created, t)
	return nil
}

func (f *fakeImageAccessTokenRepo) FindActiveByImageID(imageID uint64) (*model.ImageAccessToken, error) {
	return f.activeToken, f.activeErr
}

func (f *fakeImageAccessTokenRepo) FindByToken(token string) (*model.ImageAccessToken, error) {
	return f.tokenByString, f.tokenErr
}

func (f *fakeImageAccessTokenRepo) RevokeActiveByImageID(imageID uint64, revokedAt time.Time) (int64, error) {
	f.revokeCalledImageID = imageID
	return f.revokeCount, f.revokeErr
}

func (f *fakeImageAccessTokenRepo) IncrementUsage(id uint64) error {
	f.incrementedID = id
	return f.incrementErr
}

type fakeImageRepo struct {
	foundImage *model.Image
	findErr    error
}

func (f *fakeImageRepo) Create(image *model.Image) error { return nil }

func (f *fakeImageRepo) FindByID(id uint64) (*model.Image, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	if f.foundImage == nil {
		return nil, errors.New("not found")
	}
	return f.foundImage, nil
}

func (f *fakeImageRepo) FindByUniqueLink(link string) (*model.Image, error) {
	return f.foundImage, f.findErr
}

func (f *fakeImageRepo) FindByUserID(userID uint64, cursor string, limit int, sort string, visibility string, search string) ([]*model.Image, string, error) {
	return []*model.Image{f.foundImage}, "", f.findErr
}

func (f *fakeImageRepo) FindOriginalPathsByUserID(userID uint64) ([]*model.Image, error) {
	return []*model.Image{f.foundImage}, f.findErr
}

func (f *fakeImageRepo) Delete(id uint64) error { return nil }

func (f *fakeImageRepo) UpdateVisibility(id uint64, visibility string) error { return nil }

func (f *fakeImageRepo) IncrementViewCount(id uint64) error { return nil }
