# summeRain 部署与使用文档

> 本文档基于 `backend/` 源码与部署配置生成，覆盖**部署、配置、运维与日常使用**。
> 接口契约详见 [`API.md`](./API.md)。

---

## 目录

1. [项目概述](#1-项目概述)
2. [快速开始](#2-快速开始)
3. [配置参考（环境变量）](#3-配置参考环境变量)
4. [架构与部署](#4-架构与部署)
5. [运维手册](#5-运维手册)
6. [使用指南](#6-使用指南)
7. [限制与阈值速查](#7-限制与阈值速查)
8. [安全要点](#8-安全要点)

---

## 1. 项目概述

summeRain 是一个自托管的图床 / 图片相册服务。

- **后端**：Go 1.26 + Gin + GORM（MySQL）+ Redis；imgproxy 服务 V1 兼容路径及 V2 发布水印
- **前端**：React + Vite（构建产物由 Go 服务同源托管）
- **核心能力**：注册登录、图片上传/管理、公开/私有可见性、私密图访问令牌、多端会话（Web Cookie + 设备 Token）、通知、后台管理、浏览量统计、缩略图/格式转换。

### 技术栈职责

| 组件 | 作用 |
|---|---|
| `backend` (Go) | 业务 API、图片直链服务 `/i/:link`、SPA 兜底 |
| MySQL | 用户/图片/会话/通知/配置等持久化 |
| Redis | 限流计数、nonce 防重放、浏览量缓冲 |
| imgproxy | V1 动态兼容处理与 V2 发布水印（本地文件系统源） |
| nginx（前置） | TLS 终止、反代、限速、真实 IP 透传 |

---

## 2. 快速开始

### 2.1 本地开发

```bash
# 终端 1：只用 Compose 启动固定版本的 MySQL / Redis / imgproxy
./scripts/dev-wsl.sh deps-up
./scripts/dev-wsl.sh backend

# 终端 2：首次运行先在 frontend/ 执行 npm ci
./scripts/dev-wsl.sh frontend
```

- 后端默认监听 `127.0.0.1:18080`，健康检查 `GET http://127.0.0.1:18080/health` → `{"status":"ok"}`。
- 前端默认监听 `https://127.0.0.1:5173`，并同源代理 `/api/` 与 `/i/` 到后端。
- 首次启动会自动执行带校验和的数据库迁移及兼容模型迁移。

> 本地 `http://localhost` 下 `__Host-` 前缀 Cookie 会被浏览器拒绝设置（要求 HTTPS + 同源）。本地联调建议用自签证书或同源代理。

### 2.2 生产部署（GitHub Actions 镜像）

```bash
# 使用 GitHub Actions 已发布的精确版本；不要提交此文件
cp backend/.env.example backend/.env
chmod 0600 backend/.env
# 编辑 backend/.env，至少替换镜像版本、数据库密码、Cookie 与 imgproxy 密钥

docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build
```

应用镜像只由 GitHub Actions 构建并同步到 Docker Hub / GHCR，部署机不执行应用镜像构建。`backend/docker-compose.deploy.yml` 会拒绝缺失的 `DOCKER_IMAGE`，生产环境应使用精确 SemVer 标签或 OCI 多架构索引 digest。

生产前置 nginx 反代到 `127.0.0.1:8080`，并经 Cloudflare 暴露 443（详见第 4 节）。

---

## 3. 配置参考（环境变量）

来源：`internal/config/config.go`。**带默认值**的项可不显式设置。

### 3.1 服务

| 变量 | 默认 | 说明 |
|---|---|---|
| `SERVER_PORT` | `8080` | HTTP 监听端口 |
| `GIN_MODE` | `debug` | `debug` / `release`（生产用 `release`） |
| `COOKIE_SECRET` | `change-me-in-production` | 预留（当前会话为不透明随机串，未实际签名；仍建议设置强值） |
| `CROSS_ORIGIN_ISOLATION` | `true` | 下发 COOP/COEP，启用 wasm-vips 大图路径；禁用后超过浏览器原生安全阈值的大图无法处理 |
| `GOMEMLIMIT` | `512MiB`（Compose） | 将 Go 堆目标限制在 640 MiB 容器内，为栈、原生内存和运行时预留空间 |

`CROSS_ORIGIN_ISOLATION=true` 时，第三方脚本、字体和图片必须通过 CORS 或 `Cross-Origin-Resource-Policy` 明确允许嵌入，否则浏览器会按 COEP 拦截。50MP 上传目标依赖此隔离模式提供的 wasm-vips 路径。

### 3.2 数据库（MySQL）

| 变量 | 默认 | 说明 |
|---|---|---|
| `DB_HOST` | `mysql` | 主机 |
| `DB_PORT` | `3306` | 端口 |
| `DB_USER` | `root` | **生产建议改为受限账号**（仅 `summerain.*` 权限） |
| `DB_PASSWORD` | *(空)* | 必填 |
| `DB_NAME` | `summerain` | 库名 |
| `DB_MAX_OPEN_CONNS` | `8` | 数据库最大打开连接数 |
| `DB_MAX_IDLE_CONNS` | `4` | 数据库最大空闲连接数，不得超过打开连接数 |
| `DB_CONN_MAX_LIFETIME` | `30m` | 单连接最大复用时间 |

> DSN：`user:pass@tcp(host:port)/db?charset=utf8mb4&parseTime=True&loc=Local`

### 3.3 缓存（Redis）

| 变量 | 默认 | 说明 |
|---|---|---|
| `REDIS_ADDR` | `redis:6379` | 地址 |
| `REDIS_PASSWORD` | *(空)* | 内网可空；跨网建议设置 |
| `REDIS_DB` | `0` | 库序号 |
| `REDIS_POOL_SIZE` | `8` | Redis 客户端连接池上限 |

Compose 将 Redis 数据上限设为 `128mb`（容器上限 `192m`）并使用 `noeviction`；达到上限时写入会显式失败，避免缓存静默淘汰限流/防重放状态或触发容器 OOM。

### 3.4 图片处理（imgproxy）

| 变量 | 默认 | 说明 |
|---|---|---|
| `IMGPROXY_URL` | `http://imgproxy:8080` | 内部地址 |
| `IMGPROXY_KEY` | *(空)* | hex，**必填**（启用签名） |
| `IMGPROXY_SALT` | *(空)* | hex，**必填** |
| `IMGPROXY_PUBLIC_URL` | `/img` | 对外签名 URL 前缀 |
| `IMGPROXY_WORKERS` | `2` | 与 V2 发布 Worker 对齐的处理并发上限 |

### 3.5 存储

| 变量 | 默认 | 说明 |
|---|---|---|
| `STORAGE_PATH` | `/data/images` | 图片持久化目录（V1 original/thumbnail/processed 与 V2 固定变体） |
| `TEMP_PATH` | `/data/images/.staging` | V1 临时处理目录；Compose 与 V2 共用同一受限暂存卷 |
| `V2_STAGING_PATH` | `<STORAGE_PATH>/.staging` | V2 上传暂存目录，必须是 `STORAGE_PATH` 的子目录以支持原子固化 |
| `DISK_SOFT_LIMIT_PERCENT` | `80` | 超过该使用率后拒绝创建新的 V2 上传会话 |
| `DISK_HARD_LIMIT_PERCENT` | `90` | 超过该使用率后拒绝继续写入上传部件或发布产物 |

### 3.6 V2 上传与发布

| 变量 | 默认 | 说明 |
|---|---|---|
| `V2_UPLOAD_ENABLED` | `true` | 启用固定配方的 V2 会话式上传；关闭后才开放 V1 multipart 上传 |
| `V2_RECIPE_VERSION` | `2.0.0` | 客户端与服务端必须完全一致的处理配方版本 |
| `V2_MAX_PART_BYTES` | `67108864` | 单个 WebP 部件上限（64 MiB） |
| `V2_MAX_PIXELS` | `50000000` | 源图与部件像素上限（50 MP） |
| `V2_SESSION_TTL` | `30m` | 未完成上传会话有效期 |
| `V2_GLOBAL_UPLOAD_CONCURRENCY` | `8` | 单后端实例同时接收部件的全局上限 |
| `V2_PER_USER_UPLOAD_CONCURRENCY` | `4` | 单用户同时接收部件上限 |
| `V2_WATERMARK_CONCURRENCY` | `2` | 发布/水印 Worker 数；3 核 4 GB 共享主机的上限 |
| `V2_JOB_POLL_INTERVAL` | `1s` | 发布任务轮询间隔 |
| `V2_JOB_LEASE` | `2m` | 发布任务租约；Worker 会续租并使用 fencing token 提交 |

浏览器上传流水线并发为 2，但图片解码与编码串行；活跃服务端会话限制为 4，为其他标签页和恢复请求预留后端容量。服务端发布与 imgproxy 默认均为 2 个 Worker，相同水印快照可并行处理；若同机其他组件出现持续 CPU 或内存压力，应将两个并发值一起降为 1。中间 `publish_source` 和会话暂存文件在发布完成后删除。

### 3.7 CDN 与持久化 Outbox

| 变量 | 默认 | 说明 |
|---|---|---|
| `CDN_PUBLIC_BASE_URL` | *(空)* | 启用 purge 投递时必填的公开图片基地址 |
| `CLOUDFLARE_ZONE_ID` / `CLOUDFLARE_API_TOKEN` | *(空)* | Cloudflare purge 凭据，必须成对配置 |
| `CDN_PURGE_WEBHOOK_URL` / `CDN_PURGE_WEBHOOK_TOKEN` | *(空)* | 非 Cloudflare CDN 的通用 purge webhook |
| `OUTBOX_BATCH_SIZE` | `10` | 每轮领取的持久化事件数 |
| `OUTBOX_POLL_INTERVAL` | `2s` | Outbox 轮询间隔 |
| `OUTBOX_LEASE` | `3m` | 事件投递租约 |
| `CDN_PURGE_REQUESTS_PER_SECOND` | `4` | CDN purge 请求速率上限 |
| `CDN_PURGE_REQUEST_TIMEOUT` | `15s` | 单次 purge 超时 |

### 3.8 人机验证（可插拔，可选）

`CAPTCHA_PROVIDER` ∈ `none`(默认) | `recaptcha` | `turnstile` | `geetest_v4`，admin 亦可经 `/admin/configs`（键 `captcha_provider`）覆盖。

| 变量 | 默认 | 说明 |
|---|---|---|
| `CAPTCHA_PROVIDER` | *(空，派生)* | 空 + `RECAPTCHA_ENABLED=true` → `recaptcha`，否则 `none` |
| `RECAPTCHA_ENABLED` | `false` | reCAPTCHA 总开关（向后兼容） |
| `RECAPTCHA_SITE_KEY` / `RECAPTCHA_SECRET` | *(空)* | reCAPTCHA 公钥/密钥 |
| `RECAPTCHA_MIN_SCORE` | `0.5` | v3 最低分数 |
| `RECAPTCHA_VERIFY_URL` | `https://www.recaptcha.net/recaptcha/api/siteverify` | 校验地址（大陆友好镜像） |
| `RECAPTCHA_FAIL_CLOSED` | `true` | 上游不可用时是否拒绝 |
| `RECAPTCHA_ALLOWED_HOSTNAMES` | *(空)* | 合法域名（逗号分隔） |
| `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET` | *(空)* | Cloudflare Turnstile |
| `GEETEST_CAPTCHA_ID` / `GEETEST_CAPTCHA_KEY` | *(空)* | 极验 v4 |

> `provider=none` 时不校验、前端不加载脚本。`/api/v1/public/config` 下发当前 provider 与对应公钥。

> 默认 `CROSS_ORIGIN_ISOLATION=true` 时不支持 `geetest_v4`：其外部脚本资源会被 COEP 拦截，服务端也会拒绝启动配置或后台切换到该 provider。请使用 `none`、`recaptcha` 或 `turnstile`。只有显式关闭跨源隔离后才可使用 GeeTest，但这会失去 V2 处理 50MP 大图所需的隔离 wasm-vips 路径。

---

## 4. 架构与部署

### 4.1 请求链路

```
用户 ──HTTPS──> Cloudflare ──> nginx(:443) ──HTTP──> backend(:8080, 127.0.0.1)
                                       └─ TLS 终止 / 限速 / 真实 IP 透传
backend ──> MySQL / Redis / imgproxy（Docker 内网）
```

### 4.2 nginx 关键配置（生产）

- `set_real_ip_from <Cloudflare 段>; real_ip_header CF-Connecting-IP;` —— 还原真实客户端 IP。
- `proxy_set_header X-Forwarded-For $remote_addr;` —— **覆盖**而非追加，防 XFF 伪造。
- `location = /metrics { return 404; }` —— 收敛 Prometheus 指标暴露面。
- `client_max_body_size 20m;` —— 与单次上传体积上限匹配。
- 安全头：`HSTS` / `X-Content-Type-Options` / `X-Frame-Options` / `Content-Security-Policy`。

### 4.3 容器编排（docker-compose）

四个服务，均建议仅内网通信：

| 服务 | 端口 | 说明 |
|---|---|---|
| `backend` | 仅绑 `127.0.0.1:8080` | 应用，**非 root（UID 10001）** |
| `mysql` | 不发布 | 内网 |
| `redis` | 不发布 | 内网 |
| `imgproxy` | 不发布 | 挂载图片卷只读 |

> 生产密钥放在受 Git 忽略的 `backend/.env`（0600）。部署命令必须同时传入 `--env-file backend/.env`，确保 Compose 插值与后端容器使用同一份配置。

---

## 5. 运维手册

### 5.1 健康检查

| 端点 | 含义 |
|---|---|
| `GET /health` | 进程存活 → `{"status":"ok"}` |
| `GET /ready` | 校验 DB + Redis 连通，不可用返回 503 |
| `GET /metrics` | Prometheus 指标（生产应在 nginx 层限制访问） |

### 5.2 后台 Worker

启动时由 `worker.Manager` 并发运行（`internal/worker/`）：

| Worker | 周期 | 职责 |
|---|---|---|
| `heartbeat` | 5 分钟 | 心跳超时的设备会话置过期 |
| `view_flusher` | 60 秒 | 将 Redis `views:*` 计数落库 |
| `cleanup` | 1 小时 | 清过期会话/CSRF/访问令牌/失败上传记录/孤儿临时文件 |
| `v2_publish` | 持续、默认并发 2 | 从 `publish_source` 生成最终发布图并应用水印 |
| `v2_cleanup` | 持续增量 | 回收过期会话、暂存目录和孤儿 V2 文件 |
| `outbox` | 默认 2 秒 | 投递 CDN purge 与本地/R2 物理删除事件 |
| `user_deletion` | 5 分钟 | 以可恢复的小批次执行到期账号删除 |

所有 worker 带 `recover`，单次 panic 不影响整体。

### 5.3 日志

- 应用日志：`docker logs summerain-backend`（标准输出）。
- nginx：`/var/log/nginx/your-domain.*.log`。

### 5.4 镜像更新 / 回滚

```bash
# 升级到 GitHub Actions 已发布的新精确版本
# 编辑 backend/.env，将 DOCKER_IMAGE 改为 jaykserks/summerain:v2.0.1
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull backend
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build backend

# 回滚到上一个已知正常的不可变版本
# 编辑 backend/.env，将 DOCKER_IMAGE 改回 jaykserks/summerain:v2.0.0
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull backend
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build backend
```

需要 digest 级固定时，`DOCKER_IMAGE` 可写成 `jaykserks/summerain@sha256:<oci-index-digest>`。多架构部署应固定 OCI index / manifest-list digest；不要把只属于 `amd64` 或 `arm64` 的子 manifest digest 复用到另一架构。

> 切换为非 root 镜像时，需先对数据卷 `chown -R 10001:10001`（命名卷首次创建会继承镜像内 `/data` 属主）。

### 5.5 数据库账号（最小权限）

生产应为应用建受限账号：

```sql
CREATE USER 'image_gallery'@'%' IDENTIFIED BY '<强随机>';
GRANT SELECT,INSERT,UPDATE,DELETE,CREATE,DROP,ALTER,INDEX,REFERENCES,
      CREATE TEMPORARY TABLES,LOCK TABLES,EXECUTE,CREATE VIEW,SHOW VIEW,TRIGGER
  ON summerain.* TO 'image_gallery'@'%';
FLUSH PRIVILEGES;
```

然后将 `DB_USER`/`DB_PASSWORD` 指向该账号，避免应用持有全库 root。

---

## 6. 使用指南

> 完整请求/响应字段见 [`API.md`](./API.md)。以下为日常使用要点。

### 6.1 账号

- **注册**：仅限 Web 端（`POST /api/v1/auth/register`），用户名 3–50、邮箱、密码 8–72。注册**不**自动登录。
- **登录**：`POST /api/v1/auth/login`，成功设置 `__Host-session_token`（30 天）+ `__Host-csrf_token` Cookie。
- **改密**：`PATCH /api/v1/user/password`，成功后**清除该用户所有会话**（强制重登）并发通知。

### 6.2 上传与外链

1. Web 端接受不超过 15 MiB、50 MP 的静态 JPG/JPEG、PNG、BMP、WebP、AVIF；动图在 V2 首发版中拒绝。
2. 浏览器为每张图片生成 `master`（原分辨率、Q80）、`gallery`（400x400、Q60）、`admin`（120x160、Q60）与 `publish_source`（最长边 2048、Q80），再通过 `/api/v1/uploads/*` 分部件上传。
3. 后端固化 `master`、`gallery`、`admin`，后台从 `publish_source` 生成可带水印的 `publish`，随后删除 `publish_source` 与会话中间文件。Image Management 将 120x160 的 `admin` 文件以 CSS 60x80 显示，保留 2x 像素密度。
4. V2 发布直链为 `/i/<asset_link>.webp`，固定变体为 `/i/<asset_link>/{master|gallery|admin|publish}.webp`；查询参数不会生成额外 V2 尺寸。
5. Web 端先读取 `/api/v1/uploads/recipe` 的 `v2_enabled` 能力位；只有 `V2_UPLOAD_ENABLED=false` 时才跳过客户端预处理，并通过 `POST /api/v1/images/` 使用 V1 multipart 兼容上传。V1 任意尺寸等动态转码使用有界临时文件，不持久化为访问缓存。
6. 相同内容按 SHA-256 去重存储，并由 `reference_count` 管理物理文件生命周期。

主服务不提供历史图片批量迁移接口。未分类 V1 图片在兼容期内优先读取安全的本地路径，仅当本地原图不存在且当前 R2 target 完整可用时才尝试该精确 target；endpoint/bucket 在仍有未分类历史记录时不可切换。历史数据的校验、断点、审计和回滚由后续独立仓库迁移工具负责。

### 6.3 公开 / 私有

- 每张图 `visibility` ∈ `public` / `private`。
- **私有图**需访问令牌：query `?token=`、头 `X-Image-Token` 或 `Authorization: Bearer`。
- 生成令牌：`POST /api/v1/images/:id/tokens`（有效期 10 分钟～3 天；明文仅向 owner/admin 的签发响应和图片详情返回）。
- 私有→公开切换会**自动撤销该图所有令牌**。

### 6.4 多端

- Web：Cookie 鉴权，写操作须带 `X-CSRF-Token` 头。
- 设备（android/windows）：`device-login` 取 `identity_token`（90 天）→ `device-bootstrap`（nonce 防重放）换 `session_token`（15 分钟，靠心跳续期）。每平台最多 3 台设备。

### 6.5 后台（管理员）

需 `role=admin` 且 `platform=web`，全组带 CSRF：
- 用户列表/改状态（`suspended` 会强制全端下线）
- 系统统计、系统配置（如水印 `watermark_enabled/text/position/opacity`）

---

## 7. 限制与阈值速查

| 项目 | 值 | 来源 |
|---|---|---|
| V2 源文件上限 | 15 MiB | `frontend/src/features/images/pages/Upload.tsx` |
| V2 单图像素上限 | 50 MP | `config.go` / `v2_upload_types.go` |
| V2 固定访问变体 | `master`、400x400 `gallery`、120x160 `admin`、最长边 2048 `publish` | `v2_upload_types.go` |
| V1 multipart 上限 | 单文件 10 MiB、单请求 ≤20 个文件 | `image_service.go` |
| 默认存储配额 | 500 MiB（524288000 bytes） | `model.User` |
| 配额预警阈值 | 90% | `image_service.go` |
| 图片短链 | V2 为 12 位 hex；V1 默认 12 位，连续冲突后回退为 16 位 | `generateUniqueLink` |
| Web 会话 | 30 天 | `auth_service.go` |
| CSRF 有效期 | 24 小时（滑动续期） | `auth_service.go` |
| 设备 identity | 90 天 | `auth_service.go` |
| 设备 session | 15 分钟（心跳续期，宽限 600s） | `auth_service.go` / `model.Session` |
| 每平台设备数 | ≤ 3 | `auth_service.go` |
| 登录限流 | IP 5 次/15 分钟；用户名 3 次/15 分钟 | `auth_service.go` |
| Bootstrap 限流 | 10 次/分钟 | `auth_service.go` |
| 访问令牌有效期 | 10 分钟～3 天 | `image_service.go` |
| V1 动态转码尺寸参数 | `w/h` ≤ 4096 | `public_handler.go` |
| V2 上传格式 | 静态 jpg/jpeg/png/bmp/webp/avif | `sniff.ts` / `v2_upload_types.go` |

---

## 8. 安全要点

- **认证**：会话令牌仅存 SHA256 哈希；`__Host-` 前缀 + `SameSite=Strict` + `Secure` + `HttpOnly`；CSRF 双提交（Bearer 请求跳过）。
- **密码**：bcrypt（DefaultCost）。
- **路径**：`NoRoute` 静态服务做 `filepath.Clean` + 基目录前缀校验，防穿越。
- **限流**：登录/Bootstrap 基于 IP+用户名；生产依赖 nginx 限速黑名单为主防线。
- **图片**：扩展名 + MIME 嗅探双重校验；SVG 强制 `application/octet-stream` 下载（防 XSS）；私密图 `no-store`。
- **容器**：非 root（UID 10001）；MySQL/Redis 不发布端口；DB 最小权限账号。
- **传输**：Cloudflare Full (Strict) + nginx HSTS。

> 若 `GIN_MODE=debug`，错误响应可能回显内部细节，生产务必 `release`。

---

*文档依据：`cmd/server/main.go`、`internal/config/config.go`、`internal/{handler,service,middleware,model,worker}/*`、`Dockerfile`、`docker-compose*.yml`。*
