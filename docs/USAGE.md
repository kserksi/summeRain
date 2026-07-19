# summeRain Deployment and Usage Guide

> This guide is derived from the `backend/` source and deployment configuration.
> It covers **deployment, configuration, operations, and everyday use**. See
> [`API.md`](./API.md) for the complete API contract.

---

## Contents

1. [Project Overview](#1-project-overview)
2. [Quick Start](#2-quick-start)
3. [Configuration Reference (Environment Variables)](#3-configuration-reference-environment-variables)
4. [Architecture and Deployment](#4-architecture-and-deployment)
5. [Operations Runbook](#5-operations-runbook)
6. [Usage Guide](#6-usage-guide)
7. [Limits and Thresholds](#7-limits-and-thresholds)
8. [Security Essentials](#8-security-essentials)

---

## 1. Project Overview

summeRain is a self-hosted image hosting and photo album service.

- **Backend:** Go 1.26 + Gin + GORM (MySQL) + Redis; imgproxy serves V1
  compatibility paths and applies V2 publication watermarks.
- **Frontend:** React + Vite. The Go service hosts the compiled assets from the
  same origin.
- **Core capabilities:** registration and login, image upload and management,
  public/private visibility, private-image access tokens, multi-device sessions
  (Web Cookie + device Token), notifications, administration, view statistics,
  thumbnails, and format conversion.

### Component Responsibilities

| Component | Responsibility |
|---|---|
| `backend` (Go) | Business API, direct image service at `/i/:link`, and SPA fallback |
| MySQL | Persistent users, images, sessions, notifications, configuration, and related data |
| Redis | Rate-limit counters, nonce replay protection, and view-count buffering |
| imgproxy | Dynamic V1 compatibility processing and V2 publication watermarks from local filesystem sources |
| Fronting nginx | TLS termination, reverse proxying, rate limiting, and real-IP forwarding |

---

## 2. Quick Start

### 2.1 Local Development

```bash
# Terminal 1: start pinned MySQL / Redis / imgproxy versions through Compose only
./scripts/dev-wsl.sh deps-up
./scripts/dev-wsl.sh backend

# Terminal 2: run npm ci in frontend/ before the first launch
./scripts/dev-wsl.sh frontend
```

- The backend listens on `127.0.0.1:18080` by default. Its health endpoint is
  `GET http://127.0.0.1:18080/health` -> `{"status":"ok"}`.
- The frontend listens on `https://127.0.0.1:5173` by default and proxies
  `/api/` and `/i/` to the backend from the same origin.
- The first startup automatically runs checksummed database migrations and
  compatibility model migrations.

> Browsers reject `__Host-` cookies on local `http://localhost` because they
> require HTTPS and a same-origin context. Use a self-signed certificate or a
> same-origin proxy for local integration work.

### 2.2 Production Deployment (GitHub Actions Image)

```bash
# Use an exact version published by GitHub Actions; do not commit this file
cp backend/.env.example backend/.env
chmod 0600 backend/.env
# Edit backend/.env and replace at least the image version, database password,
# Cookie secret, and imgproxy keys

docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build
```

Only GitHub Actions builds the application image and synchronizes it to Docker
Hub / GHCR. The deployment host does not build the application image.
`backend/docker-compose.deploy.yml` rejects a missing `DOCKER_IMAGE`. Production
must use an exact SemVer tag or an OCI multi-platform index digest.

In production, the fronting nginx instance reverse proxies to
`127.0.0.1:8080`, while Cloudflare exposes port 443. See Section 4.

---

## 3. Configuration Reference (Environment Variables)

Source: `internal/config/config.go`. Values that have a **default** do not need
to be set explicitly.

### 3.1 Service

| Variable | Default | Description |
|---|---|---|
| `SERVER_PORT` | `8080` | HTTP listening port |
| `GIN_MODE` | `debug` | `debug` / `release`; use `release` in production |
| `COOKIE_SECRET` | `change-me-in-production` | Reserved. Current sessions use opaque random strings and are not signed, but a strong value is still recommended |
| `CROSS_ORIGIN_ISOLATION` | `true` | Sends COOP/COEP and enables the wasm-vips large-image path. If disabled, images above the browser-native safety threshold cannot be processed |
| `GOMEMLIMIT` | `512MiB` (Compose) | Keeps the Go heap target inside the 640 MiB container limit, reserving capacity for stacks, native memory, and the runtime |

When `CROSS_ORIGIN_ISOLATION=true`, third-party scripts, fonts, and images must
explicitly permit embedding through CORS or `Cross-Origin-Resource-Policy`.
Otherwise, the browser blocks them under COEP. The 50MP upload target depends on
the isolated wasm-vips path enabled by this mode.

### 3.2 Database (MySQL)

| Variable | Default | Description |
|---|---|---|
| `DB_HOST` | `mysql` | Host |
| `DB_PORT` | `3306` | Port |
| `DB_USER` | `root` | **Use a restricted account in production**, with privileges limited to `summerain.*` |
| `DB_PASSWORD` | *(empty)* | Required |
| `DB_NAME` | `summerain` | Database name |
| `DB_MAX_OPEN_CONNS` | `8` | Maximum open database connections |
| `DB_MAX_IDLE_CONNS` | `4` | Maximum idle database connections; must not exceed the open-connection limit |
| `DB_CONN_MAX_LIFETIME` | `30m` | Maximum lifetime for a reused connection |

> DSN: `user:pass@tcp(host:port)/db?charset=utf8mb4&parseTime=True&loc=Local`

### 3.3 Cache (Redis)

| Variable | Default | Description |
|---|---|---|
| `REDIS_ADDR` | `redis:6379` | Address |
| `REDIS_PASSWORD` | *(empty)* | May be empty on a private network; set it when crossing networks |
| `REDIS_DB` | `0` | Database number |
| `REDIS_POOL_SIZE` | `8` | Redis client connection-pool limit |

Compose limits Redis data to `128mb` within a `192m` container and uses
`noeviction`. Writes fail explicitly at the limit, preventing silent eviction of
rate-limit or replay-protection state and preventing a container OOM.

### 3.4 Image Processing (imgproxy)

| Variable | Default | Description |
|---|---|---|
| `IMGPROXY_URL` | `http://imgproxy:8080` | Internal address |
| `IMGPROXY_KEY` | *(empty)* | Hex; **required** to enable signing |
| `IMGPROXY_SALT` | *(empty)* | Hex; **required** |
| `IMGPROXY_PUBLIC_URL` | `/img` | Public signed-URL prefix |
| `IMGPROXY_WORKERS` | `2` | Processing concurrency limit aligned with the V2 publication workers |

### 3.5 Storage

| Variable | Default | Description |
|---|---|---|
| `STORAGE_PATH` | `/data/images` | Persistent image directory for V1 original/thumbnail/processed files and fixed V2 variants |
| `TEMP_PATH` | `/data/images/.staging` | V1 temporary processing directory; Compose and V2 share the same bounded staging volume |
| `V2_STAGING_PATH` | `<STORAGE_PATH>/.staging` | V2 upload staging directory; must be a child of `STORAGE_PATH` to support atomic promotion |
| `DISK_SOFT_LIMIT_PERCENT` | `80` | Reject new V2 upload sessions above this utilization |
| `DISK_HARD_LIMIT_PERCENT` | `90` | Reject further upload-part or publication-output writes above this utilization |

### 3.6 V2 Upload and Publication

| Variable | Default | Description |
|---|---|---|
| `V2_UPLOAD_ENABLED` | `true` | Enables fixed-recipe V2 session uploads. V1 multipart upload is available only when this is disabled |
| `V2_RECIPE_VERSION` | `2.0.0` | Processing recipe version that must match exactly between client and server |
| `V2_MAX_PART_BYTES` | `67108864` | Maximum size of one WebP part (64 MiB) |
| `V2_MAX_PIXELS` | `50000000` | Source-image and part pixel limit (50 MP) |
| `V2_SESSION_TTL` | `30m` | Lifetime of an unfinished upload session |
| `V2_GLOBAL_UPLOAD_CONCURRENCY` | `8` | Global concurrent part-receive limit for one backend instance |
| `V2_PER_USER_UPLOAD_CONCURRENCY` | `4` | Concurrent part-receive limit for one user |
| `V2_WATERMARK_CONCURRENCY` | `2` | Publication/watermark worker count; the upper bound for a shared 3-core, 4 GB host |
| `V2_JOB_POLL_INTERVAL` | `1s` | Publication-job polling interval |
| `V2_JOB_LEASE` | `2m` | Publication-job lease; workers renew it and commit with a fencing token |

The browser upload pipeline has concurrency 2, while image decoding and encoding
remain serial. The active server-side session limit is 4, leaving backend
capacity for other tabs and recovery requests. Server publication and imgproxy
each default to 2 workers, allowing equal watermark snapshots to run in
parallel. If other components on the same host experience sustained CPU or
memory pressure, reduce both concurrency values to 1. Intermediate
`publish_source` and session staging files are deleted after publication.

### 3.7 CDN and Durable Outbox

| Variable | Default | Description |
|---|---|---|
| `CDN_PUBLIC_BASE_URL` | *(empty)* | Public image base URL; required when purge delivery is enabled |
| `CLOUDFLARE_ZONE_ID` / `CLOUDFLARE_API_TOKEN` | *(empty)* | Cloudflare purge credentials; configure them as a pair |
| `CDN_PURGE_WEBHOOK_URL` / `CDN_PURGE_WEBHOOK_TOKEN` | *(empty)* | Generic purge webhook for a non-Cloudflare CDN |
| `OUTBOX_BATCH_SIZE` | `10` | Durable events claimed per batch |
| `OUTBOX_POLL_INTERVAL` | `2s` | Outbox polling interval |
| `OUTBOX_LEASE` | `3m` | Event-delivery lease |
| `CDN_PURGE_REQUESTS_PER_SECOND` | `4` | CDN purge request-rate limit |
| `CDN_PURGE_REQUEST_TIMEOUT` | `15s` | Timeout for one purge request |

### 3.8 CAPTCHA (Pluggable and Optional)

`CAPTCHA_PROVIDER` is one of `none` (default), `recaptcha`, `turnstile`, or
`geetest_v4`. An administrator can also override it through `/admin/configs`
with the `captcha_provider` key.

| Variable | Default | Description |
|---|---|---|
| `CAPTCHA_PROVIDER` | *(empty; derived)* | Empty + `RECAPTCHA_ENABLED=true` -> `recaptcha`; otherwise `none` |
| `RECAPTCHA_ENABLED` | `false` | Global reCAPTCHA switch retained for backward compatibility |
| `RECAPTCHA_SITE_KEY` / `RECAPTCHA_SECRET` | *(empty)* | reCAPTCHA public key / secret |
| `RECAPTCHA_MIN_SCORE` | `0.5` | Minimum v3 score |
| `RECAPTCHA_VERIFY_URL` | `https://www.recaptcha.net/recaptcha/api/siteverify` | Verification endpoint using the mainland-China-friendly mirror |
| `RECAPTCHA_FAIL_CLOSED` | `true` | Whether to reject requests when the upstream service is unavailable |
| `RECAPTCHA_ALLOWED_HOSTNAMES` | *(empty)* | Allowed hostnames, comma-separated |
| `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET` | *(empty)* | Cloudflare Turnstile |
| `GEETEST_CAPTCHA_ID` / `GEETEST_CAPTCHA_KEY` | *(empty)* | GeeTest v4 |

> With `provider=none`, the backend performs no validation and the frontend
> loads no script. `/api/v1/public/config` returns the current provider and its
> corresponding public key.

> `geetest_v4` is incompatible with the default
> `CROSS_ORIGIN_ISOLATION=true`: COEP blocks its external script resources, and
> the server rejects startup or an administrative switch to that provider. Use
> `none`, `recaptcha`, or `turnstile`. GeeTest becomes available only after
> explicitly disabling cross-origin isolation, which removes the isolated
> wasm-vips path required for V2 processing of 50MP images.

---

## 4. Architecture and Deployment

### 4.1 Request Path

```text
User --HTTPS--> Cloudflare --> nginx(:443) --HTTP--> backend(:8080, 127.0.0.1)
                                      \- TLS termination / rate limiting / real-IP forwarding
backend --> MySQL / Redis / imgproxy (Docker private network)
```

### 4.2 Critical nginx Configuration (Production)

- `set_real_ip_from <Cloudflare range>; real_ip_header CF-Connecting-IP;`
  restores the real client IP.
- `proxy_set_header X-Forwarded-For $remote_addr;` **overwrites** rather than
  appends, preventing XFF spoofing.
- `location = /metrics { return 404; }` limits exposure of Prometheus metrics.
- `client_max_body_size 20m;` matches the per-request upload-size limit.
- Security headers: `HSTS` / `X-Content-Type-Options` / `X-Frame-Options` /
  `Content-Security-Policy`.

### 4.3 Container Orchestration (Docker Compose)

All four services should communicate only over the private network:

| Service | Port exposure | Description |
|---|---|---|
| `backend` | Bound only to `127.0.0.1:8080` | Application, running as **non-root (UID 10001)** |
| `mysql` | Not published | Private network only |
| `redis` | Not published | Private network only |
| `imgproxy` | Not published | Mounts the image volume read-only |

> Store production secrets in the Git-ignored `backend/.env` with mode 0600.
> Deployment commands must also pass `--env-file backend/.env`, ensuring that
> Compose interpolation and the backend container use the same configuration.

---

## 5. Operations Runbook

### 5.1 Health Checks

| Endpoint | Meaning |
|---|---|
| `GET /health` | Process is alive -> `{"status":"ok"}` |
| `GET /ready` | Checks DB + Redis connectivity; returns 503 when unavailable |
| `GET /metrics` | Prometheus metrics; restrict access at nginx in production |

### 5.2 Background Workers

`worker.Manager` starts these concurrently from `internal/worker/`:

| Worker | Interval | Responsibility |
|---|---|---|
| `heartbeat` | 5 minutes | Marks device sessions with timed-out heartbeats as expired |
| `view_flusher` | 60 seconds | Flushes Redis `views:*` counters to the database |
| `cleanup` | 1 hour | Removes expired sessions/CSRF/access tokens, failed upload records, and orphaned temporary files |
| `v2_publish` | Continuous; default concurrency 2 | Produces final published images from `publish_source` and applies watermarks |
| `v2_cleanup` | Continuous and incremental | Reclaims expired sessions, staging directories, and orphaned V2 files |
| `outbox` | 2 seconds by default | Delivers CDN purge events and local/R2 physical-deletion events |
| `user_deletion` | 5 minutes | Executes due account deletions in small, recoverable batches |

Every worker has `recover`; a single panic does not stop the whole manager.

### 5.3 Logs

- Application: `docker logs summerain-backend` (standard output).
- nginx: `/var/log/nginx/your-domain.*.log`.

### 5.4 Image Update / Rollback

```bash
# Upgrade to a new exact version published by GitHub Actions
# Edit backend/.env and set DOCKER_IMAGE to jaykserks/summerain:v2.0.1
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull backend
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build backend

# Roll back to the previous known-good immutable version
# Edit backend/.env and restore DOCKER_IMAGE to jaykserks/summerain:v2.0.0
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull backend
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build backend
```

For digest-level pinning, set `DOCKER_IMAGE` to
`jaykserks/summerain@sha256:<oci-index-digest>`. A multi-platform deployment
must pin the OCI index / manifest-list digest. Do not reuse an `amd64`- or
`arm64`-specific child manifest digest for another architecture.

> Before switching to the non-root image, run `chown -R 10001:10001` on the data
> volume. A newly created named volume inherits ownership from `/data` in the
> image.

### 5.5 Least-Privilege Database Account

Create a restricted application account in production:

```sql
CREATE USER 'image_gallery'@'%' IDENTIFIED BY '<strong-random-value>';
GRANT SELECT,INSERT,UPDATE,DELETE,CREATE,DROP,ALTER,INDEX,REFERENCES,
      CREATE TEMPORARY TABLES,LOCK TABLES,EXECUTE,CREATE VIEW,SHOW VIEW,TRIGGER
  ON summerain.* TO 'image_gallery'@'%';
FLUSH PRIVILEGES;
```

Then point `DB_USER` / `DB_PASSWORD` to that account so the application does not
hold database-wide root privileges.

---

## 6. Usage Guide

> See [`API.md`](./API.md) for complete request and response fields. The sections
> below summarize everyday workflows.

### 6.1 Accounts

- **Registration:** Web only (`POST /api/v1/auth/register`). Usernames contain
  3-50 characters; an email and an 8-72-character password are also required.
  Registration does **not** sign the user in automatically.
- **Login:** `POST /api/v1/auth/login`. A successful response sets
  `__Host-session_token` (30 days) and `__Host-csrf_token` cookies.
- **Password change:** `PATCH /api/v1/user/password`. Success **clears every
  session for that user**, forces a new login, and sends a notification.

### 6.2 Uploads and Direct Links

1. The Web client accepts static JPG/JPEG, PNG, BMP, WebP, and AVIF files up to
   15 MiB and 50 MP. The initial V2 release rejects animated images.
2. For each image, the browser creates `master` (original resolution, Q80),
   `gallery` (400x400, Q60), `admin` (120x160, Q60), and `publish_source`
   (longest edge 2048, Q80), then uploads the parts through
   `/api/v1/uploads/*`.
3. The backend promotes `master`, `gallery`, and `admin`. A background worker
   creates the optionally watermarked `publish` asset from `publish_source`,
   then deletes `publish_source` and the session intermediates. Image Management
   displays the 120x160 `admin` file at 60x80 with CSS, retaining 2x pixel
   density.
4. The V2 published direct link is `/i/<asset_link>.webp`; fixed variants are
   `/i/<asset_link>/{master|gallery|admin|publish}.webp`. Query parameters do not
   create additional V2 sizes.
5. The Web client first reads the `v2_enabled` capability from
   `/api/v1/uploads/recipe`. Only when `V2_UPLOAD_ENABLED=false` does it skip
   client preprocessing and use V1-compatible multipart upload through
   `POST /api/v1/images/`. Dynamic V1 transcoding, including arbitrary sizes,
   uses bounded temporary files and is not persisted as an access cache.
6. Identical content is deduplicated by SHA-256, with `reference_count`
   controlling the physical-file lifecycle.

The main service does not expose a bulk migration endpoint for historical
images. During compatibility operation, unclassified V1 images first use a safe
local path. The service tries the exact current R2 target only when the local
original is absent and that target is fully available. The endpoint/bucket
cannot change while unclassified historical records remain. A later migration
tool in a separate repository will own validation, checkpoints, auditing, and
rollback for historical data.

### 6.3 Public / Private

- Each image has `visibility` set to `public` or `private`.
- A **private image** requires an access token supplied through query `?token=`,
  header `X-Image-Token`, or `Authorization: Bearer`.
- Issue a token with `POST /api/v1/images/:id/tokens`. Its lifetime is 10 minutes
  to 3 days. Plaintext is returned only to the owner/admin in the issuance
  response and image details.
- Switching from private to public **automatically revokes every token for that
  image**.

### 6.4 Multiple Devices

- Web: Cookie authentication; write operations require the `X-CSRF-Token`
  header.
- Devices (android/windows): `device-login` returns an `identity_token` valid for
  90 days. `device-bootstrap`, protected by nonce replay prevention, exchanges
  it for a `session_token` valid for 15 minutes and extended by heartbeat. Each
  platform allows at most 3 devices.

### 6.5 Administration

Administrative endpoints require `role=admin`, `platform=web`, and CSRF
protection throughout the group:

- List users and change their status. Setting `suspended` forces logout on every
  device.
- View system statistics and change system configuration, including watermark
  fields `watermark_enabled/text/position/opacity`.

---

## 7. Limits and Thresholds

| Item | Value | Source |
|---|---|---|
| V2 source-file limit | 15 MiB | `frontend/src/features/images/pages/Upload.tsx` |
| V2 per-image pixel limit | 50 MP | `config.go` / `v2_upload_types.go` |
| Fixed V2 access variants | `master`, 400x400 `gallery`, 120x160 `admin`, longest-edge-2048 `publish` | `v2_upload_types.go` |
| V1 multipart limit | 10 MiB per file, no more than 20 files per request | `image_service.go` |
| Default storage quota | 500 MiB (524288000 bytes) | `model.User` |
| Quota warning threshold | 90% | `image_service.go` |
| Image short link | V2 uses 12 hex characters; V1 defaults to 12 and falls back to 16 after repeated collisions | `generateUniqueLink` |
| Web session | 30 days | `auth_service.go` |
| CSRF lifetime | 24 hours, sliding renewal | `auth_service.go` |
| Device identity | 90 days | `auth_service.go` |
| Device session | 15 minutes, heartbeat renewal, 600s grace | `auth_service.go` / `model.Session` |
| Devices per platform | No more than 3 | `auth_service.go` |
| Login rate limit | IP: 5 attempts / 15 minutes; username: 3 attempts / 15 minutes | `auth_service.go` |
| Bootstrap rate limit | 10 attempts / minute | `auth_service.go` |
| Access-token lifetime | 10 minutes to 3 days | `image_service.go` |
| V1 dynamic-transcode size parameters | `w/h` no greater than 4096 | `public_handler.go` |
| V2 upload formats | Static jpg/jpeg/png/bmp/webp/avif | `sniff.ts` / `v2_upload_types.go` |

---

## 8. Security Essentials

- **Authentication:** session tokens are stored only as SHA256 hashes. Cookies
  use the `__Host-` prefix, `SameSite=Strict`, `Secure`, and `HttpOnly`. CSRF uses
  double submission and is skipped for Bearer requests.
- **Passwords:** bcrypt with DefaultCost.
- **Paths:** `NoRoute` static serving uses `filepath.Clean` plus a base-directory
  prefix check to prevent traversal.
- **Rate limiting:** login/Bootstrap limits use IP + username. In production,
  nginx rate limiting and deny lists are the primary perimeter defense.
- **Images:** file extensions and MIME sniffing are both validated. SVG is forced
  to download as `application/octet-stream` to prevent XSS. Private images use
  `no-store`.
- **Containers:** non-root (UID 10001); MySQL/Redis ports are not published; the
  database account uses least privilege.
- **Transport:** Cloudflare Full (Strict) + nginx HSTS.

> With `GIN_MODE=debug`, error responses may expose internal details. Production
> must use `release`.

---

*Sources: `cmd/server/main.go`, `internal/config/config.go`,
`internal/{handler,service,middleware,model,worker}/*`, `Dockerfile`, and
`docker-compose*.yml`.*
