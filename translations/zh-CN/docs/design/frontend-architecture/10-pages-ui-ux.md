# 10 - 逐页 UI/UX 规范

> [!WARNING]
> **已归档的设计记录。** 本页早于已完成的 V2 前端，可能包含已过时的版本、路径或
> 实现状态。

> 所属：[前端架构设计（索引）](./README.md)。设计令牌与组件规则见
> [design-system/MASTER.md](./design-system/MASTER.md)。以下每页布局均已在
> `mockup/index.html` 中验证。所有组件都使用 shadcn/ui 组合实现。

## 公开页面

### `/` 落地页

- **可见性：** 所有人可见；已登录用户访问时自动跳转至 `/dashboard`。
- **布局：** （1）Hero：咖啡渐变、`clamp()` 大标题、双 CTA（“立即上传”/“免费注册”）；
  （2）四卡片特性 **Bento**；（3）三步用法；（4）CTA band。
- **组件：** `Card`（Bento）+ `Button` + `Badge` + `Separator`。
- **动效：** Hero 文案错峰 `fadeUp`、网点 `breathe`；锚点平滑滚动。
- **响应式：** 移动端单列，CTA 占满宽度。

### `/login` 与 `/register`

- **布局：** 暖渐变背景上的居中 `Card`（`max-w` 440）；字段**标签在控件上方**
  （`FieldGroup` + `Field`，不用占位符）；密码显隐（`InputGroup` +
  `InputGroupAddon`）；全宽主按钮及加载状态（`Spinner` + `disabled`）。
- **人机验证槽：** `provider != none` 时内嵌于表单；见
  [03](03-features.md#pluggable-captcha-administrator-selected-default-none)。
- **组件：** `Card` + `Field` + `Input` + `Button` + `Alert`（错误）+ 自定义下拉
  （不用原生 select）。
- **错误处理：** `2001` 凭证错误 -> 字段下方红字；`2008/429` 限流 -> `Alert` +
  倒计时；`4030` 停用 -> `Alert`；`2009/1004` CAPTCHA 错误；所有文案使用 i18n。
- **流程：** 登录成功后调用 `queryClient.clear()` 并跳转 `/dashboard`；注册成功后不自动
  登录，跳转 `/login` 并显示 Toast。

## 受保护页面（`AuthGuard`）

### `/dashboard` 控制台（按角色分区）

- **布局：** 顶部 Bento 统计卡，下方是“最近图片”网格和侧栏；侧栏包含存储进度和
  管理员系统概览卡。
- **数据：** `useProfile` 提供统计/配额（含 `storage_percent`），`useImages` 提供
  最近图片。管理员额外懒加载 `useAdminStats`。`storage_used` 始终以 `useProfile` 为准。
- **组件：** 完整 `Card` 组合 + `Progress` + `Avatar` + `Chart`（管理员区域）+
  `Empty`（无图片）。
- **角色分区：** 所有人共用区域；仅 `role==='admin'` 显示系统概览卡和“进入后台”按钮。
  使用 `React.lazy`，普通用户不下载管理员代码。
- **空状态：** 无图片 -> `Empty` + “上传第一张图片”。

### `/images` 我的图片

- **布局：** 搜索、可见性筛选、网格/列表切换工具栏；图片网格/列表；无限滚动。
- **组件：** `Input`（搜索）+ 自定义下拉 / `ToggleGroup`（筛选与视图）+ 网格 `Card` +
  `Skeleton` + `Empty` + `DropdownMenu`（行操作）+ `AlertDialog`（删除确认）。
- **交互：** hover 时卡片上浮，蒙层显示文件名及查看/删除操作；写操作携带 CSRF。
- **分页：** `useInfiniteQuery`；以 `has_more` 为终止判据；底部哨兵与骨架；错误重试。
- **副作用：** 上传/删除/可见性变更使 `['images']` 失效，并调用 `refreshUser()`。

### `/images/:id` 图片详情（仅所有者）

- **布局：** 双栏；左侧大图（点击打开灯箱），右侧信息面板。
- **组件：** `Card`（直链、元信息、私密令牌）+ `Table` / 定义列表（元信息）+
  `InputGroup`（直链和复制 `InputGroupAddon`）+ `Switch` / 自定义下拉（可见性）+
  私密令牌控件：`Badge`（状态）+ 自定义下拉（TTL）+ `Button`（生成/重签/吊销）+
  `Alert`（警告）。
- **灯箱：** 点击大图打开 [MASTER 第 10 节](./design-system/MASTER.md) 描述的全屏灯箱。
- **私密令牌面板：** 当前令牌（脱敏并可复制）、分享链接 `/i/<link>?token=`、TTL 选择器
  （默认 1h，范围 10min-72h），以及明确的生成/重签/吊销操作，不自动执行任何操作。
  切换为私密时显示 `warning/tokens_revoked` Toast。
- **响应式：** 移动端单列，图片在上。

### `/upload` 上传

- **布局：** 用于点击/拖拽并带 hover bounce 的大虚线 Dropzone；可见性和标签选项；队列。
- **组件：** Dropzone（token 样式）+ `ToggleGroup` / 自定义下拉（可见性）+ `Field` +
  使用 `Card` + `Progress` + `Badge`（状态）的队列项目。
- **校验：** 单文件大小/类型/数量；`4012` 配额已满 `Alert`；`4029/429` 限流；
  成功 Toast + `refreshUser`。
- **状态：** 每项为上传中（`Spinner` + 流光进度）、已完成（`Badge` success）或失败（重试）。

### `/profile` 个人资料

- **布局：** 账号信息卡、配额用量、修改密码卡。
- **组件：** `Card` + `Badge`（角色/状态）+ `FieldGroup` / `Field`（修改密码）+
  `Separator`。
- **流程：** 修改密码成功后，后端清除全部会话；调用 `queryClient.clear()`、清除
  auth-store、跳转 `/login` 并显示 Toast。
- **范围：** 不提供头像或资料编辑端点。

## 管理员页面（`AdminGuard` + 后端 `RequireAdmin` 403 兜底）

### `/admin` 后台概览

- **组件：** `Card` 统计墙 + `Chart`（柱状图、懒加载、遵守 reduced motion）+ `Empty`。
- **自愈：** 命中 `4030/4032` 时调用 `refreshUser()` 并返回 `/dashboard`。

### `/admin/users` 用户管理

- **布局：** 搜索 + 桌面表格 / 移动端卡片。
- **组件：** `Table`（桌面，`ScrollArea` / overflow）+ `Badge`（角色/状态，颜色加文字）+
  `Avatar` + `Pagination` + `DropdownMenu`（封禁/恢复）+ `AlertDialog`（确认）+
  `Input`（搜索）。
- **交互：** 设置 `suspended` 后，后端清除该用户会话并发送通知；携带 CSRF；
  **不能删除用户。**
- **响应式：** 宽度 <=820 时，以单列卡片替代表格。首个单元格作为标题，通过
  `td::before` 显示 `data-label`，并使用自定义滚动条。

### `/admin/configs` 系统配置

- **布局：** CAPTCHA、私密令牌和其他设置的分区卡片表单 + sticky 保存条。
- **组件：** `Card` + `Field` + 自定义下拉（CAPTCHA provider）+ `Input`（key）+
  `Slider` / `Input`（TTL）+ `Button`（保存）+ `Sonner`（成功）。
- **内容：** CAPTCHA `provider`（`none/recaptcha/turnstile/geetest_v4`）+ key；
  私密令牌默认 TTL，范围为 600000-259200000 ms；切换 provider 时提示将启用对应外链。

## 全局组件（非独立页面）

- **Navbar：** sticky + 半透明模糊；左侧品牌；按角色显示链接；显式 ☀/🌙 的
  `ThemeToggle`（VT + 遮罩）；`NotificationBell`（未读 dot 锚定在按钮内）；用户菜单
  （`Avatar` + `AvatarFallback` + 角色 `Badge` + 退出）。
  - **响应式：** 宽度 <=820 时隐藏标签栏，改用汉堡菜单和侧滑 `Sheet`；包含完整导航、
    遮罩、Esc 关闭、选中即关闭。品牌在左，操作在右。
- **NotificationBell 下拉：** `DropdownMenu` 列表 + 未读 `Badge` + 全部已读/全部清空
  （CSRF）；无通知时使用 `Empty`。
- **404：** `Empty` + 返回首页的 `Button`。
- **Toaster：** `sonner` 展示成功、错误、限流、停用消息；3-5s 后自动消失；
  `aria-live=polite`。
- **自定义下拉：** 取代原生 `<select>`，提供自定义箭头、选中勾、开合动画、外部点击关闭。
- **自定义滚动条：** 使用主题色细条，替代原生灰条。

---

## 落点与角色差异

三类角色都落在 `/dashboard`。差异仅在于 `/dashboard` 是否渲染管理员区域，以及是否允许
访问 `/admin/*`。所有越权访问最终由后端 403 保护。参见 [02](02-architecture.md) 和
[09](09-decisions-and-scope.md)。

<- [09 决策与范围](09-decisions-and-scope.md) - [索引](./README.md) -
设计系统：[MASTER](./design-system/MASTER.md)
