// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package errcode

type AppError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	HTTP    int         `json:"-"`
	Data    interface{} `json:"-"`
}

func (e *AppError) Error() string {
	return e.Message
}

func New(code int, msg string, http int) *AppError {
	return &AppError{Code: code, Message: msg, HTTP: http}
}

func NewWithData(code int, msg string, http int, data interface{}) *AppError {
	return &AppError{Code: code, Message: msg, HTTP: http, Data: data}
}

// System errors (1000-1999)
var (
	ErrInternal             = New(1000, "internal server error", 500)
	ErrDatabase             = New(1001, "database error", 500)
	ErrRedis                = New(1002, "cache service error", 500)
	ErrImgproxy             = New(1003, "image processing service error", 500)
	ErrRecaptchaUnavailable = New(1004, "reCAPTCHA service temporarily unavailable; please try again later", 503)
)

// Auth errors (2000-2999)
var (
	ErrInvalidCredentials = New(2001, "incorrect username or password", 401)
	ErrLoginRateLimited   = New(2008, "too many login attempts; please try again later", 429)
	ErrRecaptchaFailed    = New(2009, "reCAPTCHA validation failed; please try again", 403)
	ErrBootstrapRateLimit = New(2090, "too many bootstrap requests", 429)
	// ErrClientErrorRateLimited bounds the public crash-report sink.
	ErrClientErrorRateLimited = New(2091, "too many client error reports", 429)
)

// Auth/Permission errors (4000-4099)
var (
	ErrSessionExpired       = New(4011, "session expired; please bootstrap again", 401)
	ErrQuotaFull            = New(4012, "storage quota exhausted", 403)
	ErrUploadRateLimited    = New(4029, "too many uploads", 429)
	ErrDeviceLimitReached   = New(4031, "device limit reached", 403)
	ErrAdminWebOnly         = New(4032, "admin endpoints are restricted to Web clients", 403)
	ErrIdentityNotForAPI    = New(4033, "identity_token cannot be used for API access; use session_token", 403)
	ErrRegistrationWebOnly  = New(4034, "registration is restricted to Web clients", 403)
	ErrPrivateTokenInvalid  = New(4037, "private image token is invalid or expired", 403)
	ErrCSRFRefreshRejected  = New(4036, "CSRF token refresh request rejected", 403)
	ErrNotificationNotFound = New(4040, "notification not found", 404)
	ErrPrivateTokenRevoked  = New(4042, "private image token revoked", 404)
	ErrNonceReplay          = New(4090, "nonce already used (replay)", 409)
	ErrVersionTooLow        = New(4260, "client version too old; please update", 426)
	ErrRecipeVersion        = New(4261, "client image recipe version unsupported", 426)
	ErrUploadSessionMissing = New(4043, "upload session missing or expired", 404)
	ErrUploadConflict       = New(4091, "upload session state conflict", 409)
	ErrUploadIncomplete     = New(4092, "upload parts incomplete", 409)
	ErrUploadBusy           = New(4291, "upload concurrency exhausted; please try again later", 429)
	ErrStoragePressure      = New(5030, "server storage pressure too high; please try again later", 503)
	ErrV2UploadRequired     = New(4262, "this deployment requires V2 client-preprocessed uploads", 426)
)

// Validation errors (3000-3999)
var (
	ErrFileTooLarge      = New(3002, "file exceeds the size limit", 413)
	ErrUnsupportedType   = New(3003, "unsupported file type", 415)
	ErrFileCountExceeded = New(3004, "too many files", 400)
	ErrDimensionExceeded = New(3010, "image dimensions exceed the limit", 400)
)
