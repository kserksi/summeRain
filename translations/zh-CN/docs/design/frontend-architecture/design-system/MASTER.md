# 设计系统 - MASTER（summeRain）

> [!WARNING]
> **已归档的设计记录。** 本页早于已完成的 V2 前端，可能包含已过时的版本、路径或
> 实现状态。

> 本文曾是前端 UI/UX 的**唯一事实来源**。所有页面
> （[10-pages-ui-ux.md](../10-pages-ui-ux.md)）均由此派生；页面级例外放在
> `design-system/pages/<page>.md`，优先级高于本文件。本系统已在
> `mockup/index.html` 原型中验证。

## 1. 风格方向：Warm Soft Studio（maia）

- 以 shadcn **maia** 预设结构为基础
  （`npx shadcn@latest apply --preset b3RZAU6YV`）。**忽略预设的颜色、字体和图标**，
  由下方咖啡令牌覆盖。
- 强调**卡片化、视觉独立的元素和大量圆角**，避免尖锐元素。使用带
  `inverted-translucent` 和轻微 accent 的**半透明 Navbar**。
- 界面保持冷静、专业、暖调。以 Linear、Vercel、Stripe 为参考，但用咖啡暖色替代冷蓝。
  内容优先，每屏只设一个主 CTA，装饰保持极简。
- 组件库：**shadcn/ui**；见第 5 节。

## 2. 色彩令牌（咖啡）

**浅色（摩卡）**

| Token | 值 | 用途 |
|---|---|---|
| `--bg` | `#F1E7DA` | 页面背景 |
| `--bg2` | `#EADFCF` | 次级表面、输入框背景、表头 |
| `--card` | `#FFFCF8` | 卡片 |
| `--text` | `#33261B` | 正文 |
| `--muted` | `#6E5C49` | 辅助文字；相对卡片为 6.24:1 |
| `--border` | `#DAC7AE` | 边框和分隔线 |
| `--primary` | `#6F4E37` | 主色（摩卡） |
| `--primaryHover` | `#573D2B` | 主色 hover |
| `--primarySoft` | `#EFE2D2` | 主色浅背景 |
| `--accent` | `#A9764F` | 辅助色和渐变 |
| `--success/--danger/--info/--warn` | `#5C7A4A / #A8412F / #4A6E92 / #B9742A` | 柔化的状态色 |

**深色（浓缩）**

| Token | 值 |
|---|---|
| `--bg / --bg2 / --card` | `#16100D / #1E1611 / #251B14` |
| `--text / --muted` | `#F2E7D6 / #C5B59E` |
| `--border` | `#4D3A29` |
| `--primary / --primaryHover / --primarySoft` | `#D4A57E / #B9895F / #33261829` |
| `--accent` | `#C39A72` |
| `success/danger/info/warn` | `#9CBF82 / #D88A78 / #86A8C9 / #E0AC64` |

- 正文与背景的对比度为 **11.98:1**，辅助文字与卡片为 **6.24:1**，均超过 AA 的 4.5:1。
- **组件内禁止使用裸 hex 值。** 一律使用 `bg-primary`、`text-muted-foreground` 等
  语义令牌类。深色模式自动通过令牌适配，**禁止手写 `dark:` 覆盖**。

## 3. 字体

- 使用**系统字体栈**，不引入 webfont，以满足
  [07 外部资源本地化](../07-production-standards.md)。
- 字号阶：12 / 14 / 16 / 18 / 24 / 32 / 48。正文为 **16**，行高 **1.65**。
- 字重：400 正文 / 500 标签 / 600-700 标题。
- 数字列、价格和计时器使用 `tabular-nums`，防止布局抖动。
- 标题使用流式 `clamp()`，例如落地页 H1 `clamp(32px,6vw,58px)`。

## 4. 圆角 / 阴影 / 间距

- **圆角（maia large）：** 卡片 22-24px / 控件 12-14px / 大容器 28-30px /
  pill 999px。
- **阴影：** 暖棕基调的多层柔影；深色模式以低明度描边取代厚重阴影。
- **间距：** 4/8 基线（4 / 8 / 12 / 16 / 24 / 32 / 48）。

## 5. 组件（shadcn/ui）与组合规则

**清单：** 表单
`Field/FieldGroup/FieldLabel/Input/InputGroup/Select/Switch/ToggleGroup/Slider/Textarea`；
数据 `Card`（完整组合）/ `Table/Badge/Avatar/Progress/Chart`；导航
`NavigationMenu/Tabs/Breadcrumb/Pagination`；覆盖层
`Dialog/AlertDialog/Sheet/Drawer/DropdownMenu/Tooltip/Popover`；反馈
`sonner/Alert/Skeleton/Spinner/Empty/Progress`；命令 `Command`；布局
`Separator/ScrollArea`。

**必须遵守的组合规则：**

- 表单统一使用 `FieldGroup` + `Field`，禁止使用 raw div + `space-y`。校验时，在 Field
  上使用 `data-invalid`，在控件上使用 `aria-invalid`。
- 间距使用 `gap-*`，禁止 `space-x/y-*`；等宽高使用 `size-*`；条件类使用 `cn()`。
- 空状态使用 `Empty`、提示使用 `Alert`、Toast 使用 `sonner`、分隔使用 `Separator`、
  占位使用 `Skeleton`、状态使用 `Badge`。**禁止自造带样式的 div 替代品。**
- 破坏性操作使用 `AlertDialog` 确认。每个 `Dialog/Sheet` 都必须包含 `Title`；视觉隐藏
  时使用 `sr-only`。
- `Card` 使用完整的 `CardHeader/Title/Description/Content/Footer` 组合。
- 覆盖层组件**禁止手写 z-index**；由组件自行管理堆叠。

**图标：** 使用单一描线风格的 **Tabler Icons**（`@tabler/icons-react`）。放入
`Button` 时使用 `data-icon="inline-start|end"`，并且**不要添加尺寸类**。图标按钮至少
44x44，并带有 `aria-label`。将 `components.json` 的 `iconLibrary` 设为 **tabler**，
替代预设的 phosphor 图标。

**需要统一外观时禁止原生控件：** 以自定义下拉替代原生 `<select>`，使用自定义滚动条，
并在移动端将表格转为卡片。

## 6. 统一状态标准

`hover`（令牌提亮）/ `active`（scale .97-.98）/ `disabled`（opacity .5 + cursor）/
`focus-visible`（2-4px 环）/ `loading`（spinner + disabled）/ `empty`
（`Empty` + 文案 + CTA）/ `error`（文案 + 重试）/ `skeleton`（>300ms）。

## 7. 响应式

- **Mobile first**；断点为 375 / 768 / 1024 / 1440。
- 宽度 <=820 时：Navbar -> **汉堡菜单 + 侧滑抽屉**；表格 -> **卡片**；所有网格
  堆叠为单列；字号和间距收窄。
- 标题使用 `clamp()`；**不得出现横向滚动**。
- 触控区至少 44x44；桌面容器使用 `max-w-7xl`。

## 8. 动效

- 微交互时长 150-300ms；进入用 ease-out，离开用 ease-in。通过全局禁用动画/过渡来
  **遵守 `prefers-reduced-motion`**。**不得对 width/height 做动画。**
- 入场使用 `fadeUp` + 卡片错峰；卡片和按钮 hover 上浮；进度条 shimmer；加载图标 spin；
  Hero 网点 breathe。
- 切页时淡入。

## 9. 主题切换（优先 View Transitions，遮罩兜底）

- 顶栏提供**显式**的 ☀/🌙 切换按钮，不隐藏在菜单中；`light` / `dark` 通过令牌适配。
- **支持 View Transitions 时**（Chrome/Edge），对 `::view-transition-new(root)` 使用
  `clip-path: circle()`，从切换按钮中心由 **0 扩散至 150%**。这会对**真实的新主题内容**
  进行圆形擦除，效果最佳。
- **不支持 View Transitions 时：** 使用 `.theme-mask`，即 300vmax 的纯色圆，并通过
  Web Animations API 执行 `transform:scale(0 to 1)` 动画。
- 使用 **`prefers-reduced-motion`** 时直接切换，不播放动画。
- 持久化到 localStorage；首次绘制时跟随系统 `prefers-color-scheme`，通过内联 pre-paint
  脚本防止闪烁。

## 10. 灯箱（详情图片）

详情图片可点击，并带有“点击放大”提示。点击后打开全屏灯箱，包含放大图、关闭按钮和遮罩。
可通过关闭按钮、点击遮罩或 Esc 关闭；进入和退出时使用缩放过渡。

## 11. 无障碍与性能

- 对比度至少为 4.5:1（AA）；焦点环可见；Tab 顺序与视觉顺序一致；图标按钮带
  `aria-label`；图片带 `alt`；标题层级具有语义。
- 图片使用 `loading="lazy"` 和尺寸占位以防 CLS；路由懒加载；包含至少 50 项的列表虚拟化。

---

<- [索引](../README.md) - 页面规范：
[10-pages-ui-ux.md](../10-pages-ui-ux.md)
