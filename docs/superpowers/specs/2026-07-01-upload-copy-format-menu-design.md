# 上传页"复制全部链接"格式菜单设计

- 日期：2026-07-01
- 状态：已通过 brainstorming，待 writing-plans 拆解
- 范围：仅前端，4 个文件改动（1 新建）

## 背景

`frontend/src/features/images/pages/Upload.tsx` 批量上传完成后提供"复制全部链接"按钮，当前硬编码输出 `${origin}/i/<link>.webp?q=85` 的纯 URL 列表。用户无法选择链接格式（Markdown/BBS/HTML/URL）或图片格式（原图/WebP/AVIF）。

同时存在可靠性问题：现有 `copyAllLinks` 函数（`Upload.tsx:186-191`）`navigator.clipboard.writeText` 未 `await`、无 `try/catch`，剪贴板被拒或非 HTTPS 上下文时会误报成功或静默失败。

## 目标

1. 在上传页"复制全部链接"按钮上提供格式菜单（链接格式 × 图片格式）
2. 用户选择跨会话记忆（localStorage）
3. Upload.tsx 整页接入 i18n（仅 zh-CN，不引入 en）
4. 所有复制类按钮（Upload + Detail）三层反馈：可靠性 + 视觉反馈 + toast

## 非目标

- 不引入英文 locale、不加语言切换 UI
- 不改 Detail.tsx 的非复制相关代码、不对 Detail.tsx 做 i18n
- 不改后端、API、其他 feature 页面

## 决策记录

| # | 项 | 选择 |
|---|----|----|
| 1 | UI 形态 | 整按钮变下拉菜单 |
| 2 | 持久化 | localStorage 记忆上次选择 |
| 3 | AVIF 质量 | `?q=85`（与 WebP 一致） |
| 4 | i18n 范围 | Upload.tsx 整页 i18n 化（仅 zh-CN） |
| 5 | alt 文本 | 原文件名去扩展名 |
| 6 | 复制反馈范围 | Upload + Detail 全部接入 |

## 设计

### URL 构造规则

| 图片格式 | URL |
|---------|-----|
| 原图 | `${origin}/i/${link}` |
| WebP | `${origin}/i/${link}.webp?q=85` |
| AVIF | `${origin}/i/${link}.avif?q=85` |

### 单行格式规则

`baseName` = `item.file.name` 去掉最后一段扩展名。

| 链接格式 | 输出 |
|---------|------|
| URL | `${url}` |
| Markdown | `![${baseName}](${url})` |
| BBS | `[img]${url}[/img]` |
| HTML | `<img src="${url}" alt="${baseName}">` |

多行之间用 `\n` 拼接。

### 状态与持久化

```ts
type CopyLinkFormat = 'url' | 'markdown' | 'bbs' | 'html'
type CopyImageFormat = 'original' | 'webp' | 'avif'

// localStorage key: 'upload:copyPrefs'
// 值: { link: CopyLinkFormat, image: CopyImageFormat }
// 默认: { link: 'url', image: 'webp' } — 与改动前行为等价
```

`useState` 初始化时读 localStorage；`useEffect` 在变化时写回。

### UI 结构

替换 `Upload.tsx:278-287` 的"复制全部链接"按钮为 `DropdownMenu`：

```
[ 复制全部链接 (N) ▾ ]              ← 点整颗按钮弹出菜单
┌─────────────────────────────┐
│ 链接格式                     │
│ ○ URL                       │
│ ○ Markdown                  │
│ ○ BBS (BBCode)              │
│ ● HTML                      │  ← RadioGroup，选中状态
├─────────────────────────────┤
│ 图片格式                     │
│ ○ 原图                       │
│ ● WebP                      │
│ ○ AVIF                      │
├─────────────────────────────┤
│ [复制 N 个链接]              │  ← 点击后复制并关菜单
└─────────────────────────────┘
```

- 触发器外观沿用 `Button variant="outline" size="sm"`，末尾加 `IconChevronDown`
- 两个 `DropdownMenuRadioGroup` 切选项不关菜单；最后一项点击执行复制并关闭
- 复制成功后触发器主图标 `IconLink → IconCheck` + 主色 1.5s

### `useCopy` hook

新建 `frontend/src/lib/use-copy.ts`：

```ts
export function useCopy(timeout = 1500): {
  copied: boolean
  copy: (text: string, successMsg?: string) => Promise<boolean>
}
```

- 内部调 `useTranslation()`，默认 toast 走 `common.copySuccess / copyFailed`
- 调用方可传 `successMsg` 覆盖默认文案
- `async + try/catch + await writeText + setCopied(true) + setTimeout 还原`
- 卸载时 `clearTimeout`（避免 setState on unmounted）

### 文件改动清单

| 文件 | 改动 |
|------|------|
| `frontend/src/i18n/locales/zh-CN.json` | 新增 `upload.*` namespace + `common.copySuccess/copyFailed` |
| `frontend/src/lib/use-copy.ts` | **新建**：`useCopy()` hook |
| `frontend/src/features/images/pages/Upload.tsx` | 全页 i18n + 下拉菜单 + 接入 hook + localStorage |
| `frontend/src/features/images/pages/Detail.tsx` | 仅替换复制逻辑接入 hook，ShareRow 内部管理 `copied` 状态 |

### i18n key 结构

新增到 `zh-CN.json`：

```jsonc
"common": {
  // ...现有
  "copySuccess": "已复制到剪贴板",
  "copyFailed": "复制失败，请检查浏览器权限"
},
"upload": {
  "title": "上传图片",
  "dropzone": "点击或拖拽图片到此处上传",
  "dropzoneHint": "支持 JPG / PNG / GIF / WebP 等格式，可多选",
  "visibility": "可见性",
  "visibilityPublic": "公开",
  "visibilityPrivate": "私密",
  "queue": "上传队列（{{count}}）",
  "startUpload": "开始上传",
  "uploading": "上传中…",
  "done": "完成",
  "remove": "移除",
  "empty": "还没有添加任何图片",
  "status": { "queued": "等待中", "uploading": "上传中", "done": "完成", "failed": "失败" },
  "toast": {
    "uploadSuccess": "已上传 {{count}} 张图片",
    "uploadAllFailed": "上传失败",
    "uploadPartial": "{{ok}} 张成功，{{fail}} 张失败",
    "itemFailed": "{{name}}: {{msg}}",
    "copied": "已复制 {{count}} 个链接（{{format}}）"
  },
  "copy": {
    "button": "复制全部链接",
    "linkFormatLabel": "链接格式",
    "imageFormatLabel": "图片格式",
    "linkFormats": { "url": "URL", "markdown": "Markdown", "bbs": "BBS (BBCode)", "html": "HTML" },
    "imageFormats": { "original": "原图", "webp": "WebP", "avif": "AVIF" },
    "action": "复制 {{count}} 个链接"
  }
}
```

### Detail.tsx 改造点

1. 删除内联 `copy()` helper（`Detail.tsx:163-170`），组件顶部改用 `const { copy } = useCopy()`
2. `ShareRow` 组件重构：移除 `onCopy` prop，内部 `useCopy()`，根据 `copied` 切换 `IconCopy ↔ IconCheck + text-primary`
3. 6 处 `<ShareRow onCopy={...}>` 调用简化为 `<ShareRow value={...} successMsg={...?} />`
4. 处理链接按钮（`Detail.tsx:432`）单独接入，图标随 `copied` 切换
5. **不动 Detail.tsx 其他代码与硬编码中文**

### 视觉反馈行为

| 按钮 | 成功后 | 失败 |
|------|--------|------|
| Upload 菜单触发器 | `IconLink→IconCheck` + 主色 1.5s + toast | `toast.error` 不变绿 |
| Detail ShareRow ×6 | `IconCopy→IconCheck` + 主色 1.5s + toast | 同上 |
| Detail 处理链接按钮 | 同上 | 同上 |

## 验收标准

1. 默认（localStorage 空）选 `URL + WebP`，复制结果与改动前**逐字节一致**
2. 切换任意 radio 立即写回 localStorage，刷新页面后状态保留
3. AVIF 选项复制出的 URL 含 `?q=85`
4. Markdown/HTML 行内 alt 为原文件名（去扩展名）
5. Upload.tsx 内不再有任何硬编码中文字面量（grep 可验证）
6. 剪贴板被拒（`navigator.clipboard` 不可用）时显示 `toast.error('复制失败，请检查浏览器权限')`，按钮不变绿
7. Detail.tsx 任一复制按钮按下后图标明确切换，1.5s 还原
8. localStorage 选择跨刷新保留
9. `useCopy` 卸载时不报 setState 警告
10. 默认 `URL+WebP` 复制时 toast 文案为 `已复制 N 个链接（URL·WebP）`

## 不改的东西

- 后端、API
- `i18n/index.ts`（不引入 en、不加语言切换 UI）
- `DropdownMenu` 组件本身（已存在）
- Detail.tsx 除复制逻辑外的其他代码与字符串
- 其他 feature 页面（auth/admin/notifications/user）
