// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/model"
)

func TestTryBearerMarksPendingDeletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now()
	sessionRepo := &authSessionRepositoryStub{session: &model.Session{
		ID:           11,
		UserID:       22,
		TokenType:    "session",
		Platform:     "android",
		ExpiresAt:    now.Add(time.Hour),
		DeviceID:     "device",
		TokenHash:    "unused-by-stub",
		CreatedAt:    now,
		LastActiveAt: now,
	}}
	middleware := &AuthMiddleware{
		sessionRepo: sessionRepo,
		userRepo: &authUserRepositoryStub{user: &model.User{
			ID: 22, Role: "user", Status: "pending_deletion",
		}},
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/images", nil)
	context.Request.Header.Set("Authorization", "Bearer pending-user-token")

	if !middleware.tryBearer(context) {
		t.Fatal("Bearer token was not handled")
	}
	value, exists := context.Get("pendingDeletion")
	if !exists || value != true {
		t.Fatalf("pendingDeletion = %#v, exists=%v", value, exists)
	}
	if !sessionRepo.expiryUpdated {
		t.Fatal("session expiry was not refreshed")
	}
}

func TestOptionalRejectsDeletingUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		platform string
		prepare  func(*http.Request)
	}{
		{
			name:     "bearer",
			platform: "android",
			prepare: func(req *http.Request) {
				req.Header.Set("Authorization", "Bearer deleting-user-token")
			},
		},
		{
			name:     "cookie",
			platform: "web",
			prepare: func(req *http.Request) {
				req.AddCookie(&http.Cookie{Name: "__Host-session_token", Value: "deleting-user-token"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionRepo := &authSessionRepositoryStub{session: validAuthTestSession(tt.platform)}
			middleware := &AuthMiddleware{
				sessionRepo: sessionRepo,
				userRepo: &authUserRepositoryStub{user: &model.User{
					ID: 22, Role: "user", Status: model.UserStatusDeleting,
				}},
			}
			nextCalled := false
			router := gin.New()
			router.Use(middleware.Optional())
			router.GET("/optional", func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusNoContent)
			})

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/optional", nil)
			tt.prepare(req)
			router.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			if nextCalled {
				t.Fatal("optional downstream handler ran for deleting user")
			}
			if sessionRepo.expiryUpdated {
				t.Fatal("deleting user's session expiry was refreshed")
			}
		})
	}
}

func TestBootstrapAuthRejectsDeletingUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	identity := validAuthTestSession("android")
	identity.TokenType = "identity"
	middleware := &AuthMiddleware{
		sessionRepo: &authSessionRepositoryStub{session: identity},
		userRepo: &authUserRepositoryStub{user: &model.User{
			ID: 22, Role: "user", Status: model.UserStatusDeleting,
		}},
	}
	nextCalled := false
	router := gin.New()
	router.Use(middleware.BootstrapAuth())
	router.POST("/bootstrap", func(c *gin.Context) {
		nextCalled = true
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer deleting-identity-token")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if nextCalled {
		t.Fatal("bootstrap downstream handler ran for deleting user")
	}
}

func TestResolveDoesNotAuthorizeDeletingUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	middleware := &AuthMiddleware{
		sessionRepo: &authSessionRepositoryStub{session: validAuthTestSession("web")},
		userRepo: &authUserRepositoryStub{user: &model.User{
			ID: 22, Role: "user", Status: model.UserStatusDeleting,
		}},
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/i/public-link", nil)
	context.Request.Header.Set("Authorization", "Bearer deleting-user-token")

	userID, role, ok := middleware.Resolve(context)

	if ok || userID != 0 || role != "" {
		t.Fatalf("Resolve() = (%d, %q, %v), want anonymous", userID, role, ok)
	}
}

func validAuthTestSession(platform string) *model.Session {
	now := time.Now()
	return &model.Session{
		ID:           11,
		UserID:       22,
		TokenType:    "session",
		Platform:     platform,
		ExpiresAt:    now.Add(time.Hour),
		DeviceID:     "device",
		TokenHash:    "unused-by-stub",
		CreatedAt:    now,
		LastActiveAt: now,
	}
}

type authSessionRepositoryStub struct {
	session       *model.Session
	expiryUpdated bool
}

func (s *authSessionRepositoryStub) FindByTokenHash(string) (*model.Session, error) {
	return s.session, nil
}

func (s *authSessionRepositoryStub) FindByTokenHashAndType(string, string) (*model.Session, error) {
	return s.session, nil
}

func (s *authSessionRepositoryStub) Delete(uint64) error { return nil }

func (s *authSessionRepositoryStub) UpdateExpiry(uint64, time.Time) error {
	s.expiryUpdated = true
	return nil
}

func (s *authSessionRepositoryStub) CreateAuditLog(*model.AuditLog) error { return nil }

type authUserRepositoryStub struct {
	user *model.User
}

func (s *authUserRepositoryStub) FindByID(uint64) (*model.User, error) { return s.user, nil }
