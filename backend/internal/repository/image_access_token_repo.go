// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"time"

	"github.com/kserksi/summerain/internal/model"
	"gorm.io/gorm"
)

type ImageAccessTokenRepo struct {
	db *gorm.DB
}

func NewImageAccessTokenRepo(db *gorm.DB) *ImageAccessTokenRepo {
	return &ImageAccessTokenRepo{db: db}
}

func (r *ImageAccessTokenRepo) Create(token *model.ImageAccessToken) error {
	return r.db.Create(token).Error
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

// RevokeActiveByImageID marks the image's active token as revoked. Returns the
// number of tokens revoked (0 = no active token). Enforces the one-active-per-
// image invariant: a new issue revokes the old active token via this method.
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
