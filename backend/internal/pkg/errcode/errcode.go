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
	ErrInternal             = New(1000, "内部服务器错误", 500)
	ErrDatabase             = New(1001, "数据库错误", 500)
	ErrRedis                = New(1002, "缓存服务错误", 500)
	ErrImgproxy             = New(1003, "图片处理服务错误", 500)
	ErrRecaptchaUnavailable = New(1004, "reCAPTCHA 服务暂时不可用，请稍后重试", 503)
)

// Auth errors (2000-2999)
var (
	ErrInvalidCredentials = New(2001, "用户名或密码错误", 401)
	ErrLoginRateLimited   = New(2008, "登录尝试过于频繁，请稍后重试", 429)
	ErrRecaptchaFailed    = New(2009, "reCAPTCHA 校验失败，请重试", 403)
	ErrBootstrapRateLimit = New(2090, "Bootstrap 请求过于频繁", 429)
)

// Auth/Permission errors (4000-4099)
var (
	ErrSessionExpired       = New(4011, "会话已过期，请重新 bootstrap", 401)
	ErrQuotaFull            = New(4012, "存储配额已满", 403)
	ErrUploadRateLimited    = New(4029, "上传过于频繁", 429)
	ErrDeviceLimitReached   = New(4031, "设备数量已达上限", 403)
	ErrAdminWebOnly         = New(4032, "管理接口仅限 Web 端访问", 403)
	ErrIdentityNotForAPI    = New(4033, "identity_token 不可用于 API 访问，请使用 session_token", 403)
	ErrRegistrationWebOnly  = New(4034, "注册仅限 Web 端", 403)
	ErrPrivateTokenInvalid  = New(4037, "私密图片令牌无效或已过期", 403)
	ErrNotificationNotFound = New(4040, "通知不存在", 404)
	ErrPrivateTokenRevoked  = New(4042, "私密图片令牌已吊销", 404)
	ErrNonceReplay          = New(4090, "Nonce 已被使用（重放攻击）", 409)
	ErrVersionTooLow        = New(4260, "客户端版本过低，请更新", 426)
)

// Validation errors (3000-3999)
var (
	ErrFileTooLarge      = New(3002, "文件大小超出限制", 413)
	ErrUnsupportedType   = New(3003, "不支持的文件类型", 415)
	ErrFileCountExceeded = New(3004, "文件数量超出限制", 400)
	ErrDimensionExceeded = New(3010, "图片尺寸超出限制", 400)
)
