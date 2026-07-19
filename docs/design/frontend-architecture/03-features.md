# 03 · 特性与页面

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> 所属：[前端架构设计（索引）](./README.md)

## 特性域与 Query/Mutation

| 特性 | 关键 Query / Mutation | 路由 / 页面 |
|---|---|---|
| auth | `useMe` `useLogin` `useRegister` `useLogout` | `/login` `/register` |
| images | `useImages`(无限滚动) `useImage` `useUpload` `useDeleteImage` `useToggleVisibility` `useImageAccessToken` `useRevokeAccessToken` | `/images` `/images/:id` `/upload` |
| captcha | `usePublicConfig` `useCaptcha` | 启动探测 + 登录/注册内嵌 |
| user | `useProfile` `useChangePassword` | `/profile` |
| notifications | `useNotifications` `useMarkRead` `useMarkAllRead` `useDeleteNotification` `useClearNotifications` | 顶栏 `NotificationBell` 下拉 |
| admin | `useAdminUsers`(分页) `useSetUserStatus` `useAdminStats` `useConfigs` `useUpdateConfigs` | `/admin` `/admin/users` `/admin/configs` |

## 首页与控制台落点

- **`/`（公开）**：永远是营销落地页（hero / 特性 / CTA），**无**公开图库（后端无此接口）
- **已登录访问 `/`** → 自动重定向 **`/dashboard`**
- **`/dashboard`（🔒 受保护，控制台落点）**：登录成功后落此；按 `auth.user.role` 条件渲染：
  - 通用区（所有人）：个人统计 + 存储配额 + 最近图片（`useProfile`、`useImages`）
  - 管理员区（仅 `role==='admin'`）：系统概览卡（`useAdminStats`）+ "进入后台"入口 → `/admin`；该区懒加载，普通用户不下载
- **角色区分机制**：靠"条件渲染 + 守卫路由"而非不同落点；`/admin/*` 由 `AdminGuard`（前端 `role` 校验）+ 后端 `RequireAdmin`（返 403）双重保护，普通用户即便手敲也进不去

## 关键交互流程（成功后行为）

- **注册 `useRegister`**：后端**不自动登录**（API.md）。成功 → 跳 `/login` + Toast"注册成功，请登录"（可预填用户名）。
- **改密 `useChangePassword`**：后端**清空所有会话**（API.md），当前 cookie 立即失效。成功 → 主动 `queryClient.clear()` + 清 auth-store + 跳 `/login` + Toast"密码已修改，请重新登录"（不等后续 401 兜底，避免卡在死会话）。
- **切换可见性 `useToggleVisibility`**：切为私密会**使该图统一令牌失效**（后端返回 `tokens_revoked`/`warning`）。成功后若 `warning` 非空 → Toast/Alert 展示，并失效 `['images']` 与 `['images', id]`（详情响应含 `access_token`，令牌随之刷新）。
- **上传/删除**：成功 → 失效 `['images']` + 调 `refreshUser()`（配额/计数变化，见 [02](02-architecture.md) 快照刷新机制）。

## 图片详情 `/images/:id`

大图、元信息、可见性切换、（私密图）统一令牌管理。仅所有者可查（后端鉴权）。

## 私密图片（统一令牌模型）

**访问规则**（`/i/:link` 且 `visibility=private`）：

| 请求者 | 行为 |
|---|---|
| 所有者或管理员（同源会话） | **自动授权**直接查看（`<img src="/i/<link>">` 无需拼 token）；API 返回该图当前统一令牌供展示/分享 |
| 第三方携带**有效且未过期**令牌 | 放行（`no-store`） |
| 第三方**未带/错令牌/已过期** | **403** |
| 第三方带**已吊销**令牌 | **404** |

**令牌规则**：
- 每张私密图**一把统一令牌**；令牌**字符不可变**（存在期间任何人不能改）
- **有效期（TTL）**：默认 **1 小时（3,600,000 ms）**，后端按**毫秒**校验；可由 owner/admin 在生成时选择，范围 **10 分钟（600,000 ms）～ 72 小时（259,200,000 ms）**；超期令牌按无效处理（→ 第三方 403）
- **吊销**：仅 owner/admin 可吊销；**吊销后不自动重签**——必须 owner/admin **再次手动申请**才会产生新令牌；在此之前该图对第三方永久不可分享（owner/admin 仍可经会话直接查看）
- 分享链：公开图 = `/i/<link>`；私密图 = `/i/<link>?token=<令牌>`（令牌进 URL，有日志/referer 泄露面，属既定设计）

**前端**：
- 详情页（owner/admin）：展示当前令牌（可复制）+ 分享链；TTL 选择器（默认 1h，范围 10min–72h，值收口 `constants.ts`）；"生成/重签令牌"、"吊销令牌"按钮（均显式动作，无自动）
- 第三方打开分享链：`?token=` 直接由后端校验，前端无会话
- 列表/详情中渲染 owner 自己的私密图：同源会话自动授权，无需 token
- 区分 403（无权/令牌无效或过期）与 404（令牌已吊销）的 UI 文案
- 钩子：`useImageAccessToken(id)`（取/展示当前令牌；`POST .../tokens` 携带所选 TTL 生成/重签）、`useRevokeAccessToken(id)`（吊销）

## 人机验证（可插拔，管理员决定，默认无）

- `captcha_provider` ∈ `none`(默认) | `recaptcha` | `turnstile` | `geetest_v4`，管理员经 `/admin/configs` 配置
- 启动探测 `GET /public/config`（公开）→ 取 `captcha_provider` 与该 provider 的客户端 key；provider=`none` 时**不加载任何外部脚本**（满足 [07 外链本地化](07-production-standards.md)）
- 启用时，登录/注册页内嵌 `<Captcha>`（按 provider 分支渲染）；`useCaptcha()` 返回该 provider 的校验载荷，随表单提交
- 错误统一走 `2009`（校验失败）/`1004`（服务不可用），文案经 i18n

**三家 provider 接口对照：**

| 维度 | reCAPTCHA v3 | Cloudflare Turnstile | 极验行为验证 v4 |
|---|---|---|---|
| 客户端脚本 | `google.com/recaptcha/api.js?render=<site_key>`（或 recaptcha.net 镜像） | `challenges.cloudflare.com/turnstile/v0/api.js` | `static.geetest.com` 下 `gcaptcha4.js` |
| 客户端公钥 | site_key | site_key | captcha_id |
| 取证调用 | `grecaptcha.execute(siteKey,{action})`→token | `turnstile.render(el,{sitekey,action,callback})`→token | `initGeetest4({captcha_id,product})`→`{lot_number,captcha_output,pass_token,gen_time}` |
| 提交载荷 | `token` + `action` | `token` | `lot_number` + `captcha_output` + `pass_token` + `gen_time` |
| 服务端校验 | POST `…/api/siteverify`(`secret`+`response`+`remoteip`) | POST `challenges.cloudflare.com/turnstile/v0/siteverify`(`secret`+`response`) | POST `gcaptcha4.geetest.com/verify`(`captcha_id`+四参，`captcha_key` HMAC 签名) |
| 判定 | `success` & `score≥阈值` & `action` & `hostname` | `success` | `result=='success'` |
| 中国可用性 | 差 | 较好 | 好（国产） |

> **载荷信封（与后端 `CaptchaPayload` 对齐，见 [API.md](../../API.md) §3）**：登录/注册请求体以 `captcha: { provider, token, action, lot_number, captcha_output, pass_token, gen_time }` 嵌入；按当前 `provider` 仅填对应字段（recaptcha→`token`+`action`；turnstile→`token`；geetest_v4→四参），传错 provider 会被拒。recaptcha 的 `action` 必须为 **`"login"`/`"register"`**（与服务端 `ExpectedAction` 严格比对，`auth_service.go` 在 login/register 分别注入）。错误码 `2009`/`1004` 已 provider 无关（命名沿用 ErrRecaptcha*，码值正确）。

## 控制台数据来源（明确）

`/dashboard` 无专属端点，聚合两个查询：
- **个人统计 + 配额** → `useProfile`（`GET /user/profile`，含 `storage_percent`、`image_count`）
- **最近图片** → `useImages` 首页（取首页前若干条）

`auth-store`/`/auth/me` 仅用于**身份与 `role`**（守卫、顶栏用户名），不作展示数据源；`storage_used` 三处都有，统一以 `useProfile` 为准，避免歧义。

## 后端能力对齐说明

各特性的接口、字段、分页与错误码完全遵循 `docs/API.md`。后端未覆盖的能力（公开图库、分类/标签、后台图片审核、删除用户）不在本前端范围，详见 [09 决策与范围](09-decisions-and-scope.md#显式不做yagni)。

**已知后端限制**：
- **通知无分页/上限**（`GET /notifications` 不带分页参数，见 API.md §7）：前端直接渲染返回列表；量大时前端做滚动/截断展示，后续若后端补分页再切换 `useInfiniteQuery`。

---

← [02 架构](02-architecture.md) · [索引](./README.md) · 下一板块：[04 主题与 UI](04-theme-and-ui.md)
