# 02 · 架构与基础设施

> [!WARNING]
> **已归档的设计记录。** 本页面早于已经完成的 V2 前端，
> 其中的版本、路径或实现状态可能已经过时。

> 所属：[前端架构设计（索引）](./README.md)

## 目录结构

React 源码位于仓库根目录中新建的 `frontend/`，构建产物输出到 `backend/web/`。

```
frontend/
├─ index.html
├─ vite.config.ts          # outDir: ../backend/web；base: '/'；开发代理 /api+/i -> :8080；HTTPS 开发环境
├─ src/styles.css          # Tailwind v4 入口：@import "tailwindcss"；@theme {...咖啡色板}；:root/.dark 变量
├─ tsconfig.json           # strict；target/lib 见 01-overview.md 的“TypeScript / ES 基线”章节
├─ components.json         # shadcn 配置（Tailwind v4 模式）
├─ package.json            # version 是 SRI 清单引用的发布 SemVer
└─ src/
   ├─ main.tsx             # 挂载 QueryClientProvider + Router + ThemeProvider + Toaster
   ├─ App.tsx              # 路由树 + AuthGuard/AdminGuard + Layout
   ├─ config/
   │  └─ constants.ts      # 集中存储所有常量（见 07-production-standards.md；消除魔法值）
   ├─ i18n/
   │  ├─ index.ts          # i18next 初始化（默认 en-US）
   │  └─ locales/
   │     ├─ en-US.json     # 默认英语文案
   │     ├─ zh-CN.json     # 简体中文文案
   │     └─ ja-JP.json     # 日语文案
   ├─ lib/
   │  ├─ api.ts            # 核心 fetch 封装
   │  ├─ csrf.ts           # 读取 __Host-csrf_token cookie
   │  ├─ query-client.ts   # QueryClient 配置
   │  └─ utils.ts          # cn() / formatSize / timeAgo / formatNumber
   ├─ store/
   │  ├─ theme-store.ts    # zustand persist（light/dark）
   │  └─ auth-store.ts     # 当前用户（hydrate / clear）
   ├─ components/
   │  ├─ ui/               # shadcn 生成（button/dialog/table/form/toast/...）
   │  └─ layout/           # Navbar / Footer / ThemeToggle / NotificationBell
   ├─ routes/
   │  └─ lazy.tsx          # React.lazy 路由级入口
   └─ features/
      ├─ auth/             # api.ts hooks.ts pages(Login, Register) components/
      ├─ captcha/          # api.ts(usePublicConfig) hooks.ts(useCaptcha) components(Captcha 按 provider 分支)
      ├─ images/           # api.ts hooks.ts pages(List, Detail, Upload) components/
      ├─ user/             # api.ts hooks.ts pages(Profile) components/
      ├─ notifications/    # api.ts hooks.ts components(Dropdown)
      └─ admin/            # api.ts hooks.ts pages(Users, Stats, Configs) components/
```

每个 `features/<域>/` 都是自包含模块：`api.ts` 放置该域接口，`hooks.ts` 放置 Query/Mutation Hook，另有 `pages/` 与 `components/`。跨域共享代码放在顶层 `components/` 与 `lib/` 中。

## 核心基础设施

### `lib/api.ts`：全站唯一请求出口

- 设置 `baseUrl = '/api/v1'`，所有请求均使用 `credentials: 'include'`。
- 非 GET 请求自动注入 `X-CSRF-Token` 头，其值通过 `lib/csrf.ts` 从 `__Host-csrf_token` cookie 读取。
- 拆解统一信封 `{ code, message, data }`：`code === 0` 时返回 `data`，否则 `throw new ApiError(code, message)`。
- **集中处理错误码**，按 `code` 而不只是 HTTP 状态分流：
  - **401 / 4010 / 4011**（未认证/会话过期）：**默认**清空 `auth-store` 并跳转到 `/login`。支持每次请求的 `{ skipAuthRedirect: true }` 豁免，供 `/auth/me` 探测使用；此时 401 仅表示匿名，不跳转，避免匿名用户在公开页面被错误踢出。
  - **4030**（账号已停用）：登出、跳转到 `/login`，并显示“账号已被停用”。被停用用户重新登录时也会收到此错误码，登录表单映射到相同提示。
  - **429 / 2008 / 4029 / 2090**（限流）：不自动重试，以免加剧负载。抛出 `ApiError`，由 UI 提示“操作过于频繁，请稍后再试”。若响应包含 `Retry-After`，显示对应倒计时。
  - **4032 / 4033**（管理接口仅限 Web/identity 误用）：属于调用方式错误，提示用户并上报，但不登出。
- **错误文案国际化：** 将 `code` 映射到当前语言资源的 `errors.<code>` 键。只有未知 `code` 才使用后端 `message` 兜底；组件绝不直接显示后端 `message`。
- 暴露便捷方法 `api.get`、`api.post`、`api.patch`、`api.del`，以及 `api.upload(formData)`。
- **不增加字段映射层。** 组件直接使用后端 snake_case 字段，例如 `created_at`、`view_count` 与 `storage_used`，维持单一数据形状。

### 鉴权流程

- 应用启动时调用 `GET /auth/me`。成功后写入 `auth-store`；收到 **401 时维持未登录状态**。该请求使用 `skipAuthRedirect: true`，因此 401 只判定访问者为匿名，不触发全局跳转；这与使用过程中会话失效导致的 401 不同。
- 同时探测无需鉴权的 `GET /public/config`，获取 `captcha_provider` 与对应客户端 key，以决定登录/注册是否加载人机验证（参见 [03](03-features.md#pluggable-captcha-administrator-selected-default-none)）。`provider=none` 时不加载任何外部脚本。
- **`/i/` 图片渲染：** owner/admin 查看自己的/任意私密图片时，由同源会话自动授权，`<img src="/i/<link>">` 无需令牌。第三方通过分享链接 `/i/<link>?token=<令牌>` 访问。私密图片响应使用 `no-store`，前端不缓存。
- `AuthGuard` 包裹需要登录的路由；`AdminGuard` 额外检查 `auth.user.role === 'admin'`。
- **AdminGuard 降级自愈：** 客户端 `role` 快照可能因管理员被降级而过期。若管理区域中的请求收到 **4030/4032**，调用 `refreshUser()` 重新请求 `/auth/me` 并更新快照，然后跳出 `/admin/*`；如果用户已无权限，则返回 `/dashboard`。
- 登录成功后，浏览器自动保存 cookie。使相关 Query 失效并跳转到 **`/dashboard`**。
- 已登录用户访问公开落点 `/` 时，通过路由内 `<Navigate>` 自动重定向到 `/dashboard`，不产生额外请求。
- 任意**非探测**请求返回 401，表示会话过期或被终止，由 `lib/api.ts` 统一登出并跳转到 `/login`。
- **登出：** 携带 CSRF 调用 `POST /auth/logout`。成功后按下节清理缓存数据，并硬刷新到 `/`。

### 会话切换时的数据清理（安全）

防止**跨用户数据残留**。例如，管理员登出后，普通用户在同一标签页登录时，不得从内存读取上一会话的管理数据。缓存的 JavaScript chunk 本身无害，因为前端不是安全边界；后端 `RequireAdmin` 返回 403，阻止未授权数据获取。此处要防的是**内存中的查询结果**。

- **登出：** 调用 `queryClient.clear()` 清空全部 Query 缓存，清空 `auth-store`，再通过 `window.location.assign('/')` 执行**硬刷新**。硬刷新会销毁 QueryClient、React state、`auth-store` 等全部内存，使下一次登录成为全新的页面会话。
- **登录成功：** 防御性地再次调用 `queryClient.clear()`，然后跳转到 `/dashboard`。
- **不持久化 `auth-store`**（见下方“客户端状态”）。localStorage 中不保留用户身份；首屏始终通过 `GET /auth/me` hydrate，并以服务端为单一事实来源。
- **后端兜底：** `/admin/*` 通过 `RequireAdmin` 检查 `role`，对普通用户返回 403。前端清缓存防止残留，后端 403 防止未授权获取，两层同时生效。
- 硬刷新不会重新下载 bundle：所有用户使用相同 chunk，并命中 HTTP 缓存，因此开销几乎为零。

### TanStack Query 约定

- Key 规范：
  - `['images', { visibility, search }]`（列表）
  - `['images', id]`（详情；响应的 `access_token` 字段包含私密图片当前令牌，不存在单独的令牌接口/key；参见 [API.md §5.3](../../API.md)）
  - `['admin', 'users', { page }]`
  - `['admin', 'stats']` 与 `['admin', 'configs']`
  - `['notifications']`、`['profile']` 与 `['public-config']`（人机验证 provider）
- 图片列表使用 `useInfiniteQuery`，配置 `getNextPageParam: last => last.has_more ? last.next_cursor : undefined`。**以 `has_more` 作为终止判据**；`next_cursor` 只作为游标。两者冲突时信任 `has_more`。
- `useMutation` 成功后，通过 `queryClient.invalidateQueries` 使对应 key 失效。例如，上传/删除/可见性变更使 `['images']` 失效。
- 默认设置 `refetchOnWindowFocus: true`，使通知数量与统计数据自动刷新。

### 客户端状态（zustand）

- `theme-store`：将 `light` 或 `dark` 持久化到**localStorage 键 `ic_theme`**，该键集中在 `constants.ts`。值变更时，在 `<html>` 上添加或移除 `.dark` 类。
- **主题切换动画**（详见 [MASTER §9](design-system/MASTER.md)）：优先在可用时调用 `document.startViewTransition`，通过 `::view-transition-new(root)` 的圆形 clip-path 揭示效果切换。不可用时，使用 `.theme-mask` 与 WAAPI `transform:scale` 动画兜底。启用 `prefers-reduced-motion` 时无动画直接切换。启动前由 pre-paint 内联脚本应用已存主题，避免闪烁。
- `auth-store`：只保存当前用户快照，用于首屏 hydrate。**不持久化**，仅存在内存中；首屏始终通过 `GET /auth/me` 重新 hydrate。不得缓存其他业务数据。
- **快照刷新：** 提供 `refreshUser()`，重新调用 `/auth/me` 并写回 store。在登录成功后、图片上传/删除/可见性变更后（影响 `storage_used` 与 `image_count`），以及 AdminGuard 降级自愈时触发。本范围没有头像/资料编辑接口，因此不存在其他刷新来源。不要将 `/auth/me` 纳入 Query 缓存，以免与 store 形成双数据源；按需显式调用。

## 路由与代码分割

- 使用 `BrowserRouter`。后端 `NoRoute` 回退到 `index.html`，因此深层链接刷新可用。设置 `base: '/'`；详见 [05 构建与部署](05-build-and-deploy.md)。
- 路由表：
  - 公开：`/`（落地页）、`/login`、`/register`
  - 由 `AuthGuard` 保护：`/dashboard`、`/images`、`/images/:id`、`/upload`、`/profile`
  - 由 `AuthGuard` + `AdminGuard` 保护：`/admin`、`/admin/users`、`/admin/configs`
  - 已登录用户访问 `/` -> `<Navigate to="/dashboard">`；登录成功 -> `/dashboard`
- `/dashboard` 是控制台落点，根据 `auth.user.role` 条件渲染：通用区域包含个人统计、配额与最近图片；仅 `admin` 可见系统概览卡与“进入后台”入口。管理区域还通过 `React.lazy` 懒加载，普通用户不会下载其代码。
- 使用 `React.lazy` 按特性/路由分割代码，以 `<Suspense>` 和全局 `<Spinner>` 作为兜底。[07 SRI 运行时守卫](07-production-standards.md)覆盖动态 chunk 的完整性。

---

<- [01 概览](01-overview.md) · [索引](./README.md) · 下一章节：[03 特性与页面](03-features.md)
