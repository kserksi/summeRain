# 01 · 概览与技术栈

> [!WARNING]
> **已归档的设计记录。** 本页面早于已经完成的 V2 前端，
> 其中的版本、路径或实现状态可能已经过时。

> 所属：[前端架构设计（索引）](./README.md)

## 背景

现有前端使用原生 JavaScript（仓库根目录中的 `index.html`、`css/` 与 `js/`，包含 localStorage 模拟数据以及咖啡主题的明暗模式）。后端已经是基于 Go、Gin、MySQL、Redis 与 imgproxy 的生产级图床服务。

**目标：** 使用 React 19、Vite 8 与 TypeScript 重写前端，直接对接真实后端 API，取代模拟数据与原生实现。

## 硬性约束

以下约束来自后端 `cmd/server/main.go` 中的 `NoRoute` 处理器：

- 后端以 SPA 模式提供 `./web/*` 静态资源，并回退到 `./web/index.html`。
- 因此前端必须构建为放置在 `backend/web/` 中的**静态产物**，并与后端**同源**部署。
- 鉴权使用 cookie（`__Host-session_token` 与 `__Host-csrf_token`），且只有同源 HTTPS 才能生效，因此排除需要独立 Node 进程的 SSR 架构。

## 功能范围

前端精确对齐后端能力，覆盖五个领域：**认证、我的图片、个人资料、通知与后台管理**。不包含后端未支持的能力：浏览公开图库、分类/标签、后台图片审核与删除用户。

## 技术栈

> 下表中的版本已于 2026-06-18 根据各包 npm `dist-tag` 中稳定的 `latest` 版本完成核实。`package.json` 使用 `^` 范围，`package-lock.json` 则锁定精确版本，以保证安装可复现。

| 类别 | 选型 |
|---|---|
| 框架 | React 19 + TypeScript 6（`strict`） |
| 构建 | Vite 8 + @vitejs/plugin-react 6 |
| 组件库 | shadcn/ui 4（参见 [04 主题与 UI](04-theme-and-ui.md)，支持 Tailwind v4） |
| 样式 | Tailwind CSS 4（**CSS-first**：通过 `@theme` 定义主题变量，无 `tailwind.config.js`） |
| 路由 | React Router 8（**Declarative 模式** + BrowserRouter） |
| 服务端状态 | TanStack Query 5 |
| 客户端状态 | Zustand 5（仅主题与当前用户） |
| 表单 | React Hook Form 7 + Zod 4（通过 @hookform/resolvers） |
| 国际化 | react-i18next 17 + i18next 26（默认 `en-US`，内置中文与日语） |
| 生产构建增强 | vite-plugin-sri3 2（注入 SHA512 SRI）+ 构建清单 |
| 常量管理 | 集中式常量模块（消除魔法值） |
| 测试 | Vitest 4 + @testing-library/react 16 + MSW 2 |
| 开发 HTTPS | @vitejs/plugin-basic-ssl 2（满足 `__Host-` cookie 的同源 HTTPS 前置条件） |

> **核查备注：** React Router 8 **保留** Declarative 模式与 `<BrowserRouter>`（依据：RR changelog 仍记录 “Declarative Mode”，官方 advisory 并列 Declarative 与 Data 两种模式，v8 讨论中也说明没有破坏性变更计划）。因此，本设计的 “RR8 + Declarative + BrowserRouter” 组合仍然成立，无需改用 `createBrowserRouter`。

## 依赖版本锁定（npm latest，2026-06-18 核实）

| 依赖 | 版本 | 文档 |
|---|---|---|
| react / react-dom | 19.2.7 | https://react.dev |
| typescript | 6.0.3 | https://www.typescriptlang.org/docs/ |
| vite | 8.0.16 | https://vite.dev/guide/ |
| @vitejs/plugin-react | 6.0.2 | https://www.npmjs.com/package/@vitejs/plugin-react |
| @vitejs/plugin-basic-ssl | 2.3.0 | https://www.npmjs.com/package/@vitejs/plugin-basic-ssl |
| tailwindcss | 4.3.1 | https://tailwindcss.com/docs/v4 |
| @tailwindcss/vite | v4 最新版 | https://tailwindcss.com/docs/v4 |
| react-router | 8.0.0 | https://reactrouter.com/home |
| @tanstack/react-query | 5.101.0 | https://tanstack.com/query/latest/docs/framework/react/overview |
| zustand | 5.0.14 | https://github.com/pmndrs/zustand |
| react-hook-form | 7.79.0 | https://react-hook-form.com/get-started |
| @hookform/resolvers | 最新版 | https://react-hook-form.com/docs/useform/SchemaValidation |
| zod | 4.4.3 | https://zod.dev/ |
| react-i18next | 17.0.8 | https://react.i18next.com/ |
| i18next | 26.3.1 | https://www.i18next.com/ |
| shadcn (CLI) | 4.11.0 | https://ui.shadcn.com/docs |
| @tabler/icons-react | 3.44.0 | https://tabler.io/icons |
| vite-plugin-sri3 | 2.0.0 | https://www.npmjs.com/package/vite-plugin-sri3 |
| vitest | 4.1.9 | https://vitest.dev/guide/ |
| @testing-library/react | 16.3.2 | https://testing-library.com/docs/react-testing-library/intro/ |
| msw | 2.14.6 | https://mswjs.io/docs/ |

## TypeScript / ECMAScript 基线（已核查）

- 使用 **TypeScript 6.0.3**；在 `tsconfig.json` 中设置 `"target": "ES2025"`、`"lib": ["ES2025", "DOM", "DOM.Iterable"]` 与 `"strict": true`。
- **核查结论（2026-06-18）：** ES2026 标准确已存在（第 17 版，于 2026 年 6 月定稿），但 **TS 6.0 没有字面量 `ES2026` target/lib**。TS 6.0 的最高命名 target 为 `ES2025`（官方说明：ES2025 不包含新的 JavaScript 语言特性，只更新内置 API 类型）；更高版本只能使用 `esnext`。字面量 `ES2026` target 预计要等到 TS 7.0（正在开发的 Go 原生实现）。
- **决策：** 评估选项后，选择稳定的命名 target **`ES2025`**，不使用 `esnext`，以保证升级可预测。
- 实际语法降级由 **Vite `build.target`**（根据现代浏览器/browserslist）处理。TypeScript 仅执行类型检查（`noEmit`）；其 `target` 主要决定默认 `lib` 与类型语义，不会直接影响最终产物。

---

<- [索引](./README.md) · 下一章节：[02 架构与基础设施](02-architecture.md)
