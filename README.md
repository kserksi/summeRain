# summeRain

> 自托管图片托管与相册服务，提供图片转码、水印、自适应尺寸、多存储后端以及 Web/设备端鉴权。

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB.svg)](https://react.dev)
[![TypeScript](https://img.shields.io/badge/TypeScript-6-3178C6.svg)](https://www.typescriptlang.org/)

## 项目概览

summeRain 采用“Go 模块化单体 + React 静态 SPA”的同源部署架构：Go 服务统一提供 `/api/v1/*` API、`/i/:link` 图片直链以及前端静态资源；MySQL 保存业务与任务真相，Redis 承担限流、浏览量缓冲和设备端防重放，imgproxy 只保留给 V1 兼容路径及服务端发布水印。

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
        MySQL 8.4         Redis 8       本地存储 / R2
```

## 核心能力

### 图片与存储

- **浏览器端预处理**：V2 启用时，静态 JPG、JPEG、PNG、BMP、WebP、AVIF 在上传前生成 `master`、`gallery`、`admin` 和 `publish_source` 四份固定 WebP 上传部件；服务端关闭 V2 时，Web 根据配方能力位自动回退到 V1 multipart。
- **固定访问变体**：发布完成后持久化 `master`、`gallery`、`admin` 和 `publish`；My Images 使用 400×400，Image Management 使用 120×160 文件并以 CSS 60×80 显示（2x 像素密度），发布源最长边 2048。中间 `publish_source` 在发布后删除。
- **文字水印**：服务端仅对发布产物应用水印，管理员可配置文案、位置、透明度、字号和颜色。
- **V1 兼容**：历史图片继续支持动态格式、尺寸和质量参数；无尺寸 WebP 与后台 AVIF 可持久化，任意尺寸等动态结果只写入有界临时文件，并在最后一个请求完成后删除。
- **内容去重**：按 SHA-256 识别相同源文件，通过 `ImageFile.reference_count` 管理物理文件生命周期。
- **多存储后端**：支持本地磁盘、Cloudflare R2 和 S3 兼容存储；公开原图可重定向到 R2/CDN。
- **历史存储安全**：主服务不执行历史图片批量迁移；未分类的 V1 记录仅走本地优先兼容读取，正式迁移由后续独立工具完成。
- **配额管理**：按用户统计存储用量，达到 90% 时发送通知；管理员可调整用户配额。
- **批量操作**：客户端处理串行、上传流水线并发 2、上传部件自适应并发 2～3，并支持流式批量下载。

### 账号、权限与分享

- **Web 鉴权**：使用 `__Host-` Secure/HttpOnly Cookie，并对写请求校验双提交 CSRF Token。
- **设备端鉴权**：Android / Windows 使用 `identity → bootstrap → session` 的 Bearer Token 流程，每个平台最多保留 3 个身份令牌。
- **私密图分享**：每张私密图片最多一个有效分享令牌，TTL 为 10 分钟～3 天；重新签发会撤销旧令牌。
- **可插拔 Captcha**：支持 `none`、reCAPTCHA v3、Cloudflare Turnstile；显式关闭默认跨源隔离后也可选择极验 v4。
- **角色与状态**：提供 `user/admin` 角色；管理员只在 `active/suspended` 间切换状态，注销状态机独立维护 `pending_deletion/deleting`。
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
| 后端 | Go 1.26 · Gin · GORM · MySQL 8.4 · Redis 8 |
| 图片与对象存储 | 浏览器 WebP 预处理 · imgproxy v4.0.11 · AWS SDK for Go v2 · Cloudflare R2/S3 |
| 运维 | Docker Compose · Prometheus · 非 root 容器（UID 10001） |
| 测试 | Go testing · Vitest · Testing Library · MSW |

## 快速开始

### 前置要求

- Go 1.24+（CI / 容器锁定 1.26.5）
- Node.js 20+（CI / 容器锁定 24.18.0 LTS）
- Docker 与 Docker Compose

### 1. 在 WSL 启动开发依赖

```bash
./scripts/dev-wsl.sh deps-up
```

该命令只启动固定版本的 MySQL、Redis 和 imgproxy，不构建应用镜像。

- MySQL：`127.0.0.1:13306`
- Redis：`127.0.0.1:16379`
- imgproxy：`127.0.0.1:18081`

可分别通过 `SUMMERAIN_DEV_MYSQL_PORT`、`SUMMERAIN_DEV_REDIS_PORT` 和 `SUMMERAIN_DEV_IMGPROXY_PORT` 覆盖这些端口。

### 2. 启动后端开发进程

```bash
./scripts/dev-wsl.sh backend
```

- API 默认监听 `http://127.0.0.1:18080`，可通过 `SUMMERAIN_DEV_BACKEND_PORT` 覆盖
- 存活检查：`GET /health`
- 就绪检查：`GET /ready`（同时检查 MySQL 与 Redis）
- Prometheus：`GET /metrics`
- 首次启动自动执行数据库迁移

### 3. 启动前端开发服务器

```bash
cd frontend
npm ci
cd ..
./scripts/dev-wsl.sh frontend
```

脚本会在首次运行时生成本地开发证书，并默认启动 `https://127.0.0.1:5173`；若 5173 已被占用，Vite 会自动选择下一个可用端口。开发服务器会把 `/api/` 和 `/i/` 同源代理到 `127.0.0.1:18080`，也可通过 `VITE_DEV_BACKEND_URL` 覆盖目标地址。

> 首次访问需要在浏览器中接受自签名开发证书。Web 鉴权 Cookie 使用 `__Host-` 前缀，必须保持 HTTPS 与同源代理。开发与生产默认启用 COOP/COEP 跨源隔离以支持 wasm-vips 处理 50MP 大图；引入第三方脚本、字体或图片时，资源服务器必须提供兼容的 CORS 或 CORP 响应头。GeeTest v4 不满足默认隔离策略，隔离开启时请使用 `none`、reCAPTCHA 或 Turnstile。

### 4. 构建前端

```bash
cd frontend
npm run build
```

构建产物输出到 `backend/web/`。从 `backend/` 目录启动 Go 服务后，未知的非 API 路由会回退到 `web/index.html`，因此 SPA 深链刷新可用。

### 5. 通过 GitHub Actions 发布容器镜像

推送到 `main` 或 `master` 后，GitHub Actions 会先运行前后端检查，再使用根目录的多阶段 Dockerfile 在 GitHub Runner 上构建 `linux/amd64` 和 `linux/arm64` 镜像，并同时推送到 GHCR 和 Docker Hub。整个发布过程不需要在本地构建镜像。

- Docker Hub：`jaykserks/summerain`
- GHCR：`ghcr.io/kserksi/summerain`

工作流还会把仓库根目录的 `README.md` 同步到 Docker Hub 仓库说明。GitHub 仓库需要配置 Actions Secrets：`DOCKERHUB_USERNAME`（值为 `Jaykserks`）以及具有 `read/write/delete` 权限的 `DOCKERHUB_TOKEN`。

`main` / `master` 的普通推送会发布 `edge` 和 `sha-<short-commit>`，不会覆盖稳定版 `latest`。修改根目录 `VERSION` 才会触发正式发布，并自动创建同名 Git Tag 和 GitHub Release。

稳定版 `1.2.3` 会发布 `v1.2.3`、`1.2.3`、`1.2`、`1`、`latest` 和提交标签；`1.3.0-rc.1` 等预发布版只发布精确版本与提交标签，不更新稳定别名。精确 SemVer 标签在 Docker Hub 中自动设为不可变，`latest`、主版本和次版本别名保持可移动。

正式发布任务可安全重跑：已有 Git tag 必须仍指向当前发布提交；工作流会校验 Docker Hub 与 GHCR 中 `vX.Y.Z`、`X.Y.Z` 的多架构清单摘要。任一 registry 留有完整或局部发布结果时，会从已有摘要跨 registry 补齐精确标签，并重新指向缺失或过期的稳定别名与提交标签，不会重推已有不可变标签；发现摘要冲突则停止发布。

版本号严格禁止 core 数字和纯数字预发布标识的前导零；由于容器标签无法无损表达 `+`，正式版本不使用 build metadata。工作流引用的第三方 Actions 均固定到完整 commit SHA。

正式发布只需更新 `VERSION` 并推送：

```bash
printf '1.2.3\n' > VERSION
git add VERSION
git commit -m "chore: release v1.2.3"
git push origin HEAD:main
```

完整版本规则、发布检查和回滚约定见 [发布与标签管理](docs/RELEASING.md)。

Compose 通过受忽略的 `backend/.env` 同时向服务和 Compose 插值提供配置。先从示例创建并编辑它，将 `DOCKER_IMAGE` 改为已发布的精确版本，再拉取镜像；`--no-build` 会阻止部署机本地构建：

```bash
cp backend/.env.example backend/.env
chmod 0600 backend/.env
# 编辑 backend/.env 中的镜像版本、数据库密码、Cookie 与 imgproxy 密钥
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build
```

跨架构部署如需 digest 级固定，应使用 OCI 多架构索引 digest；平台专属的 child manifest digest 不应在 `amd64` 与 `arm64` 之间复用。依赖镜像锁定策略见 [`requirements.lock`](requirements.lock)。

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
| 单个源文件上限 | 15 MB |
| 单图像素上限 | 50 MP |
| 客户端处理 / 上传流水线 | 1 / 2 |
| 浏览器活跃上传会话 | 4（后端每用户上限 8） |
| 上传部件并发 | 初始 2，成功后自适应到 3 |
| 上传输入格式 | 静态 JPG / JPEG / PNG / BMP / WebP / AVIF |
| V2 持久化格式 | WebP |
| 默认/最小用户配额 | 500 MB |
| 配额预警阈值 | 90% |
| 图片短链 | 12 位 hex（48 bit 熵，冲突时重试） |
| Web 会话有效期 | 30 天 |
| CSRF Token 有效期 | 24 小时，成功写操作后续期 |
| 设备身份令牌 | 90 天，每个平台最多 3 个 |
| 设备 API 会话 | 15 分钟，活动时续期 |
| 私密图片令牌 | 10 分钟～3 天 |
| V2 固定访问变体 | `master`、400×400 `gallery`、120×160 `admin`（CSS 60×80，2x）、最长边 2048 `publish` |

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
