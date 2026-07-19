# summeRain Backend API Reference

> This reference was verified file by file against the `backend/` source (Go +
> Gin + GORM/MySQL + Redis + imgproxy) and serves as the integration contract
> between the frontend and backend.
>
> - **Base URL:** `/api/v1`
> - **Default port:** `8080` (`SERVER_PORT`)
> - **Direct image route:** `GET /i/:link` (outside `/api/v1`)
> - **Verification baseline:** `v2.0.0`. Early V2 releases may change frequently;
>   the source and release notes for the relevant version take precedence.

---

## Contents

1. [General Conventions](#1-general-conventions)
2. [Authentication and CSRF](#2-authentication-and-csrf)
3. [Authentication](#3-authentication)
4. [Images](#4-images)
5. [Image Access Tokens](#5-image-access-tokens)
6. [User](#6-user)
7. [Notifications](#7-notifications)
8. [Administration](#8-administration)
9. [Public Endpoints](#9-public-endpoints)
10. [Error Code Reference](#10-error-code-reference)
11. [Frontend Integration Notes](#11-frontend-integration-notes)

---

## 1. General Conventions

### Response Envelope

Application API endpoints return the following `response.Response` envelope:

```json
{
  "code": 0,
  "message": "success",
  "data": { },
  "request_id": "optional; returned only for errors"
}
```

| Field | Description |
|---|---|
| `code` | `0` indicates success; any other value is an application error code. See [Section 10](#10-error-code-reference). |
| `message` | Human-readable message |
| `data` | Application data; present on success and omitted on errors unless the error includes additional data |
| `request_id` | Request trace ID, **returned only in error responses** |

- HTTP status codes follow application semantics: 200 success, 201 created, 400 invalid parameters, 401 unauthenticated, 403 forbidden, 404 not found, 413 payload too large, 429 rate limited, and 500 server error.

### Data Model Field Names

Backend JSON uses **snake_case** throughout, for example `storage_used`,
`created_at`, `view_count`, `unique_link`, and `user_id`.

### Runtime Dependencies

MySQL and Redis must be reachable when the service starts; `main.go` calls
`Fatal` if either `Ping` fails. imgproxy handles bounded V1 dynamic
transformations and the V2 publication stage when watermarking is enabled.
Existing persisted variants can be read directly. The infrastructure can be
started with `docker-compose.yml`.

Health endpoints:

- `GET /health` -> `{"status":"ok"}`
- `GET /ready` -> checks DB and Redis connectivity; returns 503 when unavailable
- `GET /metrics` -> Prometheus metrics

---

## 2. Authentication and CSRF

The system supports two authentication methods.

### 2.1 Web: Cookie Authentication

After a successful login, the server sets two cookies:

| Cookie | Purpose | HttpOnly | Lifetime |
|---|---|---|---|
| `__Host-session_token` | Session credential | Yes | 30 days (2592000 seconds) |
| `__Host-csrf_token` | CSRF protection | No; readable by the frontend | Cookie Max-Age of 30 days; server record of 24 hours |

- The cookies use the `__Host-` prefix and require **HTTPS and same-site deployment**, `SameSite=Strict`, and `Secure`.
- The frontend only needs to send requests with `credentials: 'include'`; the browser attaches the cookies automatically.
- The server-side CSRF record is renewed after valid write operations. When it expires, `POST /api/v1/auth/csrf/refresh` can restore it. The frontend automatically refreshes and replays only explicitly idempotent requests.

### 2.2 CSRF Protection

**Every write operation (POST, PUT, PATCH, or DELETE) authenticated by cookie
must include this request header:**

```text
X-CSRF-Token: <value of the __Host-csrf_token cookie>
```

Rules from `middleware/csrf.go`:

- GET, HEAD, and OPTIONS requests are not checked.
- Requests using `Authorization: Bearer <token>` **skip** CSRF validation for device clients.
- With cookie authentication, a missing header returns `4035 CSRF token required`; a mismatched value returns `4036 Invalid CSRF token`.

> Frontend implementation: read the `__Host-csrf_token` cookie and add the
> `X-CSRF-Token` header to every non-GET request.

### 2.3 Device Clients: Bearer Token Authentication

Send `Authorization: Bearer <session_token>` together with the
`X-Platform: android|windows` and `X-Client-Version` headers. The device-login,
bootstrap, and heartbeat flows do not apply to Web integration.

---

## 3. Authentication

### 3.1 Registration

`POST /api/v1/auth/register`

> [!WARNING]
> Web only. If `X-Platform` is present and is not `web`, the endpoint returns
> `4034 Registration is restricted to Web clients`. Login rate limiting applies.

**Request body**

```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "at least 8 characters",
  "captcha": {
    "provider": "recaptcha|turnstile|geetest_v4",
    "token": "recaptcha / turnstile token",
    "action": "register",
    "lot_number": "GeeTest v4",
    "captcha_output": "GeeTest v4",
    "pass_token": "GeeTest v4",
    "gen_time": "GeeTest v4"
  }
}
```

Validation: `username` must contain 3-50 characters, `email` must be a valid
email address of at most 100 characters, and `password` must contain 8-72
characters.

> The `captcha` payload depends on the current `captcha_provider`; see
> [Section 9.1](#91-public-configuration). It can be omitted when
> `provider=none`. reCAPTCHA requires `token` and `action`; Turnstile requires
> `token`; GeeTest v4 requires `lot_number`, `captcha_output`, `pass_token`, and
> `gen_time`. A mismatched provider is rejected. GeeTest v4 is unavailable with
> the default `CROSS_ORIGIN_ISOLATION=true`; see [Section 9.1](#91-public-configuration).

**Success: 201**

```json
{
  "code": 0,
  "message": "created",
  "data": { "id": 12, "username": "alice", "email": "alice@example.com" }
}
```

> Registration **does not** create a login session. Call the login endpoint
> separately.

### 3.2 Login

`POST /api/v1/auth/login`

**Request body**

```json
{ "username": "alice", "password": "xxxxxx",
  "captcha": { "provider": "recaptcha", "token": "...", "action": "login" } }
```

> `username` accepts a username. IP and username rate limits apply; repeated
> failures return `2008`. The `captcha` payload matches
> [registration](#31-registration).

**Success: 200** - also sets `__Host-session_token` and `__Host-csrf_token`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "user": { "id": 12, "username": "alice", "role": "user" }
  }
}
```

> `UserSummary` contains only `id`, `username`, and `role`.

### 3.3 Current User

`GET /api/v1/auth/me` (authentication required)

**Success: 200**

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

> Returns the complete `model.User`; `password_hash` is hidden by `json:"-"`.

### 3.4 Logout

`POST /api/v1/auth/logout` (authentication and CSRF required)

Clears both cookies and deletes the server-side session. Returns
`{"code":0,"data":null}`.

### 3.5 Refresh a CSRF Token

`POST /api/v1/auth/csrf/refresh` (authentication required)

This endpoint restores an expired CSRF token during a long-running upload. It
does not require the old `X-CSRF-Token`, but it must be a same-origin Web
request. The server validates `Origin` and, when the browser supplies it,
`Sec-Fetch-Site: same-origin`. On success it resets `__Host-csrf_token`. Only
requests with idempotent semantics may be replayed automatically after a
refresh.

### 3.6 Device Endpoints

Web integrations can ignore these endpoints.

| Method | Path | Description |
|---|---|---|
| POST | `/auth/device-login` | Log in a device and return an identity_token |
| POST | `/auth/device-bootstrap` | Exchange an identity_token for a session_token |
| POST | `/auth/device-heartbeat` | Keep the session alive with a heartbeat |
| DELETE | `/auth/device-shutdown` | Terminate a device session |
| GET | `/auth/device-identities` | List device identities |
| DELETE | `/auth/device-identities/:id` | Revoke an identity; CSRF required |
| GET | `/auth/sessions` | List active sessions |
| DELETE | `/auth/sessions/:id` | Revoke a session; CSRF required |

---

## 4. Images

> All endpoints in this section require authentication. Images are scoped to
> their owner: `GET /images/:id` verifies `image.user_id == current user` and
> otherwise returns `4031 Forbidden`.

### 4.1 Image List with Cursor Pagination

`GET /api/v1/images/`

**Query parameters**

| Parameter | Default | Description |
|---|---|---|
| `cursor` | Empty | Pagination cursor from the previous response's `next_cursor` |
| `limit` | `20` | Items per page |
| `sort` | `-created_at` | Sort expression; a `-` prefix selects descending order |
| `visibility` | Empty | Filter by `public` or `private` |
| `search` | Empty | Keyword search |

**Success: 200**

```json
{
  "code": 0,
  "data": {
    "images": [ { "id": 1, "user_id": 12, "unique_link": "abc...", "filename": "a.jpg",
                  "title": "", "visibility": "public", "view_count": 5,
                  "width": 800, "height": 600, "file_size": 123456,
                  "created_at": "2026-06-18T10:00:00Z", "updated_at": "..." } ],
    "next_cursor": "cursor for the next page; empty when there are no more results",
    "has_more": true
  }
}
```

`Image` fields from `model/image.go`:

| Field | Type | Description |
|---|---|---|
| `id` | uint64 | Image ID |
| `user_id` | uint64 | Owner |
| `image_file_id` | uint64 | Underlying file record |
| `unique_link` | string | Unique short link used to build `/i/<unique_link>` |
| `title` / `filename` / `description` | string | Metadata |
| `visibility` | string | `public` or `private` |
| `pipeline_version` | uint16 | `1` for the historical pipeline; `2` for client preprocessing |
| `processing_status` | string | `pending`, `processing`, `completed`, or `failed` |
| `asset_link` | string? | Current V2 origin alias; private images use `<unique_link>S` |
| `view_count` | uint64 | View count |
| `width` / `height` | int | Dimensions |
| `file_size` | int64 | Bytes |
| `created_at` / `updated_at` | time | Timestamps |

> [!WARNING]
> The backend `Image` model has no `category` or `tags` fields. Categories or
> tags require a backend extension or local frontend storage.

### 4.2 V2 Client-Preprocessed Uploads

V2 is enabled by default. The browser accepts static JPG/JPEG, PNG, BMP, WebP,
and AVIF files, rejects animation, and generates four WebP parts:
`master` at the original resolution and Q80; `gallery` as a 400x400 cover crop
at Q60; `admin` as a 120x160 cover crop at Q60, displayed in Image Management
at 60x80 CSS pixels for 2x pixel density; and `publish_source` with a longest
edge of 2048 at Q80. The server receives and validates complete WebP containers
while streaming and applies a watermark only to the final `publish` asset in
the background.

1. `GET /api/v1/uploads/recipe` (authentication required): returns the active recipe, part limits, pixel limit, and session TTL. `v2_enabled` is the capability switch for new uploads. When it is `false`, the Web client skips local preprocessing and uses V1 multipart upload.
2. `POST /api/v1/uploads/` (authentication and CSRF required): creates a session. Send an `Idempotency-Key` of at most 64 characters. A key can replay only an identical manifest.
3. `PUT /api/v1/uploads/:uploadID/parts/:kind` (authentication and CSRF required): uploads the raw `image/webp` request body to the response's `put_url`. `Content-Length`, SHA-256, dimensions, and the complete RIFF container must match the manifest.
4. `POST /api/v1/uploads/:uploadID/complete` (authentication and CSRF required): atomically promotes `master`, `gallery`, and `admin`, then creates a durable publication job that produces `publish` from `publish_source`.
5. `POST /api/v1/uploads/status` (authentication and CSRF required): queries 1-100 `upload_ids` in a batch. Missing or unauthorized IDs produce a uniform 404 without partial results.
6. `GET /api/v1/uploads/:uploadID` (authentication required): queries one status. `DELETE` on the same path can cancel a session that has not entered processing.

**Manifest example**

```json
{
  "filename": "photo.jpg",
  "visibility": "public",
  "processor_version": "wasm-vips-0.0.18",
  "recipe_version": "2.0.0",
  "source": { "mime_type": "image/jpeg", "width": 8000, "height": 6000, "animated": false },
  "parts": [
    { "kind": "master", "size": 8200000, "sha256": "<64 lowercase hex>", "mime_type": "image/webp", "width": 8000, "height": 6000, "quality": 80 },
    { "kind": "gallery", "size": 32000, "sha256": "<64 lowercase hex>", "mime_type": "image/webp", "width": 400, "height": 400, "quality": 60 },
    { "kind": "admin", "size": 9000, "sha256": "<64 lowercase hex>", "mime_type": "image/webp", "width": 120, "height": 160, "quality": 60 },
    { "kind": "publish_source", "size": 410000, "sha256": "<64 lowercase hex>", "mime_type": "image/webp", "width": 2048, "height": 1536, "quality": 80 }
  ]
}
```

**Session response**

```json
{
  "upload_id": "32-character-url-safe-id",
  "status": "initiated",
  "expires_at": "2026-07-16T12:30:00Z",
  "parts": [
    { "kind": "master", "status": "pending", "put_url": "/api/v1/uploads/.../parts/master", "size": 8200000, "sha256": "...", "width": 8000, "height": 6000 }
  ]
}
```

The normal status sequence is `initiated`, `uploading`, `processing`, then
`completed`; `failed` and `cancelled` are terminal. After publication, the four
persisted access variants are `master`, `gallery`, `admin`, and `publish`.
The intermediate `publish_source` is deleted. A `cleanup_pending` response with
`image_id`, `unique_link`, and `asset_link` means the image has been published
and only intermediate cleanup remains; otherwise treat it as a failed terminal
state. Client polling has a ten-minute limit.

`POST /api/v1/images/` remains as the V1 multipart compatibility endpoint only
when `V2_UPLOAD_ENABLED=false`; it returns `4262` while V2 is enabled. The recipe
endpoint is always available and returns `"v2_enabled": false` when V2 is off,
allowing the client to choose the compatibility pipeline before processing the
source image.

### 4.3 Image Details

`GET /api/v1/images/:id` (authentication required; owner or admin)

Returns one `Image` with the same structure as a list item. For a **private
image**, an owner or admin response also includes the current unified token:

```json
{ "code": 0, "data": {
    "...Image fields...": "...",
    "access_token": "current plaintext unified token; omitted when none is active",
    "token_expires_at": "2026-06-18T11:00:00Z"
}}
```

### 4.4 Delete an Image

`DELETE /api/v1/images/:id` (authentication and CSRF required)

**Success: 200** (`DeleteResult`)

```json
{ "code": 0, "data": {
    "image_id": 50,
    "storage_freed_bytes": 123456,
    "storage_used": 1876544,
    "storage_quota": 1073741824
}}
```

### 4.5 Change Visibility

`PATCH /api/v1/images/:id/visibility` (authentication and CSRF required)

**Request body:** `{ "visibility": "public" }` (`public` or `private` only)

**Success: 200** (`VisibilityResult`)

```json
{ "code": 0, "data": {
    "image_id": 50,
    "visibility": "public",
    "tokens_revoked": 2,
    "warning": "Changing from private to public revoked every access token for this image",
    "asset_link": "current V2 publication link; private state appends S to the filename"
}}
```

`tokens_revoked` can be greater than zero only for a `private` to `public`
change. A `public` to `private` change immediately switches the V2 `asset_link`
to the alias with an `S` suffix. The CDN may retain the old public URL for at
most ten minutes, and a purge event is also added to the outbox.

---

## 5. Image Access Tokens

Each private image has **at most one unified token**. The token value is
**immutable**. After revocation, an owner or admin must explicitly issue a new
one; until then, the image cannot be shared with third parties.

### 5.1 Issue or Reissue a Token

`POST /api/v1/images/:id/tokens` (authentication and CSRF required; owner or admin)

**Request body:** `{ "ttl_ms": 3600000 }` (optional). The default comes from
`private_token_ttl_default_ms` and is clamped to `[600000, 259200000]` ms, or
ten minutes through three days.

> Issuing a token **automatically invalidates** the image's existing active
> token, preserving the one-active-token rule, and returns a new plaintext token.

**Success: 200** (`AccessTokenResult`)

```json
{ "code": 0, "data": {
    "token_id": 7,
    "token": "plaintext token",
    "expires_at": "2026-06-18T11:00:00Z",
    "warning": "Save this token now. Its value is immutable, and a revoked token must be reissued."
}}
```

### 5.2 Revoke a Token

`DELETE /api/v1/images/:id/tokens` (authentication and CSRF required; owner or admin) -> `{ "image_id": 7, "revoked": true }`

> `revoked=false` means no active token existed. After revocation, the image
> cannot be shared with third parties until another token is issued.

### 5.3 Current Token

There is no separate list endpoint. When an owner or admin calls
`GET /api/v1/images/:id`, a private image response includes `access_token` and
`token_expires_at` when a token is active. See [Section 4.3](#43-image-details).

### 5.4 Upload Queue Status

`GET /api/v1/upload/queue/:id` (authentication required) queries an asynchronous
`upload_queue` record.

---

## 6. User

### 6.1 Profile

`GET /api/v1/user/profile` (authentication required)

**Success: 200** (`UserProfile`, which adds the calculated `storage_percent` to
`model.User`)

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

### 6.2 Change Password

`PATCH /api/v1/user/password` (authentication and CSRF required)

**Request body:** `{ "old_password": "old password", "new_password": "new password of at least 8 characters" }`

> A successful password change **deletes every session for the user**, forcing
> a new login, and sends a notification. An incorrect old password returns
> `2001`.

---

## 7. Notifications

> Every endpoint requires authentication; write operations require CSRF. See
> `model/notification.go`.

| Method | Path | Description |
|---|---|---|
| GET | `/notifications/` | List notifications |
| PATCH | `/notifications/:id/read` | Mark one as read; CSRF required |
| PATCH | `/notifications/batch-read` | Mark all as read; CSRF required |
| DELETE | `/notifications/:id` | Delete one; CSRF required |
| DELETE | `/notifications/clear` | Delete all; CSRF required |

`Notification` fields are `id`, `user_id`, `type`, `title`, `message`, `is_read`,
`metadata` as a JSON string, and `created_at`.

---

## 8. Administration

> Every endpoint requires authentication, `platform == "web"`, and
> `role == "admin"`; `RequireAdmin` performs all three checks. CSRF middleware
> applies to the entire admin route group.

### 8.1 Paginated User List

`GET /api/v1/admin/users?page=1&page_size=20`

**Success: 200** (`UserListResult`)

```json
{ "code": 0, "data": {
    "items": [ { /* every model.User field */ } ],
    "total": 42, "page": 1
}}
```

> `page_size` has a maximum of 100. `items` are ordered by `id ASC`. Each item
> includes `storage_used`, `storage_quota`, `image_count`, `status`, and the
> other user fields.

### 8.2 Change User Status

`PATCH /api/v1/admin/users/:id/status` (CSRF required)

**Request body:** `{ "status": "active" }`

`status` accepts only `active` or `suspended`. The account-deletion state
machine controls `pending_deletion` and `deleting`; this endpoint cannot set
them directly.

> Setting `suspended` causes subsequent authentication to reject the user's
> existing sessions, effectively forcing logout, and sends an "Account
> disabled" notification. The backend has no `banned` state; the corresponding
> concept is `suspended`. A missing user returns `4041`.

### 8.3 Request or Cancel User Deletion

| Method | Path | Request | Description |
|---|---|---|---|
| POST | `/admin/users/:id/request-deletion?admin=<administrator username>` | `{ "username": "username to delete" }` | Accepts only an `active` non-admin user; enters `pending_deletion` and schedules deletion after 24 hours |
| POST | `/admin/users/:id/cancel-deletion` | None | Accepts only `pending_deletion`; restores `active` |

Requesting deletion clears every session for the target user. During the lock
period the user can log in again and batch-download their data, but uploads,
image deletion, and image modification return `4038`. When the deadline worker
claims the task, the account enters the internal `deleting` phase. Authentication
and business access then fail closed, cancellation is no longer possible, and
physical file deletion retries through the durable outbox.

Repeated requests, disallowed source states, or concurrent state changes return
`4095`; a username mismatch returns `3000`; administrator accounts cannot be
deleted. Both endpoints return `{"code":0,"data":null}` on success.

### 8.4 System Statistics

`GET /api/v1/admin/stats`

**Success: 200** (`SystemStats`)

```json
{ "code": 0, "data": {
    "total_users": 42,
    "total_images": 1024,
    "storage_used": 5368709120,
    "active_users": 38,
    "total_sessions": 17
}}
```

### 8.5 System Configuration

| Method | Path | Description |
|---|---|---|
| GET | `/admin/configs` | Return all configuration entries (`config_key`, `config_value`, and related fields) |
| PATCH | `/admin/configs` | Update a batch with `{ "items": [ { "key": "...", "value": "..." } ] }` |

---

## 9. Public Endpoints

### 9.1 Public Configuration

`GET /api/v1/public/config` (no authentication required)

```json
{ "code": 0, "data": {
    "captcha_provider": "none",
    "captcha_site_key": "",
    "site_language": "en-US"
}}
```

| Field | Description |
|---|---|
| `captcha_provider` | `none`, `recaptcha`, `turnstile`, or `geetest_v4`; an admin can override it through `/admin/configs` with key `captcha_provider` |
| `captcha_site_key` | Client public key for the active provider: site key for reCAPTCHA/Turnstile, `captcha_id` for GeeTest, or empty for `none` |
| `site_language` | Site language such as `en-US` or `zh-CN`; the frontend selects its language at startup |

The default `CROSS_ORIGIN_ISOLATION=true` sends COOP/COEP headers to enable the
wasm-vips 50 MP processing path. GeeTest v4 external script resources do not
satisfy this isolation policy, so the server rejects `captcha_provider=geetest_v4`
while isolation is enabled. Use `none`, `recaptcha`, or `turnstile`. Explicitly
disabling cross-origin isolation enables GeeTest but removes the isolated
execution path required for V2 large-image processing and is not recommended
for 50 MP upload deployments.

### 9.2 Direct Image Service

`GET /i/:link`

- V2 published image: `GET /i/<asset_link>.webp`.
- V2 fixed variants: `GET /i/<asset_link>/master.webp`, `gallery.webp`, `admin.webp`, and `publish.webp`. Query parameters do not generate additional sizes. Only owners and admins can access `master` and `admin`.
- A V1 `link` can still be `<unique_link>` or `<unique_link>.<ext>`, where `ext` is webp, avif, jpg, jpeg, png, or gif. A link without an extension returns the original. Existing no-size WebP and background AVIF variants are read directly; other formats or requests with `w`/`h` up to 4096 and `q` use the bounded imgproxy compatibility path. Arbitrary-size dynamic results use temporary files only while the request is active. Concurrent identical requests are coalesced, and the file is deleted after the final response releases it.
- **Private images:**
  - An owner or admin with a same-origin session through `__Host-session_token` or `Bearer` is admitted directly without an image token.
  - A third party can use query parameter `?token=xxx`, header `X-Image-Token`, or `Authorization: Bearer xxx`.
    - A valid token is admitted with `no-store`.
    - A missing, incorrect, or expired token returns **`4037` (403)**.
    - A revoked token returns **`4042` (404)**.
- Every access asynchronously increments the Redis `views:<id>` counter, which the `view_flusher` worker persists.
- Cache headers: private images use `no-store`; public origin responses are cached for at most ten minutes. A visibility change also writes a durable CDN purge event to the outbox.

### 9.3 Public Statistics

`GET /api/v1/public/stats` (no authentication required)

```json
{ "code": 0, "data": {
    "images": 1024,
    "users": 42,
    "views": 53821,
    "storage_used": 5368709120
}}
```

| Field | Description |
|---|---|
| `images` | Total hosted images |
| `users` | Active registered users with `status=active` |
| `views` | Cumulative views from `SUM(view_count)`, persisted with approximately 60 seconds of delay |
| `storage_used` | Site-wide storage usage in bytes |

---

## 10. Error Code Reference

| code | HTTP | Meaning |
|---|---|---|
| 1000 | 500 | Internal server error |
| 1001 | 500 | Database error |
| 1002 | 500 | Cache service error |
| 1003 | 500 | Image processing service error (imgproxy) |
| 1004 | 503 | reCAPTCHA service unavailable |
| 2001 | 401 | Incorrect username or password |
| 2008 | 429 | Too many login attempts |
| 2009 | 403 | reCAPTCHA validation failed |
| 2090 | 429 | Too many bootstrap requests |
| 3000 | 400 | Parameter validation error |
| 3001 | 400 | Missing file or invalid ID |
| 3002 | 413 | File exceeds the size limit |
| 3003 | 415 | Unsupported file type |
| 3004 | 400 | Too many files |
| 3005 | 400/404 | Invalid V2 upload manifest, upload ID, or part parameter |
| 3006 | 400 | Upload stream read failure or invalid R2 URL configuration |
| 3007 | 422 | Upload part SHA-256 verification failed |
| 3008 | 422 | Upload part dimensions do not match the manifest |
| 3010 | 400 | Image dimensions exceed the limit |
| 4010 | 401 | Unauthenticated or invalid token |
| 4011 | 401 | Session expired |
| 4012 | 403 | Storage quota exhausted |
| 4029 | 429 | Upload rate limit exceeded |
| 4030 | 403 | Insufficient permission or suspended account |
| 4031 | 403 | Device limit reached or image access forbidden |
| 4032 | 403 | Admin endpoint restricted to Web clients |
| 4033 | 403 | identity_token cannot be used for API access |
| 4034 | 403 | Registration restricted to Web clients |
| 4035 | 403 | CSRF token required |
| 4036 | 403 | Invalid CSRF token |
| 4037 | 403 | Private-image token invalid or expired |
| 4038 | 403 | Image writes forbidden during the account-deletion lock period |
| 4039 | 403 | Batch-download allowance exhausted during the deletion lock period |
| 4040 | 404 | Notification not found |
| 4041 | 404 | User or file not found |
| 4042 | 404 | Private-image token revoked |
| 4043 | 404 | Upload session missing, expired, or inaccessible |
| 4090 | 409 | Nonce replay |
| 4091 | 409 | Upload session state conflict |
| 4092 | 409 | Upload parts incomplete |
| 4093 | 409 | Image still processing or cleaning up |
| 4094 | 409 | R2 storage target still referenced by historical files or pending cleanup and cannot be changed |
| 4095 | 409 | Current user state does not allow the requested transition |
| 4261 | 426 | Client image recipe version unsupported |
| 4262 | 426 | Deployment requires V2 client-preprocessed upload |
| 4260 | 426 | Client version too old |
| 4291 | 429 | Upload concurrency or active-session capacity exhausted |
| 5030 | 503 | Server storage pressure too high |
| 5031 | 503 | V2 upload temporarily disabled |

---

## 11. Frontend Integration Notes

### 11.1 Request Wrapper Requirements

1. **Credentials:** set `credentials: 'include'` on every request so the browser sends cookies automatically.
2. **CSRF:** read the `__Host-csrf_token` cookie and add `X-CSRF-Token` to every non-GET request.
3. **Response handling:** treat `body.code === 0` as success; otherwise display `message`. Redirect 401 responses to login.
4. **`__Host-` cookie constraints:** HTTPS and same-origin deployment are mandatory. A browser may reject these cookies on local `http://localhost`; use a same-origin proxy or a self-signed certificate.

### 11.2 Existing Frontend Mock Field Mapping

| Frontend mock field | Backend field | Notes |
|---|---|---|
| `userId` | `user_id` | snake_case |
| `uploadedAt` | `created_at` | ISO 8601 string |
| `views` | `view_count` | |
| `size` | `file_size` | |
| `isPublic: boolean` | `visibility: "public"|"private"` | Boolean to string |
| `id` (string) | `id` (uint64) | Numeric |
| `url`/`thumb` | `/i/<unique_link>` | Frontend must construct the direct URL |
| `banned` status | `suspended` | The backend has no `banned` state |

### 11.3 Frontend Features Not Covered by the Backend

- **Public gallery or discovery page:** the backend has no public image-list endpoint; `images` returns only the current user's images. Add `GET /images/public` or remove the feature from the frontend.
- **Categories or tags:** the `Image` model has no `category` or `tags` fields.

The current frontend already integrates administrator image listing and
deletion, user deletion requests and cancellation, and display of
`pending_deletion` and `deleting`; these are no longer capability gaps.

---

*Sources used to generate this document: `cmd/server/main.go`,
`internal/handler/*`, `internal/service/*`, `internal/model/*`,
`internal/middleware/{auth,csrf}.go`, and
`internal/pkg/{response,errcode}/*`.*
