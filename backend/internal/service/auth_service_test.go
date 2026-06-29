// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/summerain/image-gallery/internal/model"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
)

func TestRevokeSessionRejectsOtherUsersSession(t *testing.T) {
	sessionRepo := newFakeAuthSessionRepo(seedAuthSession(1, 2, "session"))
	svc := NewAuthService(&fakeAuthUserRepo{}, sessionRepo, nil, nil)

	appErr := svc.RevokeSession(1, 1, "127.0.0.1")

	if appErr == nil || appErr.HTTP != 404 {
		t.Fatalf("RevokeSession error = %v, want 404", appErr)
	}
	if sessionRepo.deletedID != 0 {
		t.Fatalf("deleted session id = %d, want 0", sessionRepo.deletedID)
	}
}

func TestRevokeSessionRejectsIdentityToken(t *testing.T) {
	sessionRepo := newFakeAuthSessionRepo(seedAuthSession(1, 1, "identity"))
	svc := NewAuthService(&fakeAuthUserRepo{}, sessionRepo, nil, nil)

	appErr := svc.RevokeSession(1, 1, "127.0.0.1")

	if appErr == nil || appErr.HTTP != 404 {
		t.Fatalf("RevokeSession error = %v, want 404", appErr)
	}
	if sessionRepo.deletedID != 0 {
		t.Fatalf("deleted session id = %d, want 0", sessionRepo.deletedID)
	}
}

func TestRevokeSessionWritesAuditOnSuccess(t *testing.T) {
	sessionRepo := newFakeAuthSessionRepo(seedAuthSession(1, 1, "session"))
	svc := NewAuthService(&fakeAuthUserRepo{}, sessionRepo, nil, nil)

	appErr := svc.RevokeSession(1, 1, "127.0.0.1")

	if appErr != nil {
		t.Fatalf("RevokeSession returned error: %v", appErr)
	}
	if sessionRepo.deletedID != 1 {
		t.Fatalf("deleted session id = %d, want 1", sessionRepo.deletedID)
	}
	if len(sessionRepo.auditLogs) != 1 {
		t.Fatalf("audit log count = %d, want 1", len(sessionRepo.auditLogs))
	}
	audit := sessionRepo.auditLogs[0]
	if audit.Action != "auth.session_revoked" || audit.ResourceType != "session" || audit.ResourceID != 1 {
		t.Fatalf("audit log = %+v, want auth.session_revoked for session 1", audit)
	}
}

func TestDeviceShutdownRejectsOtherUsersSession(t *testing.T) {
	sessionRepo := newFakeAuthSessionRepo(seedAuthSession(1, 2, "session"))
	svc := NewAuthService(&fakeAuthUserRepo{}, sessionRepo, nil, nil)

	appErr := svc.DeviceShutdown(1, 1, "127.0.0.1")

	if appErr == nil || appErr.HTTP != 404 {
		t.Fatalf("DeviceShutdown error = %v, want 404", appErr)
	}
	if sessionRepo.deletedID != 0 {
		t.Fatalf("deleted session id = %d, want 0", sessionRepo.deletedID)
	}
}

func TestLoginResponseDoesNotMarshalTokens(t *testing.T) {
	data, err := json.Marshal(LoginResponse{SessionToken: "session-secret", CSRFToken: "csrf-secret", User: UserSummary{ID: 1, Username: "alice", Role: "user"}})
	if err != nil {
		t.Fatalf("marshal LoginResponse: %v", err)
	}
	body := string(data)
	if body == "" || body == "{}" {
		t.Fatalf("LoginResponse should still marshal user data, got %s", body)
	}
	if containsAny(body, "session-secret", "csrf-secret", "session_token", "csrf_token") {
		t.Fatalf("LoginResponse leaked token fields: %s", body)
	}
}

func TestRegisterStopsBeforeUserLookupWhenRecaptchaFails(t *testing.T) {
	userRepo := &fakeAuthUserRepo{failOnLookup: true}
	svc := NewAuthService(userRepo, newFakeAuthSessionRepo(), nil, &fakeCaptchaVerifier{err: errcode.ErrRecaptchaFailed})

	_, appErr := svc.Register(context.Background(), &RegisterInput{Username: "alice", Email: "alice@example.com", Password: "password123", Captcha: CaptchaPayload{Token: "bad", Action: "register"}}, "127.0.0.1", "example.com")

	if appErr == nil || appErr.Code != errcode.ErrRecaptchaFailed.Code {
		t.Fatalf("Register error = %v, want recaptcha failed", appErr)
	}
	if userRepo.lookupCount != 0 {
		t.Fatalf("user lookup count = %d, want 0", userRepo.lookupCount)
	}
}

func TestLoginStopsBeforeUserLookupWhenRecaptchaFails(t *testing.T) {
	userRepo := &fakeAuthUserRepo{failOnLookup: true}
	svc := NewAuthService(userRepo, newFakeAuthSessionRepo(), nil, &fakeCaptchaVerifier{err: errcode.ErrRecaptchaFailed})

	_, appErr := svc.Login(context.Background(), &LoginInput{Username: "alice", Password: "password123", Captcha: CaptchaPayload{Token: "bad", Action: "login"}}, "127.0.0.1", "ua", "example.com")

	if appErr == nil || appErr.Code != errcode.ErrRecaptchaFailed.Code {
		t.Fatalf("Login error = %v, want recaptcha failed", appErr)
	}
	if userRepo.lookupCount != 0 {
		t.Fatalf("user lookup count = %d, want 0", userRepo.lookupCount)
	}
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if len(needle) > 0 && contains(s, needle) {
			return true
		}
	}
	return false
}

func contains(s string, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func seedAuthSession(id uint64, userID uint64, tokenType string) *model.Session {
	return &model.Session{ID: id, UserID: userID, TokenType: tokenType, ExpiresAt: time.Now().Add(time.Hour)}
}

type fakeAuthUserRepo struct {
	failOnLookup bool
	lookupCount  int
}

func (f *fakeAuthUserRepo) Create(user *model.User) error { return nil }
func (f *fakeAuthUserRepo) FindByUsername(username string) (*model.User, error) {
	f.lookupCount++
	if f.failOnLookup {
		return nil, fmt.Errorf("unexpected username lookup")
	}
	return nil, errors.New("not found")
}
func (f *fakeAuthUserRepo) FindByEmail(email string) (*model.User, error) {
	f.lookupCount++
	if f.failOnLookup {
		return nil, fmt.Errorf("unexpected email lookup")
	}
	return nil, errors.New("not found")
}
func (f *fakeAuthUserRepo) FindByID(id uint64) (*model.User, error) {
	return nil, errors.New("not found")
}
func (f *fakeAuthUserRepo) UpdatePassword(userID uint64, hash string) error { return nil }

type fakeCaptchaVerifier struct {
	err     *errcode.AppError
	seenExp string
}

func (f *fakeCaptchaVerifier) Verify(ctx context.Context, payload CaptchaPayload, remoteIP string, requestHost string) *errcode.AppError {
	f.seenExp = payload.ExpectedAction
	return f.err
}

type fakeAuthSessionRepo struct {
	sessions  map[uint64]*model.Session
	deletedID uint64
	auditLogs []*model.AuditLog
}

func newFakeAuthSessionRepo(sessions ...*model.Session) *fakeAuthSessionRepo {
	repo := &fakeAuthSessionRepo{sessions: map[uint64]*model.Session{}}
	for _, session := range sessions {
		repo.sessions[session.ID] = session
	}
	return repo
}

func (f *fakeAuthSessionRepo) Create(session *model.Session) error { return nil }
func (f *fakeAuthSessionRepo) FindByTokenHashAndType(tokenHash string, tokenType string) (*model.Session, error) {
	return nil, errors.New("not found")
}
func (f *fakeAuthSessionRepo) FindByID(id uint64) (*model.Session, error) {
	session, ok := f.sessions[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return session, nil
}
func (f *fakeAuthSessionRepo) Delete(id uint64) error {
	f.deletedID = id
	delete(f.sessions, id)
	return nil
}
func (f *fakeAuthSessionRepo) DeleteByIdentityTokenID(identityTokenID uint64) error { return nil }
func (f *fakeAuthSessionRepo) DeleteByUserID(userID uint64) error                   { return nil }
func (f *fakeAuthSessionRepo) UpdateHeartbeat(id uint64) error                      { return nil }
func (f *fakeAuthSessionRepo) CreateCSRFToken(csrf *model.CSRFToken) error          { return nil }
func (f *fakeAuthSessionRepo) DeleteCSRFBySessionID(sessionID uint64) error         { return nil }
func (f *fakeAuthSessionRepo) CheckNonce(ctx context.Context, nonceHash string, identityTokenID uint64) (bool, error) {
	return true, nil
}
func (f *fakeAuthSessionRepo) CheckRateLimit(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	return true, nil
}
func (f *fakeAuthSessionRepo) CountIdentitiesByPlatform(userID uint64, platform string) (int64, error) {
	return 0, nil
}
func (f *fakeAuthSessionRepo) FindIdentitiesByPlatform(userID uint64, platform string) ([]model.Session, error) {
	return nil, nil
}
func (f *fakeAuthSessionRepo) ExpireSessionsByIdentity(identityTokenID uint64) error { return nil }
func (f *fakeAuthSessionRepo) CreateAuditLog(log *model.AuditLog) error {
	f.auditLogs = append(f.auditLogs, log)
	return nil
}
func (f *fakeAuthSessionRepo) FindIdentities(userID uint64) ([]model.Session, error) { return nil, nil }
func (f *fakeAuthSessionRepo) FindByUserID(userID uint64) ([]model.Session, error)   { return nil, nil }
