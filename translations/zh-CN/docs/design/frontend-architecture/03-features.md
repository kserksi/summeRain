# 03 · 特性与页面

> [!WARNING]
> **已归档的设计记录。** 本页面早于已经完成的 V2 前端，
> 其中的版本、路径或实现状态可能已经过时。

> 所属：[前端架构设计（索引）](./README.md)

## 特性域与查询/变更

| 特性 | 关键 Query / Mutation | 路由 / 页面 |
|---|---|---|
| auth | `useMe` `useLogin` `useRegister` `useLogout` | `/login` `/register` |
| images | `useImages`（无限滚动）`useImage` `useUpload` `useDeleteImage` `useToggleVisibility` `useImageAccessToken` `useRevokeAccessToken` | `/images` `/images/:id` `/upload` |
| captcha | `usePublicConfig` `useCaptcha` | 启动探测 + 登录/注册内嵌 |
| user | `useProfile` `useChangePassword` | `/profile` |
| notifications | `useNotifications` `useMarkRead` `useMarkAllRead` `useDeleteNotification` `useClearNotifications` | 顶栏 `NotificationBell` 下拉菜单 |
| admin | `useAdminUsers`（分页）`useSetUserStatus` `useAdminStats` `useConfigs` `useUpdateConfigs` | `/admin` `/admin/users` `/admin/configs` |

## 落地页与控制台落点

- **`/`（公开）：** 始终是营销落地页（主视觉、特性与行动号召），**不包含**公开图库，因为后端没有相应接口。
- **已登录用户访问 `/`** 时，自动重定向到 **`/dashboard`**。
- **`/dashboard`（受保护的控制台落点）：** 登录后的目标页面，根据 `auth.user.role` 条件渲染：
  - 通用区域（所有用户）：个人统计、存储配额与最近图片（`useProfile`、`useImages`）。
  - 管理员区域（仅 `role==='admin'`）：系统概览卡片（`useAdminStats`）与指向 `/admin` 的“进入后台”链接。该区域懒加载，普通用户不会下载。
- **角色区分：** 使用条件渲染与路由守卫，而不是不同落点。`/admin/*` 同时受 `AdminGuard`（前端 `role` 检查）与后端 `RequireAdmin`（返回 403）保护，普通用户即使直接输入 URL 也无法进入。

## 关键交互流程（成功后的行为）

- **注册 `useRegister`：** 后端**不会自动登录用户**（API.md）。成功后跳转到 `/login`，并显示“注册成功，请登录”Toast；可以预填用户名。
- **修改密码 `useChangePassword`：** 后端会**清空所有会话**（API.md），当前 cookie 立即失效。成功后主动调用 `queryClient.clear()`、清空 `auth-store`、跳转到 `/login`，并显示“密码已修改，请重新登录”。不要等待后续 401 兜底，否则界面会停留在失效会话中。
- **切换可见性 `useToggleVisibility`：** 将图片设为私密会**使该图片的统一令牌失效**（后端返回 `tokens_revoked`/`warning`）。成功后若 `warning` 非空，则通过 Toast/Alert 显示，并使 `['images']` 与 `['images', id]` 失效。详情响应包含 `access_token`，因此令牌也会随之刷新。
- **上传/删除：** 成功后使 `['images']` 失效，并调用 `refreshUser()`，因为配额与计数发生了变化（参见 [02](02-architecture.md) 中的快照刷新机制）。

## 图片详情 `/images/:id`

展示大图、元数据、可见性控制，以及私密图片的统一令牌管理。只有所有者可以获取记录（由后端鉴权）。

## 私密图片（统一令牌模型）

`visibility=private` 时 `/i/:link` 的**访问规则**：

| 请求者 | 行为 |
|---|---|
| 所有者或管理员（同源会话） | **自动授权**并直接查看（`<img src="/i/<link>">` 无需令牌）；API 返回图片当前的统一令牌，用于展示/分享 |
| 携带**有效且未过期**令牌的第三方 | 放行，并使用 `no-store` |
| 令牌**缺失、错误或已过期**的第三方 | 返回 **403** |
| 携带**已吊销**令牌的第三方 | 返回 **404** |

**令牌规则：**
- 每张私密图片只有**一把统一令牌**；令牌存在期间，其**字符不可变**。
- **TTL：** 默认 **1 小时（3,600,000 ms）**，后端按**毫秒**校验。owner/admin 可在生成时选择有效期，范围为 **10 分钟（600,000 ms）至 72 小时（259,200,000 ms）**。过期令牌视为无效，第三方访问返回 403。
- **吊销：** 只有 owner/admin 可以吊销。**吊销后不会自动签发新令牌**；owner/admin 必须**手动再次申请**。在此之前，图片无法分享给第三方，但 owner/admin 会话仍可直接查看。
- 分享链接：公开图片 = `/i/<link>`；私密图片 = `/i/<link>?token=<令牌>`。令牌进入 URL 会带来日志/referrer 泄露风险，这是已接受的设计决策。

**前端行为：**
- owner/admin 的详情页展示当前令牌（支持复制）与分享链接；提供 TTL 选择器（默认 1h，范围 10min–72h，值集中在 `constants.ts`）；以及显式的“生成/重签令牌”和“吊销令牌”按钮，不执行任何自动操作。
- 第三方打开分享链接时，后端直接校验 `?token=`，不涉及前端会话。
- 列表与详情通过同源会话授权渲染所有者自己的私密图片，无需令牌。
- UI 文案区分 403（无权或令牌无效/过期）与 404（令牌已吊销）。
- Hook：`useImageAccessToken(id)` 获取/展示当前令牌，并通过携带所选 TTL 的 `POST .../tokens` 生成/重签令牌；`useRevokeAccessToken(id)` 用于吊销。

<a id="pluggable-captcha-administrator-selected-default-none"></a>
## 人机验证（可插拔，由管理员选择，默认无）

- `captcha_provider` 可为 `none`（默认）、`recaptcha`、`turnstile` 或 `geetest_v4`，由管理员通过 `/admin/configs` 配置。
- 启动时探测公开接口 `GET /public/config`，获取 `captcha_provider` 与该 provider 的客户端 key。`provider=none` 时**不加载任何外部脚本**（满足 [07 外部资源本地化](07-production-standards.md#local-external-resources)）。
- 启用后，在登录与注册页内嵌 `<Captcha>`，根据 provider 分支渲染。`useCaptcha()` 返回对应 provider 的校验载荷，随表单提交。
- 错误统一为 `2009`（校验失败）与 `1004`（服务不可用），文案由国际化资源提供。

**Provider 对照：**

| 维度 | reCAPTCHA v3 | Cloudflare Turnstile | 极验 CAPTCHA v4 |
|---|---|---|---|
| 客户端脚本 | `google.com/recaptcha/api.js?render=<site_key>`（或 recaptcha.net 镜像） | `challenges.cloudflare.com/turnstile/v0/api.js` | `static.geetest.com` 下的 `gcaptcha4.js` |
| 客户端公钥 | site_key | site_key | captcha_id |
| 取证调用 | `grecaptcha.execute(siteKey,{action})` -> token | `turnstile.render(el,{sitekey,action,callback})` -> token | `initGeetest4({captcha_id,product})` -> `{lot_number,captcha_output,pass_token,gen_time}` |
| 提交载荷 | `token` + `action` | `token` | `lot_number` + `captcha_output` + `pass_token` + `gen_time` |
| 服务端校验 | POST `…/api/siteverify`（`secret` + `response` + `remoteip`） | POST `challenges.cloudflare.com/turnstile/v0/siteverify`（`secret` + `response`） | POST `gcaptcha4.geetest.com/verify`（`captcha_id` + 四个参数，并使用 `captcha_key` 进行 HMAC 签名） |
| 判定 | `success` & `score≥阈值` & `action` & `hostname` | `success` | `result=='success'` |
| 中国可用性 | 差 | 较好 | 好（国产 provider） |

> **载荷信封（与后端 `CaptchaPayload` 对齐；参见 [API.md](../../API.md) §3）：** 在登录/注册请求体中嵌入 `captcha: { provider, token, action, lot_number, captcha_output, pass_token, gen_time }`。只填写当前 `provider` 对应字段：recaptcha -> `token` + `action`；turnstile -> `token`；geetest_v4 -> 四个参数。provider 不匹配时请求会被拒绝。recaptcha 的 `action` 必须严格为 **`"login"` 或 `"register"`**，与服务端 `ExpectedAction` 一致；`auth_service.go` 会分别为登录和注册注入该值。错误码 `2009` 与 `1004` 与 provider 无关；ErrRecaptcha* 命名继续保留，但其数值正确。

## 控制台数据来源（明确）

`/dashboard` 没有专属接口，而是聚合两个查询：
- **个人统计 + 配额** -> `useProfile`（`GET /user/profile`，包含 `storage_percent` 与 `image_count`）。
- **最近图片** -> `useImages` 的第一页（只取前若干条记录）。

`auth-store` 与 `/auth/me` 只用于**身份和 `role`**（守卫与顶栏用户名），不能作为展示数据源。`storage_used` 在三处出现；统一以 `useProfile` 为事实来源，避免歧义。

## 后端能力对齐

所有特性均遵循 `docs/API.md` 中的接口、字段、分页与错误码。后端未覆盖的能力，包括公开图库、分类/标签、后台图片审核与删除用户，不在本前端范围内；详见 [09 明确排除项](09-decisions-and-scope.md#explicit-exclusions-yagni)。

**已知后端限制：**
- **通知没有分页或数量上限**（`GET /notifications` 不接受分页参数；参见 API.md §7）。直接渲染返回列表，结果较多时在前端滚动/截断。若后端未来增加分页，则切换为 `useInfiniteQuery`。

---

<- [02 架构](02-architecture.md) · [索引](./README.md) · 下一章节：[04 主题与 UI](04-theme-and-ui.md)
