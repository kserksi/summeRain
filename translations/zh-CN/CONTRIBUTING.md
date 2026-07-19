# 贡献指南

感谢你对 summeRain 的关注。本文档说明如何参与项目开发。

## 开发环境

### 前置要求

- Go 1.24 或更高版本
- Node.js `^20.19.0 || >=22.12.0`，用于前端构建；CI 使用 24.18.0 LTS
- Docker 和 Docker Compose，仅用于运行 MySQL、Redis 与 imgproxy 开发依赖
- OpenSSL，用于生成本地 HTTPS 证书

### 在 WSL 中启动后端

```bash
./scripts/dev-wsl.sh deps-up   # 启动 MySQL、Redis 和 imgproxy；不构建应用镜像
./scripts/dev-wsl.sh backend   # 直接运行 Go 服务
```

应用默认监听 `127.0.0.1:18080`，健康检查端点为
`GET http://127.0.0.1:18080/health`。

### 启动前端开发服务器

```bash
cd frontend
npm ci
cd ..
./scripts/dev-wsl.sh frontend  # https://127.0.0.1:5173，并为 /api 和 /i 提供同源代理
```

### 将前端构建到后端

```bash
cd frontend
npm run build                  # 输出目录：../backend/web/
```

## 代码规范

### Go 后端

- 遵循 [Effective Go](https://go.dev/doc/effective_go)、`gofmt` 和 `go vet`。
- 新功能需在 `_test.go` 文件中包含测试。
- import 顺序依次为：标准库、第三方包、`github.com/kserksi/summerain/...` 下的项目包。
- 每个源文件均需保留版权头：`// Copyright 2026 The summeRain Authors`。

提交前运行以下检查：

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

### TypeScript 前端

- 使用项目现有的 React 19、TanStack Query 和 shadcn/ui 技术栈。
- 新功能需包含 Vitest 测试。
- 遵循现有 ESLint 配置。

提交前运行以下检查：

```bash
cd frontend
npm run lint
npm run build
npx vitest run
```

## 提交规范

使用 [Conventional Commits](https://www.conventionalcommits.org/) 格式：

```text
<type>(<scope>): <description>

[optional body]
```

常用类型包括 `feat`、`fix`、`docs`、`refactor`、`test`、`chore` 和 `i18n`。

示例：

```text
feat(upload): add drag-and-drop reordering
fix(auth): handle expired session on token refresh
i18n(extract): move navbar strings to locale files
```

## 提交 Pull Request

1. 更新本地 `dev` 分支并从中创建功能分支，将 `dev` 设为 Pull Request 的目标分支。
2. 确认 `go build`、`npm run build` 和相关测试均通过。
3. 在 Pull Request 描述中说明改动目的和影响范围。
4. UI 改动需附截图。
5. 每个 Pull Request 只处理一个关注点，以便审查。

## 项目结构

```text
backend/    Go + Gin + GORM (MySQL) + Redis + imgproxy
frontend/   React 19 + Vite + TypeScript + Tailwind + shadcn/ui
docs/       API 契约、部署指南与架构记录
```

历史前端架构记录索引位于
[docs/design/frontend-architecture/README.md](docs/design/frontend-architecture/README.md)。
标记为归档的内容仅用于设计追溯。

## 文档翻译

仓库根目录下的英文文档是权威版本。完整的简体中文和日文镜像分别位于
`translations/zh-CN/` 与 `translations/ja-JP/`，并与其权威页面保持相同的相对路径。
修改英文文档时，必须在同一次变更中更新两种翻译，然后刷新翻译源哈希并运行
GitBook 文档校验器。

```bash
bash scripts/update-translation-source-hashes.sh
bash scripts/verify-gitbook-docs.sh
```

## 行为准则

参与本项目即表示你同意遵守[行为准则](CODE_OF_CONDUCT.md)。请保持友善与尊重。
