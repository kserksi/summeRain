# 后端调整方案（仅方案，不含实现）

> [!WARNING]
> **已归档的设计记录。** 本文记录 2026 年 6 月 18 日、V2 实施前的一份方案。
> 文中描述的令牌与 CAPTCHA 工作后来已经实现或被替代。当前行为应以现有源码、
> API 参考和发布说明为准。

- **日期：** 2026-06-18
- **状态：** 方案；撰写时待实现
- **范围：** 为支持 `docs/design/frontend-architecture/` 中定义的两项既定前端规则，
  列出 Go 后端（`backend/`）需要配套调整的内容。
- **原则：** 本文仅描述改什么、在哪里改以及为什么改，不包含实现代码。

---

## 一、私密图片：统一令牌模型

### 原有状态（不符合目标）

- `internal/service/image_service.go`：
  `GenerateAccessToken/ListTokens/RevokeAccessToken` 实现的是
  **多令牌并带过期时间**的模型。
- `internal/handler/public_handler.go::ServeImage`：私密图片请求缺少令牌时返回
  `4010`（401），**没有 owner/admin 会话旁路**；使用已吊销令牌访问也没有
  `404` 语义。
- `internal/handler/image_handler.go::Get`：`Image` 响应不包含当前令牌。

### 前端设计采用的目标规则

- 每张私密图片只有**一个统一令牌**，且令牌的字符内容**不可变**。
- **TTL：** 默认 1h（3,600,000 ms），由后端按**毫秒**校验。owner/admin 签发时
  可选，有效范围为 **600,000 ms 至 259,200,000 ms**。
- **吊销：** 仅 owner/admin 可执行。吊销后**不得自动重签**；owner/admin 必须再次
  明确申请新令牌。在此之前，该图片对第三方永久不可分享。
- **owner/admin 通过同源会话直接访问：** 自动授权，访问 `/i/` 无需令牌；API 返回
  当前统一令牌用于展示。
- 第三方：令牌缺失、错误或过期 -> **403**；令牌已吊销 -> **404**。

### 调整项

**1）模型 / 仓储**（`model/image_access_token.go`、
`repository/image_access_token_repo.go`）

- 约束每张图片最多一个活跃令牌；活跃令牌指尚未吊销且未过期。令牌字段包含字符串、
  `expires_at`、`revoked_at` 和 `status`。
- 新增或调整 `FindActiveByImageID(imageID)`、`Issue(imageID, ttlMs)`（若已有活跃
  令牌，先使其失效再签发，以保持只有一个统一活跃令牌）、`Revoke(imageID)` 以及
  `Validate(imageID, token)`。`Validate` 返回 `valid`、`expired`、`revoked` 或
  `not_found`。

**2）服务层**（`service/image_service.go`）

- 使用 `IssueAccessToken(userID, imageID, ttlMs)`（包含 owner/admin 校验）和
  `RevokeAccessToken(userID, imageID)` 取代多令牌方法组。
- TTL 由 handler 传入；服务层将其限制在 `[600000, 259200000]`，默认值为
  `3600000`。
- 移除所有自动重签路径；签发必须是显式动作。

**3）图片直链**（`handler/public_handler.go::ServeImage`）

- 私密图片分支首先进行**可选会话解析**。读取 `__Host-session_token` / Bearer，复用
  `middleware` 的查询逻辑，但不强制鉴权；如果会话属于 owner 或 admin，则直接放行。
- 否则校验令牌，并按 `Validate` 结果分流：`valid` 时带 `no-store` 放行；
  `expired` / `not_found` 返回 **403**；`revoked` 返回 **404**。
- 缺少令牌同样映射为 **403**，替代原有 `4010` 行为。

**4）图片详情**（`handler/image_handler.go::Get`）

- 请求者为 owner/admin 时，在响应中加入 `access_token`（当前统一令牌的明文，仅由
  此端点返回）和 `token_expires_at`。

**5）错误码**（`internal/pkg/errcode/errcode.go`）

- 新增 `4037 私密图片令牌无效或已过期`（403）。
- 新增 `4042 私密图片令牌已吊销`（404）。
- `/i/` 缺令牌场景使用 `4037`，取代原有 `4010`。

**6）配置**（`model/system_config.go` + `/admin/configs`）

- 新增 `private_token_ttl_default_ms`，默认 `3600000`。管理员可配置该值，服务层会
  再次将其限制到固定边界内。
- 将 `600000` / `259200000` 上下限保留为**代码常量**，不放入配置，避免配置变更
  破坏规则。

**7）更新 `docs/API.md`：** 记录 `/i/:link` 的私密访问语义（owner/admin 旁路与
403/404），将 `POST /images/:id/tokens` 的入参改为 `ttl_ms`，并在
`GET /images/:id` 响应中加入 `access_token`。

---

## 二、CAPTCHA：三个可插拔 Provider

### 原有状态（不符合目标）

- 只有 `internal/service/recaptcha.go`（reCAPTCHA v3），通过
  `recaptchaVerifier` 接口注入 `auth_service`。
- `config.go::RecaptchaConfig` 仅包含 reCAPTCHA 字段；
  `public_config_service.go` 仅返回 `recaptcha_enabled` /
  `recaptcha_site_key`。
- `LoginInput` / `RegisterInput` 仅包含 `recaptcha_token` /
  `recaptcha_action`。

### 目标规则

- `captcha_provider` 取值为 `none`（默认）、`recaptcha`、`turnstile` 或
  `geetest_v4`，管理员通过 `/admin/configs` 配置。
- 使用 `none` 时，后端**不执行校验**，前端也不加载脚本。
- 登录与注册接收所选 provider 对应的载荷，后端调用该 provider 的校验器。

### 调整项

**1）配置**（`config.go`）

- 以 `CaptchaConfig` 取代 `RecaptchaConfig`。新增 `Provider string`、Turnstile
  （`SiteKey` / `Secret`）和 GeeTest（`CaptchaID` / `CaptchaKey`），并保留
  reCAPTCHA 字段。
- 默认 `Provider=none`。
- 新增环境变量 `CAPTCHA_PROVIDER`、
  `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET` 以及
  `GEETEST_CAPTCHA_ID` / `GEETEST_CAPTCHA_KEY`，同时兼容旧的 `RECAPTCHA_*`
  变量。

**2）校验抽象与实现**（`service/`）

- 抽取 `CaptchaVerifier` 接口：
  `Verify(ctx, payload, remoteIP, requestHost) *errcode.AppError`。
- 让 `recaptcha.go` 实现该接口，并保持原有行为。
- 新增 `turnstile.go`：向
  `https://challenges.cloudflare.com/turnstile/v0/siteverify` POST 表单字段
  `secret` + `response`（+ `remoteip`），并检查 `success`。超时和失败按
  `FailClosed` 处理。
- 新增 `geetest_v4.go`：使用 `lot_number` + `captcha_output` + `pass_token` +
  `gen_time` + `captcha_id`，以 `captcha_key` 执行 HMAC-SHA256 签名，POST 至
  `https://gcaptcha4.geetest.com/verify`，并要求 `result=="success"`。
- 新增 `NewCaptchaVerifier(cfg)` 工厂，按所选 provider 返回对应实现；`none` 返回 nil。

**3）Auth 服务 / Handler**（`service/auth_service.go`、
`handler/auth_handler.go`）

- 注入 `CaptchaVerifier`，取代 `recaptchaVerifier`。
- 扩展 `LoginInput` / `RegisterInput` 为通用 CAPTCHA 载荷：`captcha_provider`
  加 `{recaptcha_token, recaptcha_action}`、`{turnstile_token}` 或
  `{lot_number, captcha_output, pass_token, gen_time}`。建议使用按 provider 选择的
  嵌套 `captcha` 对象。
- `Login` / `Register` 按**已配置的 provider**校验对应字段；`provider=none` 时
  跳过校验。前端传入的 provider 与配置不一致时必须拒绝。

**4）公共配置**（`service/public_config_service.go`、
`handler/public_handler.go::GetConfig`）

- 返回 `captcha_provider` 以及该 provider 的客户端公钥：reCAPTCHA/Turnstile 使用
  `site_key`，GeeTest 使用 `captcha_id`。`provider=none` 时返回空公钥。

**5）限流与错误**

- 复用现有登录限流。保持 `2009`（校验失败）与 `1004`（服务不可用）不变。
- 将校验失败也计入限流，以降低暴力尝试风险。

**6）更新 `docs/API.md`：** 记录 `/public/config` 新字段、登录/注册的多形态 CAPTCHA
载荷，以及 `/admin/configs` 中新增的 CAPTCHA 配置项。

---

## 三、数据库迁移（AutoMigrate 处理结构变更，但仍需关注数据）

- `image_access_tokens`：新增 `revoked_at`，并将唯一约束调整为每张图片一个活跃令牌。
  存量多令牌数据需要清理脚本：保留最新的一个活跃令牌，或吊销全部令牌。
- `system_configs`：新增 `private_token_ttl_default_ms`、`captcha_provider` 以及各
  provider 的 key 记录。
- 除 `cmd/server/main.go::AutoMigrate` 外，还需提供一份**数据迁移脚本**，用于清理
  存量令牌并写入默认配置。

## 四、不在本文范围

- 实现代码、单元测试和 CI 调整；这些内容原计划在方案确认后另行排期。
- 前端实现；相关内容已经在 `frontend-architecture/` 各部分定义。

## 五、风险与取舍

- **私密图片 owner/admin 旁路：** 公开路由 `/i/:link` 需要可选会话解析。不要让该
  路由强制鉴权，否则第三方令牌访问会失败。应尝试解析会话，并且仅对 owner/admin
  绕过令牌校验。
- **毫秒级 TTL 校验：** Go 内部使用纳秒。存储和比较必须统一采用 `UnixMilli`，
  避免精度误差。
- **GeeTest 签名算法：** 必须严格遵循其官方 v4 文档，包括签名字符串的拼接顺序和
  HMAC-SHA256；使用前应对照官方 demo 验证实现。
- **中国大陆可用性：** 即使使用 `recaptcha.net` 镜像，reCAPTCHA 在中国大陆仍不稳定。
  面向主要受众时，应优先选择 `turnstile` 或 `geetest_v4`。
