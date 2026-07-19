# 09 · 决策与范围

> [!WARNING]
> **已归档的设计记录。** 本页面早于已经完成的 V2 前端，
> 其中的版本、路径或实现状态可能已经过时。

> 所属：[前端架构设计（索引）](./README.md)

## 迁移与清理

- 仓库根目录 `index.html`、`css/` 与 `js/` 中的现有原生 JavaScript 前端将由 `frontend/` 取代。
- 新前端就绪并验证后，删除旧的原生前端文件。
- 过渡期内两套实现并存且互不影响：旧前端从仓库根目录访问，新前端在开发阶段使用 Vite，构建后输出到 `backend/web/`。

<a id="explicit-exclusions-yagni"></a>
## 明确排除项（YAGNI）

- 公开图库、发现页或社区精选（后端没有公开列表接口）
- 图片分类或标签（后端 Image 模型没有相应字段）
- 后台图片审核列表（后端没有相应接口）
- 删除用户（后端只支持修改状态）
- 服务端渲染或 Node 中间层
- 字段映射转换层（直接使用后端 snake_case 字段）

<a id="decision-record"></a>
## 决策记录

| 决策点 | 选择 | 理由 |
|---|---|---|
| 技术栈 | React + Vite + TypeScript + shadcn/ui + Tailwind | 生态广泛、组件库成熟，适合静态 SPA |
| 功能范围 | 精确对齐后端 | 避免实现后端不支持的能力 |
| 工程结构 | 仓库根目录 `frontend/` -> 构建到 `backend/web/` | 隔离前后端源码，同时满足后端静态服务约定 |
| 代码组织 | 基于特性 + TanStack Query | 五个领域边界清晰且高内聚，服务端状态集中管理 |
| shadcn 预设 | `apply --preset b3RZAU6YV`，忽略其颜色与字体 | 复用预设组件结构，同时保留咖啡色板 |
| 路由 | BrowserRouter + 懒加载 | 后端 SPA fallback 支持，深层链接可用 |
| 国际化 | 默认 `en-US`，内置 `zh-CN` 与 `ja-JP`，所有文案进入国际化资源 | 以英语为主要语言，同时保留完整本地化能力 |
| 生产产物 | SHA512 SRI + SemVer + 资源本地化 + 无魔法值 | 用户定义的生产质量硬性要求，开发阶段豁免 |
| TypeScript/ECMAScript 基线 | target `ES2025`（TS 6.0 最高命名 target） | TS 6.0 不支持字面量 ES2026 target；选择稳定的命名 target，实际降级交给 Vite |
| 编码规范 | [08 编码规范](08-coding-standards.md)：官方指南 + 用户要求，通过 Prettier/ESLint/tsc 门禁执行 | 形成统一、可量化、可检查的实现准则 |
| 会话切换安全 | 登出/登录调用 `queryClient.clear()` 并硬刷新；不持久化 `auth-store`；保留后端 `RequireAdmin` 403 校验 | 防止内存数据跨用户残留，并重申前端不是安全边界（参见 [02](02-architecture.md) 与 [08](08-coding-standards.md)） |
| 私密图片令牌模型 | 每张图片一把统一令牌；TTL 默认 1h，可在 10min–72h 调整并按毫秒校验；令牌字符不可变；吊销后不自动重签，owner/admin 必须手动再次申请；owner/admin 会话绕过令牌直接访问；缺失/错误/过期 -> 403，已吊销 -> 404 | 用户定义的规则，后端已经实现（API.md §4.3/§5，错误码 4037/4042） |
| 人机验证 | 三个可插拔 provider（reCAPTCHA/Turnstile/极验 v4），由管理员选择，默认 none | 面向中国用户优先采用 Turnstile/极验；CAPTCHA 是 §07 唯一受控外部资源例外，默认 none 不破坏规则 |
| 视觉设计与页面 UI/UX | Warm Soft Studio（maia 卡片风格 + 大圆角 + 咖啡令牌）；Tabler Icons；逐页规范见[设计系统 MASTER](design-system/MASTER.md)与 [10-pages-ui-ux.md](10-pages-ui-ux.md) | 原型已经验证；使用 shadcn/ui，禁止原生控件 |
| 图标库 | **Tabler Icons**（`@tabler/icons-react`），覆盖预设的 Phosphor 图标 | 单一描边风格，外观统一；`components.json` 使用 `iconLibrary=tabler` |
| 主题切换 | 主要使用 View Transitions（真实内容圆形揭示）并以 `.theme-mask` WAAPI 兜底；reduced motion 时立即切换 | 顶栏入口明确、效果最佳、浏览器覆盖广且满足无障碍要求 |

## 参考资料

- 后端 API 契约：`docs/API.md`
- 后端入口（SPA fallback/静态服务）：`backend/cmd/server/main.go`
- 后端鉴权与 CSRF：`backend/internal/middleware/{auth,csrf}.go`
- 现有咖啡主题（待迁移色值来源）：`css/style.css`
- 官方编码指南：[React](https://react.dev/learn/thinking-in-react) · [TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html) · [typescript-eslint](https://typescript-eslint.io) · [Tailwind CSS](https://tailwindcss.com/docs) · [Prettier](https://prettier.io) · [WCAG](https://www.w3.org/WAI/standards-guidelines/wcag/)

---

<- [08 编码规范](08-coding-standards.md) · [索引](./README.md)
