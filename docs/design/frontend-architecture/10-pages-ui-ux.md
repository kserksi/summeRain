# 10 · 逐页 UI/UX 规范

> 所属：[前端架构设计（索引）](./README.md) · 设计令牌与组件规则见 [design-system/MASTER.md](./design-system/MASTER.md)。以下每页布局已在 `mockup/index.html` 验证。所有组件用 shadcn/ui 组合实现。

## 公开页

### `/` 落地页
- **可见性**：所有人；已登录访问 → 自动跳 `/dashboard`
- **布局**：① Hero（咖啡渐变 + `clamp()` 大标题 + 双 CTA「立即上传/免费注册」）② 特性 **Bento**（4 卡）③ 三步用法 ④ CTA band
- **组件**：`Card`(Bento) + `Button` + `Badge` + `Separator`
- **动效**：hero 文案 `fadeUp` 错峰、网点 `breathe`；锚点平滑滚动
- **响应**：移动单列、CTA 全宽

### `/login` · `/register`
- **布局**：居中 `Card`（max-w 440）置于暖渐变背景；字段**标签在上**（`FieldGroup`+`Field`，非占位符）；密码显隐（`InputGroup`+`InputGroupAddon`）；主按钮全宽 + loading（`Spinner`+`disabled`）
- **人机验证槽**：provider≠none 时表单内嵌（见 [03](03-features.md#人机验证可插拔管理员决定默认无)）
- **组件**：`Card` + `Field` + `Input` + `Button` + `Alert`(错误) + 自定义下拉(无原生 select)
- **错误处理**：`2001` 凭证错→字段下红字；`2008/429` 限流→`Alert`+倒计时；`4030` 停用→`Alert`；`2009/1004` captcha；文案 i18n
- **流程**：登录成功 `queryClient.clear()`→`/dashboard`；注册成功（不自动登录）→`/login`+Toast

## 受保护页（AuthGuard）

### `/dashboard` 控制台（按角色分区）
- **布局**：顶部 Bento 统计卡 → 「最近图片」网格 + 侧栏（存储进度 + 管理员系统概览卡）
- **数据**：`useProfile`（统计/配额，含 `storage_percent`）+ `useImages`（最近）；管理员额外 `useAdminStats`（懒加载）；`storage_used` 统一以 `useProfile` 为准
- **组件**：`Card`(全套) + `Progress` + `Avatar` + `Chart`(admin 区) + `Empty`(无图)
- **角色分区**：通用区（所有人）+ 仅 `role==='admin'` 的系统概览卡 +「进入后台」按钮（`React.lazy`，普通用户不下载）
- **空态**：无图 → `Empty` +「上传第一张」

### `/images` 我的图片
- **布局**：工具栏（搜索 + 可见性筛选 + 网格/列表切换）→ 图片网格/列表 → 无限滚动
- **组件**：`Input`(搜索) + 自定义下拉/`ToggleGroup`(筛选/视图) + `Card` 网格 + `Skeleton` + `Empty` + `DropdownMenu`(行操作) + `AlertDialog`(删确认)
- **交互**：hover 卡片上浮 + 蒙层显文件名 + 操作（查看/删）；写操作带 CSRF
- **分页**：`useInfiniteQuery`，`has_more` 为终止判据；底部哨兵 + 骨架；错误重试
- **副作用**：上传/删除/改可见性 → 失效 `['images']` + `refreshUser()`

### `/images/:id` 图片详情（仅所有者）
- **布局**：双栏 —— 左大图（可点灯箱）/ 右信息面板
- **组件**：`Card`(外链/元信息/私密令牌) + `Table`/定义列表(元信息) + `InputGroup`(外链+复制 `InputGroupAddon`) + `Switch`/自定义下拉(可见性) + 私密令牌：`Badge`(状态) + 自定义下拉(TTL) + `Button`(生成/重签/吊销) + `Alert`(warning)
- **灯箱**：点大图 → 全屏灯箱（[MASTER §10](./design-system/MASTER.md)）
- **私密令牌面板**：当前令牌（脱敏+复制）、分享链 `/i/<link>?token=`、TTL 选择器（默认 1h，范围 10min–72h）、生成/重签/吊销（均显式，无自动）；切私密→`warning/tokens_revoked` Toast
- **响应**：移动单列（图在上）

### `/upload` 上传
- **布局**：大虚线 Dropzone（点击/拖拽，hover bounce）+ 选项卡（可见性、标签）+ 队列
- **组件**：Dropzone(token 样式) + `ToggleGroup`/自定义下拉(可见性) + `Field` + 队列：`Card`+`Progress`+`Badge`(状态)
- **校验**：单文件大小/类型/数量；`4012` 配额满 `Alert`；`4029/429` 限流；成功 Toast + `refreshUser`
- **状态**：每项 上传中(`Spinner`+流光进度)/完成(`Badge` success)/失败(重试)

### `/profile` 个人资料
- **布局**：账号信息卡 + 配额用量 + 修改密码卡
- **组件**：`Card` + `Badge`(角色/状态) + `FieldGroup`/`Field`(改密) + `Separator`
- **流程**：改密成功（后端清所有会话）→ `queryClient.clear()` + 清 auth-store + 跳 `/login` + Toast
- **范围**：无头像/资料编辑端点

## 管理员页（AdminGuard + 后端 RequireAdmin 403 兜底）

### `/admin` 后台概览
- **组件**：`Card` 统计墙 + `Chart`(柱状，懒加载，遵守 reduced-motion) + `Empty`
- **自愈**：命中 `4030/4032` → `refreshUser()` 并回 `/dashboard`

### `/admin/users` 用户管理
- **布局**：搜索 + 表格（桌面）/ 卡片（移动）
- **组件**：`Table`(桌面，`ScrollArea`/overflow) + `Badge`(角色/状态，色+文字) + `Avatar` + `Pagination` + `DropdownMenu`(封禁/解封) + `AlertDialog`(确认) + `Input`(搜索)
- **交互**：设 `suspended` → 后端清该用户会话+发通知；CSRF；**不能删用户**
- **响应**：≤820 表格 → 单列卡片（首格作头部、`td::before` 显 `data-label`，自定义滚动条）

### `/admin/configs` 系统配置
- **布局**：分区卡片表单（人机验证 / 私密令牌 / 其他）+ sticky 保存条
- **组件**：`Card` + `Field` + 自定义下拉(captcha provider) + `Input`(key) + `Slider`/`Input`(TTL) + `Button`(保存) + `Sonner`(成功)
- **内容**：captcha `provider`(none/recaptcha/turnstile/geetest_v4)+key；私密令牌默认 TTL（受 600000–259200000 ms 上下限）；切换 provider 提示将启用对应外链

## 全局组件（非独立页）

- **Navbar**：sticky + 半透明模糊；品牌（左）/ 按角色显隐链接 / `ThemeToggle`（显式 ☀/🌙，VT+遮罩）/ `NotificationBell`（带未读 dot，锚定按钮内）/ 用户菜单（`Avatar`+`AvatarFallback`+角色 `Badge`+退出）
  - **响应**：≤820 隐藏标签栏 → 汉堡 + 侧滑 `Sheet`（含全量导航、遮罩、Esc 关、选中即关）；品牌左、操作右
- **NotificationBell 下拉**：`DropdownMenu` 列表 + 未读 `Badge` + 全部已读/清空（CSRF）；空态 `Empty`
- **404**：`Empty` + `Button`(回首页)
- **Toaster**：`sonner`（success/error/限流/停用），3–5s 自动消失，`aria-live=polite`
- **自定义下拉**：替代原生 `<select>`（自定义箭头/选中勾/开合动画/外部点击关闭）
- **自定义滚动条**：主题色细条（去原生灰条）

---

## 落点与角色差异（重申）
三类角色落点一致（`/dashboard`）；差异仅在 `/dashboard` 是否渲染 admin 区、`/admin/*` 是否放行；所有越权由后端 403 兜底（见 [02](02-architecture.md)、[09](09-decisions-and-scope.md)）。

← [09 决策与范围](09-decisions-and-scope.md) · [索引](./README.md) · 设计系统：[MASTER](./design-system/MASTER.md)
