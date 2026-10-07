# summeRain 部署与使用文档

> 本文档覆盖**部署、配置、运维与日常使用**。接口契约详见 [`API.md`](./API.md)。

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

summeRain 是一个自托管的图片托管与相册服务。

- **后端：** Go 1.24+（CI 与容器构建使用 Go 1.26.5）+ Gin + GORM（MySQL）+ Redis；
  imgproxy 提供 V1 兼容路径和 V2 发布水印。
- **前端：** React + Vite（构建产物由 Go 服务同源托管）。
- **核心能力：** 注册登录、图片上传与管理、公开/私有可见性、私密图片访问令牌、
  多端会话（Web Cookie + 设备 Token）、通知、后台管理、浏览量统计、缩略图和格式转换。

### 技术栈职责

| 组件 | 作用 |
|---|---|
| `backend`（Go） | 业务 API、图片直链服务 `/i/:link`、SPA 兜底 |
| MySQL | 用户、图片、会话、通知、配置等持久化数据 |
| Redis | 限流计数、nonce 防重放、浏览量缓冲 |
| imgproxy | V1 动态兼容处理与 V2 发布水印（本地文件系统源） |
| nginx（前置） | TLS 终止、反向代理、限速、真实 IP 透传 |

---

## 2. 快速开始

### 2.1 本地开发

本地开发环境（MySQL、Redis、imgproxy、后端与前端）的启动方式见根 README 的
[WSL 快速开始](../README.md)。

### 2.2 生产部署（GitHub Actions 镜像）

准备一个部署目录，放入 Compose 文件、环境变量文件与图片配方：

```text
/srv/summerain/
|-- docker-compose.yml    backend/docker-compose.deploy.yml 的副本
|-- .env                  backend/.env.example 的副本，权限 0600
`-- config/
    `-- image-recipe.json backend/internal/config/image-recipe.json 的副本
```

```bash
# 编辑 .env，设置精确的 DOCKER_IMAGE、数据库密码、Cookie secret 与 imgproxy 密钥。
# 配方以只读方式覆盖镜像内置配方；编辑它可调整变体、阈值与接受的格式。
docker compose --env-file .env pull
docker compose --env-file .env up -d --no-build
```

应用镜像只由 GitHub Actions 构建并同步到 Docker Hub / GHCR；部署机不构建应用镜像。
`backend/docker-compose.deploy.yml` 会拒绝缺失的 `DOCKER_IMAGE`，生产环境应使用精确
SemVer 标签或 OCI 多架构索引 digest。缺少 `config/image-recipe.json` 时 Compose 会拒绝启动。

生产前置 nginx 反向代理至 `127.0.0.1:8080`，并经 Cloudflare 暴露 443（详见第 4 节）。

---

## 3. 配置参考（环境变量）

**带默认值**的项目可不显式设置。所有环境变量仅在启动时读取一次，修改后需要重启。

### 3.1 服务

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SERVER_PORT` | `8080` | HTTP 监听端口 |
| `GIN_MODE` | `debug` | `debug` / `release`（生产使用 `release`） |
| `COOKIE_SECRET` | `change-me-in-production` | 预留（当前会话使用不透明随机串，未实际签名；仍建议设置强值） |
| `CROSS_ORIGIN_ISOLATION` | `true` | 下发 COOP/COEP，启用 wasm-vips 大图路径；禁用后，超过浏览器原生安全阈值的大图无法处理 |
| `GOMEMLIMIT` | `512MiB`（Compose） | 将 Go 堆目标限制在 640 MiB 容器内；由 Compose 配置注入，应用本身不读取该变量 |

`CROSS_ORIGIN_ISOLATION=true` 时，第三方脚本、字体和图片必须通过 CORS 或
`Cross-Origin-Resource-Policy` 明确允许嵌入，否则浏览器会按 COEP 拦截。与 CAPTCHA
的交互见 3.8 节。

### 3.2 数据库（MySQL）

| 变量 | 默认值 | 说明 |
|---|---|---|
| `DB_HOST` | `mysql` | 主机 |
| `DB_PORT` | `3306` | 端口 |
| `DB_USER` | `root` | **生产建议改为受限账号**（仅授予 `summerain.*` 权限） |
| `DB_PASSWORD` | *（空）* | 必填 |
| `DB_NAME` | `summerain` | 数据库名 |
| `DB_MAX_OPEN_CONNS` | `8` | 数据库最大打开连接数 |
| `DB_MAX_IDLE_CONNS` | `4` | 数据库最大空闲连接数，不得超过打开连接数 |
| `DB_CONN_MAX_LIFETIME` | `30m` | 单连接最大复用时间 |

> DSN：`user:pass@tcp(host:port)/db?charset=utf8mb4&parseTime=True&loc=Local`

### 3.3 缓存（Redis）

| 变量 | 默认值 | 说明 |
|---|---|---|
| `REDIS_ADDR` | `redis:6379` | 地址 |
| `REDIS_PASSWORD` | *（空）* | 内网可留空；跨网络建议设置 |
| `REDIS_DB` | `0` | 数据库编号 |
| `REDIS_POOL_SIZE` | `8` | Redis 客户端连接池上限 |

Compose 将 Redis 数据上限设为 `128mb`（容器上限 `192m`）并使用 `noeviction`；达到上限
时写入会明确失败，以避免缓存静默淘汰限流/防重放状态或触发容器 OOM。

### 3.4 图片处理（imgproxy）

| 变量 | 默认值 | 说明 |
|---|---|---|
| `IMGPROXY_URL` | `http://imgproxy:8080` | 内部地址 |
| `IMGPROXY_KEY` | *（空）* | hex，**必填**（启用签名） |
| `IMGPROXY_SALT` | *（空）* | hex，**必填** |
| `IMGPROXY_PUBLIC_URL` | `/img` | 对外签名 URL 前缀 |
| `IMGPROXY_WORKERS` | `2` | 与 V2 发布 Worker 对齐的处理并发上限 |

### 3.5 存储

| 变量 | 默认值 | 说明 |
|---|---|---|
| `STORAGE_PATH` | `/data/images` | 图片持久化目录（V1 original/thumbnail/processed 与 V2 固定变体） |
| `TEMP_PATH` | `/data/images/.staging` | V1 临时处理目录；Compose 与 V2 共用同一受限暂存卷 |
| `V2_STAGING_PATH` | `<STORAGE_PATH>/.staging` | V2 上传暂存目录，必须是 `STORAGE_PATH` 的子目录以支持原子固化 |
| `DISK_SOFT_LIMIT_PERCENT` | `80` | 超过该使用率后拒绝创建新的 V2 上传会话 |
| `DISK_HARD_LIMIT_PERCENT` | `90` | 超过该使用率后拒绝继续写入上传部件或发布产物 |

### 3.6 V2 上传与发布

| 变量 | 默认值 | 说明 |
|---|---|---|
| `V2_UPLOAD_ENABLED` | `true` | 启用固定配方的 V2 会话式上传；关闭后才开放 V1 multipart 上传 |
| `IMAGE_RECIPE_FILE` | `/app/config/image-recipe.json` | 固定图片策略文件：变体尺寸、像素与体积上限、可接受的源格式 |
| `IMAGE_RECIPE_REQUIRED` | `false` | 配方文件缺失时拒绝启动，而不是回退到内置默认配方 |
| `V2_SESSION_TTL` | `30m` | 未完成上传会话的有效期 |
| `V2_GLOBAL_UPLOAD_CONCURRENCY` | `8` | 单个后端实例同时接收部件的全局上限 |
| `V2_PER_USER_UPLOAD_CONCURRENCY` | `4` | 单个用户同时接收部件的上限 |
| `V2_WATERMARK_CONCURRENCY` | `2` | 发布/水印 Worker 数 |
| `V2_JOB_POLL_INTERVAL` | `1s` | 发布任务轮询间隔 |
| `V2_JOB_LEASE` | `2m` | 发布任务租约；Worker 会续租，并使用 fencing token 提交 |
| `CLIENT_UPLOAD_PIPELINE_CONCURRENCY` | `2` | `/api/v1/uploads/recipe` 下发的浏览器上传流水线并发提示 |
| `CLIENT_ACTIVE_SESSION_CONCURRENCY` | `4` | 并发上传会话提示值，不得超过 `V2_MAX_ACTIVE_SESSIONS_PER_USER` |
| `CLIENT_MAX_NATIVE_CONCURRENCY` | `2` | 原生（Canvas/Pica）处理器并发提示 |
| `V2_MAX_SOURCE_BYTES` | `15728640` | 处理前浏览器接受的源文件体积上限（15 MiB） |

图片配方属于服务端策略，而不是环境变量：变体尺寸、像素与体积上限、可接受的源格式，以及
浏览器必须发送的 `recipe_version` 都来自 `IMAGE_RECIPE_FILE`。容器会把默认配方写入
`/app/config/image-recipe.json`，二进制文件也内置了同一份默认配方，且 Compose 会将部署目录中的
`config/image-recipe.json` 以只读方式覆盖到该路径。修改配方需要重启并提升
`recipe_version`，配方永远不会通过管理员 API 暴露。

`/api/v1/uploads/recipe` 下发的客户端提示用于让浏览器自行决定流水线规模；解码与编码
串行执行，实际并发取服务器提示、设备能力与浏览器上限的最小值。服务端仍会独立校验每个
请求。中间 `publish_source` 和会话暂存文件会在发布完成后删除。主机持续出现 CPU 或内存
压力时，将 `V2_WATERMARK_CONCURRENCY` 与 `IMGPROXY_WORKERS` 降为 1。

### 3.7 CDN 与持久化 Outbox

| 变量 | 默认值 | 说明 |
|---|---|---|
| `CDN_PUBLIC_BASE_URL` | *（空）* | 启用 purge 投递时必填的公开图片基地址 |
| `CLOUDFLARE_ZONE_ID` / `CLOUDFLARE_API_TOKEN` | *（空）* | Cloudflare purge 凭据，必须成对配置 |
| `CDN_PURGE_WEBHOOK_URL` / `CDN_PURGE_WEBHOOK_TOKEN` | *（空）* | 非 Cloudflare CDN 的通用 purge webhook |
| `OUTBOX_BATCH_SIZE` | `10` | 每轮领取的持久化事件数 |
| `OUTBOX_POLL_INTERVAL` | `2s` | Outbox 轮询间隔 |
| `OUTBOX_LEASE` | `3m` | 事件投递租约 |
| `CDN_PURGE_REQUESTS_PER_SECOND` | `4` | CDN purge 请求速率上限 |
| `CDN_PURGE_REQUEST_TIMEOUT` | `15s` | 单次 purge 超时 |

### 3.8 人机验证（可插拔，可选）

`CAPTCHA_PROVIDER` 取值为 `none`（默认）、`recaptcha`、`turnstile` 或
`geetest_v4`；管理员也可通过 `/admin/configs`（键 `captcha_provider`）覆盖。

| 变量 | 默认值 | 说明 |
|---|---|---|
| `CAPTCHA_PROVIDER` | *（空，派生）* | 空值 + `RECAPTCHA_ENABLED=true` -> `recaptcha`，否则为 `none` |
| `RECAPTCHA_ENABLED` | `false` | reCAPTCHA 总开关（向后兼容） |
| `RECAPTCHA_SITE_KEY` / `RECAPTCHA_SECRET` | *（空）* | reCAPTCHA 公钥/密钥 |
| `RECAPTCHA_MIN_SCORE` | `0.5` | v3 最低分数 |
| `RECAPTCHA_VERIFY_URL` | `https://www.recaptcha.net/recaptcha/api/siteverify` | 校验地址（中国大陆友好镜像） |
| `RECAPTCHA_FAIL_CLOSED` | `true` | 上游不可用时是否拒绝 |
| `RECAPTCHA_ALLOWED_HOSTNAMES` | *（空）* | 合法域名（逗号分隔） |
| `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET` | *（空）* | Cloudflare Turnstile |
| `GEETEST_CAPTCHA_ID` / `GEETEST_CAPTCHA_KEY` | *（空）* | GeeTest v4 |

> `provider=none` 时不校验，前端也不加载脚本。`/api/v1/public/config` 会下发当前
> provider 与对应公钥。

> 默认 `CROSS_ORIGIN_ISOLATION=true` 时不支持 `geetest_v4`：其外部脚本资源会被
> COEP 拦截，服务端也会拒绝使用该配置启动或拒绝后台切换到该 provider。请使用
> `none`、`recaptcha` 或 `turnstile`。只有明确关闭跨源隔离后才可使用 GeeTest，
> 但这会失去 V2 处理 50MP 大图所需的隔离 wasm-vips 路径。

---

## 4. 架构与部署

### 4.1 请求链路

生产流量经 Cloudflare 进入，由 nginx 终止 TLS 后转发到 `127.0.0.1:8080` 的后端；
后端通过 Docker 内网访问 MySQL、Redis 与 imgproxy。完整拓扑图见根 README。

### 4.2 nginx 关键配置（生产）

- `set_real_ip_from <Cloudflare 段>; real_ip_header CF-Connecting-IP;`：还原真实客户端 IP。
- `proxy_set_header X-Forwarded-For $remote_addr;`：**覆盖**而不是追加，防止 XFF 伪造。
- `location = /metrics { return 404; }`：收敛 Prometheus 指标暴露面。
- `client_max_body_size 64m;`：容纳单次上传的最大请求体，即受配方 `max_part_bytes`（默认 64 MiB）约束的 V2 部件。V1 multipart 批量请求可能超过该值；使用 V1 批量上传时应上调此限制，后端仍会强制单文件 10 MiB、单请求最多 20 个文件。
- 安全头：`HSTS` / `X-Content-Type-Options` / `X-Frame-Options` /
  `Content-Security-Policy`。

### 4.3 容器编排（Docker Compose）

建议四个服务均仅在内网通信：

| 服务 | 端口 | 说明 |
|---|---|---|
| `backend` | 仅绑定 `127.0.0.1:8080` | 应用，**非 root（UID 10001）** |
| `mysql` | 不发布 | 内网 |
| `redis` | 不发布 | 内网 |
| `imgproxy` | 不发布 | 以只读方式挂载图片卷 |

> 生产密钥存放在部署目录的 `.env`（0600）中。部署命令必须同时传入
> `--env-file .env`，确保 Compose 插值和后端容器使用同一份配置。

后端以只读方式挂载 `config/image-recipe.json`。`image_storage` 命名卷保存全部持久化图片
文件，`mysql_data` 与 `redis_data` 卷分别保存数据库；升级前请备份 `mysql_data` 与
`image_storage`，并将体积很小的 `.env` 与 `config/` 一并归档。

---

## 5. 运维手册

### 5.1 健康检查

| 端点 | 含义 |
|---|---|
| `GET /health` | 进程存活 -> `{"status":"ok"}` |
| `GET /ready` | 检查 DB + Redis 连通性；不可用时返回 503 |
| `GET /metrics` | Prometheus 指标（生产环境应在 nginx 层限制访问） |

### 5.2 后台 Worker

启动时由 `worker.Manager` 并发运行（`internal/worker/`）：

| Worker | 周期 | 职责 |
|---|---|---|
| `heartbeat` | 5 分钟 | 将心跳超时的设备会话标记为过期 |
| `view-flusher` | 60 秒 | 将 Redis `views:*` 计数写入数据库 |
| `cleanup` | 1 小时 | 清理过期会话/CSRF/访问令牌/失败上传记录/孤儿临时文件 |
| `v2-publish` | 持续运行，默认并发 2 | 从 `publish_source` 生成最终发布图片并应用水印 |
| `v2-cleanup` | 持续增量 | 回收过期会话、暂存目录和孤儿 V2 文件 |
| `outbox` | 默认 2 秒 | 投递 CDN purge 与本地/R2 物理删除事件 |
| `user-deletion` | 5 分钟 | 以可恢复的小批次执行到期账号删除 |

所有 Worker 都带有 `recover`，单次 panic 不会影响整体。

### 5.3 日志

- 应用日志：`docker logs summerain-backend`（标准输出）。
- nginx：`/var/log/nginx/your-domain.*.log`。

### 5.4 镜像更新 / 回滚

```bash
# 升级到 GitHub Actions 已发布的新精确版本
# 编辑 .env，将 DOCKER_IMAGE 设为 jaykserks/summerain:<new-version>
docker compose --env-file .env pull backend
docker compose --env-file .env up -d --no-build backend

# 回滚到上一个已知正常的不可变版本
# 编辑 .env，将 DOCKER_IMAGE 改回 jaykserks/summerain:<previous-version>
docker compose --env-file .env pull backend
docker compose --env-file .env up -d --no-build backend
```

需要 digest 级固定时，可将 `DOCKER_IMAGE` 写为
`jaykserks/summerain@sha256:<oci-index-digest>`。多架构部署应固定 OCI index /
manifest-list digest。

> 切换至非 root 镜像时，需要先对数据卷执行 `chown -R 10001:10001`（命名卷首次创建
> 时会继承镜像内 `/data` 的所有者）。

### 5.5 数据库账号（最小权限）

生产环境应为应用创建受限账号：

```sql
CREATE USER 'image_gallery'@'%' IDENTIFIED BY '<strong-random-value>';
GRANT SELECT,INSERT,UPDATE,DELETE,CREATE,DROP,ALTER,INDEX,REFERENCES,
      CREATE TEMPORARY TABLES,LOCK TABLES,EXECUTE,CREATE VIEW,SHOW VIEW,TRIGGER
  ON summerain.* TO 'image_gallery'@'%';
FLUSH PRIVILEGES;
```

随后将 `DB_USER` / `DB_PASSWORD` 指向该账号，避免应用持有全库 root 权限。

---

## 6. 使用指南

> 完整请求/响应字段见 [`API.md`](./API.md)。以下为日常使用要点。

### 6.1 账号

- **注册：** 仅限 Web（`POST /api/v1/auth/register`）；用户名 3-50 个字符，另需邮箱和
  8-72 个字符的密码。注册后**不会**自动登录。
- **登录：** `POST /api/v1/auth/login`；成功后设置 `__Host-session_token`（30 天）和
  `__Host-csrf_token` Cookie。
- **修改密码：** `PATCH /api/v1/user/password`；成功后**清除该用户的所有会话**，
  强制重新登录，并发出通知。

### 6.2 上传与外链

1. Web 接受静态 JPG/JPEG、PNG、BMP、WebP、AVIF（上限见第 7 节），并拒绝动图。
2. 浏览器为每张图片生成固定配方变体（`master`、`gallery`、`admin` 和
   `publish_source`），然后通过 `/api/v1/uploads/*` 分部件上传；具体几何尺寸与质量
   取自服务端配方。
3. 后端固化 `master`、`gallery`、`admin`；后台从 `publish_source` 生成可带水印的
   `publish`，随后删除 `publish_source` 和会话中间文件。
4. 发布文件通过 `/i/<asset_link>.webp` 提供，固定变体位于 `/i/<asset_link>/` 下；
   具体路由见 API 参考。查询参数不会生成额外的 V2 尺寸。
5. Web 先读取 `/api/v1/uploads/recipe` 的 `v2_enabled` 能力位；只有
   `V2_UPLOAD_ENABLED=false` 时，才跳过客户端预处理，并通过
   `POST /api/v1/images/` 使用 V1 multipart 兼容上传。V1 任意尺寸等动态转码使用有界
   临时文件，不会持久化为访问缓存。V1 动态转换在固定并发窗口与短队列内运行；饱和时
   返回 `1003`(503) 与 `Retry-After`，不会无界排队。
6. 相同内容按 SHA-256 去重存储，并通过 `reference_count` 管理物理文件生命周期。

主服务不提供历史图片批量迁移接口。兼容期内，未分类 V1 图片优先读取安全的本地路径；
仅当本地原图不存在且当前 R2 target 完整可用时，才尝试该精确 target。仍有未分类历史
记录时，不得切换 endpoint/bucket。历史数据的校验、断点续传、审计和回滚由后续独立
仓库中的迁移工具负责。

### 6.3 公开 / 私有

- 每张图片的 `visibility` 为 `public` 或 `private`。
- **私密图片**需要访问令牌：query `?token=`、请求头 `X-Image-Token` 或
  `Authorization: Bearer`。
- 生成令牌：`POST /api/v1/images/:id/tokens`；有效期有界且可配置（见第 7 节）。明文只在
  owner/admin 的签发响应和图片详情中返回。
- 从私有切换为公开时，会**自动撤销该图片的所有令牌**。

### 6.4 多端

- Web：使用 Cookie 鉴权；写操作必须携带 `X-CSRF-Token` 请求头。
- 设备（android/windows）：通过 `device-login` 获取 `identity_token`（90 天），再通过
  `device-bootstrap`（nonce 防重放）换取 `session_token`（15 分钟，靠心跳续期）。
  每个平台最多 3 台设备。

### 6.5 后台（管理员）

要求 `role=admin` 且 `platform=web`，整组接口均要求 CSRF：

- 用户列表和状态修改（设置为 `suspended` 会强制所有设备下线）。
- 系统统计与系统配置（水印键 `watermark_enabled`、`watermark_text`、
  `watermark_position`、`watermark_opacity`、`watermark_size`、`watermark_color`）。

---

## 7. 限制与阈值速查

| 项目 | 值 |
|---|---|
| V2 源文件上限 | 15 MiB |
| V2 单图像素上限 | 50 MP |
| V2 固定访问变体 | `master`、`gallery`、`admin`、`publish`（固定配方；见根 README 管线表） |
| V1 multipart 上限 | 单文件 10 MiB、单请求不超过 20 个文件 |
| 默认存储配额 | 500 MiB（524288000 bytes） |
| 配额预警阈值 | 90% |
| 图片短链 | V2 为 12 位 hex；V1 默认 12 位，连续冲突后回退为 16 位 |
| Web 会话 | 30 天 |
| CSRF 有效期 | 24 小时（滑动续期） |
| 设备 identity | 90 天 |
| 设备 session | 15 分钟（心跳续期，宽限 600s） |
| 每平台设备数 | 不超过 3 |
| 登录限流 | IP 5 次/15 分钟；用户名 3 次/15 分钟 |
| Bootstrap 限流 | 10 次/分钟 |
| 访问令牌有效期 | 10 分钟至 3 天 |
| V1 动态转码尺寸参数 | `w/h` 不超过 4096 |
| V2 上传格式 | 静态 jpg/jpeg/png/bmp/webp/avif |

---

## 8. 安全要点

- **认证：** 会话令牌仅存储 SHA256 哈希；Cookie 使用 `__Host-` 前缀、
  `SameSite=Strict`、`Secure`、`HttpOnly`；CSRF 采用双提交（Bearer 请求跳过）。
- **密码：** bcrypt（DefaultCost）。
- **路径：** `NoRoute` 静态服务使用 `filepath.Clean` 和基目录前缀校验，防止路径穿越。
- **限流：** 登录/Bootstrap 基于 IP + 用户名；生产环境以 nginx 限速黑名单为主要防线。
- **图片：** 扩展名与 MIME 嗅探双重校验；SVG 强制以 `application/octet-stream`
  下载以防 XSS；私密图片使用 `no-store`。
- **容器：** 非 root（UID 10001）；MySQL/Redis 不发布端口；数据库使用最小权限账号。
- **传输：** Cloudflare Full (Strict) + nginx HSTS。

> 如果 `GIN_MODE=debug`，错误响应可能回显内部细节；生产环境必须使用 `release`。
