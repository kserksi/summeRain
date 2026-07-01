# 上传页"复制全部链接"改用 Popover 设计

- 日期：2026-07-01
- 状态：已通过 brainstorming，待 writing-plans 拆解
- 范围：仅前端，1 新建 + 1 修改
- 前序 spec：`2026-07-01-upload-copy-format-menu-design.md`（已实现）

## 背景

上一版把"复制全部链接"按钮做成了 `DropdownMenu`，菜单内含链接格式（URL/Markdown/BBS/HTML）与图片格式（原图/WebP/AVIF）两个 RadioGroup + 一个"复制 N 个链接"动作项。问题：DropdownMenu 的语义就是"选一项即关"，用户每次切换选项时菜单会闪退，体验差，与"用户先调好选项再复制"的心智模型不符。

## 目标

把容器从 `DropdownMenu` 换成 `Popover`：

- Popover 在用户与内部控件交互时**保持打开**（不像 DropdownMenu 那样自动关）
- 用户调好两组格式后点"复制全部 N 个链接"按钮 → Sonner toast 弹出成功提示 → **同步关闭 popover**
- 复制失败时 popover **保持打开**（让用户看到状态、可重试），仅 `toast.error` 提示
- 其他行为（默认值、localStorage 持久化、trigger 图标反馈、toast 文案）保持不变

## 非目标

- 不动 Detail.tsx（Detail 用的是 ShareRow 内嵌 useCopy，与本变更无关）
- 不动 use-copy.ts、copy-format.ts、i18n keys
- 不引入 RadioGroup 组件（用现有 ToggleGroup 承载选项）
- 不引入语言切换、不引入英文 locale

## 决策记录

| # | 项 | 选择 |
|---|----|----|
| 1 | 容器组件 | Popover（受控 open state） |
| 2 | 选项承载 | 现有 `ToggleGroup`（`type="single"` `variant="outline"` `size="sm"` `orientation="vertical"`） |
| 3 | 成功反馈 | Sonner toast（沿用现有 `upload.toast.copied`）+ 同步关闭 popover |
| 4 | 失败处理 | toast.error，popover 保持打开 |
| 5 | 外部点击 | 标准 Popover 行为：关闭（作为逃生口） |

## 设计

### 交互时序

```
1. 点 trigger → Popover 打开
2. 用户在两个 ToggleGroup 切选项 → popover 保持打开
3. 点"复制全部 N 个链接" → copyAllLinks 调 useCopy.copy() (async)
4. copy 成功 → toast.success 弹出 → setOpen(false) 关 popover
5. toast 按默认寿命（~4s）自动消失
6. trigger 上的图标短暂切到 IconCheck（1.5s，useCopy.copied 状态）
```

失败分支：`copy` 抛错 → `toast.error` 弹出，popover **保持打开**。

### 改动文件清单

| 文件 | 改动 |
|------|------|
| `frontend/src/components/ui/popover.tsx` | **新建**：shadcn Popover（Radix Popover 薄封装） |
| `frontend/src/features/images/pages/Upload.tsx` | 替换 DropdownMenu JSX 为 Popover；DropdownMenuRadioGroup → ToggleGroup；引入受控 open state；copyAllLinks 改为 async 并在成功后关 popover |

### `popover.tsx` 设计

标准的 shadcn Popover（与项目 `dropdown-menu.tsx`/`sheet.tsx` 风格一致）：

```tsx
import { Popover as PopoverPrimitive } from "radix-ui"
import { cn } from "@/lib/utils"

function Popover({...}) { /* Root */ }
function PopoverTrigger({...}) { /* Trigger */ }
function PopoverContent({...}) { /* Portal + Content with arrow/animation */ }

export { Popover, PopoverTrigger, PopoverContent }
```

完整实现交由 plan 阶段产出（参考 shadcn 官方 popover.tsx 模板 + 项目现有 `dropdown-menu.tsx` 的 `PopoverContent` className 风格）。

### Upload.tsx 关键改动

**① 新增受控 open state（顶部组件状态区，紧邻现有 prefs state）：**

```ts
const [copyMenuOpen, setCopyMenuOpen] = useState(false)
```

**② copyAllLinks 改为 async，成功后才关 popover：**

```ts
const copyAllLinks = async () => {
  if (!completedLinks.length) return
  const text = buildCopyText(
    window.location.origin,
    completedLinks.map((i) => ({ uniqueLink: i.uniqueLink!, fileName: i.file.name })),
    linkFormat,
    imageFormat,
  )
  const fmt = `${t(`upload.copy.linkFormats.${linkFormat}`)}·${t(`upload.copy.imageFormats.${imageFormat}`)}`
  const ok = await copy(text, t('upload.toast.copied', { count: completedLinks.length, format: fmt }))
  if (ok) setCopyMenuOpen(false)
}
```

（与上一版的差异：函数变为 `async`，使用 `await copy(...)` 获取成功标志，只有 `ok` 为 true 时才 `setCopyMenuOpen(false)`）

**③ 替换 DropdownMenu JSX 为 Popover：**

- imports 调整：移除 `DropdownMenu*` 系列、`DropdownMenuLabel`、`DropdownMenuRadioGroup`、`DropdownMenuRadioItem`、`DropdownMenuSeparator`、`DropdownMenuItem`、`DropdownMenuTrigger`、`DropdownMenuContent`；新增 `Popover, PopoverContent, PopoverTrigger` from `@/components/ui/popover`；新增 `ToggleGroup, ToggleGroupItem` from `@/components/ui/toggle-group`
- `<DropdownMenu>` → `<Popover open={copyMenuOpen} onOpenChange={setCopyMenuOpen}>`
- `<DropdownMenuTrigger asChild>` → `<PopoverTrigger asChild>`（trigger button 内部不变：保留 `copied ? IconCheck : IconLink` + 文案 + count + `IconChevronDown`）
- `<DropdownMenuContent align="end" className="min-w-56">` → `<PopoverContent align="end" className="w-64">`
- 内容布局：
  - `<Label>{t('upload.copy.linkFormatLabel')}</Label>` + `<ToggleGroup type="single" variant="outline" size="sm" orientation="vertical" value={linkFormat} onValueChange={(v) => v && setLinkFormat(v as CopyLinkFormat)} className="w-full">`
    - 4 个 `<ToggleGroupItem value="url|markdown|bbs|html" className="w-full justify-start">{t(...)}</ToggleGroupItem>`
  - `<Separator />`
  - `<Label>{t('upload.copy.imageFormatLabel')}</Label>` + 同上结构，3 个 ToggleGroupItem
  - `<Separator />`
  - `<Button onClick={copyAllLinks} className="w-full"><IconLink />{t('upload.copy.action', { count: completedLinks.length })}</Button>`

**ToggleGroup `onValueChange` 注意点**：Radix ToggleGroup 在反选当前项时会传空字符串 `''`。必须用 `v && setLinkFormat(v as CopyLinkFormat)` 守卫，避免清空选择。这是 spec 明确要求的行为——选项一旦选定不能被反选清空（保证 linkFormat/imageFormat 永远是有效 enum 值）。

### 不变的方面

- 默认值 `URL + WebP`、localStorage 持久化逻辑（`loadPrefs`/`savePrefs` 调用点不动）
- `buildCopyText` 调用方式、toast 文案格式
- trigger 按钮位置、外观、图标反馈时序（useCopy.copied）
- i18n key 全部复用，无新增
- Detail.tsx 完全不动

## 验收标准

1. 点 trigger → popover 弹出，不闪退
2. 在任一 ToggleGroup 里切选项，popover 保持打开（与 DropdownMenu 的关键差异）
3. 选项一旦选定，再次点击同一个不会清空（始终保留一个选中）
4. 点"复制 N 个链接"→ toast 弹出"已复制 N 个链接（X·Y）" → popover 同步关闭
5. trigger 图标短暂 IconCheck（1.5s）后恢复 IconLink
6. 模拟剪贴板拒绝（DevTools 删 `navigator.clipboard`）→ `toast.error` 弹出，popover **保持打开**
7. 点 popover 外部 → popover 关闭（标准 Popover 逃生口行为）
8. 按 ESC → popover 关闭（Radix Popover 默认行为）
9. 默认 `URL+WebP` 复制结果与上一版**逐字节一致**
10. 单测 31/31 仍通过；不引入新的 lint 错误；typecheck 通过

## 不改的东西

- 后端、API
- `i18n/index.ts`、locale 文件
- Detail.tsx、use-copy.ts、copy-format.ts
- DropdownMenu 组件本身（仍保留供其他地方使用，如 NotificationBell）
- 其他 feature 页面
