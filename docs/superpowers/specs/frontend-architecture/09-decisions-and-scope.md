# 09 · 决策与范围

> 所属：[前端架构设计（索引）](./README.md)

## 迁移与清理

- 现有原生 JS 前端（根目录 `index.html` / `css/` / `js/`）将被 `frontend/` 取代
- 新前端就绪并验证后，删除旧的原生前端文件
- 过渡期两者并存，互不影响（旧前端访问根目录，新前端开发期走 Vite、构建后落 `backend/web/`）

## 显式不做（YAGNI）

- 公开图库 / 发现页 / 社区精选（后端无公开列表接口）
- 图片分类 / 标签（后端 Image 模型无此字段）
- 后台图片审核列表（后端无此接口）
- 删除用户（后端仅支持改状态）
- 服务端渲染 / Node 中间层
- 字段映射转换层（后端 snake_case 直接使用）

## 决策记录

| 决策点 | 选择 | 理由 |
|---|---|---|
| 技术栈 | React + Vite + TS + shadcn/ui + Tailwind | 生态最广、组件库成熟，适配静态 SPA |
| 功能范围 | 精准对齐后端 | 避免做后端不支持的功能 |
| 工程结构 | 根目录 `frontend/` → 构建 `backend/web/` | 前后端源码隔离，满足后端静态服务约定 |
| 代码组织 | feature-based + TanStack Query | 五域边界清晰、高内聚、服务端状态集中 |
| shadcn 预设 | `apply --preset b3RZAU6YV`，忽略其颜色字体 | 复用预设组件骨架，主题沿用咖啡色板 |
| 路由 | BrowserRouter + 懒加载 | 后端 SPA fallback 支持；深链可用 |
| i18n | 保留能力，默认 zh-CN，文案进 i18n 组件 | 面向中国用户且需可扩展；覆盖早期 YAGNI |
| 生产产物 | SHA512 SRI + SemVer + 资源本地化 + 无魔法值 | 用户生产质量硬性要求，dev 阶段豁免 |
| TS/ES 基线 | target `ES2025`（TS 6.0 最高命名 target） | ES2026 字面量 TS 6.0 不支持；选稳定命名 target，实际降级交 Vite |
| 编码规范 | [08 编码规范](08-coding-standards.md)：官方指南 + 用户要求，Prettier/ESLint/tsc 门禁 | 统一实现准则，可量化可检查 |
| 会话切换安全 | 登出/登录 `queryClient.clear()` + 硬刷新；`auth-store` 不持久化；后端 `RequireAdmin` 403 兜底 | 防跨用户内存数据残留；重申前端非安全边界（见 [02](02-architecture.md)、[08](08-coding-standards.md)） |
| 私密图令牌模型 | 每图一把统一令牌；TTL 默认 1h（可调 10min–72h，毫秒校验）；字符不可变；吊销不自动重签、须 owner/admin 手动再申请；owner/admin 会话旁路直查；缺/错/过期→403，吊销→404 | 用户既定规则；后端已实现（API.md §4.3/§5，错误码 4037/4042） |
| 人机验证 | 可插拔三 provider（reCAPTCHA/Turnstile/极验 v4），管理员决定、默认 none | 中国受众主推 Turnstile/极验；captcha 为 §07 唯一受控外链豁免（默认 none 不破规则） |
| 视觉与页面 UI/UX | Warm Soft Studio（maia 卡片风 + 大圆角 + 咖啡 token）；Tabler Icons；逐页规范见 [设计系统 MASTER](design-system/MASTER.md)、[10-pages-ui-ux.md](10-pages-ui-ux.md) | 原型已验证；组件库 shadcn/ui，禁原生控件 |
| 图标库 | **Tabler Icons**（`@tabler/icons-react`），覆盖预设 phosphor | 单一描线、统一外观；`components.json` iconLibrary=tabler |
| 主题切换 | VT 为主（真内容圆形擦除）+ `.theme-mask` WAAPI 兜底；reduced-motion 直接切换 | 显式顶栏入口；最佳效果+全浏览器可用+无障碍 |

## 参考

- 后端 API 契约：`docs/API.md`
- 后端入口（SPA fallback / 静态服务）：`backend/cmd/server/main.go`
- 后端鉴权与 CSRF：`backend/internal/middleware/{auth,csrf}.go`
- 现有咖啡主题（待移植色值来源）：`css/style.css`
- 官方编码指南：[React](https://react.dev/learn/thinking-in-react) · [TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html) · [typescript-eslint](https://typescript-eslint.io) · [Tailwind CSS](https://tailwindcss.com/docs) · [Prettier](https://prettier.io) · [WCAG](https://www.w3.org/WAI/standards-guidelines/wcag/)

---

← [08 编码规范](08-coding-standards.md) · [索引](./README.md)
