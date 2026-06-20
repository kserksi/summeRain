# ImgCloud 部署与使用文档

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

ImgCloud 是一个自托管的图床 / 图片相册服务。

- **后端**：Go 1.23 + Gin + GORM（MySQL）+ Redis + imgproxy
- **前端**：Flutter Web（构建产物以只读卷挂载，由后端 `NoRoute` 兜底托管）
- **核心能力**：注册登录、图片上传/管理、公开/私有可见性、私密图访问令牌、多端会话（Web Cookie + 设备 Token）、通知、后台管理、浏览量统计、缩略图/格式转换。

### 技术栈职责

| 组件 | 作用 |
|---|---|
| `backend` (Go) | 业务 API、图片直链服务 `/i/:link`、SPA 兜底 |
| MySQL | 用户/图片/会话/通知/配置等持久化 |
| Redis | 限流计数、nonce 防重放、浏览量缓冲 |
| imgproxy | 图片缩略、格式转换、水印（本地文件系统源） |
| nginx（前置） | TLS 终止、反代、限速、真实 IP 透传 |

---

## 2. 快速开始

### 2.1 本地开发

```bash
cd backend
cp .env.example .env          # 按需修改密钥
docker compose up -d          # 启动 mysql / redis / imgproxy / backend
```

- 应用监听 `:8080`，健康检查 `GET http://localhost:8080/health` → `{"status":"ok"}`。
- 首次启动会自动执行 `AutoMigrate` 建表。

> 本地 `http://localhost` 下 `__Host-` 前缀 Cookie 会被浏览器拒绝设置（要求 HTTPS + 同源）。本地联调建议用自签证书或同源代理。

### 2.2 生产部署（Docker）

```bash
# 构建镜像（多阶段：builder 编译 Go，运行时 alpine + 非 root 用户）
docker build -t summerain:latest .

# 通过 docker compose 启动全部依赖
docker compose -f docker-compose.deploy.yml up -d
```

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

### 3.2 数据库（MySQL）

| 变量 | 默认 | 说明 |
|---|---|---|
| `DB_HOST` | `mysql` | 主机 |
| `DB_PORT` | `3306` | 端口 |
| `DB_USER` | `root` | **生产建议改为受限账号**（仅 `image_gallery.*` 权限） |
| `DB_PASSWORD` | *(空)* | 必填 |
| `DB_NAME` | `image_gallery` | 库名 |

> DSN：`user:pass@tcp(host:port)/db?charset=utf8mb4&parseTime=True&loc=Local`

### 3.3 缓存（Redis）

| 变量 | 默认 | 说明 |
|---|---|---|
| `REDIS_ADDR` | `redis:6379` | 地址 |
| `REDIS_PASSWORD` | *(空)* | 内网可空；跨网建议设置 |
| `REDIS_DB` | `0` | 库序号 |

### 3.4 图片处理（imgproxy）

| 变量 | 默认 | 说明 |
|---|---|---|
| `IMGPROXY_URL` | `http://imgproxy:8080` | 内部地址 |
| `IMGPROXY_KEY` | *(空)* | hex，**必填**（启用签名） |
| `IMGPROXY_SALT` | *(空)* | hex，**必填** |
| `IMGPROXY_PUBLIC_URL` | `/img` | 对外签名 URL 前缀 |

### 3.5 存储

| 变量 | 默认 | 说明 |
|---|---|---|
| `STORAGE_PATH` | `/data/images` | 图片持久化目录（original/thumbnail/processed 子目录） |
| `TEMP_PATH` | `/data/temp` | 上传处理临时目录（worker 每小时清理 1h 前文件） |

### 3.6 人机验证（可插拔，可选）

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

> 生产密钥建议放 `.env.backend`（0600），compose 用 `env_file` 引用，避免明文入库。

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

所有 worker 带 `recover`，单次 panic 不影响整体。

### 5.3 日志

- 应用日志：`docker logs summerain-backend`（标准输出）。
- nginx：`/var/log/nginx/image.kserks.org.*.log`。

### 5.4 镜像更新 / 回滚

```bash
# 升级前先打 rollback 标签
docker tag summerain:latest summerain:rollback
docker build -t summerain:latest .
docker compose up -d backend      # 自动重建

# 回滚
docker tag summerain:rollback summerain:latest
docker compose up -d backend
```

> 切换为非 root 镜像时，需先对数据卷 `chown -R 10001:10001`（命名卷首次创建会继承镜像内 `/data` 属主）。

### 5.5 数据库账号（最小权限）

生产应为应用建受限账号：

```sql
CREATE USER 'image_gallery'@'%' IDENTIFIED BY '<强随机>';
GRANT SELECT,INSERT,UPDATE,DELETE,CREATE,DROP,ALTER,INDEX,REFERENCES,
      CREATE TEMPORARY TABLES,LOCK TABLES,EXECUTE,CREATE VIEW,SHOW VIEW,TRIGGER
  ON image_gallery.* TO 'image_gallery'@'%';
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

1. `POST /api/v1/images/`（multipart，字段 `images` 可多个 ≤20，可选 `visibility`）。
2. 返回每张图的 `unique_link`，拼出直链：`/i/<unique_link>`。
3. 带扩展名请求转码：`/i/<link>.webp?w=300&h=200&q=80`（`w/h` 上限 4096）。
4. 相同内容（SHA256）自动去重存储（`reference_count` 引用计数）。

### 6.3 公开 / 私有

- 每张图 `visibility` ∈ `public` / `private`。
- **私有图**需访问令牌：query `?token=`、头 `X-Image-Token` 或 `Authorization: Bearer`。
- 生成令牌：`POST /api/v1/images/:id/tokens`（有效期 0.17–87600 小时；明文仅返回一次）。
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
| 单文件上限 | 10 MB | `image_service.go` |
| 单次上传文件数 | ≤ 20 | `image_service.go` |
| 默认存储配额 | 1 GB（1073741824） | `model.User` |
| 配额预警阈值 | 90% | `image_service.go` |
| 图片短链 | 12 位 hex（48 bit 熵） | `generateUniqueLink` |
| Web 会话 | 30 天 | `auth_service.go` |
| CSRF 有效期 | 24 小时（滑动续期） | `auth_service.go` |
| 设备 identity | 90 天 | `auth_service.go` |
| 设备 session | 15 分钟（心跳续期，宽限 600s） | `auth_service.go` / `model.Session` |
| 每平台设备数 | ≤ 3 | `auth_service.go` |
| 登录限流 | IP 5 次/15 分钟；用户名 3 次/15 分钟 | `auth_service.go` |
| Bootstrap 限流 | 10 次/分钟 | `auth_service.go` |
| 访问令牌有效期 | 0.17–87600 小时；描述 ≤200 字 | `image_service.go` |
| 转码尺寸参数 | `w/h` ≤ 4096 | `public_handler.go` |
| 支持扩展名 | png/jpg/jpeg/webp/gif | `image_service.go` |

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
