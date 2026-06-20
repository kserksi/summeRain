# ImgCloud

高速稳定的图片托管服务 —— 全球 CDN 加速，外链永久有效。

## 项目结构

\\\
imgcloud/
├─ frontend/          React 19 + Vite 8 + TS 6 + Tailwind 4 + shadcn/ui
├─ backend/           Go + Gin + MySQL + Redis + imgproxy
├─ docs/              API 文档 + 前端架构设计 + 后端调整方案
└─ .gitignore
\\\

## 技术栈

| 层 | 技术 |
|---|---|
| 前端 | React 19, Vite 8, TypeScript 6 (ES2025), Tailwind CSS 4, shadcn/ui (maia), Tabler Icons |
| 后端 | Go, Gin, GORM (MySQL), Redis, imgproxy v4 |
| 主题 | 咖啡色板 (浅·摩卡 / 深·浓缩) + VT 圆形扩散切换 |

## 快速开始

### 前端开发

\\\ash
cd frontend
npm install
npm run dev          # https://localhost:5173
\\\

### 后端

\\\ash
cd backend
docker-compose up -d  # MySQL + Redis + imgproxy
go run ./cmd/server    # :8080
\\\

### 构建部署

\\\ash
cd frontend
npm run build          # 输出到 ../backend/web/
\\\

后端自动提供 \web/\ 静态资源 + \/api\ + \/i\ 图片直链。

## 文档

- [API 契约](docs/API.md)
- [前端架构设计](docs/superpowers/specs/frontend-architecture/)
- [后端调整方案](docs/backend-changes-plan.md)

## License

© kserks 2026
