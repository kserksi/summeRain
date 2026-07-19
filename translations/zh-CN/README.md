# summeRain

> 一款自托管的图床与相册服务，具备资源感知的上传管线、固定图片变体、水印、私密分享，以及对现有 V1 图片的兼容能力。

[![CI 与 Docker](https://github.com/kserksi/summeRain/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/kserksi/summeRain/actions/workflows/ci.yml)
[![发布版本](https://img.shields.io/github/v/release/kserksi/summeRain)](https://github.com/kserksi/summeRain/releases)
[![Docker Hub](https://img.shields.io/docker/v/jaykserks/summerain?label=docker)](https://hub.docker.com/r/jaykserks/summerain)
[![许可证：Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://github.com/kserksi/summeRain/blob/main/LICENSE)

> **早期版本提示：** V2 仍处于早期发布阶段。上传协议、浏览器处理管线、数据库结构、兼容行为和运维默认值可能频繁变化。每次升级前请阅读完整变更日志，备份 MySQL 与图片卷，并在生产环境固定精确发布标签或 OCI 索引摘要。

[在线文档](https://summerain-1.gitbook.io/summerain/zh-cn/) | [V2.0.0 发布说明](./docs/releases/v2.0.0.md) | [Docker Hub](https://hub.docker.com/r/jaykserks/summerain) | [GHCR](https://github.com/kserksi/summeRain/pkgs/container/summerain)

## 概览

summeRain 采用模块化 Go 单体架构和 React 单页应用。Go 服务提供 `/api/v1/*` API，通过 `/i/*` 提供图片，并从同一源托管编译后的前端。MySQL 是应用与任务状态的权威数据源。Redis 提供有界缓存、限流、浏览量缓冲与设备重放防护。imgproxy 负责有界的 V1 动态转换，以及可选的 V2 水印阶段。

```text
浏览器 / Android / Windows
            |
            v
      Go + Gin (:8080)
        |-- /api/v1/* --> middleware --> handler --> service --> repository
        |-- /i/*
        |     |-- V2 --> 授权 --> 固定本地资源
        |     `-- V1 --> 已存储资源或有界 imgproxy 转换
        |-- 发布 worker --> 可选 imgproxy 水印 --> 本地发布资源
        `-- /*        --> backend/web (React SPA)
                                      |
                         +------------+------------+
                         |            |            |
                         v            v            v
                     MySQL 8.4     Redis 8    本地图片卷
```

## V2 图片管线

V2 将高开销的解码、缩放、格式转换和压缩工作移至浏览器。后端接收遵循固定配方并由清单声明的 WebP 部件，在将其流式写入暂存区的同时完成校验，并固化固定变体；不会仅为校验而重新读取上传内容。

| 资源 | 几何尺寸 | 质量 | 生命周期与用途 |
|---|---|---:|---|
| `master` | 方向校正后的原始尺寸 | 80 | 持久化全分辨率 WebP；owner/admin 可访问 |
| `gallery` | 400x400 cover 裁剪 | 60 | 持久化的“我的图片”和控制台预览 |
| `admin` | 120x160 cover 裁剪 | 60 | 持久化的图片管理预览，以 60x80 显示 |
| `publish_source` | 最长边不超过 2048 px | 80 | 服务端发布的临时输入 |
| `publish` | 从 `publish_source` 派生 | 80 | 持久化的分享资源，可带水印 |

发布完成后会删除 `publish_source` 和会话暂存数据。服务端仅对最终 `publish` 资源应用水印。V2 不会在首次访问时创建任意尺寸，从而避免让 V1 在热点资源突发时易受影响的无界缩略图处理。

### 上传行为

- 支持静态 JPEG、PNG、BMP、WebP 和 AVIF 输入。
- V2.0.0 不支持动图与 GIF 上传。
- 源文件上限为 15 MiB、50 MP。
- 浏览器图片处理并发为 1，上传管线并发为 2。
- 部件上传初始并发为 2，并可在能力充足的客户端自适应到 3。
- 上传会话支持断点续传和幂等操作，并持久化状态轮询；客户端轮询截止时间为 10 分钟，之后仍可恢复状态查询。
- 高容量浏览器路径使用 `wasm-vips`；不具备所需浏览器隔离或内存能力时，使用有界的 Canvas/Pica 路径。
- 现有 V1 图片仍可通过原短链读取，并保留有界动态转换能力。

## 功能

### 图片与存储

- 不可变、内容寻址的 `master`、`gallery`、`admin` 资源，以及按修订版本区分的 `publish` 资源。
- 服务端文本水印，可配置文本、位置、不透明度、尺寸与颜色。
- SHA-256 内容标识与采用引用计数的物理文件生命周期。
- V2 资源使用本地存储；V1 兼容路径根据存储沿袭信息从本地或 R2 读取。
- 通过持久 outbox 投递 CDN purge 与本地/R2 物理删除任务。
- 在待注销锁定期内流式生成账户归档，不在应用内存中组装 ZIP。
- 存储配额、通知、磁盘压力准入与有界清理。

### 账户、隐私与分享

- Secure、HttpOnly 的 `__Host-session_token`，以及浏览器可读、服务端有记录并通过 CSRF 请求头提交的 `__Host-csrf_token`。
- Android 与 Windows 客户端的 Bearer token bootstrap 与会话。
- 每张私密图片只允许一个有效分享令牌，有效期可在 10 分钟到 3 天之间配置。
- 用户与管理员角色、会话管理、审计日志，以及持久化的延迟账户注销流程。
- 可选集成 reCAPTCHA v3 和 Cloudflare Turnstile。只有明确禁用跨源隔离时才能使用 GeeTest v4。
- 公开/私密原站别名即时切换，并持久化 CDN purge 工作。

### Web 与运维

- 认证、控制台、上传队列、图片管理、个人资料、通知和后台管理视图。
- 英文、简体中文与日文界面资源。
- 浅色与深色主题，并支持减少动态效果。
- `/health`、`/ready` 和 `/metrics` 运维端点。
- 使用 advisory lock 与不可变校验和的版本化 MySQL 迁移。
- 应用镜像仅由 GitHub Actions 构建为 `linux/amd64` 与 `linux/arm64` 多平台版本，并发布到 Docker Hub 与 GHCR。

## 技术栈

| 层级 | 技术 |
|---|---|
| 前端 | React 19、React Router 8、Vite 8、TypeScript 6、Tailwind CSS 4、shadcn/ui |
| 前端数据 | TanStack Query 5、Zustand 5、React Hook Form 7、Zod 4 |
| 后端 | Go、Gin、GORM、MySQL 8.4、Redis 8 |
| 图片处理 | wasm-vips、Pica、Web API、imgproxy 4 |
| 存储 | V2 本地文件系统；V1 沿袭数据支持 Cloudflare R2 与兼容 path-style 的 S3 端点 |
| 测试 | Go testing、Vitest、Testing Library、MSW |

CI、服务与浏览器处理组件的精确版本记录在 [`requirements.lock`](https://github.com/kserksi/summeRain/blob/main/requirements.lock) 中。Go 与 npm 依赖图分别由 `backend/go.sum` 和 `frontend/package-lock.json` 锁定。

## WSL 快速开始

### 前置要求

- Go 1.24 或更高版本。CI 与容器构建当前使用 Go 1.26.5。
- Node.js `^20.19.0 || >=22.12.0`。CI 与容器构建当前使用 Node.js 24.18.0 LTS。
- Docker Engine 与 Docker Compose。
- OpenSSL，用于生成本地 HTTPS 证书。

### 1. 启动开发依赖

```bash
./scripts/dev-wsl.sh deps-up
```

该命令启动固定版本的 MySQL、Redis 与 imgproxy 容器，不会在本地构建 summeRain 应用镜像。

| 服务 | 默认地址 |
|---|---|
| MySQL | `127.0.0.1:13306` |
| Redis | `127.0.0.1:16379` |
| imgproxy | `127.0.0.1:18081` |

可通过 `SUMMERAIN_DEV_MYSQL_PORT`、`SUMMERAIN_DEV_REDIS_PORT` 与 `SUMMERAIN_DEV_IMGPROXY_PORT` 修改端口。

### 2. 启动后端

```bash
./scripts/dev-wsl.sh backend
```

API 默认监听 `http://127.0.0.1:18080`。首次启动会执行待处理的数据库迁移。可通过 `SUMMERAIN_DEV_BACKEND_PORT` 修改端口。

### 3. 启动前端

```bash
cd frontend
npm ci
cd ..
./scripts/dev-wsl.sh frontend
```

开发服务器默认使用 `https://127.0.0.1:5173`，并将 `/api/` 和 `/i/` 代理到本地后端。首次使用时请接受生成的开发证书。`__Host-` 会话 Cookie 要求 HTTPS 与同源代理。

开发模式默认启用 COOP/COEP，以支持 50 MP 的 wasm-vips 路径。因此所有第三方脚本、字体和图片必须提供兼容的 CORS 或 Cross-Origin-Resource-Policy 头。

### 4. 运行校验

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

```bash
cd frontend
npm run lint
npm run build
npx vitest run
```

## 生产部署

应用镜像由 GitHub Actions 构建。生产主机应拉取精确的已发布标签或 OCI 索引摘要，并且必须使用 `--no-build`。

```bash
cp backend/.env.example backend/.env
chmod 0600 backend/.env
# 编辑 backend/.env，设置精确的 DOCKER_IMAGE、数据库密码、
# Cookie secret，以及 imgproxy key/salt。

docker compose --env-file backend/.env \
  -f backend/docker-compose.deploy.yml pull

docker compose --env-file backend/.env \
  -f backend/docker-compose.deploy.yml up -d --no-build
```

稳定镜像示例：

```text
jaykserks/summerain:2.0.0
```

已发布的镜像仓库：

- Docker Hub：`jaykserks/summerain`
- GHCR：`ghcr.io/kserksi/summerain`

跨架构按摘要固定时，请使用 OCI 多平台索引摘要。不要在 `amd64` 与 `arm64` 主机上复用仅属于某一架构的子 manifest 摘要。

完整环境配置、nginx/CDN、健康检查、升级与回滚说明请参阅[部署与使用](./docs/USAGE.md)。

## 发布通道

- 普通推送到 `main` 会发布 `edge` 与 `sha-<12-character-commit>`。
- `2.0.0` 这样的稳定 `VERSION` 会发布 `v2.0.0`、`2.0.0`、`2.0`、`2`、`latest` 与 commit 标签。
- `2.1.0-rc.1` 这样的预发布版本会发布精确版本标签与 commit 标签，但不更新稳定别名。
- 精确语义版本标签不可变；移动别名仍可移动。
- 重新运行发布流程时，会根据已验证的 manifest 摘要协调 Docker Hub 与 GHCR；精确标签发生冲突时停止。
- 发布成功后，根目录 README 会同步到 Docker Hub。

完整发布契约见[发布与标签管理](./docs/RELEASING.md)。

## 资源配置

默认 Compose 配置面向与其他服务共享的 3 核、4 GiB 主机。

| 服务 | CPU 上限 | 内存上限 |
|---|---:|---:|
| 后端 | 0.75 CPU | 640 MiB |
| MySQL | 0.75 CPU | 1024 MiB |
| Redis | 0.15 CPU | 192 MiB |
| imgproxy | 0.70 CPU | 512 MiB |

后端默认允许全局 8 个、每用户 4 个并发部件上传。MySQL 与 Redis 连接池均有上限。达到默认 80% 磁盘软限制时拒绝新建 V2 会话，达到 90% 硬限制时拒绝新部件或发布输出。在主机持续资源争用时，将 imgproxy 与水印 worker 从 2 个降为 1 个。

这些限制只是保守起点，并非通用容量保证。磁盘延迟、数据库延迟、水印复杂度与同机工作负载都会影响吞吐量。

## 仓库结构

```text
summeRain/
|-- backend/
|   |-- cmd/server/                 应用入口
|   |-- internal/                   handler、service、repository 与 worker
|   |-- migrations/                 版本化 SQL 迁移
|   `-- web/                        生成的前端构建产物
|-- frontend/
|   |-- src/features/               面向领域的应用特性
|   |-- src/components/             共享 UI 与布局
|   |-- src/lib/                    API、CSRF、错误与工具
|   |-- src/store/                  用户与主题状态
|   `-- src/i18n/                   英文、中文与日文资源
|-- docs/                           API、运维、发布与架构文档
|-- translations/                   简体中文与日文文档镜像
|-- scripts/                        开发与仓库校验脚本
|-- .gitbook.yaml                   GitBook Git Sync 配置
`-- SUMMARY.md                      GitBook 导航
```

后端请求遵循以下边界：

```text
request -> middleware -> handler -> service -> repository -> MySQL / Redis
                                      `-> filesystem / R2 / imgproxy
```

## 文档

简体中文在线文档发布于
[summerain-1.gitbook.io/summerain/zh-cn](https://summerain-1.gitbook.io/summerain/zh-cn/)。
所有受 Git 跟踪的项目文档都通过 [`SUMMARY.md`](./SUMMARY.md) 纳入 GitBook 导航源。主要参考资料包括：

- [部署与使用](./docs/USAGE.md)
- [API 参考](./docs/API.md)
- [发布与标签管理](./docs/RELEASING.md)
- [前端架构](./docs/design/frontend-architecture/README.md)
- [数据库结构迁移](./backend/migrations/README.md)
- [贡献指南](./CONTRIBUTING.md)
- [安全策略](./SECURITY.md)

英文是文档的权威语言。可审查的简体中文与日文翻译在 `translations/zh-CN` 和 `translations/ja-JP` 下按相同页面路径镜像，并由 GitBook 作为同一文档站点的语言变体发布。

本仓库是 GitBook Git Sync 的权威数据源。README 与导航变更应通过 Git 管理，不要在 GitBook 编辑器中创建重复的 README 页面。

## 已知限制

- V2.0.0 仅接受静态图片；动图支持列入后续计划。
- V2 持久化固定 WebP 变体，不公开任意动态缩放组合。
- V2 不保留原始编码源字节；`master` 是全分辨率、质量 80 的 WebP。
- 现有 V1 图片不会自动转换或分配 V2 变体。
- 历史 R2 迁移有意委托给独立迁移工具，不由主服务执行。
- 禁用跨源隔离可启用 GeeTest v4，但会移除高容量 wasm-vips 浏览器路径。

## 贡献与安全

提交 Pull Request 前请阅读 [CONTRIBUTING.md](./CONTRIBUTING.md)，并在项目空间遵守 [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md)。安全问题应通过 [SECURITY.md](./SECURITY.md) 中的私密流程报告，不要公开提交 Issue。本仓库可使用 GitHub 的私密[安全公告表单](https://github.com/kserksi/summeRain/security/advisories/new)。

## 许可证

Copyright 2026 The summeRain Authors

本项目根据 [Apache License 2.0](https://github.com/kserksi/summeRain/blob/main/LICENSE) 授权。
