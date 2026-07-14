# 贡献指南

感谢你对 summeRain 的兴趣！本文档说明如何参与开发。

## 开发环境

### 前置要求

- Go 1.24+
- Node.js 20+（前端构建）
- Docker & Docker Compose（运行完整后端栈）

### 启动后端（Docker Compose 全栈）

```bash
cd backend
cp .env.example .env          # 必改：DB_PASSWORD、IMGPROXY_KEY、IMGPROXY_SALT、COOKIE_SECRET
docker compose up -d           # mysql + redis + imgproxy + backend
```

应用监听 `:8080`，健康检查 `GET http://localhost:8080/health`。

### 启动前端开发服务器

```bash
cd frontend
npm install
npm run dev                    # http://localhost:5173，代理 /api 和 /i 到 localhost:8080
```

### 构建前端到后端

```bash
cd frontend
npm run build                  # 输出到 ../backend/web/
```

## 代码规范

### Go（后端）

- 遵循 [Effective Go](https://go.dev/doc/effective_go) 与 `gofmt` / `go vet`
- 新增功能需附带测试（`_test.go`）
- import 顺序：标准库 → 第三方 → 本项目（`github.com/kserksi/summerain/...`）
- 每个源文件保留版权头：`// Copyright 2026 The summeRain Authors`

提交前运行：

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

### TypeScript（前端）

- 使用项目现有的 React 19 + TanStack Query + shadcn/ui 技术栈
- 新增功能需附带测试（vitest）
- 遵循现有 ESLint 配置

提交前运行：

```bash
cd frontend
npm run build
npx vitest run
```

## 提交规范

使用 [Conventional Commits](https://www.conventionalcommits.org/) 格式：

```
<type>(<scope>): <description>

[optional body]
```

常用 type：`feat`、`fix`、`docs`、`refactor`、`test`、`chore`、`i18n`

示例：

```
feat(upload): add drag-and-drop reordering
fix(auth): handle expired session on token refresh
i18n(extract): move navbar strings to locale files
```

## 提交 Pull Request

1. 从 `main` 拉取最新代码创建分支
2. 确保本地 `go build` / `npm run build` / 测试通过
3. PR 描述说明改动目的与影响范围
4. 如改动 UI，附截图
5. 一个 PR 只做一件事，便于 review

## 项目结构

```
backend/    Go + Gin + GORM (MySQL) + Redis + imgproxy
frontend/   React 19 + Vite + TypeScript + Tailwind + shadcn/ui
docs/       API 契约 + 部署手册 + 架构设计
```

详细架构见 [docs/design/frontend-architecture/](docs/design/frontend-architecture/)。

## 行为准则

参与本项目即代表你同意遵守 [Code of Conduct](CODE_OF_CONDUCT.md)。请保持友善与尊重。
