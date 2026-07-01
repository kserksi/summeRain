# 上传页 Popover 改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把上传页"复制全部链接"的容器从 DropdownMenu 换成受控 Popover，让用户切换格式选项时菜单保持打开，点"复制全部"成功后才同步关闭（同时弹 Sonner toast），失败时保持打开。

**Architecture:** 新增 1 个 shadcn Popover 组件文件（Radix Popover 薄封装，复用项目现有 popover-like className 风格）；Upload.tsx 替换 DropdownMenu JSX 为 Popover，DropdownMenuRadioGroup 换成现有 ToggleGroup（type=single），新增受控 `copyMenuOpen` state，copyAllLinks 改为 async 并仅在 `copy()` 返回 true 时关 popover。

**Tech Stack:** React 19 + TypeScript 6 + Radix Popover + 现有 ToggleGroup + Sonner + react-i18next

**Spec:** `docs/superpowers/specs/2026-07-01-upload-copy-popover-design.md`

---

## File Structure

| 文件 | 责任 | 性质 |
|------|------|------|
| `frontend/src/components/ui/popover.tsx` | Radix Popover 薄封装：Popover / PopoverTrigger / PopoverContent | 新建 |
| `frontend/src/features/images/pages/Upload.tsx` | 替换 DropdownMenu 为 Popover；ToggleGroup 承载选项；受控 open state；copyAllLinks async + 成功才关 | 修改 |

---

### Task 1: 新增 Popover 组件

**Files:**
- Create: `frontend/src/components/ui/popover.tsx`

- [ ] **Step 1: 创建 `frontend/src/components/ui/popover.tsx`**

```typescript
// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { Popover as PopoverPrimitive } from "radix-ui"
import { cn } from "@/lib/utils"

function Popover({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Root>) {
  return <PopoverPrimitive.Root data-slot="popover" {...props} />
}

function PopoverTrigger({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return (
    <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />
  )
}

function PopoverContent({
  className,
  align = "center",
  sideOffset = 4,
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="popover-content"
        align={align}
        sideOffset={sideOffset}
        className={cn(
          "dark z-50 w-72 origin-(--radix-popover-content-transform-origin) rounded-2xl p-4 text-popover-foreground shadow-2xl ring-1 ring-foreground/5 duration-200 animate-none! relative bg-popover/70 before:pointer-events-none before:absolute before:inset-0 before:-z-1 before:rounded-[inherit] before:backdrop-blur-2xl before:backdrop-saturate-150 data-[state=closed]:opacity-0",
          className,
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  )
}

export { Popover, PopoverTrigger, PopoverContent }
```

**设计说明：** 这是从 shadcn 官方 popover.tsx 模板派生的，className 采用项目 `dropdown-menu.tsx` 与 `sheet.tsx` 已有的"毛玻璃"风格（`bg-popover/70` + `before:backdrop-blur-2xl` + `before:backdrop-saturate-150` + `ring-1 ring-foreground/5`），保证视觉与既有浮层一致。导出仅 3 个：Popover / PopoverTrigger / PopoverContent（不导出 PopoverAnchor、PopoverClose 等未使用部分，YAGNI）。

- [ ] **Step 2: 验证 typecheck 通过**

Run: `npx tsc -b` (workdir: `D:\book\frontend`)
Expected: 无错误退出。若报 `Cannot find module 'radix-ui'` 或 `Popover as PopoverPrimitive` 不存在，先确认 `radix-ui` omnibus 包是否包含 Popover（参考 `frontend/src/components/ui/dropdown-menu.tsx:7` 的 `import { DropdownMenu as DropdownMenuPrimitive } from "radix-ui"` 同款写法，已验证可用）。

- [ ] **Step 3: 验证现有测试无回归**

Run: `npx vitest run` (workdir: `D:\book\frontend`)
Expected: 31/31 PASS。

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/ui/popover.tsx
git commit -m "feat(ui): add shadcn Popover component

Thin Radix Popover wrapper with the project's popover-like frosted-glass
className (matches dropdown-menu.tsx / sheet.tsx visual style). Exports
Popover, PopoverTrigger, PopoverContent."
```

---

### Task 2: 改造 Upload.tsx — DropdownMenu 换 Popover + ToggleGroup

**Files:**
- Modify: `frontend/src/features/images/pages/Upload.tsx`

- [ ] **Step 1: 调整 imports — 移除 DropdownMenu*，加 Popover 与 ToggleGroup**

找到当前 imports 中的这块（Task 5 上一版引入的）：

```typescript
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Label } from '@/components/ui/label'
```

替换为：

```typescript
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
```

注意：
- 完全移除 `@/components/ui/dropdown-menu` 整个 import 块
- `Label` import 顺序保持与原文件一致（按字母序，与 dropdown-menu 同组的位置现在空出，Label 自然上移）
- 新增 `Popover` 与 `ToggleGroup` import；按现有 `@/components/ui/*` 字母序插入

- [ ] **Step 2: 新增 `copyMenuOpen` 受控 state**

在 Upload 组件顶部 state 区，找到这块（Task 5 引入的 prefs state 旁边）：

```typescript
  const [linkFormat, setLinkFormat] = useState<CopyLinkFormat>(() => loadPrefs().link)
  const [imageFormat, setImageFormat] = useState<CopyImageFormat>(() => loadPrefs().image)
```

在 `imageFormat` state 之后追加一行（保持顺序，让所有 useState 集中）：

```typescript
  const [linkFormat, setLinkFormat] = useState<CopyLinkFormat>(() => loadPrefs().link)
  const [imageFormat, setImageFormat] = useState<CopyImageFormat>(() => loadPrefs().image)
  const [copyMenuOpen, setCopyMenuOpen] = useState(false)
```

- [ ] **Step 3: 把 `copyAllLinks` 改为 async，仅成功时关 popover**

找到当前的 `copyAllLinks`（Task 5 引入的）：

```typescript
  const copyAllLinks = () => {
    if (!completedLinks.length) return
    const text = buildCopyText(
      window.location.origin,
      completedLinks.map((i) => ({ uniqueLink: i.uniqueLink!, fileName: i.file.name })),
      linkFormat,
      imageFormat,
    )
    const fmt = `${t(`upload.copy.linkFormats.${linkFormat}`)}·${t(`upload.copy.imageFormats.${imageFormat}`)}`
    copy(text, t('upload.toast.copied', { count: completedLinks.length, format: fmt }))
  }
```

替换为：

```typescript
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

变化点：
- 函数从 `() => {...}` 变为 `async () => {...}`
- 用 `const ok = await copy(...)` 接住 `useCopy.copy()` 返回的 `Promise<boolean>`
- 仅 `ok === true` 时调用 `setCopyMenuOpen(false)` —— 失败时（剪贴板被拒等）popover 保持打开

- [ ] **Step 4: 替换 DropdownMenu JSX 为 Popover + ToggleGroup**

找到当前的 DropdownMenu 块（Task 5 引入的，包含 trigger + content + radio groups + 复制按钮）：

```tsx
                {completedLinks.length > 0 && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button size="sm" variant="outline">
                        {copied ? (
                          <IconCheck className="text-primary" />
                        ) : (
                          <IconLink />
                        )}
                        {t('upload.copy.button')} ({completedLinks.length})
                        <IconChevronDown className="size-3.5 opacity-60" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="min-w-56">
                      <DropdownMenuLabel>{t('upload.copy.linkFormatLabel')}</DropdownMenuLabel>
                      <DropdownMenuRadioGroup
                        value={linkFormat}
                        onValueChange={(v) => setLinkFormat(v as CopyLinkFormat)}
                      >
                        {(['url', 'markdown', 'bbs', 'html'] as const).map((f) => (
                          <DropdownMenuRadioItem key={f} value={f}>
                            {t(`upload.copy.linkFormats.${f}`)}
                          </DropdownMenuRadioItem>
                        ))}
                      </DropdownMenuRadioGroup>
                      <DropdownMenuSeparator />
                      <DropdownMenuLabel>{t('upload.copy.imageFormatLabel')}</DropdownMenuLabel>
                      <DropdownMenuRadioGroup
                        value={imageFormat}
                        onValueChange={(v) => setImageFormat(v as CopyImageFormat)}
                      >
                        {(['original', 'webp', 'avif'] as const).map((f) => (
                          <DropdownMenuRadioItem key={f} value={f}>
                            {t(`upload.copy.imageFormats.${f}`)}
                          </DropdownMenuRadioItem>
                        ))}
                      </DropdownMenuRadioGroup>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem onClick={copyAllLinks}>
                        <IconLink />
                        {t('upload.copy.action', { count: completedLinks.length })}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
```

替换为：

```tsx
                {completedLinks.length > 0 && (
                  <Popover open={copyMenuOpen} onOpenChange={setCopyMenuOpen}>
                    <PopoverTrigger asChild>
                      <Button size="sm" variant="outline">
                        {copied ? (
                          <IconCheck className="text-primary" />
                        ) : (
                          <IconLink />
                        )}
                        {t('upload.copy.button')} ({completedLinks.length})
                        <IconChevronDown className="size-3.5 opacity-60" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent align="end" className="w-64">
                      <div className="space-y-3">
                        <div className="space-y-1.5">
                          <Label className="text-xs text-muted-foreground">
                            {t('upload.copy.linkFormatLabel')}
                          </Label>
                          <ToggleGroup
                            type="single"
                            variant="outline"
                            size="sm"
                            orientation="vertical"
                            value={linkFormat}
                            onValueChange={(v) => v && setLinkFormat(v as CopyLinkFormat)}
                            className="w-full"
                          >
                            {(['url', 'markdown', 'bbs', 'html'] as const).map((f) => (
                              <ToggleGroupItem
                                key={f}
                                value={f}
                                className="w-full justify-start"
                              >
                                {t(`upload.copy.linkFormats.${f}`)}
                              </ToggleGroupItem>
                            ))}
                          </ToggleGroup>
                        </div>
                        <Separator />
                        <div className="space-y-1.5">
                          <Label className="text-xs text-muted-foreground">
                            {t('upload.copy.imageFormatLabel')}
                          </Label>
                          <ToggleGroup
                            type="single"
                            variant="outline"
                            size="sm"
                            orientation="vertical"
                            value={imageFormat}
                            onValueChange={(v) => v && setImageFormat(v as CopyImageFormat)}
                            className="w-full"
                          >
                            {(['original', 'webp', 'avif'] as const).map((f) => (
                              <ToggleGroupItem
                                key={f}
                                value={f}
                                className="w-full justify-start"
                              >
                                {t(`upload.copy.imageFormats.${f}`)}
                              </ToggleGroupItem>
                            ))}
                          </ToggleGroup>
                        </div>
                        <Separator />
                        <Button onClick={copyAllLinks} className="w-full">
                          <IconLink />
                          {t('upload.copy.action', { count: completedLinks.length })}
                        </Button>
                      </div>
                    </PopoverContent>
                  </Popover>
                )}
```

**关键差异说明（实现者必读）：**
1. **Popover 受控**：`open={copyMenuOpen} onOpenChange={setCopyMenuOpen}` —— 让 `copyAllLinks` 能在成功后通过 `setCopyMenuOpen(false)` 主动关，同时保留 Radix 默认的外部点击/ESC 关闭行为（onOpenChange 会自动被 Radix 调用）
2. **PopoverContent 用 div 包装内部**：Popover 不像 DropdownMenuContent 自带间距与分隔逻辑，需手动用 `<div className="space-y-3">` + `<Separator />` 组织布局
3. **每个 section 再包一层 `<div className="space-y-1.5">`**：让 Label 与对应 ToggleGroup 紧密成组
4. **ToggleGroup `type="single"`**：单选语义，替代 DropdownMenuRadioGroup
5. **`onValueChange={(v) => v && setLinkFormat(v as CopyLinkFormat)}`**：守卫空字符串。Radix ToggleGroup 在用户点击已选中的 item 时会反选并传入空字符串 `''`，必须守卫，否则 linkFormat/imageFormat 会变成无效值导致 `t()` 查 key 失败。`v &&` 短路：空字符串时跳过 set，保持原选择
6. **ToggleGroup `className="w-full"`**：覆盖组件内置的 `w-fit`，让 ToggleGroup 撑满 popover 宽度
7. **ToggleGroupItem `className="w-full justify-start"`**：每个选项撑满宽度、文字左对齐（默认居中）
8. **底部 Button `className="w-full"`**：复制按钮独占一行（不再是 DropdownMenuItem）

- [ ] **Step 5: 验证 typecheck 通过**

Run: `npx tsc -b` (workdir: `D:\book\frontend`)
Expected: 无错误退出。常见报错：
- `DropdownMenu` 仍被引用 → Step 1 移除 import 不彻底，或 Step 4 漏了某处替换
- `ToggleGroupItem` 未导入 → 检查 Step 1 的 import 块
- `copyMenuOpen` 未定义 → Step 2 漏加 state

- [ ] **Step 6: 验证现有测试无回归**

Run: `npx vitest run` (workdir: `D:\book\frontend`)
Expected: 31/31 PASS（本任务不新增测试，因 Popover/ToggleGroup 是 Radix 封装无自定义逻辑；Upload.tsx 是页面组件，无既有测试基线；底层 copy-format.ts 与 use-copy.ts 已被测试覆盖）。

- [ ] **Step 7: 验证没有残留 DropdownMenu 引用**

Run: `npx grep -nE "DropdownMenu" frontend/src/features/images/pages/Upload.tsx` (workdir: `D:\book`)
Expected: 无匹配。

- [ ] **Step 8: Commit**

```bash
git add frontend/src/features/images/pages/Upload.tsx
git commit -m "refactor(upload): replace DropdownMenu with controlled Popover

- Popover persists while user toggles format options (DropdownMenu
  auto-closed on every selection, breaking the 'set options then copy'
  mental model)
- ToggleGroup (type=single, vertical) replaces DropdownMenuRadioGroup
- copyAllLinks becomes async; popover closes only after copy() resolves
  true (success). On failure (clipboard denied), popover stays open so
  user can see state and retry
- onValueChange guards against empty string (Radix ToggleGroup sends
  '' when user re-clicks the selected item; we ignore it to keep an
  option always selected)"
```

---

### Task 3: 最终验证

**Files:** 无修改

- [ ] **Step 1: 跑全部单测**

Run: `npx vitest run` (workdir: `D:\book\frontend`)
Expected: 31/31 PASS。

- [ ] **Step 2: 跑生产构建**

Run: `npm run build` (workdir: `D:\book\frontend`)
Expected: 构建成功，输出 `../backend/web/`，无 TS 错误。注意任何 warning。

- [ ] **Step 3: 跑 lint**

Run: `npm run lint` (workdir: `D:\book\frontend`)
Expected: 在本 plan 触及的文件（`popover.tsx`、`Upload.tsx`）上**不引入新的** eslint 错误。其他文件预存的 lint 错误可接受（不要修）。

- [ ] **Step 4: 静态验收检查**

从 `D:\book` 运行：

(a) 确认 Upload.tsx 不再含 DropdownMenu 引用：
```
npx grep -nE "DropdownMenu" frontend/src/features/images/pages/Upload.tsx
```
Expected: 无匹配。

(b) 确认 onValueChange 守卫存在（防 ToggleGroup 反选清空）：
```
npx grep -nE "v && set(Link|Image)Format" frontend/src/features/images/pages/Upload.tsx
```
Expected: 至少 2 个匹配（一个 setLinkFormat，一个 setImageFormat）。

(c) 确认 popover.tsx 导出 3 个组件：
```
npx grep -n "^export" frontend/src/components/ui/popover.tsx
```
Expected: `export { Popover, PopoverTrigger, PopoverContent }`

(d) 确认 popover.tsx 不存在多余的 PopoverAnchor/PopoverClose/PopoverPortal 导出（YAGNI）。

- [ ] **Step 5: 列出本 plan 全部 commit**

Run: `git log --oneline 0de940e..HEAD` (workdir: `D:\book`)
Expected: 2 个实现 commit（Task 1 popover.tsx + Task 2 Upload.tsx）。

- [ ] **Step 6: 最终提交（仅在有 lint/build 修复时）**

如 Step 1-3 全过则无需提交；如需修复则只 stage 修复的文件，commit message：`chore: fix lint/build issues from popover refactor`。

---

## Self-Review Notes

**Spec coverage:**
- 决策 1（容器换成 Popover，受控）→ Task 1 创建组件 + Task 2 Step 2 引入 state、Step 4 `<Popover open={...} onOpenChange={...}>` ✓
- 决策 2（ToggleGroup 承载）→ Task 2 Step 1 import、Step 4 JSX ✓
- 决策 3（成功 toast + 同步关 popover）→ Task 2 Step 3 `if (ok) setCopyMenuOpen(false)` ✓
- 决策 4（失败保持打开）→ Task 2 Step 3 仅在 `ok === true` 才关；catch 路径走 useCopy 内 toast.error，不调 setCopyMenuOpen ✓
- 决策 5（外部点击关闭）→ Radix Popover 默认行为，受控 onOpenChange 自动处理 ✓
- ToggleGroup 反选空字符串 gotcha → Task 2 Step 4 关键差异 #5 + 验收 4(b) ✓
- 验收标准 1-10 → Task 3 Step 4 静态检查覆盖结构层面；行为层面（如 popover 实际打开/关闭时序）需手动验证或可后续补集成测试（不在本 plan 范围）

**Placeholder scan:** 所有 step 含完整代码或完整命令。无 TBD/TODO。Task 1 的 popover.tsx 含完整实现；Task 2 每个 step 含完整 old → new 代码块。

**Type consistency:**
- `copyMenuOpen: boolean` state 与 `Popover` 的 `open?: boolean` prop 一致 ✓
- `setCopyMenuOpen` 与 `onOpenChange?: (open: boolean) => void` 一致 ✓
- `copyAllLinks: () => Promise<void>`（async）与 `<Button onClick={copyAllLinks}>` 兼容（onClick 接受同步或异步函数）✓
- ToggleGroup `type="single"` 对应 `value: string` + `onValueChange: (value: string) => void` ✓
