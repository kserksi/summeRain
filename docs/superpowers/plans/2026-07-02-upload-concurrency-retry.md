# 上传并发限流 + 重试 + 格式收紧 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 前端并发上限 5、单项+批量重试（保留顺序）、前端格式过滤与后端白名单对齐。

**Architecture:** 抽 `runWithConcurrency` 纯函数到独立模块单测；Upload.tsx 改 `Promise.all` 为并发池调用，加 retry 函数与 UI；前端过滤从 `image/*` 收紧到 5 扩展名 + 4 MIME 与后端精确对齐。

**Tech Stack:** React 19 + TypeScript 6 + Vitest + react-i18next + @tabler/icons-react

**Spec:** `docs/superpowers/specs/2026-07-02-upload-concurrency-retry-design.md`

---

## File Structure

| 文件 | 责任 | 性质 |
|------|------|------|
| `frontend/src/features/images/upload-concurrency.ts` | 纯函数 `runWithConcurrency<T,R>(items, fn, limit)`，N-worker 抢队列模式 | 新建 |
| `frontend/src/features/images/upload-concurrency.test.ts` | 单测：顺序、并发上限、空数组、错误传播 | 新建 |
| `frontend/src/i18n/locales/zh-CN.json` | 新增 `upload.retry` / `upload.retryAll` | 修改 |
| `frontend/src/features/images/pages/Upload.tsx` | 并发池接入、retry 函数+UI、格式过滤收紧 | 修改 |

---

### Task 1: 创建 runWithConcurrency 纯函数（TDD）

**Files:**
- Create: `frontend/src/features/images/upload-concurrency.ts`
- Create: `frontend/src/features/images/upload-concurrency.test.ts`

- [ ] **Step 1: 写测试 `frontend/src/features/images/upload-concurrency.test.ts`**

```typescript
// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'

import { runWithConcurrency } from './upload-concurrency'

describe('runWithConcurrency', () => {
  it('returns empty array for empty input', async () => {
    const result = await runWithConcurrency([], async (x) => x, 3)
    expect(result).toEqual([])
  })

  it('preserves order in results array regardless of completion timing', async () => {
    // Make later items complete faster to scramble natural completion order
    const fn = async (x: number) => {
      await new Promise((r) => setTimeout(r, 50 - x * 10))
      return x * 10
    }
    const items = [1, 2, 3, 4]
    const results = await runWithConcurrency(items, fn, 2)
    expect(results).toEqual([10, 20, 30, 40])
  })

  it('limits peak concurrency to the specified limit', async () => {
    let active = 0
    let peak = 0
    const fn = async (x: number) => {
      active++
      peak = Math.max(peak, active)
      await new Promise((r) => setTimeout(r, 10))
      active--
      return x
    }
    const items = Array.from({ length: 20 }, (_, i) => i)
    await runWithConcurrency(items, fn, 5)
    expect(peak).toBeLessThanOrEqual(5)
    expect(peak).toBeGreaterThanOrEqual(5) // should actually hit the limit with 20 items
  })

  it('limit larger than items length processes all items', async () => {
    const fn = async (x: number) => x + 1
    const items = [1, 2, 3]
    const results = await runWithConcurrency(items, fn, 10)
    expect(results).toEqual([2, 3, 4])
  })

  it('propagates rejection when fn throws', async () => {
    const fn = async (x: number) => {
      if (x === 2) throw new Error('boom')
      return x
    }
    await expect(runWithConcurrency([1, 2, 3], fn, 2)).rejects.toThrow('boom')
  })
})
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `npx vitest run src/features/images/upload-concurrency.test.ts` (workdir: `D:\book\frontend`)
Expected: FAIL — `Failed to resolve import "./upload-concurrency"`

- [ ] **Step 3: 实现 `frontend/src/features/images/upload-concurrency.ts`**

```typescript
// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

/**
 * Run an async function over a list of items with at most `limit` concurrent
 * invocations. Results are returned in the same order as the input items,
 * regardless of completion timing.
 *
 * Classic "N workers draining a shared queue" pattern. `index++` is atomic
 * under JavaScript's single-threaded execution model.
 *
 * If `fn` rejects, the whole promise rejects (error propagation is NOT
 * isolated — callers must handle if they want to continue on partial failure).
 */
export async function runWithConcurrency<T, R>(
  items: readonly T[],
  fn: (item: T) => Promise<R>,
  limit: number,
): Promise<R[]> {
  const results: R[] = new Array(items.length)
  let index = 0
  await Promise.all(
    Array.from({ length: Math.min(limit, items.length) }, async () => {
      while (index < items.length) {
        const i = index++
        results[i] = await fn(items[i])
      }
    }),
  )
  return results
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run: `npx vitest run src/features/images/upload-concurrency.test.ts` (workdir: `D:\book\frontend`)
Expected: 5 tests PASS.

- [ ] **Step 5: 验证全套测试无回归**

Run: `npx vitest run` (workdir: `D:\book\frontend`)
Expected: 全部 PASS（应比之前多 5 个，共 36 个：原 31 + 新 5）。

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/images/upload-concurrency.ts frontend/src/features/images/upload-concurrency.test.ts
git commit -m "feat(images): add runWithConcurrency helper with tests

N-worker pool that drains a shared queue. Limits peak concurrency to
the specified limit while preserving result order. Will be used by
Upload.tsx to cap simultaneous uploads at 5 (prevents imgproxy timeout
cascades when batch-uploading 50+ images)."
```

---

### Task 2: 新增 i18n keys

**Files:**
- Modify: `frontend/src/i18n/locales/zh-CN.json`

- [ ] **Step 1: 在 `upload` namespace 末尾加 2 个 key**

找到 `upload.copy` 块（应该在 `upload.toast` 之后），它结尾长这样：

```json
    "copy": {
      "button": "复制全部链接",
      "linkFormatLabel": "链接格式",
      "imageFormatLabel": "图片格式",
      "linkFormats": {
        "url": "URL",
        "markdown": "Markdown",
        "bbs": "BBS (BBCode)",
        "html": "HTML"
      },
      "imageFormats": {
        "original": "原图",
        "webp": "WebP",
        "avif": "AVIF"
      },
      "action": "复制 {{count}} 个链接"
    }
  }
}
```

在 `"copy": { ... }` 块之后、最后的 `}` 之前，加 `"retry"` 和 `"retryAll"` 两个 sibling key：

```json
    "copy": {
      "button": "复制全部链接",
      "linkFormatLabel": "链接格式",
      "imageFormatLabel": "图片格式",
      "linkFormats": {
        "url": "URL",
        "markdown": "Markdown",
        "bbs": "BBS (BBCode)",
        "html": "HTML"
      },
      "imageFormats": {
        "original": "原图",
        "webp": "WebP",
        "avif": "AVIF"
      },
      "action": "复制 {{count}} 个链接"
    },
    "retry": "重试",
    "retryAll": "重试全部失败（{{count}}）"
  }
}
```

注意 `"copy"` 块的右大括号后要加逗号 `,`（之前是最后一个子块所以没逗号，现在 `retry`/`retryAll` 跟在后面了）。

- [ ] **Step 2: 验证 JSON 合法**

Run: `node -e "JSON.parse(require('fs').readFileSync('frontend/src/i18n/locales/zh-CN.json','utf8')); console.log('OK')"` (workdir: `D:\book`)
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add frontend/src/i18n/locales/zh-CN.json
git commit -m "feat(i18n): add upload.retry and upload.retryAll keys"
```

---

### Task 3: 改造 Upload.tsx — 并发限流 + 重试 + 格式收紧

**Files:**
- Modify: `frontend/src/features/images/pages/Upload.tsx`

这是个大任务，分 10 步。每步都是精确的 old → new 替换。请严格按顺序。

- [ ] **Step 1: imports 加 `IconRefresh` 与 `runWithConcurrency`**

找到 `@tabler/icons-react` 的 import 块（应该已经包含 IconCheck, IconChevronDown, IconLink, IconLoader2, IconPhoto, IconUpload, IconX）。

把：
```typescript
import {
  IconCheck,
  IconChevronDown,
  IconLink,
  IconLoader2,
  IconPhoto,
  IconUpload,
  IconX,
} from '@tabler/icons-react'
```

替换为（字母序插入 `IconRefresh`，在 `IconPhoto` 前）：
```typescript
import {
  IconCheck,
  IconChevronDown,
  IconLink,
  IconLoader2,
  IconPhoto,
  IconRefresh,
  IconUpload,
  IconX,
} from '@tabler/icons-react'
```

然后在文件底部 `../copy-format` import 之后加一行：
```typescript
import { runWithConcurrency } from '../upload-concurrency'
```

具体：找到现有的：
```typescript
import {
  buildCopyText,
  loadPrefs,
  savePrefs,
  type CopyImageFormat,
  type CopyLinkFormat,
} from '../copy-format'
```

在它之后追加：
```typescript
import { runWithConcurrency } from '../upload-concurrency'
```

- [ ] **Step 2: 加 `CONCURRENCY` / `ALLOWED_EXTS` / `ALLOWED_TYPES` 模块级常量**

找到现有的 `STATUS_VARIANT` 常量定义：

```typescript
const STATUS_VARIANT: Record<Status, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  queued: 'outline',
  uploading: 'secondary',
  done: 'default',
  failed: 'destructive',
}
```

在它之后追加 3 个常量：

```typescript
const STATUS_VARIANT: Record<Status, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  queued: 'outline',
  uploading: 'secondary',
  done: 'default',
  failed: 'destructive',
}

const CONCURRENCY = 5

const ALLOWED_EXTS = ['.png', '.jpg', '.jpeg', '.webp', '.gif']
const ALLOWED_TYPES = ['image/png', 'image/jpeg', 'image/webp', 'image/gif']
```

- [ ] **Step 3: `addFiles` 过滤收紧（扩展名 + MIME 双校验）**

找到现有的 `addFiles`：

```typescript
  const addFiles = useCallback((files: FileList | File[]) => {
    const next = Array.from(files)
      .filter((f) => f.type.startsWith('image/'))
      .map((f) => ({
        id: `${f.name}-${f.size}-${Math.random().toString(36).slice(2, 8)}`,
        file: f,
        preview: URL.createObjectURL(f),
        progress: 0,
        status: 'queued' as Status,
      }))
    if (next.length) setItems((prev) => [...prev, ...next])
  }, [])
```

替换为（filter 改为扩展名 + MIME 双校验；map 不变）：

```typescript
  const addFiles = useCallback((files: FileList | File[]) => {
    const next = Array.from(files)
      .filter((f) => {
        const dot = f.name.lastIndexOf('.')
        const ext = dot >= 0 ? f.name.slice(dot).toLowerCase() : ''
        return ALLOWED_EXTS.includes(ext) && ALLOWED_TYPES.includes(f.type)
      })
      .map((f) => ({
        id: `${f.name}-${f.size}-${Math.random().toString(36).slice(2, 8)}`,
        file: f,
        preview: URL.createObjectURL(f),
        progress: 0,
        status: 'queued' as Status,
      }))
    if (next.length) setItems((prev) => [...prev, ...next])
  }, [])
```

- [ ] **Step 4: `<input>` 的 accept 收紧**

找到 dropzone 内的 input：

```tsx
        <input
          ref={inputRef}
          type="file"
          accept="image/*"
          multiple
          className="hidden"
```

把 `accept="image/*"` 改为显式扩展名 + MIME 双轨：

```tsx
        <input
          ref={inputRef}
          type="file"
          accept=".png,.jpg,.jpeg,.webp,.gif,image/png,image/jpeg,image/webp,image/gif"
          multiple
          className="hidden"
```

- [ ] **Step 5: 抽 `runUploads` 公共函数；`startUpload` / `retryAllFailed` / `retryOne` 委托给它**

找到现有的 `startUpload`：

```typescript
  const startUpload = async () => {
    const pending = items.filter((i) => i.status === 'queued')
    if (!pending.length || uploading) return
    setUploading(true)
    const statuses = await Promise.all(pending.map(uploadOne))
    setUploading(false)
    qc.invalidateQueries({ queryKey: ['images'] })
    refreshUser()
    const ok = statuses.filter((s) => s === 'done').length
    const fail = statuses.length - ok
    if (fail === 0) toast.success(t('upload.toast.uploadSuccess', { count: ok }))
    else if (ok === 0) toast.error(t('upload.toast.uploadAllFailed'))
    else toast.warning(t('upload.toast.uploadPartial', { ok, fail }))
  }
```

整体替换为（4 个函数：`runUploads` / `startUpload` / `retryAllFailed` / `retryOne`）：

```typescript
  const runUploads = async (toUpload: QueueItem[]) => {
    if (!toUpload.length || uploading) return
    setUploading(true)
    const statuses = await runWithConcurrency(toUpload, uploadOne, CONCURRENCY)
    setUploading(false)
    qc.invalidateQueries({ queryKey: ['images'] })
    refreshUser()
    const ok = statuses.filter((s) => s === 'done').length
    const fail = statuses.length - ok
    if (fail === 0) toast.success(t('upload.toast.uploadSuccess', { count: ok }))
    else if (ok === 0) toast.error(t('upload.toast.uploadAllFailed'))
    else toast.warning(t('upload.toast.uploadPartial', { ok, fail }))
  }

  const startUpload = () => runUploads(items.filter((i) => i.status === 'queued'))
  const retryAllFailed = () => runUploads(items.filter((i) => i.status === 'failed'))
  const retryOne = (id: string) =>
    runUploads(items.filter((i) => i.id === id && i.status === 'failed'))
```

**关键说明：**
- `runWithConcurrency(toUpload, uploadOne, CONCURRENCY)` 替换原来的 `Promise.all(pending.map(uploadOne))` —— 这是问题 1 的修复
- `uploadOne` 内部不动，它通过 `setItems((prev) => prev.map(...))` 原地 patch 对应 id 的 item（不改 id、不改数组位置）—— 这是问题 2"顺序保持"的关键
- `retryAllFailed` 和 `retryOne` 都通过 `items.filter` 选出目标项再交给 `runUploads`

- [ ] **Step 6: 计算 `failedCount`**

找到现有的 `completedLinks` 和 `hasQueued`：

```typescript
  const hasQueued = items.some((i) => i.status === 'queued')
  const completedLinks = items.filter((i) => i.status === 'done' && i.uniqueLink)
```

在它们之后追加一行：

```typescript
  const hasQueued = items.some((i) => i.status === 'queued')
  const completedLinks = items.filter((i) => i.status === 'done' && i.uniqueLink)
  const failedCount = items.filter((i) => i.status === 'failed').length
```

- [ ] **Step 7: 队列标题栏加"重试全部失败 (N)"按钮**

找到队列标题栏里"复制全部链接"Popover 那块的前面（在 `{completedLinks.length > 0 && (` 这个 Popover 块之前），定位上下文：

```tsx
                <Button
                  size="sm"
                  variant="outline"
                  disabled={uploading}
                  onClick={() => navigate('/images')}
                >
                  {t('upload.done')}
                </Button>
                {completedLinks.length > 0 && (
                  <Popover open={copyMenuOpen} onOpenChange={setCopyMenuOpen}>
```

在 `{t('upload.done')}` 那个 Button 关闭后、`{completedLinks.length > 0 && (` 之前，插入批量重试按钮：

```tsx
                <Button
                  size="sm"
                  variant="outline"
                  disabled={uploading}
                  onClick={() => navigate('/images')}
                >
                  {t('upload.done')}
                </Button>
                {failedCount > 0 && (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={uploading}
                    onClick={retryAllFailed}
                  >
                    <IconRefresh />
                    {t('upload.retryAll', { count: failedCount })}
                  </Button>
                )}
                {completedLinks.length > 0 && (
                  <Popover open={copyMenuOpen} onOpenChange={setCopyMenuOpen}>
```

- [ ] **Step 8: 队列每个 `<li>` 内，failed 项加"重试"小图标按钮**

找到队列 `<li>` 渲染处，里面有"移除"按钮：

```tsx
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      disabled={uploading}
                      onClick={() => removeItem(item.id)}
                      aria-label={t('upload.remove')}
                    >
                      <IconX />
                    </Button>
                  </li>
```

在"移除"按钮之前、Badge 之后，插入 failed 项的重试按钮（仅 `item.status === 'failed'` 时渲染）：

```tsx
                    {item.status === 'failed' && (
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        disabled={uploading}
                        onClick={() => retryOne(item.id)}
                        aria-label={t('upload.retry')}
                      >
                        <IconRefresh />
                      </Button>
                    )}
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      disabled={uploading}
                      onClick={() => removeItem(item.id)}
                      aria-label={t('upload.remove')}
                    >
                      <IconX />
                    </Button>
                  </li>
```

- [ ] **Step 9: 验证 typecheck + tests**

Run: `npx tsc -b` (workdir: `D:\book\frontend`) — expect clean.
Run: `npx vitest run` (workdir: `D:\book\frontend`) — expect 36/36 PASS（不引入新测试，但既有测试不应回归）。

常见报错：
- `IconRefresh` 未导入 → Step 1 没改全
- `runWithConcurrency` 未导入 → Step 1 的第二个 import 块没加
- `retryAllFailed` / `retryOne` 未定义 → Step 5 没改全
- `failedCount` 未定义 → Step 6 漏了
- `ALLOWED_EXTS` 未定义 → Step 2 漏了

- [ ] **Step 10: Commit**

```bash
git add frontend/src/features/images/pages/Upload.tsx
git commit -m "feat(upload): concurrency limit, retry, and format tightening

Three frontend-only fixes for reported upload issues:

1. Concurrency limit (5) replaces Promise.all — prevents imgproxy timeout
   cascades when batch-uploading many images. Uses runWithConcurrency
   helper from upload-concurrency.ts.

2. Retry mechanism preserves queue order:
   - Per-item retry button (IconRefresh) shown only for failed items
   - Bulk 'retry all failed (N)' button appears in queue header when
     failedCount > 0
   - Both reuse uploadOne via runUploads, which patches items in place
     by id (no array reordering)

3. Format filter tightened to match backend's 5-ext + 4-MIME whitelist
   exactly. <input accept> now lists extensions explicitly so the OS
   file picker greys out unsupported types (svg/avif/bmp/heic).
   addFiles filter rejects files that the backend would reject anyway,
   failing fast at selection time instead of after a wasted upload.

Drive-by: refactored startUpload into a shared runUploads helper that
all three entry points (start/retryAll/retryOne) delegate to."
```

---

### Task 4: 最终验证

**Files:** 无修改

- [ ] **Step 1: 跑全部单测**

Run: `npx vitest run` (workdir: `D:\book\frontend`)
Expected: 36/36 PASS — 含新的 `upload-concurrency.test.ts` 5 个。

- [ ] **Step 2: 跑生产构建**

Run: `npm run build` (workdir: `D:\book\frontend`)
Expected: 构建成功，输出 `../backend/web/`，无 TS 错误。注意任何 warning。

- [ ] **Step 3: 跑 lint**

Run: `npm run lint` (workdir: `D:\book\frontend`)
Expected: 在本 plan 触及的文件（`upload-concurrency.ts`、`upload-concurrency.test.ts`、`Upload.tsx`、`zh-CN.json`）上**不引入新的** eslint 错误。其他文件预存错误可接受（不要修）。

- [ ] **Step 4: 静态验收检查**

从 `D:\book` 运行：

(a) 确认 Upload.tsx 不再用裸 `Promise.all` 跑上传：
```
npx grep -nE "Promise\.all\(pending\.map" frontend/src/features/images/pages/Upload.tsx
```
Expected: 无匹配（已被 `runWithConcurrency` 替代）。

(b) 确认 `runWithConcurrency` 被引用：
```
npx grep -nE "runWithConcurrency" frontend/src/features/images/pages/Upload.tsx
```
Expected: 至少 2 个匹配（import + 调用）。

(c) 确认 retry 函数存在：
```
npx grep -nE "retryAllFailed|retryOne" frontend/src/features/images/pages/Upload.tsx
```
Expected: 至少 4 个匹配（2 个定义 + 2 个 onClick 引用）。

(d) 确认格式常量存在且与后端白名单一致：
```
npx grep -nE "ALLOWED_EXTS|ALLOWED_TYPES" frontend/src/features/images/pages/Upload.tsx
```
Expected: 至少 4 个匹配（2 个定义 + 至少 2 处使用）。

(e) 确认 `<input accept>` 不再是 `image/*`：
```
npx grep -nE "accept=\"image/\*\"" frontend/src/features/images/pages/Upload.tsx
```
Expected: 无匹配。

(f) 确认 i18n key 已加：
```
npx grep -nE "\"retry\"|\"retryAll\"" frontend/src/i18n/locales/zh-CN.json
```
Expected: 2 个匹配。

- [ ] **Step 5: 列出本 plan 全部 commit**

Run: `git log --oneline bb6aa54..HEAD` (workdir: `D:\book`)
Expected: 3 个实现 commit：
1. `feat(images): add runWithConcurrency helper with tests`
2. `feat(i18n): add upload.retry and upload.retryAll keys`
3. `feat(upload): concurrency limit, retry, and format tightening`

- [ ] **Step 6: 最终提交（仅在有 lint/build 修复时）**

如 Step 1-3 全过则无需提交；如需修复则只 stage 修复的文件，commit message：`chore: fix lint/build issues from upload concurrency work`。

---

## Self-Review Notes

**Spec coverage:**
- 决策 1（并发数=5）→ Task 1 实现 helper + Task 3 Step 2 加 `CONCURRENCY = 5` + Step 5 用 `runWithConcurrency` ✓
- 决策 2（单项+批量重试都加）→ Task 3 Step 5 定义 `retryOne` / `retryAllFailed` + Step 7 批量按钮 + Step 8 单项按钮 ✓
- 决策 3（不加自动重试）→ 全 plan 无 `setTimeout` 重试 / `retryCount` / backoff 逻辑 ✓
- 决策 4（后端不动）→ 全 plan 不涉及任何 `backend/` 文件 ✓
- 决策 5（前端格式过滤收紧）→ Task 3 Step 2 加常量 + Step 3 改 `addFiles` + Step 4 改 `<input accept>` ✓
- 决策 6（不开放 AVIF 上传）→ `ALLOWED_EXTS` 不含 `.avif`；`ALLOWED_TYPES` 不含 `image/avif` ✓
- 验收标准 1（XHR ≤ 5）→ runWithConcurrency 单测验证 peak ≤ 5 ✓
- 验收标准 2-5（重试相关）→ Task 3 Step 5/7/8 ✓
- 验收标准 6-7（格式拒收）→ Task 3 Step 3/4 ✓
- 验收标准 8（单测通过）→ Task 1 Step 4 + Task 4 Step 1 ✓
- 验收标准 9（typecheck/lint/build）→ Task 4 Step 1-3 ✓
- 验收标准 10（不回归）→ 全 plan 不动 popover/copyAllLinks/useCopy/copy-format ✓

**Placeholder scan:** 所有 step 含完整代码或完整命令。无 TBD/TODO。Task 1 含完整 helper + 5 个测试；Task 3 每个 step 含完整 old → new 代码块。

**Type consistency:**
- `runWithConcurrency<T, R>(items: readonly T[], fn: (item: T) => Promise<R>, limit: number): Promise<R[]>` — Task 1 定义、Task 3 调用一致 ✓
- `QueueItem` 类型在 Upload.tsx 已有定义，Task 3 Step 5 `runUploads(toUpload: QueueItem[])` 复用 ✓
- `Status` 联合类型 `'queued' | 'uploading' | 'done' | 'failed'` — Task 3 Step 5/8 的 `i.status === 'failed'` / `i.status === 'queued'` 都在此范围内 ✓
