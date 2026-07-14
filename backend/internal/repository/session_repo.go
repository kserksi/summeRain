// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/kserksi/summerain/internal/model"
	"gorm.io/gorm"
)

type SessionRepo struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewSessionRepo(db *gorm.DB, rdb *redis.Client) *SessionRepo {
	return &SessionRepo{db: db, rdb: rdb}
}

func (r *SessionRepo) Create(session *model.Session) error {
	return r.db.Create(session).Error
}

func (r *SessionRepo) FindByTokenHash(tokenHash string) (*model.Session, error) {
	var session model.Session
	err := r.db.Where("token_hash = ?", tokenHash).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionRepo) FindByTokenHashAndType(tokenHash string, tokenType string) (*model.Session, error) {
	var session model.Session
	err := r.db.Where("token_hash = ? AND token_type = ?", tokenHash, tokenType).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionRepo) FindByID(id uint64) (*model.Session, error) {
	var session model.Session
	err := r.db.First(&session, id).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionRepo) Delete(id uint64) error {
	return r.db.Delete(&model.Session{}, id).Error
}

func (r *SessionRepo) DeleteByIdentityTokenID(identityTokenID uint64) error {
	return r.db.Where("identity_token_id = ?", identityTokenID).Delete(&model.Session{}).Error
}

func (r *SessionRepo) DeleteByUserID(userID uint64) error {
	return r.db.Where("user_id = ?", userID).Delete(&model.Session{}).Error
}

func (r *SessionRepo) UpdateHeartbeat(id uint64) error {
	now := time.Now()
	return r.db.Model(&model.Session{}).Where("id = ?", id).Update("last_heartbeat_at", &now).Error
}

func (r *SessionRepo) UpdateExpiry(id uint64, expiresAt time.Time) error {
	return r.db.Model(&model.Session{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"expires_at":    gorm.Expr("GREATEST(expires_at, ?)", expiresAt),
			"last_active_at": time.Now(),
		}).Error
}

func (r *SessionRepo) FindByUserID(userID uint64) ([]model.Session, error) {
	var sessions []model.Session
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&sessions).Error
	return sessions, err
}

func (r *SessionRepo) FindIdentities(userID uint64) ([]model.Session, error) {
	var sessions []model.Session
	err := r.db.Where("user_id = ? AND token_type = 'identity' AND expires_at > NOW()", userID).
		Order("created_at DESC").Find(&sessions).Error
	return sessions, err
}

func (r *SessionRepo) CountIdentitiesByPlatform(userID uint64, platform string) (int64, error) {
	var count int64
	err := r.db.Model(&model.Session{}).
		Where("user_id = ? AND platform = ? AND token_type = 'identity' AND expires_at > NOW()", userID, platform).
		Count(&count).Error
	return count, err
}

func (r *SessionRepo) FindIdentitiesByPlatform(userID uint64, platform string) ([]model.Session, error) {
	var sessions []model.Session
	err := r.db.Where("user_id = ? AND platform = ? AND token_type = 'identity' AND expires_at > NOW()", userID, platform).
		Find(&sessions).Error
	return sessions, err
}

func (r *SessionRepo) ExpireSessionsByIdentity(identityTokenID uint64) error {
	return r.db.Model(&model.Session{}).
		Where("identity_token_id = ? AND token_type = 'session'", identityTokenID).
		Update("expires_at", time.Now()).Error
}

func (r *SessionRepo) CreateCSRFToken(csrf *model.CSRFToken) error {
	return r.db.Create(csrf).Error
}

func (r *SessionRepo) FindCSRFBySessionAndHash(sessionID uint64, tokenHash string) (*model.CSRFToken, error) {
	var csrf model.CSRFToken
	err := r.db.Where("session_id = ? AND token_hash = ? AND expires_at > NOW()", sessionID, tokenHash).
		First(&csrf).Error
	if err != nil {
		return nil, err
	}
	return &csrf, nil
}

func (r *SessionRepo) RenewCSRFExpiry(id uint64) error {
	return r.db.Model(&model.CSRFToken{}).Where("id = ?", id).
		Update("expires_at", time.Now().Add(24*time.Hour)).Error
}

func (r *SessionRepo) DeleteCSRFBySessionID(sessionID uint64) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&model.CSRFToken{}).Error
}

func (r *SessionRepo) CreateAuditLog(log *model.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *SessionRepo) CheckNonce(ctx context.Context, nonceHash string, identityTokenID uint64) (bool, error) {
	key := fmt.Sprintf("nonce:%s", nonceHash)
	ok, err := r.rdb.SetNX(ctx, key, identityTokenID, 30*time.Second).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (r *SessionRepo) CheckRateLimit(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	current, err := r.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if current == 1 {
		r.rdb.Expire(ctx, key, window)
	}
	return current <= limit, nil
}
