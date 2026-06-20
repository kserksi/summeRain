# ImgCloud 后端 API 文档

> 本文档基于 `backend/` 源码（Go + Gin + GORM/MySQL + Redis + imgproxy）逐文件核对生成，作为前后端对接契约。
>
> - **Base URL**：`/api/v1`
> - **默认端口**：`8080`（`SERVER_PORT`）
> - **图片直链**：`GET /i/:link`（不在 `/api/v1` 下）

---

## 目录

1. [通用约定](#1-通用约定)
2. [鉴权与 CSRF](#2-鉴权与-csrf)
3. [认证 Auth](#3-认证-auth)
4. [图片 Images](#4-图片-images)
5. [图片访问令牌 Access Tokens](#5-图片访问令牌-access-tokens)
6. [用户 User](#6-用户-user)
7. [通知 Notifications](#7-通知-notifications)
8. [后台 Admin](#8-后台-admin)
9. [公共 Public](#9-公共-public)
10. [错误码参考](#10-错误码参考)
11. [前端对接说明](#11-前端对接说明)

---

## 1. 通用约定

### 统一响应体

所有接口返回如下结构（`response.Response`）：

```json
{
  "code": 0,
  "message": "success",
  "data": { },
  "request_id": "可选，仅错误时返回"
}
```

| 字段 | 说明 |
|---|---|
| `code` | `0` = 成功；非 `0` = 业务错误码（见[第 10 节](#10-错误码参考)） |
| `message` | 文案 |
| `data` | 业务数据，成功时必有；错误时省略（除非带附加数据） |
| `request_id` | 请求追踪 ID，**仅错误响应返回** |

- HTTP 状态码与业务语义一致（200 成功 / 201 创建 / 400 参数 / 401 未认证 / 403 无权 / 404 不存在 / 413 文件过大 / 429 限流 / 500 服务端错误）。

### 数据模型字段命名

后端 JSON 一律 **snake_case**，例如 `storage_used`、`created_at`、`view_count`、`unique_link`、`user_id`。

### 运行依赖

服务启动需 MySQL + Redis 可连通（`main.go` 启动时 `Ping` 失败会 `Fatal`）。图片缩略/格式转换依赖 imgproxy。基础设施可用 `docker-compose.yml` 一键启动。

健康检查：
- `GET /health` → `{"status":"ok"}`
- `GET /ready` → 校验 DB + Redis 连通性，503 表示不可用
- `GET /metrics` → Prometheus 指标

---

## 2. 鉴权与 CSRF

系统支持两种鉴权方式：

### 2.1 Web 端 —— Cookie 鉴权（前端对接用此方式）

登录成功后服务端设置两个 Cookie：

| Cookie | 用途 | HttpOnly | 有效期 |
|---|---|---|---|
| `__Host-session_token` | 会话凭证 | ✅ 是 | 30 天（2592000 秒） |
| `__Host-csrf_token` | CSRF 防护 | ❌ 否（前端可读） | 30 天 |

- Cookie 为 `__Host-` 前缀，要求 **HTTPS + 同站**，`SameSite=Strict`、`Secure`。
- 前端只需 `credentials: 'include'` 发请求即可，浏览器自动带 cookie。

### 2.2 CSRF 保护（关键）

**所有写操作（POST/PATCH/DELETE）在使用 Cookie 鉴权时，必须携带请求头：**

```
X-CSRF-Token: <__Host-csrf_token cookie 的值>
```

规则（`middleware/csrf.go`）：
- GET / HEAD / OPTIONS 不校验。
- 使用 `Authorization: Bearer <token>` 的请求**跳过** CSRF 校验（设备端）。
- Cookie 鉴权下：缺头返回 `4035 CSRF token required`；值不匹配返回 `4036 Invalid CSRF token`。

> 前端实现：从 `__Host-csrf_token` cookie 读取值，所有非 GET 请求加 `X-CSRF-Token` 头。

### 2.3 设备端 —— Bearer Token 鉴权（客户端用，Web 不涉及）

`Authorization: Bearer <session_token>`，并配合 `X-Platform: android|windows`、`X-Client-Version` 头。涉及 device-login / bootstrap / heartbeat 等流程，Web 端对接可忽略。

---

## 3. 认证 Auth

### 3.1 注册

`POST /api/v1/auth/register`

> ⚠️ 仅限 Web 端：若带 `X-Platform` 且非 `web`，返回 `4034 注册仅限 Web 端`。受登录限流中间件保护。

**请求体**
```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "至少8位",
  "captcha": {
    "provider": "recaptcha|turnstile|geetest_v4",
    "token": "recaptcha / turnstile token",
    "action": "register",
    "lot_number": "极验 v4",
    "captcha_output": "极验 v4",
    "pass_token": "极验 v4",
    "gen_time": "极验 v4"
  }
}
```
校验：`username` 3-50，`email` 合法邮箱 ≤100，`password` 8-72。
> `captcha` 载荷形态由当前 `captcha_provider` 决定（见 [9.1](#91-公共配置)）；`provider=none` 时可不传。recaptcha 需 `token`+`action`；turnstile 需 `token`；geetest_v4 需 `lot_number`/`captcha_output`/`pass_token`/`gen_time`。前端传错 provider 会被拒。

**成功 201**
```json
{
  "code": 0,
  "message": "created",
  "data": { "id": 12, "username": "alice", "email": "alice@example.com" }
}
```
> 注册**不会**自动登录，需另行调用登录接口。

### 3.2 登录

`POST /api/v1/auth/login`

**请求体**
```json
{ "username": "alice", "password": "xxxxxx",
  "captcha": { "provider": "recaptcha", "token": "...", "action": "login" } }
```
> `username` 可为用户名。受 IP + 用户名限流，频繁失败返回 `2008`。`captcha` 载荷同[注册](#31-注册)。

**成功 200** —— 同时 Set-Cookie（`__Host-session_token`、`__Host-csrf_token`）
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "user": { "id": 12, "username": "alice", "role": "user" }
  }
}
```
> `UserSummary` 仅含 `id`、`username`、`role`。

### 3.3 当前用户

`GET /api/v1/auth/me` 🔒

**成功 200**
```json
{
  "code": 0,
  "data": {
    "id": 12, "username": "alice", "email": "alice@example.com",
    "role": "user", "status": "active", "avatar_url": null,
    "storage_used": 1048576, "storage_quota": 1073741824,
    "image_count": 3, "created_at": "2026-01-01T00:00:00Z", "updated_at": "..."
  }
}
```
> 返回完整 `model.User`（`password_hash` 已 `json:"-"` 隐藏）。

### 3.4 登出

`POST /api/v1/auth/logout` 🔒 CSRF

清除两个 Cookie，服务端删除会话。返回 `{"code":0,"data":null}`。

### 3.5 设备端接口（Web 对接可忽略）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/auth/device-login` | 设备登录，返回 identity_token |
| POST | `/auth/device-bootstrap` | 用 identity_token 换 session_token |
| POST | `/auth/device-heartbeat` | 心跳保活 |
| DELETE | `/auth/device-shutdown` | 终止设备会话 |
| GET | `/auth/device-identities` | 列出设备身份 |
| DELETE | `/auth/device-identities/:id` | 吊销身份 CSRF |
| GET | `/auth/sessions` | 列出活跃会话 |
| DELETE | `/auth/sessions/:id` | 吊销会话 CSRF |

---

## 4. 图片 Images

> 所有接口 🔒 需登录。图片归属用户：`GET /images/:id` 校验 `image.user_id == 当前用户`，否则 `4031 无权访问`。

### 4.1 图片列表（游标分页）

`GET /api/v1/images/`

**Query 参数**

| 参数 | 默认 | 说明 |
|---|---|---|
| `cursor` | 空 | 分页游标（取上页 `next_cursor`） |
| `limit` | `20` | 每页数量 |
| `sort` | `-created_at` | 排序，`-` 前缀为降序 |
| `visibility` | 空 | 过滤：`public` / `private` |
| `search` | 空 | 关键词搜索 |

**成功 200**
```json
{
  "code": 0,
  "data": {
    "images": [ { "id": 1, "user_id": 12, "unique_link": "abc...", "filename": "a.jpg",
                  "title": "", "visibility": "public", "view_count": 5,
                  "width": 800, "height": 600, "file_size": 123456,
                  "created_at": "2026-06-18T10:00:00Z", "updated_at": "..." } ],
    "next_cursor": "下一页游标，无更多则为空字符串",
    "has_more": true
  }
}
```

`Image` 字段（`model/image.go`）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | uint64 | 图片 ID |
| `user_id` | uint64 | 所有者 |
| `image_file_id` | uint64 | 底层文件记录 |
| `unique_link` | string | 唯一短链，拼出图地址 `/i/<unique_link>` |
| `title` / `filename` / `description` | string | 元信息 |
| `visibility` | string | `public` / `private` |
| `view_count` | uint64 | 浏览量 |
| `width` / `height` | int | 尺寸 |
| `file_size` | int64 | 字节 |
| `created_at` / `updated_at` | time | 时间 |

> ⚠️ **后端 Image 模型无 `category` / `tags` 字段**。前端若需分类标签，需后端扩展或前端本地维护。

### 4.2 上传图片

`POST /api/v1/images/` 🔒 CSRF

**请求**：`multipart/form-data`
- `images`：文件字段（可多个，最多 20 个）
- `visibility`：表单字段，`public` 或 `private`（可选）

```bash
curl -X POST /api/v1/images/ \
  -H "X-CSRF-Token: <csrf>" \
  -F "images=@photo.jpg" \
  -F "visibility=public"
```

**成功 200**（`UploadResponse`）
```json
{
  "code": 0,
  "data": {
    "upload_id": 99,
    "total": 1,
    "results": [
      {
        "filename": "photo.jpg",
        "success": true,
        "image_id": 50,
        "unique_link": "xY9k2...",
        "thumbnail_url": "/i/xY9k2....webp",
        "processed_url": "/i/xY9k2....jpg",
        "error": "",
        "error_code": 0
      }
    ],
    "storage_used": 2000000,
    "storage_quota": 1073741824,
    "storage_percent": 0.18
  }
}
```

> 每个 `UploadResult` 独立标记成功/失败。配额满返回 `4012 存储配额已满`；文件过大 `3002`；类型不支持 `3003`；数量超限 `3004`。

### 4.3 图片详情

`GET /api/v1/images/:id` 🔒（owner 或 admin）

返回单个 `Image`（同列表项结构）。**私密图**且请求者为 owner/admin 时，响应额外附带当前统一令牌：

```json
{ "code": 0, "data": {
    "...Image 字段...": "...",
    "access_token": "当前统一令牌明文（无活跃令牌则不带）",
    "token_expires_at": "2026-06-18T11:00:00Z"
}}
```

### 4.4 删除图片

`DELETE /api/v1/images/:id` 🔒 CSRF

**成功 200**（`DeleteResult`）
```json
{ "code": 0, "data": {
    "image_id": 50,
    "storage_freed_bytes": 123456,
    "storage_used": 1876544,
    "storage_quota": 1073741824
}}
```

### 4.5 切换可见性

`PATCH /api/v1/images/:id/visibility` 🔒 CSRF

**请求体**：`{ "visibility": "public" }`（必须 `public` 或 `private`）

**成功 200**（`VisibilityResult`）
```json
{ "code": 0, "data": {
    "image_id": 50,
    "visibility": "private",
    "tokens_revoked": 2,
    "warning": "改为私密后已撤销 N 个外链令牌"
}}
```

---

## 5. 私密图片访问令牌（统一令牌模型）

每张私密图**至多一把统一令牌**；令牌字符**不可变**，吊销后须 owner/admin 再次手动申请（在此之前对第三方永久不可分享）。

### 5.1 签发 / 重签令牌

`POST /api/v1/images/:id/tokens` 🔒 CSRF（owner / admin）

**请求体**：`{ "ttl_ms": 3600000 }`（可选；缺省取系统配置 `private_token_ttl_default_ms`，clamp 到 `[600000, 259200000]` ms，即 10 分钟 ~ 3 天）

> 签发会**自动作废**该图原有活跃令牌（保持"单活跃"），返回一把全新明文令牌。

**成功 200**（`AccessTokenResult`）
```json
{ "code": 0, "data": {
    "token_id": 7,
    "token": "明文令牌",
    "expires_at": "2026-06-18T11:00:00Z",
    "warning": "请立即保存此令牌。令牌字符不可变，吊销后需重新申请。"
}}
```

### 5.2 撤销令牌

`DELETE /api/v1/images/:id/tokens` 🔒 CSRF（owner / admin）→ `{ "image_id": 7, "revoked": true }`

> `revoked=false` 表示原本就没有活跃令牌。撤销后该图对第三方永久不可分享，直至重新签发。

### 5.3 当前令牌（随详情返回）

无单独列表接口；owner/admin 调 `GET /api/v1/images/:id` 时，私密图响应附带 `access_token` 与 `token_expires_at`（无活跃令牌则不带）。详见 [4.3](#43-图片详情)。

### 5.4 上传队列状态

`GET /api/v1/upload/queue/:id` 🔒 —— 查询异步上传队列记录（`upload_queue`）。

---

## 6. 用户 User

### 6.1 个人资料

`GET /api/v1/user/profile` 🔒

**成功 200**（`UserProfile`，比 `model.User` 多算 `storage_percent`）
```json
{
  "code": 0,
  "data": {
    "id": 12, "username": "alice", "email": "alice@example.com",
    "role": "user", "status": "active", "avatar_url": null,
    "storage_used": 1048576, "storage_quota": 1073741824,
    "storage_percent": 0.09,
    "image_count": 3,
    "created_at": "2026-01-01T00:00:00Z"
  }
}
```

### 6.2 修改密码

`PATCH /api/v1/user/password` 🔒 CSRF

**请求体**：`{ "old_password": "旧密码", "new_password": "新密码至少8位" }`

> 改密成功后会**清除该用户所有会话**（强制重新登录），并发通知。旧密码错误返回 `2001`。

---

## 7. 通知 Notifications

> 🔒 全部需登录；写操作需 CSRF。`model/notification.go`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/notifications/` | 通知列表 |
| PATCH | `/notifications/:id/read` | 标记已读 CSRF |
| PATCH | `/notifications/batch-read` | 全部已读 CSRF |
| DELETE | `/notifications/:id` | 删除一条 CSRF |
| DELETE | `/notifications/clear` | 清空 CSRF |

`Notification` 字段：`id`、`user_id`、`type`、`title`、`message`、`is_read`、`metadata`(JSON 字符串)、`created_at`。

---

## 8. 后台 Admin

> 🔒 需登录 **且** `platform == "web"` **且** `role == "admin"`（三重校验，`RequireAdmin`）。整个 admin 路由组都挂了 CSRF。

### 8.1 用户列表（分页）

`GET /api/v1/admin/users?page=1&page_size=20`

**成功 200**（`UserListResult`）
```json
{ "code": 0, "data": {
    "items": [ { /* model.User 全字段 */ } ],
    "total": 42, "page": 1
}}
```
> `page_size` 上限 100。`items` 按 `id ASC`。每项含 `storage_used`/`storage_quota`/`image_count`/`status` 等。

### 8.2 修改用户状态

`PATCH /api/v1/admin/users/:id/status` CSRF

**请求体**：`{ "status": "active" }`

`status` 仅允许三值之一：`active` / `suspended` / `pending`。

> 设为 `suspended` 时：服务端自动删除该用户所有会话（强制下线）并发"账号已被禁用"通知。注意：后端无 `banned`，对应概念是 `suspended`。用户不存在返回 `4041`。

### 8.3 系统统计

`GET /api/v1/admin/stats`

**成功 200**（`SystemStats`）
```json
{ "code": 0, "data": {
    "total_users": 42,
    "total_images": 1024,
    "storage_used": 5368709120,
    "active_users": 38,
    "total_sessions": 17
}}
```

### 8.4 系统配置

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/admin/configs` | 取全部配置项（`config_key`/`config_value`/...） |
| PATCH | `/admin/configs` | 批量更新，请求体 `{ "items": [ { "key": "...", "value": "..." } ] }` |

---

## 9. 公共 Public

### 9.1 公共配置

`GET /api/v1/public/config` （无需登录）

```json
{ "code": 0, "data": {
    "captcha_provider": "none",
    "captcha_site_key": ""
}}
```
| 字段 | 说明 |
|---|---|
| `captcha_provider` | `none` / `recaptcha` / `turnstile` / `geetest_v4`；可被 admin 经 `/admin/configs`（键 `captcha_provider`）覆盖 |
| `captcha_site_key` | 当前 provider 的客户端公钥（recaptcha/turnstile→site key；geetest→captcha_id；`none`→空） |

### 9.2 图片直链服务

`GET /i/:link`

- `link` 形如 `<unique_link>` 或 `<unique_link>.<ext>`（ext ∈ webp/avif/jpg/jpeg/png/gif）。
- 无扩展名 → 返回原图（原始 mime）；带扩展名 → 经 imgproxy 转码，支持 `w`/`h`（≤4096）/`q` 查询参数。
- **私密图片**：
  - owner/admin（同源会话，`__Host-session_token` / `Bearer`）→ **直接放行**，无需令牌。
  - 第三方：query `?token=xxx`、头 `X-Image-Token` 或 `Authorization: Bearer xxx`。
    - 有效 → 放行（`no-store`）
    - 未带 / 错令牌 / 已过期 → **`4037`(403)**
    - 已吊销 → **`4042`(404)**
- 每次访问异步累加浏览量（Redis `views:<id>`，由 `view_flusher` worker 落库）。
- 缓存头：私密图 `no-store`，公开图 `no-cache, must-revalidate`。

### 9.3 公开统计

`GET /api/v1/public/stats` （无需登录）

```json
{ "code": 0, "data": {
    "images": 1024,
    "users": 42,
    "views": 53821,
    "storage_used": 5368709120
}}
```

| 字段 | 说明 |
|---|---|
| `images` | 托管图片总数 |
| `users` | 活跃注册用户数（`status=active`） |
| `views` | 累计浏览量（`SUM(view_count)`，约 60s 延迟落库） |
| `storage_used` | 全站已用存储（字节） |

---

## 10. 错误码参考

| code | HTTP | 含义 |
|---|---|---|
| 1000 | 500 | 内部服务器错误 |
| 1001 | 500 | 数据库错误 |
| 1002 | 500 | 缓存服务错误 |
| 1003 | 500 | 图片处理服务错误（imgproxy） |
| 1004 | 503 | reCAPTCHA 服务不可用 |
| 2001 | 401 | 用户名或密码错误 |
| 2008 | 429 | 登录尝试过于频繁 |
| 2009 | 403 | reCAPTCHA 校验失败 |
| 2090 | 429 | Bootstrap 请求过于频繁 |
| 3000 | 400 | 参数校验错误 |
| 3001 | 400 | 缺少文件 / 无效 ID |
| 3002 | 413 | 文件大小超出限制 |
| 3003 | 415 | 不支持的文件类型 |
| 3004 | 400 | 文件数量超出限制 |
| 3010 | 400 | 图片尺寸超出限制 |
| 4010 | 401 | 未认证 / 无效令牌 |
| 4011 | 401 | 会话已过期 |
| 4012 | 403 | 存储配额已满 |
| 4029 | 429 | 上传过于频繁 |
| 4030 | 403 | 权限不足 / 账户已被禁用 |
| 4031 | 403 | 设备数量上限 / 无权访问此图片 |
| 4032 | 403 | 管理接口仅限 Web 端 |
| 4033 | 403 | identity_token 不可用于 API |
| 4034 | 403 | 注册仅限 Web 端 |
| 4035 | 403 | CSRF token required |
| 4036 | 403 | Invalid CSRF token |
| 4037 | 403 | 私密图片令牌无效或已过期 |
| 4040 | 404 | 通知不存在 |
| 4041 | 404 | 用户/文件不存在 |
| 4042 | 404 | 私密图片令牌已吊销 |
| 4090 | 409 | Nonce 重放 |
| 4260 | 426 | 客户端版本过低 |

---

## 11. 前端对接说明

### 11.1 请求封装要点

1. **凭据**：所有请求带 `credentials: 'include'`（cookie 自动随请求）。
2. **CSRF**：从 `__Host-csrf_token` cookie 读值，所有非 GET 请求加头 `X-CSRF-Token`。
3. **响应判断**：以 `body.code === 0` 判定成功，否则取 `message` 提示；401 跳登录。
4. **`__Host-` cookie 限制**：必须 HTTPS + 同源部署，本地开发（http://localhost）下浏览器可能拒绝设置，需用代理同源或自签证书。

### 11.2 与现有前端 mock 的字段映射

| 前端 mock 字段 | 后端字段 | 备注 |
|---|---|---|
| `userId` | `user_id` | snake_case |
| `uploadedAt` | `created_at` | ISO8601 字符串 |
| `views` | `view_count` | |
| `size` | `file_size` | |
| `isPublic: boolean` | `visibility: "public"\|"private"` | 布尔 → 字符串 |
| `id` (string) | `id` (uint64) | 数字 |
| `url`/`thumb` | `/i/<unique_link>` | 需前端拼接直链 |
| 状态 `banned` | `suspended` | 后端无 banned |

### 11.3 后端未覆盖的前端功能（需协调）

- **公开图库 / 发现页**：后端无公开图片列表接口，`images` 列表仅按用户返回。需新增 `GET /images/public` 或前端移除该功能。
- **分类 / 标签**：`Image` 模型无 `category`/`tags` 字段。
- **管理员图片审核列表**：无对应接口。
- **用户删除**：admin 仅支持改状态，无删除用户接口。

---

*文档生成依据：`cmd/server/main.go`、`internal/handler/*`、`internal/service/*`、`internal/model/*`、`internal/middleware/{auth,csrf}.go`、`internal/pkg/{response,errcode}/*`。*
