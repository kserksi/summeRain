# summeRain

> 自托管图片托管与相册服务。原生 AVIF/WebP 转码、文字水印、自适应尺寸、多存储后端、多端鉴权。

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8.svg)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB.svg)](https://react.dev)
[![TypeScript](https://img.shields.io/badge/TypeScript-6-3178C6.svg)](https://www.typescriptlang.org/)

## 项目结构

```
imgcloud/
├─ frontend/   React 19 + Vite 8 + TS 6 + Tailwind 4 + shadcn/ui
├─ backend/    Go + Gin + GORM (MySQL) + Redis + imgproxy v3.12
├─ docs/       API 契约 + 部署手册 + 架构设计
└─ LICENSE     Apache-2.0
```

## 核心特性

### 图片处理

- **AVIF / WebP 原生转码** — 基于 imgproxy v3.12，URL 加扩展名即时返回：`/i/<link>.avif`
- **文字水印** — SVG 渲染，管理员可配置文案、位置、透明度；上传时自动盖水印
- **自适应尺寸** — 查询参数即时缩放：`?w=300&h=200`（边长上限 4096）
- **质量压缩** — `?q=80` 即时调整编码质量
- **格式互转** — `webp / avif / jpg / jpeg / png / gif` 任意切换
- **内容去重** — SHA256 校验，相同文件 `reference_count` 引用计数，节省存储
- **多存储后端** — 本地磁盘 / Cloudflare R2 / S3 兼容；支持仅 R2 模式（无本地落盘）

### 安全与权限

- **三方可插拔 Captcha** — reCAPTCHA v3 / Cloudflare Turnstile / 极验 v4，admin 可切换；`none` 模式跳过
- **Cookie 鉴权** — `__Host-` 前缀 + `SameSite=Strict` + `Secure` + `HttpOnly`，CSRF 双提交
- **设备端鉴权** — Android / Windows 客户端走 Bearer Token（identity → bootstrap → session），每平台限 3 台
- **私密图统一令牌** — 一图一令牌，TTL 可调（10 分钟 ~ 3 天），吊销后永久失效
- **bcrypt 密码哈希** + 路径穿越防护 + SVG 强制下载（防 XSS）
- **审计日志** + 角色（user / admin）+ 用户状态（active / suspended / pending_deletion）

### 平台与运维

- **Prometheus 指标** + 三端点健康检查（`/health` / `/ready` / `/metrics`）
- **后台 Worker** — 浏览量缓冲落库（60s）、临时文件清理（1h）、设备心跳（5min）、用户注销
- **限流** — IP + 用户名双维度（登录 5/15min、Bootstrap 10/min）
- **Docker Compose 一键部署** — 非 root 容器（UID 10001），MySQL/Redis/imgproxy 仅内网通信
- **i18n** — 中 / 英双语界面
- **多端会话管理** — 可吊销任意会话、查看活跃设备

## 与同类开源项目对比

| 项目 | AVIF | 文字水印 | 自适应尺寸 | 现代栈 | 维护状态 | 协议 |
|------|:----:|:-------:|:---------:|:------:|:-------:|------|
| **summeRain** | 是 | 是 | 是 | 是 | 活跃 | Apache-2.0 |
| EasyImages2.0 | 限制 | 是 | 是 | 一般 | 放缓 | GPL-2.0 |
| Lsky Pro | 否 | 是 | 是 | 一般 | 开源版停维 | GPL-3.0 |
| modern-images | 是 | 否 | 需 CDN | 是 | 活跃 | 未声明 |
| Picsur | 否 | 否 | 是 | 是 | 弃维 | AGPL-3.0 |

## 技术栈

| 层 | 技术 |
|---|---|
| 前端 | React 19.2 · Vite 8 · TypeScript 6 · Tailwind CSS 4 · shadcn/ui · Zustand · TanStack Query · react-hook-form · zod · i18next · MSW |
| 后端 | Go 1.24 · Gin · GORM（MySQL 8）· Redis 7 · imgproxy v3.12 · AWS SDK v2（R2/S3） · Prometheus |
| 主题 | 咖啡色板（浅·摩卡 / 深·浓缩）+ 圆形扩散切换 |
| 部署 | Docker Compose · 非 root 容器 · nginx + Cloudflare |

## 快速开始

### 1. 后端（Docker Compose 全栈）

```bash
cd backend
cp .env.example .env   # 必改：DB_PASSWORD、IMGPROXY_KEY、IMGPROXY_SALT、COOKIE_SECRET
docker compose up -d   # mysql + redis + imgproxy + backend
```

- 应用监听 `:8080`
- 健康检查：`GET http://localhost:8080/health` → `{"status":"ok"}`
- 首次启动自动 `AutoMigrate` 建表
- 生产部署用 `docker-compose.deploy.yml`

> 本地 `http://localhost` 下 `__Host-` 前缀 Cookie 会被浏览器拒绝（要求 HTTPS + 同源）。本地联调建议用自签证书或同源代理。

### 2. 前端开发

```bash
cd frontend
npm install
npm run dev    # https://localhost:5173
```

### 3. 构建部署

```bash
cd frontend
npm run build  # 输出到 ../backend/web/
```

构建产物以只读卷挂载，后端 `NoRoute` 兜底，自动提供 `web/` 静态资源 + `/api/*` + `/i/:link` 图片直链。

## 文档

- [API 契约](docs/API.md) — 602 行，逐接口字段、错误码、CSRF 规则
- [部署使用手册](docs/USAGE.md) — 环境变量、运维手册、安全要点、阈值速查
- [后端调整方案](docs/backend-changes-plan.md) — 私密图统一令牌、Captcha 三 Provider
- [前端架构设计](docs/design/frontend-architecture/) — 设计文档集合

## 限制与阈值速查

| 项目 | 值 |
|------|-----|
| 单文件上限 | 10 MB |
| 单次上传文件数 | ≤ 20 |
| 默认存储配额 | 1 GB |
| 配额预警阈值 | 90% |
| 图片短链 | 12 位 hex（48 bit 熵） |
| Web 会话有效期 | 30 天 |
| 转码尺寸参数 | `w/h` ≤ 4096 |
| 支持扩展名 | png / jpg / jpeg / webp / gif / avif |

完整阈值见 [USAGE.md 第 7 节](docs/USAGE.md#7-限制与阈值速查)。

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
