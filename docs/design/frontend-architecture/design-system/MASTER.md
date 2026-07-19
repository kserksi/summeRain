# 设计系统 · MASTER（summeRain）

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> 前端 UI/UX 的**单一真相源**。所有页面（[10-pages-ui-ux.md](../10-pages-ui-ux.md)）据此派生；页面级例外放 `design-system/pages/<page>.md`，优先级高于本文件。本系统已在 `mockup/index.html` 原型验证通过。

## 1. 风格方向：Warm Soft Studio（maia）

- 以 shadcn **maia** 预设骨架为底（`npx shadcn@latest apply --preset b3RZAU6YV`），**忽略预设的颜色 / 字体 / 图标**，由下方咖啡令牌覆盖
- **卡片化、元素独立、大量圆角**（避免尖锐元素）、**半透明导航**（`inverted-translucent` + subtle accent）
- 冷静、专业、暖调（对标 Linear / Vercel / Stripe，用咖啡暖调替代冷蓝）；内容优先、每屏单一主 CTA、装饰极简
- 组件库：**shadcn/ui**（见 §5）

## 2. 色彩令牌（咖啡）

**浅色（摩卡）**

| token | 值 | 用途 |
|---|---|---|
| `--bg` | `#F1E7DA` | 页面背景 |
| `--bg2` | `#EADFCF` | 次级/输入底/表头 |
| `--card` | `#FFFCF8` | 卡片 |
| `--text` | `#33261B` | 正文 |
| `--muted` | `#6E5C49` | 辅助文字（对卡片 6.24:1） |
| `--border` | `#DAC7AE` | 边框/分隔 |
| `--primary` | `#6F4E37` | 主色（摩卡） |
| `--primaryHover` | `#573D2B` | 主色 hover |
| `--primarySoft` | `#EFE2D2` | 主色浅底 |
| `--accent` | `#A9764F` | 辅助/渐变 |
| `--success/--danger/--info/--warn` | `#5C7A4A / #A8412F / #4A6E92 / #B9742A` | 状态（柔化） |

**深色（浓缩）**

| token | 值 |
|---|---|
| `--bg / --bg2 / --card` | `#16100D / #1E1611 / #251B14` |
| `--text / --muted` | `#F2E7D6 / #C5B59E` |
| `--border` | `#4D3A29` |
| `--primary / --primaryHover / --primarySoft` | `#D4A57E / #B9895F / #33261829` |
| `--accent` | `#C39A72` |
| `success/danger/info/warn` | `#9CBF82 / #D88A78 / #86A8C9 / #E0AC64` |

- 正文对背景 **11.98:1**、辅助对卡片 **6.24:1**（均超 AA 4.5:1）
- **组件内禁裸 hex**，一律语义 token 类（`bg-primary`/`text-muted-foreground` 等）；暗色经 token 自动适配，**禁手写 `dark:` 覆盖**

## 3. 字体

- **系统字体栈**（不引入任何 webfont，满足 [07 外部资源本地化](../07-production-standards.md)）
- 字号阶：12 / 14 / 16 / 18 / 24 / 32 / 48；正文 **16**、行高 **1.65**
- 字重：400 正文 / 500 标签 / 600–700 标题
- 数字列、价格、计时用 `tabular-nums`（防布局抖动）
- 标题用 `clamp()` 流式（如落地 H1 `clamp(32px,6vw,58px)`）

## 4. 圆角 / 阴影 / 间距

- **圆角（maia · large）**：卡片 22–24px / 控件 12–14px / 大容器 28–30px / pill 999px
- **阴影**：暖棕基调多层柔影；**深色模式以低明度描边替代重阴影**
- **间距**：4/8 基线（4 / 8 / 12 / 16 / 24 / 32 / 48）

## 5. 组件（shadcn/ui）+ 组合规则

**清单**：表单 `Field/FieldGroup/FieldLabel/Input/InputGroup/Select/Switch/ToggleGroup/Slider/Textarea`；数据 `Card(全套)/Table/Badge/Avatar/Progress/Chart`；导航 `NavigationMenu/Tabs/Breadcrumb/Pagination`；覆盖层 `Dialog/AlertDialog/Sheet/Drawer/DropdownMenu/Tooltip/Popover`；反馈 `sonner/Alert/Skeleton/Spinner/Empty/Progress`；命令 `Command`；布局 `Separator/ScrollArea`。

**必守组合规则**：
- 表单一律 `FieldGroup`+`Field`（禁 raw div + `space-y`）；校验 `data-invalid`(Field)+`aria-invalid`(控件)
- 间距用 `gap-*`（禁 `space-x/y-*`）；等维度用 `size-*`；条件类用 `cn()`
- 空态用 `Empty`、提示用 `Alert`、toast 用 `sonner`、分隔用 `Separator`、占位用 `Skeleton`、状态用 `Badge`（**禁自造样式 div**）
- 破坏性操作用 `AlertDialog` 确认；`Dialog/Sheet` 必含 `Title`（视觉隐藏加 `sr-only`）
- `Card` 用全套 `CardHeader/Title/Description/Content/Footer`
- 覆盖层组件**禁手写 z-index**（自行管理堆叠）

**图标**：**Tabler Icons**（`@tabler/icons-react`），单一描线；入 `Button` 用 `data-icon="inline-start|end"`、**不加尺寸类**；图标按钮 ≥44×44 且带 `aria-label`。`components.json` 的 `iconLibrary` 设 **tabler**（覆盖预设的 phosphor）。

**禁原生控件**（统一外观）：自定义下拉取代原生 `<select>`；自定义滚动条；表格移动端转卡片。

## 6. 状态标准（统一）

`hover`（token 提亮）/ `active`（scale .97–.98）/ `disabled`（opacity .5 + cursor）/ `focus-visible`（2–4px 环）/ `loading`（spinner + 禁用）/ `empty`（Empty + 文案 + CTA）/ `error`（文案 + 重试）/ `skeleton`（>300ms）。

## 7. 响应式

- **mobile-first**；断点 375 / 768 / 1024 / 1440
- ≤820：导航 → **汉堡 + 侧滑抽屉**；表格 → **卡片**；各网格堆叠单列；字号/间距收窄
- 标题用 `clamp()`；**无横向滚动**
- 触控区 ≥44×44；容器桌面 `max-w-7xl`

## 8. 动效

- 微交互 150–300ms；ease-out 入 / ease-in 出；**尊重 `prefers-reduced-motion`**（全局禁用动画/过渡）；**不动画 width/height**
- 入场 `fadeUp` + 卡片错峰 stagger；卡片/按钮 hover 上浮；进度条 shimmer；loading 图标 spin；hero 网点 breathe
- 切页淡入

## 9. 主题切换（VT 为主 + 遮罩兜底）

- 顶栏**显式**切换钮（☀/🌙，不藏菜单）；`light`/`dark` 经 token 适配
- **支持 View Transitions**（Chrome/Edge）：`::view-transition-new(root)` 用 `clip-path: circle()` 从切换钮中心 **0→150%** 扩散——**真实新主题内容**圆形擦除（最佳效果）
- **不支持 VT**：`.theme-mask`（300vmax 圆，`transform:scale(0→1)`，Web Animations API 驱动）纯色圆形扩散兜底
- **`prefers-reduced-motion`**：直接切换、不播动画
- 持久化：localStorage；首屏跟随系统 `prefers-color-scheme`，pre-paint 内联脚本防闪烁

## 10. 灯箱（详情图）

详情图可点击（带"点击放大"提示）→ 全屏灯箱：放大图 + 关闭钮 + 遮罩；关闭方式 = 关闭钮 / 遮罩点击 / Esc；进出缩放过渡。

## 11. 无障碍与性能

- 对比度 ≥4.5:1（AA）；焦点环可见；Tab 序 = 视觉序；图标按钮 `aria-label`；图片 `alt`；语义标题层级
- 图片 `loading="lazy"` + 尺寸占位（防 CLS）；路由懒加载；列表 ≥50 虚拟化

---

← [索引](../README.md) · 页面规范：[10-pages-ui-ux.md](../10-pages-ui-ux.md)
