package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/summerain/image-gallery/internal/model"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"github.com/summerain/image-gallery/internal/pkg/token"
	"golang.org/x/crypto/bcrypt"
)

var deviceIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type AuthService struct {
	userRepo    authUserRepository
	sessionRepo authSessionRepository
	rdb         *redis.Client
	captcha     CaptchaVerifier
	rdbRepo     rdbConfigRepo
}

type rdbConfigRepo interface {
	FindByKey(key string) (*rdbConfigValue, error)
}

type rdbConfigValue struct {
	Value string
}

type authUserRepository interface {
	Create(user *model.User) error
	FindByUsername(username string) (*model.User, error)
	FindByEmail(email string) (*model.User, error)
	FindByID(id uint64) (*model.User, error)
	UpdatePassword(userID uint64, hash string) error
}

type authSessionRepository interface {
	Create(session *model.Session) error
	FindByTokenHashAndType(tokenHash string, tokenType string) (*model.Session, error)
	FindByID(id uint64) (*model.Session, error)
	Delete(id uint64) error
	DeleteByIdentityTokenID(identityTokenID uint64) error
	DeleteByUserID(userID uint64) error
	UpdateHeartbeat(id uint64) error
	CreateCSRFToken(csrf *model.CSRFToken) error
	DeleteCSRFBySessionID(sessionID uint64) error
	CheckNonce(ctx context.Context, nonceHash string, identityTokenID uint64) (bool, error)
	CheckRateLimit(ctx context.Context, key string, limit int64, window time.Duration) (bool, error)
	CountIdentitiesByPlatform(userID uint64, platform string) (int64, error)
	FindIdentitiesByPlatform(userID uint64, platform string) ([]model.Session, error)
	ExpireSessionsByIdentity(identityTokenID uint64) error
	CreateAuditLog(log *model.AuditLog) error
	FindIdentities(userID uint64) ([]model.Session, error)
	FindByUserID(userID uint64) ([]model.Session, error)
}

func NewAuthService(userRepo authUserRepository, sessionRepo authSessionRepository, rdb *redis.Client, captcha CaptchaVerifier, cfgRepo rdbConfigRepo) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		rdb:         rdb,
		captcha:     captcha,
		rdbRepo:     cfgRepo,
	}
}

func (s *AuthService) isCaptchaEnabled() bool {
	if s.captcha == nil {
		return false
	}
	if s.rdbRepo != nil {
		if cfg, err := s.rdbRepo.FindByKey("captcha_provider"); err == nil && cfg.Value == "none" {
			return false
		}
	}
	return true
}

type RegisterInput struct {
	Username string         `json:"username" binding:"required,min=3,max=50"`
	Email    string         `json:"email" binding:"required,email,max=100"`
	Password string         `json:"password" binding:"required,min=8,max=72"`
	Captcha  CaptchaPayload `json:"captcha"`
}

type LoginInput struct {
	Username string         `json:"username" binding:"required"`
	Password string         `json:"password" binding:"required"`
	Captcha  CaptchaPayload `json:"captcha"`
}

type DeviceLoginInput struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	DeviceName string `json:"device_name" binding:"required,max=100"`
	DeviceID   string `json:"device_id" binding:"required,max=100"`
}

type DeviceBootstrapInput struct {
	DeviceID string `json:"device_id" binding:"required,max=100"`
	Nonce    string `json:"nonce" binding:"required,min=16,max=128,hexadecimal"`
}

type DeviceLoginResponse struct {
	IdentityToken   string      `json:"identity_token"`
	SessionToken    string      `json:"session_token"`
	IdentityExpires time.Time   `json:"identity_expires_at"`
	SessionTTL      int         `json:"session_ttl_seconds"`
	User            UserSummary `json:"user"`
	Warning         string      `json:"warning"`
}

type DeviceBootstrapResponse struct {
	SessionToken string `json:"session_token"`
	TTLSeconds   int    `json:"ttl_seconds"`
}

type UserSummary struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type LoginResponse struct {
	SessionToken string      `json:"-"`
	CSRFToken    string      `json:"-"`
	User         UserSummary `json:"user"`
}

type DeviceLimitResponse struct {
	Platform        string          `json:"platform"`
	ExistingDevices []DeviceSummary `json:"existing_devices"`
	Hint            string          `json:"hint"`
}

type DeviceSummary struct {
	ID         uint64    `json:"id"`
	DeviceName string    `json:"device_name"`
	LastActive time.Time `json:"last_active"`
}

func (s *AuthService) Register(ctx context.Context, input *RegisterInput, remoteIP string, requestHost string) (*model.User, *errcode.AppError) {
	if s.isCaptchaEnabled() {
		payload := input.Captcha
		payload.ExpectedAction = "register"
		if appErr := s.captcha.Verify(ctx, payload, remoteIP, requestHost); appErr != nil {
			return nil, appErr
		}
	}
	if _, err := s.userRepo.FindByUsername(input.Username); err == nil {
		return nil, errcode.New(3001, "用户名已存在", 409)
	}
	if _, err := s.userRepo.FindByEmail(input.Email); err == nil {
		return nil, errcode.New(3001, "邮箱已被注册", 409)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errcode.ErrInternal
	}

	user := &model.User{
		Username:     input.Username,
		Email:        input.Email,
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
	}
	if err := s.userRepo.Create(user); err != nil {
		return nil, errcode.ErrDatabase
	}
	return user, nil
}

func (s *AuthService) Login(ctx context.Context, input *LoginInput, ip string, userAgent string, requestHost string) (*LoginResponse, *errcode.AppError) {
	if s.isCaptchaEnabled() {
		payload := input.Captcha
		payload.ExpectedAction = "login"
		if appErr := s.captcha.Verify(ctx, payload, ip, requestHost); appErr != nil {
			return nil, appErr
		}
	}
	user, err := s.userRepo.FindByUsername(input.Username)
	if err != nil {
		return nil, errcode.ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		return nil, errcode.ErrInvalidCredentials
	}
	if user.Status != "active" {
		return nil, errcode.New(4030, "账户已被禁用", 403)
	}

	sessionPlain, sessionHash, err := token.Generate(32)
	if err != nil {
		return nil, errcode.ErrInternal
	}

	now := time.Now()
	session := &model.Session{
		UserID:       user.ID,
		TokenHash:    sessionHash,
		TokenType:    "session",
		Platform:     "web",
		IPAddress:    ip,
		UserAgent:    userAgent,
		LastActiveAt: now,
		ExpiresAt:    now.Add(30 * 24 * time.Hour),
	}
	if err := s.sessionRepo.Create(session); err != nil {
		return nil, errcode.ErrDatabase
	}

	csrfPlain, csrfHash, err := token.Generate(32)
	if err != nil {
		return nil, errcode.ErrInternal
	}

	csrf := &model.CSRFToken{
		SessionID: session.ID,
		TokenHash: csrfHash,
		ExpiresAt: now.Add(24 * time.Hour),
	}
	if err := s.sessionRepo.CreateCSRFToken(csrf); err != nil {
		return nil, errcode.ErrDatabase
	}

	return &LoginResponse{
		SessionToken: sessionPlain,
		CSRFToken:    csrfPlain,
		User: UserSummary{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	}, nil
}

func (s *AuthService) Logout(sessionID uint64) *errcode.AppError {
	s.sessionRepo.DeleteCSRFBySessionID(sessionID)
	if err := s.sessionRepo.Delete(sessionID); err != nil {
		return errcode.ErrDatabase
	}
	return nil
}

func (s *AuthService) GetMe(userID uint64) (*model.User, *errcode.AppError) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, errcode.New(4040, "用户不存在", 404)
	}
	return user, nil
}

func (s *AuthService) DeviceLogin(input *DeviceLoginInput, platform string, ip string, userAgent string) (*DeviceLoginResponse, *errcode.AppError) {
	if !deviceIDRegex.MatchString(input.DeviceID) {
		return nil, errcode.New(3001, "device_id 格式无效", 400)
	}

	user, err := s.userRepo.FindByUsername(input.Username)
	if err != nil {
		return nil, errcode.ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		s.sessionRepo.CreateAuditLog(&model.AuditLog{
			UserID:    user.ID,
			Action:    "auth.device_login_failed",
			IPAddress: ip,
		})
		return nil, errcode.ErrInvalidCredentials
	}
	if user.Status != "active" {
		return nil, errcode.New(4030, "账户已被禁用", 403)
	}

	count, err := s.sessionRepo.CountIdentitiesByPlatform(user.ID, platform)
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	if count >= 3 {
		existing, _ := s.sessionRepo.FindIdentitiesByPlatform(user.ID, platform)
		devices := make([]DeviceSummary, 0, len(existing))
		for _, sess := range existing {
			devices = append(devices, DeviceSummary{
				ID:         sess.ID,
				DeviceName: sess.DeviceName,
				LastActive: sess.LastActiveAt,
			})
		}
		return nil, errcode.NewWithData(
			errcode.ErrDeviceLimitReached.Code,
			errcode.ErrDeviceLimitReached.Message,
			errcode.ErrDeviceLimitReached.HTTP,
			map[string]interface{}{
				"platform":         platform,
				"existing_devices": devices,
				"hint":             "请先在已有设备中撤销一台，或通过 Web 端管理。",
			},
		)
	}

	identityPlain, identityHash, err := token.Generate(32)
	if err != nil {
		return nil, errcode.ErrInternal
	}

	now := time.Now()
	identitySession := &model.Session{
		UserID:       user.ID,
		TokenHash:    identityHash,
		TokenType:    "identity",
		Platform:     platform,
		DeviceID:     input.DeviceID,
		DeviceName:   input.DeviceName,
		IPAddress:    ip,
		UserAgent:    userAgent,
		LastActiveAt: now,
		ExpiresAt:    now.Add(90 * 24 * time.Hour),
	}
	if err := s.sessionRepo.Create(identitySession); err != nil {
		return nil, errcode.ErrDatabase
	}

	sessionPlain, sessionHash, err := token.Generate(32)
	if err != nil {
		return nil, errcode.ErrInternal
	}

	sessionRow := &model.Session{
		UserID:          user.ID,
		TokenHash:       sessionHash,
		TokenType:       "session",
		IdentityTokenID: &identitySession.ID,
		Platform:        platform,
		DeviceID:        input.DeviceID,
		DeviceName:      input.DeviceName,
		IPAddress:       ip,
		UserAgent:       userAgent,
		LastActiveAt:    now,
		LastHeartbeatAt: &now,
		ExpiresAt:       now.Add(15 * time.Minute),
	}
	if err := s.sessionRepo.Create(sessionRow); err != nil {
		return nil, errcode.ErrDatabase
	}

	s.sessionRepo.CreateAuditLog(&model.AuditLog{
		UserID:    user.ID,
		Action:    "auth.device_login",
		IPAddress: ip,
	})

	return &DeviceLoginResponse{
		IdentityToken:   identityPlain,
		SessionToken:    sessionPlain,
		IdentityExpires: identitySession.ExpiresAt,
		SessionTTL:      900,
		User: UserSummary{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
		Warning: "请安全保存 identity_token。session_token 为临时令牌。",
	}, nil
}

func (s *AuthService) DeviceBootstrap(identityTokenPlain string, input *DeviceBootstrapInput, platform string, ip string) (*DeviceBootstrapResponse, *errcode.AppError) {
	if _, err := hex.DecodeString(input.Nonce); err != nil {
		return nil, errcode.New(3000, "nonce 必须为 hex 编码", 400)
	}

	identityHash := token.SHA256(identityTokenPlain)
	identity, err := s.sessionRepo.FindByTokenHashAndType(identityHash, "identity")
	if err != nil || identity.ExpiresAt.Before(time.Now()) {
		s.sessionRepo.CreateAuditLog(&model.AuditLog{
			UserID:    0,
			Action:    "auth.device_bootstrap_failed",
			IPAddress: ip,
		})
		return nil, errcode.ErrSessionExpired
	}

	if identity.DeviceID != input.DeviceID {
		return nil, errcode.New(4030, "设备标识不匹配", 403)
	}

	if identity.Platform != platform {
		return nil, errcode.New(4030, "平台不匹配", 403)
	}

	nonceHash := token.SHA256(input.Nonce)
	ok, err := s.sessionRepo.CheckNonce(context.Background(), nonceHash, identity.ID)
	if err != nil {
		return nil, errcode.ErrRedis
	}
	if !ok {
		s.sessionRepo.CreateAuditLog(&model.AuditLog{
			UserID:    identity.UserID,
			Action:    "auth.nonce_replay",
			IPAddress: ip,
			Metadata:  fmt.Sprintf(`{"identity_token_id":%d}`, identity.ID),
		})
		return nil, errcode.ErrNonceReplay
	}

	s.sessionRepo.ExpireSessionsByIdentity(identity.ID)

	sessionPlain, sessionHash, err := token.Generate(32)
	if err != nil {
		return nil, errcode.ErrInternal
	}

	now := time.Now()
	session := &model.Session{
		UserID:          identity.UserID,
		TokenHash:       sessionHash,
		TokenType:       "session",
		IdentityTokenID: &identity.ID,
		Platform:        identity.Platform,
		DeviceID:        identity.DeviceID,
		DeviceName:      identity.DeviceName,
		IPAddress:       ip,
		LastActiveAt:    now,
		LastHeartbeatAt: &now,
		ExpiresAt:       now.Add(15 * time.Minute),
	}
	if err := s.sessionRepo.Create(session); err != nil {
		return nil, errcode.ErrDatabase
	}

	s.sessionRepo.CreateAuditLog(&model.AuditLog{
		UserID:    identity.UserID,
		Action:    "auth.device_bootstrap",
		IPAddress: ip,
	})

	return &DeviceBootstrapResponse{
		SessionToken: sessionPlain,
		TTLSeconds:   900,
	}, nil
}

func (s *AuthService) DeviceHeartbeat(sessionID uint64) *errcode.AppError {
	if err := s.sessionRepo.UpdateHeartbeat(sessionID); err != nil {
		return errcode.ErrDatabase
	}
	return nil
}

func (s *AuthService) DeviceShutdown(sessionID uint64, userID uint64, ip string) *errcode.AppError {
	session, err := s.sessionRepo.FindByID(sessionID)
	if err != nil || session.UserID != userID || session.TokenType != "session" {
		return errcode.New(4040, "会话不存在", 404)
	}
	if err := s.sessionRepo.Delete(sessionID); err != nil {
		return errcode.ErrDatabase
	}
	s.sessionRepo.CreateAuditLog(&model.AuditLog{
		UserID:    userID,
		Action:    "auth.device_shutdown",
		IPAddress: ip,
	})
	return nil
}

func (s *AuthService) ListDeviceIdentities(userID uint64) ([]model.Session, *errcode.AppError) {
	identities, err := s.sessionRepo.FindIdentities(userID)
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	return identities, nil
}

func (s *AuthService) RevokeIdentity(id uint64, userID uint64, ip string) *errcode.AppError {
	identity, err := s.sessionRepo.FindByID(id)
	if err != nil || identity.UserID != userID || identity.TokenType != "identity" {
		return errcode.New(4040, "身份令牌不存在", 404)
	}
	s.sessionRepo.DeleteByIdentityTokenID(id)
	if err := s.sessionRepo.Delete(id); err != nil {
		return errcode.ErrDatabase
	}
	s.sessionRepo.CreateAuditLog(&model.AuditLog{
		UserID:    userID,
		Action:    "auth.identity_revoked",
		IPAddress: ip,
		Metadata:  fmt.Sprintf(`{"identity_token_id":%d}`, id),
	})
	return nil
}

func (s *AuthService) ListSessions(userID uint64) ([]model.Session, *errcode.AppError) {
	sessions, err := s.sessionRepo.FindByUserID(userID)
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	return sessions, nil
}

func (s *AuthService) RevokeSession(id uint64, userID uint64, ip string) *errcode.AppError {
	session, err := s.sessionRepo.FindByID(id)
	if err != nil || session.UserID != userID || session.TokenType != "session" {
		return errcode.New(4040, "会话不存在", 404)
	}
	if err := s.sessionRepo.Delete(id); err != nil {
		return errcode.ErrDatabase
	}
	s.sessionRepo.CreateAuditLog(&model.AuditLog{
		UserID:       userID,
		Action:       "auth.session_revoked",
		ResourceType: "session",
		ResourceID:   id,
		IPAddress:    ip,
	})
	return nil
}

func (s *AuthService) CheckLoginRateLimit(ctx context.Context, ip string, username string) *errcode.AppError {
	ipKey := fmt.Sprintf("login:ip:%s", ip)
	ok, err := s.sessionRepo.CheckRateLimit(ctx, ipKey, 5, 15*time.Minute)
	if err != nil {
		return errcode.ErrRedis
	}
	if !ok {
		return errcode.ErrLoginRateLimited
	}

	userKey := fmt.Sprintf("login:user:%s", username)
	ok, err = s.sessionRepo.CheckRateLimit(ctx, userKey, 3, 15*time.Minute)
	if err != nil {
		return errcode.ErrRedis
	}
	if !ok {
		return errcode.ErrLoginRateLimited
	}
	return nil
}

func (s *AuthService) CheckBootstrapRateLimit(ctx context.Context, ip string) *errcode.AppError {
	key := fmt.Sprintf("bootstrap:ip:%s", ip)
	ok, err := s.sessionRepo.CheckRateLimit(ctx, key, 10, time.Minute)
	if err != nil {
		return errcode.ErrRedis
	}
	if !ok {
		return errcode.ErrBootstrapRateLimit
	}
	return nil
}

func (s *AuthService) PasswordChange(userID uint64, ip string) *errcode.AppError {
	if err := s.sessionRepo.DeleteByUserID(userID); err != nil {
		return errcode.ErrDatabase
	}
	metadata, _ := json.Marshal(map[string]interface{}{"user_id": userID})
	s.sessionRepo.CreateAuditLog(&model.AuditLog{
		UserID:    userID,
		Action:    "auth.all_sessions_revoked",
		IPAddress: ip,
		Metadata:  string(metadata),
	})
	return nil
}
