# 01 · 概览与技术栈

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> 所属：[前端架构设计（索引）](./README.md)

## 背景

现有前端为原生 JS（仓库根 `index.html` + `css/` + `js/`，含 localStorage mock 与咖啡主题/暗黑模式）。后端已就位为 Go + Gin + MySQL + Redis + imgproxy 的生产级图床服务。

**目标**：以 React 19 + Vite 8 + TypeScript 重写前端，直接对接真实后端 API，取代 mock 与原生实现。

## 硬约束

来自后端 `cmd/server/main.go` 的 `NoRoute`：

- 后端以 SPA 模式提供 `./web/*` 静态资源，并 fallback 到 `./web/index.html`
- 因此前端须构建为**静态产物**放入 `backend/web/`，与后端**同源**部署
- 鉴权为 cookie（`__Host-session_token` / `__Host-csrf_token`），同源 HTTPS 方可生效 → 排除带独立 Node 进程的 SSR 方案

## 功能范围

精准对齐后端能力，包含**认证、我的图片、个人资料、通知、后台**五域；不含后端未支撑的能力（公开图库浏览、分类/标签、后台图片审核、删除用户）。

## 技术栈

> 版本已于 2026-06-18 经 npm `dist-tags` 核实为各包 `latest` 稳定版，见下表。`package.json` 用 `^` 范围、`package-lock.json` 锁定精确版本以保证可复现。

| 类别 | 选型 |
|---|---|
| 框架 | React 19 + TypeScript 6（strict） |
| 构建 | Vite 8 + @vitejs/plugin-react 6 |
| 组件库 | shadcn/ui 4（见 [04 主题与 UI](04-theme-and-ui.md)，已支持 Tailwind v4） |
| 样式 | Tailwind CSS 4（**CSS-first**：`@theme` 变量主题，无 `tailwind.config.js`） |
| 路由 | React Router 8（**Declarative 模式** + BrowserRouter） || 服务端状态 | TanStack Query 5 |
| 客户端状态 | Zustand 5（仅主题 + 当前用户） |
| 表单 | React Hook Form 7 + Zod 4（经 @hookform/resolvers） |
| i18n | react-i18next 17 + i18next 26（默认 en-US，内置中文和日语） |
| 生产构建增强 | vite-plugin-sri3 2（注入 SHA512 SRI）+ 构建清单 |
| 常量管理 | 集中常量模块（消除魔法值） |
| 测试 | Vitest 4 + @testing-library/react 16 + MSW 2 |
| dev HTTPS | @vitejs/plugin-basic-ssl 2（满足 `__Host-` cookie 同源 HTTPS 前置） |

> **核查备注**：React Router 8 **保留** Declarative 模式与 `<BrowserRouter>`（依据：RR CHANGELOG「Declarative Mode」仍在、官方 advisory 并列 Declarative/Data 两模式、v8 讨论"无破坏性变更计划"）。故本设计的"RR8 + Declarative + BrowserRouter"成立，无需改用 `createBrowserRouter`。

## 依赖版本锁定（npm latest，2026-06-18 核实）

| 依赖 | 版本 | 文档 |
|---|---|---|
| react / react-dom | 19.2.7 | https://react.dev |
| typescript | 6.0.3 | https://www.typescriptlang.org/docs/ |
| vite | 8.0.16 | https://vite.dev/guide/ |
| @vitejs/plugin-react | 6.0.2 | https://www.npmjs.com/package/@vitejs/plugin-react |
| @vitejs/plugin-basic-ssl | 2.3.0 | https://www.npmjs.com/package/@vitejs/plugin-basic-ssl |
| tailwindcss | 4.3.1 | https://tailwindcss.com/docs/v4 |
| @tailwindcss/vite | v4 最新 | https://tailwindcss.com/docs/v4 |
| react-router | 8.0.0 | https://reactrouter.com/home |
| @tanstack/react-query | 5.101.0 | https://tanstack.com/query/latest/docs/framework/react/overview |
| zustand | 5.0.14 | https://github.com/pmndrs/zustand |
| react-hook-form | 7.79.0 | https://react-hook-form.com/get-started |
| @hookform/resolvers | 最新 | https://react-hook-form.com/docs/useform/SchemaValidation |
| zod | 4.4.3 | https://zod.dev/ |
| react-i18next | 17.0.8 | https://react.i18next.com/ |
| i18next | 26.3.1 | https://www.i18next.com/ |
| shadcn (CLI) | 4.11.0 | https://ui.shadcn.com/docs |
| @tabler/icons-react | 3.44.0 | https://tabler.io/icons |
| vite-plugin-sri3 | 2.0.0 | https://www.npmjs.com/package/vite-plugin-sri3 |
| vitest | 4.1.9 | https://vitest.dev/guide/ |
| @testing-library/react | 16.3.2 | https://testing-library.com/docs/react-testing-library/intro/ |
| msw | 2.14.6 | https://mswjs.io/docs/ |

## TypeScript / ES 基线（已核查）

- 采用 **TypeScript 6.0.3**；`tsconfig.json` 设 `"target": "ES2025"`、`"lib": ["ES2025", "DOM", "DOM.Iterable"]`、`"strict": true`。
- **核查结论（2026-06-18）**：ES2026 标准确已存在（第 17 版，2026 年 6 月定稿），但 **TS 6.0 无 `ES2026` 字面 target/lib**——TS 6.0 命名 target 最高为 `ES2025`（官方：ES2025 不含新 JS 语言特性，仅更新内置 API 类型），更高只能用 `esnext`。`ES2026` 字面 target 预计要等 TS 7.0（Go 原生版，开发中）。
- **决策**：经选项评估选定稳定命名 target **`ES2025`**（不取 `esnext` 以保证可预测升级）。
- 实际语法降级由 **Vite `build.target`**（按现代浏览器/browserslist）处理；TS 仅做类型检查（`noEmit`），`target` 主要决定默认 `lib` 与类型语义，对最终产物无直接影响。

---

← [索引](./README.md) · 下一板块：[02 架构与基础设施](02-architecture.md)
