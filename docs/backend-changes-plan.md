# 后端调整方案（仅方案，不含实现）

- **日期**：2026-06-18
- **状态**：方案，待实现
- **范围**：为支持前端设计（见 `docs/superpowers/specs/frontend-architecture/`）的两项既定规则，列出 Go 后端（`backend/`）需配套调整的 delta。
- **原则**：本文仅描述"改什么、在哪、为什么"，不写实现代码。

---

## 一、私密图片：统一令牌模型

### 现状（与目标不符）
- `internal/service/image_service.go`：`GenerateAccessToken/ListTokens/RevokeAccessToken` 为**多令牌 + 过期**模型
- `internal/handler/public_handler.go::ServeImage`：私密图缺令牌返 `4010`(401)，**无 owner/admin 会话旁路**；吊销令牌访问无 `404` 语义
- `internal/handler/image_handler.go::Get`：`Image` 响应不含当前令牌

### 目标规则（前端已据此设计）
- 每张私密图**一把统一令牌**；令牌**字符不可变**
- **TTL**：默认 1h（3,600,000 ms），后端**毫秒**校验；owner/admin 生成时可选，范围 **600,000 ms ～ 259,200,000 ms**
- **吊销**：仅 owner/admin；**吊销后不自动重签**，须 owner/admin 再次手动申请；在此之前对第三方永久不可分享
- **owner/admin（同源会话）直接查看**：自动授权，`/i/` 无需令牌；API 返回当前统一令牌供展示
- 第三方：未带/错令牌/过期 → **403**；已吊销令牌 → **404**

### 调整项

**1) 模型 / 仓储**（`model/image_access_token.go`、`repository/image_access_token_repo.go`）
- 约束"每图至多一条有效令牌"（唯一活跃：未吊销且未过期）；令牌字段（字符串、`expires_at`、`revoked_at`、`status`）
- 新增/调整方法：`FindActiveByImageID(imageID)`、`Issue(imageID, ttlMs)`（若存在活跃令牌则先失效再签发，保持"统一=单活跃"）、`Revoke(imageID)`、`Validate(imageID, token)` 返回枚举（`valid` / `expired` / `revoked` / `not_found`）

**2) 服务层**（`service/image_service.go`）
- 用 `IssueAccessToken(userID, imageID, ttlMs)`（owner/admin 校验）+ `RevokeAccessToken(userID, imageID)` 取代多令牌套件
- TTL 入参由 handler 传入，服务层 clamp 到 `[600000, 259200000]`，缺省 `3600000`
- 移除任何"自动重签"路径；签发为显式动作

**3) 图片直链**（`handler/public_handler.go::ServeImage`）
- 私密图分支先做**可选会话解析**（读 `__Host-session_token` / Bearer，复用 `middleware` 的查询逻辑，但不强制）：若为 owner 或 admin → 直接放行
- 否则进入令牌校验：`Validate` 结果分流 → `valid` 放行（`no-store`）；`expired`/`not_found` → **403**；`revoked` → **404**
- 缺令牌也归 **403**（区别于现 `4010`）

**4) 图片详情**（`handler/image_handler.go::Get`）
- 当请求者为 owner/admin 时，响应附带 `access_token`（当前统一令牌明文，仅此接口返回）与 `token_expires_at`

**5) 错误码**（`internal/pkg/errcode/errcode.go`）
- 新增 `4037 私密图片令牌无效或已过期`(403)
- 新增 `4042 私密图片令牌已吊销`(404)
- （原 `4010` 在 `/i/` 缺令牌场景被 `4037` 取代）

**6) 配置**（`model/system_config.go` + `/admin/configs`）
- 新增 `private_token_ttl_default_ms`（默认 3600000，admin 可调，服务层再 clamp 到固定上下限）
- 上下限 `600000`/`259200000` 为**代码常量**（不放配置，避免被改破规则）

**7) `docs/API.md` 同步**：`/i/:link` 私密访问语义（owner/admin 旁路、403/404）、`POST /images/:id/tokens` 入参改 `ttl_ms`、`GET /images/:id` 返回 `access_token` 字段。

---

## 二、人机验证：可插拔三 Provider

### 现状（与目标不符）
- 仅 `internal/service/recaptcha.go`（reCAPTCHA v3），`auth_service` 经 `recaptchaVerifier` 接口注入
- `config.go::RecaptchaConfig` 仅 reCAPTCHA 字段；`public_config_service.go` 仅返回 `recaptcha_enabled`/`recaptcha_site_key`
- `LoginInput`/`RegisterInput` 仅 `recaptcha_token`/`recaptcha_action`

### 目标规则
- `captcha_provider` ∈ `none`(默认) | `recaptcha` | `turnstile` | `geetest_v4`，管理员经 `/admin/configs` 配置
- `none` 时**不校验、前端不加载脚本**
- 登录/注册接收**该 provider 对应载荷**；后端按 provider 调对应校验

### 调整项

**1) 配置**（`config.go`）
- `RecaptchaConfig` → `CaptchaConfig`：增 `Provider string`、Turnstile（`SiteKey`/`Secret`）、GeeTest（`CaptchaID`/`CaptchaKey`）；保留 reCAPTCHA 字段
- 默认 `Provider=none`
- 环境变量：`CAPTCHA_PROVIDER`、`TURNSTILE_SITE_KEY`/`TURNSTILE_SECRET`、`GEETEST_CAPTCHA_ID`/`GEETEST_CAPTCHA_KEY`（兼容旧 `RECAPTCHA_*`）

**2) 校验抽象与实现**（`service/`）
- 抽出 `CaptchaVerifier` 接口：`Verify(ctx, payload, remoteIP, requestHost) *errcode.AppError`
- `recaptcha.go` 实现该接口（保持现状逻辑）
- 新增 `turnstile.go`：POST `https://challenges.cloudflare.com/turnstile/v0/siteverify`，表单 `secret`+`response`(+`remoteip`)，判 `success`；超时/失败按 `FailClosed`
- 新增 `geetest_v4.go`：用 `lot_number`+`captcha_output`+`pass_token`+`gen_time` + `captcha_id`，`captcha_key` 做 HMAC-SHA256 签名，POST `https://gcaptcha4.geetest.com/verify`，判 `result=="success"`
- 工厂 `NewCaptchaVerifier(cfg)` 按 `Provider` 返回对应实现或 nil（none）

**3) Auth 服务 / Handler**（`service/auth_service.go`、`handler/auth_handler.go`）
- 注入 `CaptchaVerifier` 替代 `recaptchaVerifier`
- `LoginInput`/`RegisterInput` 扩展为通用 captcha 载荷：`captcha_provider` + `{recaptcha_token, recaptcha_action}` 或 `{turnstile_token}` 或 `{lot_number, captcha_output, pass_token, gen_time}`（建议用嵌套 `captcha` 对象，按 provider 取值）
- `Login`/`Register` 按**配置的 provider** 校验对应字段（provider=none 跳过）；前端传错 provider 须拒

**4) 公共配置**（`service/public_config_service.go`、`handler/public_handler.go::GetConfig`）
- 返回 `captcha_provider` + 该 provider 客户端公钥（recaptcha/turnstile→`site_key`；geetest→`captcha_id`）；provider=none 时公钥为空

**5) 限流与错误**
- 复用现有登录限流；`2009`(校验失败)/`1004`(服务不可用) 不变
- 速率：建议对"校验失败"也计入限流计数，防爆破

**6) `docs/API.md` 同步**：`/public/config` 新字段、登录/注册 captcha 载荷多形态、`/admin/configs` 新增 captcha 配置项。

---

## 三、数据库迁移（AutoMigrate 会自动处理结构变更，需注意数据）

- `image_access_tokens`：加 `revoked_at`、调整唯一约束（per image 单活跃）；存量多令牌数据需清理脚本（保留最新活跃一条/全部吊销）
- `system_configs`：新增 `private_token_ttl_default_ms`、`captcha_provider`、各 provider key 记录
- 迁移须在 `cmd/server/main.go::AutoMigrate` 之外补一份**数据迁移脚本**（清理存量令牌、写入默认配置）

## 四、不在本文范围
- 实现代码、单元测试、CI 调整（待方案确认后另行排期）
- 前端实现（已在 `frontend-architecture/` 各板块定义）

## 五、风险与取舍
- **私密图 owner/admin 旁路**：需在公开路由 `/i/:link` 上做"可选会话解析"，注意不要把该路由纳入强制鉴权（否则第三方带 token 访问会失败）——仅"尝试解析，成功且属主/管理员才旁路"
- **TTL 毫秒校验**：Go 内部用 ns，存储/比较统一以 `UnixMilli` 口径，避免精度误差
- **极验签名算法**须严格按其官方 v4 文档（签名串拼接顺序 + HMAC-SHA256），实现前以官方 demo 校验
- **中国可用性**：reCAPTCHA 即便 `recaptcha.net` 镜像在大陆仍不稳；主受众场景建议 provider 选 `turnstile` 或 `geetest_v4`
