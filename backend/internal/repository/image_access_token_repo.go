// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"errors"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAccessTokenImageNotFound = errors.New("access token image not found")
	ErrAccessTokenForbidden     = errors.New("access token image access forbidden")
)

type ImageAccessTokenRepo struct {
	db *gorm.DB
}

func NewImageAccessTokenRepo(db *gorm.DB) *ImageAccessTokenRepo {
	return &ImageAccessTokenRepo{db: db}
}

// ReplaceActiveForImage serializes issuance on the image row so concurrent
// callers cannot leave more than one non-revoked token behind.
func (r *ImageAccessTokenRepo) ReplaceActiveForImage(actorID, imageID uint64, isAdmin bool, accessToken *model.ImageAccessToken, revokedAt time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var image model.Image
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "user_id").
			First(&image, imageID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAccessTokenImageNotFound
			}
			return err
		}
		if image.UserID != actorID && !isAdmin {
			return ErrAccessTokenForbidden
		}

		if err := tx.Model(&model.ImageAccessToken{}).
			Where("image_id = ? AND revoked_at IS NULL", imageID).
			Update("revoked_at", revokedAt).Error; err != nil {
			return err
		}

		accessToken.ImageID = imageID
		accessToken.UserID = actorID
		return tx.Create(accessToken).Error
	})
}

// FindActiveByImageID returns the single active (non-revoked, non-expired)
// token for an image, or ErrRecordNotFound if none.
func (r *ImageAccessTokenRepo) FindActiveByImageID(imageID uint64) (*model.ImageAccessToken, error) {
	var token model.ImageAccessToken
	if err := r.db.Where("image_id = ? AND revoked_at IS NULL AND expires_at > ?", imageID, time.Now()).
		First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

// FindByToken returns a token by its plaintext value in ANY state
// (valid / expired / revoked), so the service can classify it.
func (r *ImageAccessTokenRepo) FindByToken(token string) (*model.ImageAccessToken, error) {
	var t model.ImageAccessToken
	if err := r.db.Where("token = ?", token).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// RevokeActiveByImageID marks the image's active token as revoked and returns
// the number of affected rows. Issuance uses ReplaceActiveForImage so its
// revoke-and-create sequence remains atomic.
func (r *ImageAccessTokenRepo) RevokeActiveByImageID(imageID uint64, revokedAt time.Time) (int64, error) {
	res := r.db.Model(&model.ImageAccessToken{}).
		Where("image_id = ? AND revoked_at IS NULL", imageID).
		Update("revoked_at", revokedAt)
	return res.RowsAffected, res.Error
}

func (r *ImageAccessTokenRepo) IncrementUsage(id uint64) error {
	return r.db.Model(&model.ImageAccessToken{}).Where("id = ?", id).
		UpdateColumn("usage_count", gorm.Expr("usage_count + 1")).Error
}
