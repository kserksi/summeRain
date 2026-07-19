# 前端架构设计 · summeRain（索引）

> [!WARNING]
> **已归档的设计记录。** 本组设计文档写于 2026 年 6 月 18 日，早于已经完成的 V2 前端。
> 部分路径、版本、状态说明和范围决策已与当前实现不符。请仅将其作为历史设计背景，
> 不要作为当前运维指南。

- **日期：** 2026-06-18
- **状态：** 已敲定，待实现
- **范围：** 使用现代 React 技术栈重写图床前端，并与 `backend/` 中的 Go API 精确对齐（参见 `docs/API.md`）

本设计按主题拆分为独立文档，便于单独阅读与维护。各章节通过相对链接互相引用。

## 文档地图

| # | 章节 | 内容 | 文件 |
|---|---|---|---|
| 01 | 概览与技术栈 | 背景、目标、硬性约束、技术栈、依赖版本锁定、TypeScript/ECMAScript 基线 | [01-overview.md](01-overview.md) |
| 02 | 架构与基础设施 | 目录结构、API 封装、鉴权流程、TanStack Query、zustand、路由与代码分割 | [02-architecture.md](02-architecture.md) |
| 03 | 特性与页面 | 五大特性域、查询与变更、路由与首页策略 | [03-features.md](03-features.md) |
| 04 | 主题与 UI | shadcn 预设策略、咖啡色板、字体与国际化 | [04-theme-and-ui.md](04-theme-and-ui.md) |
| 05 | 构建与部署 | Vite 构建、产物输出、开发代理与 HTTPS 前置条件 | [05-build-and-deploy.md](05-build-and-deploy.md) |
| 06 | 测试策略 | Vitest、RTL 与 MSW 的测试范围 | [06-testing.md](06-testing.md) |
| 07 | 生产产物规范 | 模块化、SRI/SemVer、资源本地化、变量与常量、HTML 精简及国际化（硬性要求） | [07-production-standards.md](07-production-standards.md) |
| 08 | 编码规范 | 命名、TypeScript、React、状态、样式、Lint 门禁、安全与无障碍、性能及提交 | [08-coding-standards.md](08-coding-standards.md) |
| 09 | 决策与范围 | 迁移清理、明确排除项（YAGNI）、决策记录与参考资料 | [09-decisions-and-scope.md](09-decisions-and-scope.md) |
| 10 | 逐页 UI/UX | 8 个页面及全局组件、shadcn 映射、灯箱、私密令牌面板与响应式行为 | [10-pages-ui-ux.md](10-pages-ui-ux.md) |
| DS | 设计系统 | 风格、令牌、组件、状态、响应式、动效与主题切换（单一事实来源） | [design-system/MASTER.md](design-system/MASTER.md) |

## 建议阅读顺序

1. **01 概览** -> 建立全局认知：要构建什么、使用哪些工具、受哪些约束
2. **02 架构** -> 理解代码结构与数据流
3. **03 特性** + **04 主题** -> 了解业务与视觉两个维度
4. **05 构建** -> 了解如何运行与部署应用
5. **07 生产规范** + **08 编码规范** -> 遵循实现阶段的规则
6. **06 测试** + **09 决策** -> 理解验证方式及历史决策依据

> 原单文件 `2026-06-18-frontend-architecture-design.md` 已由本目录下的文档取代。
