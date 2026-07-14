# 前端架构设计 · summeRain（索引）

- **日期**：2026-06-18
- **状态**：已敲定，待实现
- **范围**：用现代 React 栈重写图床前端，精准对齐 `backend/` 的 Go API（见 `docs/API.md`）

本设计已按板块拆分为独立文档，便于单独阅读与维护。各板块之间通过相对链接互相引用。

## 文档地图

| # | 板块 | 内容 | 文件 |
|---|---|---|---|
| 01 | 概览与技术栈 | 背景、目标、硬约束、技术栈、依赖版本锁定、TS/ES 基线 | [01-overview.md](01-overview.md) |
| 02 | 架构与基础设施 | 目录结构、API 封装、鉴权流、TanStack Query、zustand、路由与代码分割 | [02-architecture.md](02-architecture.md) |
| 03 | 特性与页面 | 五大特性域、Query/Mutation、路由与首页策略 | [03-features.md](03-features.md) |
| 04 | 主题与 UI | shadcn 预设策略、咖啡色板、字体、i18n | [04-theme-and-ui.md](04-theme-and-ui.md) |
| 05 | 构建与部署 | Vite 构建、产物输出、dev 代理、HTTPS 前置 | [05-build-and-deploy.md](05-build-and-deploy.md) |
| 06 | 测试策略 | Vitest + RTL + MSW 范围 | [06-testing.md](06-testing.md) |
| 07 | 生产产物规范 | 模块化、SRI/SemVer、资源本地化、变量/常量、HTML 精简、i18n（硬性要求） | [07-production-standards.md](07-production-standards.md) |
| 08 | 编码规范 | 命名、TS、React、状态、样式、Lint 门禁、安全无障碍、性能、提交 | [08-coding-standards.md](08-coding-standards.md) |
| 09 | 决策与范围 | 迁移清理、显式不做（YAGNI）、决策记录、参考 | [09-decisions-and-scope.md](09-decisions-and-scope.md) |
| 10 | 逐页 UI/UX | 8 页 + 全局组件、shadcn 映射、灯箱、私密令牌面板、响应式 | [10-pages-ui-ux.md](10-pages-ui-ux.md) |
| DS | 设计系统 | 风格/令牌/组件/状态/响应/动效/主题切换（单一真相源） | [design-system/MASTER.md](design-system/MASTER.md) |

## 阅读顺序建议

1. **01 概览** → 建立全局认知（做什么、用什么、约束）
2. **02 架构** → 理解代码骨架与数据流
3. **03 特性** + **04 主题** → 业务面与视觉面
4. **05 构建** → 如何跑起来与上线
5. **07 生产规范** + **08 编码规范** → 实现期准则
6. **06 测试** + **09 决策** → 验证与历史决策依据

> 原 `2026-06-18-frontend-architecture-design.md` 单文件已被本目录拆分取代。
