# summeRain

> 自托管图片托管与相册服务，提供图片转码、水印、自适应尺寸、多存储后端以及 Web/设备端鉴权。

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8.svg)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB.svg)](https://react.dev)
[![TypeScript](https://img.shields.io/badge/TypeScript-6-3178C6.svg)](https://www.typescriptlang.org/)

## 项目概览

summeRain 采用“Go 模块化单体 + React 静态 SPA”的同源部署架构：Go 服务统一提供 `/api/v1/*` API、`/i/:link` 图片直链以及前端静态资源；MySQL 保存业务数据，Redis 承担限流、浏览量缓冲和设备端防重放，imgproxy 负责图片转换与水印处理。

```text
Browser / Android / Windows
          │
          ▼
   Go + Gin (:8080)
     ├─ /api/v1/* ─→ middleware → handler → service → repository
     ├─ /i/:link  ─→ 权限校验 → 本地/R2 → imgproxy
     └─ /*        ─→ backend/web/（React SPA）
                              │
             ┌────────────────┼────────────────┐
             ▼                ▼                ▼
          MySQL 8          Redis 7       本地存储 / R2
```

## 核心能力

### 图片与存储

- **WebP / AVIF 转码**：使用 `/i/<link>.<format>` 获取转换结果；AVIF 首次缺失时可先回退 WebP，并在后台生成。
- **自适应尺寸与质量**：支持 `?w=300&h=200&q=80`，宽高最大 4096，质量范围 1～100。
- **文字水印**：上传时生成处理图，管理员可配置文案、位置、透明度、字号和颜色。
- **内容去重**：按 SHA-256 识别相同源文件，通过 `ImageFile.reference_count` 管理物理文件生命周期。
- **多存储后端**：支持本地磁盘、Cloudflare R2 和 S3 兼容存储；公开原图可重定向到 R2/CDN。
- **配额管理**：按用户统计存储用量，达到 90% 时发送通知；管理员可调整用户配额。
- **批量操作**：前端最多 5 个上传请求并发，支持批量下载用户原图。

### 账号、权限与分享

- **Web 鉴权**：使用 `__Host-` Secure/HttpOnly Cookie，并对写请求校验双提交 CSRF Token。
- **设备端鉴权**：Android / Windows 使用 `identity → bootstrap → session` 的 Bearer Token 流程，每个平台最多保留 3 个身份令牌。
- **私密图分享**：每张私密图片最多一个有效分享令牌，TTL 为 10 分钟～3 天；重新签发会撤销旧令牌。
- **可插拔 Captcha**：支持 `none`、reCAPTCHA v3、Cloudflare Turnstile 和极验 v4，由管理员选择 Provider。
- **角色与状态**：提供 `user/admin` 角色，以及 `active/suspended/pending_deletion` 用户状态。
- **会话管理**：用户可查看并吊销 Web 或设备会话；设备会话支持心跳、版本下限和平台一致性校验。
- **账号生命周期**：管理员可发起 24 小时延迟注销并在执行前撤销，后台 Worker 负责最终清理。
- **审计与安全**：bcrypt 密码哈希、审计日志、请求 ID、安全响应头、路径穿越防护和 SVG 下载保护。

### Web 界面与运维

- **功能域**：登录注册、控制台、图片列表/上传/详情、个人资料、通知和管理后台。
- **管理后台**：用户状态与配额、延迟注销、全站图片、系统统计、Captcha、水印、语言及 R2 配置。
- **头像**：支持浏览器端裁剪并更新个人头像。
- **主题**：咖啡色板浅色/深色主题，并支持圆形扩散切换动画和 reduced-motion 降级。
- **国际化**：以英语为默认语言，内置简体中文和日语，站点语言由管理员统一配置。
- **可观测性**：提供 `/health`、`/ready`、`/metrics`，并导出 Prometheus 指标。
- **后台任务**：设备心跳监控、浏览量批量落库、临时数据清理和用户延迟注销。

## 仓库结构

```text
summeRain/
├─ backend/
│  ├─ cmd/server/          服务入口、依赖装配、路由和优雅关闭
│  ├─ internal/
│  │  ├─ config/           环境变量配置
│  │  ├─ handler/          HTTP 参数与响应适配
│  │  ├─ middleware/       鉴权、CSRF、限流和安全头
│  │  ├─ model/            GORM 数据模型
│  │  ├─ repository/       MySQL/Redis 数据访问
│  │  ├─ service/          认证、图片、管理、通知、Captcha、R2
│  │  ├─ worker/           后台周期任务
│  │  └─ pkg/              token、响应、错误码、imgproxy 签名
│  └─ web/                 前端构建产物（由 Vite 生成，不提交）
├─ frontend/
│  ├─ src/features/        auth、captcha、images、user、notifications、admin
│  ├─ src/components/      共享组件、布局和 shadcn/ui
│  ├─ src/lib/             API、CSRF、Query Client、错误和工具
│  ├─ src/store/           用户与主题状态
│  ├─ src/i18n/            英语、中文和日语资源
│  └─ src/App.tsx          路由、懒加载和权限守卫
├─ docs/                   API、部署手册和架构设计
└─ LICENSE                 Apache-2.0
```

### 后端分层

```text
request → middleware → handler → service → repository → MySQL/Redis
                                      └─→ imgproxy / filesystem / R2
```

`cmd/server/main.go` 是组合根：负责连接 MySQL/Redis、执行迁移与默认配置初始化、构造各层依赖、注册路由、启动 Worker，并在收到退出信号后完成优雅关闭。

### 前端数据流

前端按业务域组织，每个 `features/<domain>/` 通常包含 `api.ts`、`hooks.ts`、`pages/` 和 `components/`：

- `lib/api.ts` 是统一请求出口，负责 Cookie、CSRF、响应信封和全局鉴权错误。
- TanStack Query 管理图片、通知、个人资料和后台数据等服务端状态。
- Zustand 只保存当前用户快照和主题；用户身份不持久化，刷新后重新请求 `/auth/me`。
- React Router 使用 `BrowserRouter`，页面按路由懒加载；`AuthGuard` 和 `AdminGuard` 负责前端导航保护，后端中间件仍是最终权限边界。

## 技术栈

| 层 | 技术 |
|---|---|
| 前端 | React 19 · React Router 8 · Vite 8 · TypeScript 6 · Tailwind CSS 4 · shadcn/ui |
| 前端数据 | TanStack Query 5 · Zustand 5 · React Hook Form 7 · Zod 4 |
| 前端体验 | i18next · Sonner · Tabler Icons · View Transitions · SRI |
| 后端 | Go 1.24 · Gin · GORM · MySQL 8 · Redis 7 |
| 图片与对象存储 | imgproxy v3.12 · AWS SDK for Go v2 · Cloudflare R2/S3 |
| 运维 | Docker Compose · Prometheus · 非 root 容器（UID 10001） |
| 测试 | Go testing · Vitest · Testing Library · MSW |

## 快速开始

### 前置要求

- Go 1.24+
- Node.js 20+
- Docker 与 Docker Compose

### 1. 启动后端依赖与服务

```bash
cd backend
cp .env.example .env
docker compose up -d
```

启动前至少应修改 `.env` 中的 `DB_PASSWORD`、`COOKIE_SECRET`、`IMGPROXY_KEY` 和 `IMGPROXY_SALT`。开发 Compose 使用 `secrets/mysql_password.txt` 初始化 MySQL，该文件的内容需要与 `.env` 的 `DB_PASSWORD` 保持一致。

- API 默认监听 `http://localhost:8080`
- 存活检查：`GET /health`
- 就绪检查：`GET /ready`（同时检查 MySQL 与 Redis）
- Prometheus：`GET /metrics`
- 首次启动自动执行 GORM `AutoMigrate`

### 2. 启动前端开发服务器

```bash
cd frontend
npm ci
npm run dev
```

Vite 默认使用 `http://localhost:5173`；若 `frontend/` 下存在本地证书 `localhost+1.pem` 和 `localhost+1-key.pem`，则自动启用 HTTPS。开发服务器会把 `/api/` 和 `/i/` 代理到 `localhost:8080`。

> Web 鉴权 Cookie 使用 `__Host-` 前缀，要求 HTTPS 且同源。需要完整测试登录和写操作时，请使用本地 HTTPS 或同源反向代理。

### 3. 构建前端

```bash
cd frontend
npm run build
```

构建产物输出到 `backend/web/`。从 `backend/` 目录启动 Go 服务后，未知的非 API 路由会回退到 `web/index.html`，因此 SPA 深链刷新可用。

### 4. 通过 GitHub Actions 发布容器镜像

推送到 `main` 或 `master` 后，GitHub Actions 会先运行前后端检查，再使用根目录的多阶段 Dockerfile 在 GitHub Runner 上构建 `linux/amd64` 和 `linux/arm64` 镜像，并同时推送到 GHCR 和 Docker Hub。整个发布过程不需要在本地构建镜像。

- Docker Hub：`jaykserks/summerain`
- GHCR：`ghcr.io/kserksi/summerain`

工作流还会把仓库根目录的 `README.md` 同步到 Docker Hub 仓库说明。GitHub 仓库需要配置 Actions Secrets：`DOCKERHUB_USERNAME`（值为 `Jaykserks`）以及具有 `read/write/delete` 权限的 `DOCKERHUB_TOKEN`。

普通分支构建会发布 `edge` 和 `sha-<commit>`，不会覆盖稳定版 `latest`。修改根目录 `VERSION` 才会触发正式发布，并自动创建同名 Git Tag 和 GitHub Release。

稳定版 `1.2.3` 会发布 `v1.2.3`、`1.2.3`、`1.2`、`1`、`latest` 和提交标签；`1.3.0-rc.1` 等预发布版只发布精确版本与提交标签，不更新稳定别名。精确 SemVer 标签在 Docker Hub 中自动设为不可变，`latest`、主版本和次版本别名保持可移动。

正式发布只需更新 `VERSION` 并推送：

```bash
printf '1.2.3\n' > VERSION
git add VERSION
git commit -m "chore: release v1.2.3"
git push origin HEAD:main
```

完整版本规则、发布检查和回滚约定见 [发布与标签管理](docs/RELEASING.md)。

Compose 可通过 `DOCKER_IMAGE` 选择远程镜像。部署机拉取镜像后，可用 `--no-build` 阻止本地重新构建：

```bash
DOCKER_IMAGE=jaykserks/summerain:1.0.0 docker compose -f backend/docker-compose.deploy.yml pull
DOCKER_IMAGE=jaykserks/summerain:1.0.0 docker compose -f backend/docker-compose.deploy.yml up -d --no-build
```

生产环境的 nginx/Cloudflare 前置和回滚流程见 [部署与使用文档](docs/USAGE.md)。

## 开发验证

提交前建议运行：

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

代码贡献规范和 Conventional Commits 约定见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 限制与阈值

| 项目 | 当前值 |
|---|---|
| 单文件上限 | 10 MB |
| 单个后端上传请求 | 最多 20 个文件 |
| 前端上传并发 | 5 个请求 |
| 上传输入格式 | PNG / JPG / JPEG / WebP / GIF |
| 直链输出格式 | WebP / AVIF / JPG / JPEG / PNG / GIF |
| 默认/最小用户配额 | 500 MB |
| 配额预警阈值 | 90% |
| 图片短链 | 12 位 hex（48 bit 熵，冲突时重试） |
| Web 会话有效期 | 30 天 |
| CSRF Token 有效期 | 24 小时，成功写操作后续期 |
| 设备身份令牌 | 90 天，每个平台最多 3 个 |
| 设备 API 会话 | 15 分钟，活动时续期 |
| 私密图片令牌 | 10 分钟～3 天 |
| 转码尺寸 | `w/h` 最大 4096 |

完整配置和接口约定分别见 [docs/USAGE.md](docs/USAGE.md) 与 [docs/API.md](docs/API.md)。

## 文档

- [API 契约](docs/API.md)：接口字段、鉴权、CSRF、错误码与图片直链规则。
- [部署与使用](docs/USAGE.md)：环境变量、Docker、nginx、运维和安全要点。
- [前端架构设计](docs/design/frontend-architecture/)：技术选型、功能域、设计系统、构建与测试规范。
- [后端调整方案](docs/backend-changes-plan.md)：私密图统一令牌与多 Captcha Provider 的设计背景。
- [贡献指南](CONTRIBUTING.md)：开发门禁、提交格式和 PR 约定。
- [安全策略](SECURITY.md)：漏洞报告方式与支持范围。

## License

Copyright 2026 The summeRain Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
