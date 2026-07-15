# 02 · 架构与基础设施

> 所属：[前端架构设计（索引）](./README.md)

## 目录结构

React 源码位于仓库根新建的 `frontend/`，构建产物输出到 `backend/web/`。

```
frontend/
├─ index.html
├─ vite.config.ts          # outDir: ../backend/web; base: '/'; dev proxy /api+/i → :8080; https dev
├─ src/styles.css          # Tailwind v4 入口：@import "tailwindcss"; @theme {...咖啡色板}; :root/.dark 变量
├─ tsconfig.json           # strict；target/lib 见 01-overview.md「TypeScript / ES 基线」节
├─ components.json         # shadcn 配置（Tailwind v4 模式）
├─ package.json            # version 字段即发布 SemVer（供 SRI 清单引用）
└─ src/
   ├─ main.tsx             # 挂载 QueryClientProvider + Router + ThemeProvider + Toaster
   ├─ App.tsx              # 路由树 + AuthGuard/AdminGuard + Layout
   ├─ config/
   │  └─ constants.ts      # 集中存储所有常量（见 07-production-standards.md，消除魔法值）
   ├─ i18n/
   │  ├─ index.ts          # i18next 初始化（默认 en-US）
   │  └─ locales/
   │     ├─ en-US.json     # 默认英语文案
   │     ├─ zh-CN.json     # 简体中文文案
   │     └─ ja-JP.json     # 日语文案
   ├─ lib/
   │  ├─ api.ts            # fetch 封装（核心）
   │  ├─ csrf.ts           # 读 __Host-csrf_token cookie
   │  ├─ query-client.ts   # QueryClient 配置
   │  └─ utils.ts          # cn() / formatSize / timeAgo / formatNumber
   ├─ store/
   │  ├─ theme-store.ts    # zustand persist（light/dark）
   │  └─ auth-store.ts     # 当前用户（hydrate / clear）
   ├─ components/
   │  ├─ ui/               # shadcn 生成（button/dialog/table/form/toast/...）
   │  └─ layout/           # Navbar / Footer / ThemeToggle / NotificationBell
   ├─ routes/
   │  └─ lazy.tsx          # React.lazy 路由级懒加载入口
   └─ features/
      ├─ auth/             # api.ts hooks.ts pages(Login, Register) components/
      ├─ captcha/          # api.ts(usePublicConfig) hooks.ts(useCaptcha) components(Captcha 按分支渲染)
      ├─ images/           # api.ts hooks.ts pages(List, Detail, Upload) components/
      ├─ user/             # api.ts hooks.ts pages(Profile) components/
      ├─ notifications/    # api.ts hooks.ts components(Dropdown)
      └─ admin/            # api.ts hooks.ts pages(Users, Stats, Configs) components/
```

每个 `features/<域>/` 自包含：`api.ts`（该域接口）、`hooks.ts`（Query/Mutation 钩子）、`pages/` 与 `components/`。跨域共享放 `components/` 与 `lib/`。

## 核心基础设施

### `lib/api.ts` —— 全站唯一请求出口

- `baseUrl = '/api/v1'`，所有请求 `credentials: 'include'`
- 非 GET 自动注入头 `X-CSRF-Token`（值取自 `__Host-csrf_token` cookie，见 `lib/csrf.ts`）
- 解析统一信封 `{ code, message, data }`：`code === 0` → 返回 `data`；否则 `throw new ApiError(code, message)`
- **错误码集中处理**（按 `code` 分流，不只看 HTTP）：
  - **401 / 4010 / 4011**（未认证/会话过期）：**默认**清空 auth-store、跳 `/login`；支持 per-request `{ skipAuthRedirect: true }` 豁免（`/auth/me` 探测用，401 仅判匿名不跳转），避免匿名用户在公开页被误踢
  - **4030**（账户已被禁用）：登出 + 跳 `/login` + 提示"账号已被停用"（被封用户重新登录也是此码，登录表单同样映射此提示）
  - **429 / 2008 / 4029 / 2090**（限流）：不自动重试（避免加剧），抛 `ApiError` 由 UI 提示"操作过于频繁，请稍后再试"；若响应含 `Retry-After` 头，按其给出倒计时
  - **4032 / 4033**（管理接口仅 Web / identity 误用）：属调用方式错误，提示并上报，不登出
- **错误文案 i18n**：前端按 `code` 映射到当前语言资源的 `errors.<code>` 键渲染（可翻译、不硬编码）；后端 `message` 仅作未知 `code` 的兜底文案。组件不直接显示后端 `message`。
- 暴露便捷方法：`api.get / post / patch / del`，以及 `api.upload(formData)`
- **不设字段映射层**：组件直接使用后端 snake_case 字段（如 `created_at`、`view_count`、`storage_used`），保持单一数据形状

### 鉴权流

- App 启动调用 `GET /auth/me`：成功 → 写 auth-store；**401 → 维持未登录态**。该请求带 `skipAuthRedirect: true`，401 不触发全局跳转，仅判定为匿名（区别于"会话中途失效"的 401）。
- 启动同时探测 `GET /public/config`（公开，无鉴权）→ 取 `captcha_provider` 与客户端 key，决定登录/注册是否加载人机验证（见 [03](03-features.md#人机验证可插拔管理员决定默认无)）；`provider=none` 不加载任何外部脚本。
- **`/i/` 图片渲染**：owner/admin 查看自己的/任意私密图 → 同源会话自动授权，`<img src="/i/<link>">` 无需 token；第三方经分享链 `/i/<link>?token=<令牌>` 访问。私密图响应 `no-store`，前端不缓存。
- `AuthGuard` 包裹需登录路由；`AdminGuard` 额外校验 `auth.user.role === 'admin'`
- **AdminGuard 降级自愈**：客户端 `role` 快照可能过期（管理员被降级）。若 admin 区请求命中 **4030/4032**，触发 `refreshUser()`（重拉 `/auth/me` 更新快照）并重定向离开 `/admin/*`；不再具备权限则回 `/dashboard`
- 登录成功后 cookie 由浏览器自动保存，前端失效相关 query 并跳转 **`/dashboard`**
- 已登录用户访问公开落点 `/` → 自动重定向 `/dashboard`（路由内 `<Navigate>`，无额外请求）
- 任意**非探测**请求 401（会话过期/被踢）→ 由 `lib/api.ts` 统一登出并跳 `/login`
- **登出**：调 `POST /auth/logout`（带 CSRF）成功后，按下方「会话切换的数据清理」清缓存并硬刷新回 `/`

### 会话切换的数据清理（安全）

防止跨用户**数据残留**：例如管理员登出后，同一标签页再登录的普通用户，不应从内存里读到上一会话的 admin 数据（注意：缓存的 JS chunk 本身无害——前端非安全边界，数据越权由后端 `RequireAdmin` 403 兜底；此处防的是**内存里的查询结果**）。

- **登出**：`queryClient.clear()` 清空全部 Query 缓存 → `auth-store` 清空 → `window.location.assign('/')` **硬刷新**。硬刷新销毁全部内存（QueryClient / React state / auth-store），新登录为全新页面会话。
- **登录成功**：同样 `queryClient.clear()` 一次（防御性），再跳 `/dashboard`。
- **`auth-store` 不持久化**（见下「客户端状态」）：localStorage 不留用户身份；首屏恒以 `GET /auth/me` 重新 hydrate，服务端为唯一真相。
- **后端兜底**：`/admin/*` 经 `RequireAdmin` 校验 `role`，普通用户 403。前端清缓存防"数据残留"，后端 403 防"越权取数"，二者叠加。
- 硬刷新不重新下载 bundle（全员同一份 chunk，命中 HTTP 缓存），几乎零开销。

### TanStack Query 约定

- key 规范：
  - `['images', { visibility, search }]`（列表）
  - `['images', id]`（详情；**私密图当前令牌随此响应返回的 `access_token` 字段**，无单独令牌接口/key——见 [API.md §5.3](../../API.md)）
  - `['admin', 'users', { page }]`
  - `['admin', 'stats']`、`['admin', 'configs']`
  - `['notifications']`、`['profile']`、`['public-config']`（人机验证 provider）
- 图片列表用 `useInfiniteQuery`：`getNextPageParam: last => last.has_more ? last.next_cursor : undefined`（**以 `has_more` 为终止判据**，`next_cursor` 仅作游标；二者不一致时信 `has_more`）
- 写操作 `useMutation` 成功后 `queryClient.invalidateQueries` 失效对应 key（如上传/删除/改可见性 → 失效 `['images']`）
- 默认 `refetchOnWindowFocus: true`（通知数、统计自动刷新）

### 客户端状态（zustand）

- `theme-store`：`persist` 存 `light` / `dark`（**localStorage 键名 `ic_theme`**，收口 `constants.ts`）；变更时给 `<html>` 加/去 `.dark` 类
- **主题切换动画**（详见 [MASTER §9](design-system/MASTER.md)）：优先 `document.startViewTransition`（可用性检测）做 `::view-transition-new(root)` clip-path 圆形扩散；不可用则 `.theme-mask`（WAAPI `transform:scale`）兜底；`prefers-reduced-motion` 直接切换不动画；切换前 pre-paint 内联脚本按已存主题防闪烁
- `auth-store`：仅存当前用户对象快照（首屏 hydrate），**不持久化**（仅内存；首屏恒以 `GET /auth/me` 重 hydrate），不缓存其他业务数据
- **快照刷新机制**：提供 `refreshUser()`（重调 `/auth/me` 写回 store），在以下时机触发——登录成功后、图片上传/删除/改可见性后（`storage_used`、`image_count` 变）、AdminGuard 降级自愈时。注：本范围无头像/资料编辑端点，故无其他刷新源；`/auth/me` 不纳入 Query 缓存（避免与 store 双源），仅按需显式调用。

## 路由与代码分割

- `BrowserRouter`（后端 `NoRoute` fallback 到 `index.html`，深链刷新可用；`base: '/'`，见 [05 构建与部署](05-build-and-deploy.md)）
- 路由表：
  - 公开：`/`（落地页）、`/login`、`/register`
  - 🔒 受保护（`AuthGuard`）：`/dashboard`、`/images`、`/images/:id`、`/upload`、`/profile`
  - 🔒 + `AdminGuard`：`/admin`、`/admin/users`、`/admin/configs`
  - 已登录访问 `/` → `<Navigate to="/dashboard">`；登录成功 → 跳 `/dashboard`
- `/dashboard` 为控制台落点，**按 `auth.user.role` 条件渲染**：通用区（个人统计/配额/最近图片）+ 仅 `admin` 的系统概览卡与"进入后台"入口；管理员区块**再经 `React.lazy` 懒加载**，普通用户不下载 admin 代码
- `React.lazy` 按特性/路由分包，`<Suspense>` + 全局 `<Spinner>` 兜底（动态 chunk 的完整性由 [07 SRI 运行时守卫](07-production-standards.md) 覆盖）

---

← [01 概览](01-overview.md) · [索引](./README.md) · 下一板块：[03 特性与页面](03-features.md)
