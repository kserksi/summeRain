# 上传页复制格式菜单 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在上传页"复制全部链接"按钮上加格式菜单（链接格式 × 图片格式），同时引入 `useCopy` hook 让 Upload + Detail 所有复制按钮获得 async 可靠性 + 视觉反馈 + i18n。

**Architecture:** 抽出纯函数 `copy-format.ts`（URL/行格式构造 + localStorage 持久化）和 `useCopy` hook（clipboard + toast + 1.5s 图标切换），Upload.tsx 接入 i18n + DropdownMenu，Detail.tsx 仅替换复制路径不动其他。

**Tech Stack:** React 19 + TypeScript 6 + react-i18next + shadcn DropdownMenu + Vitest + @testing-library/react

**Spec:** `docs/superpowers/specs/2026-07-01-upload-copy-format-menu-design.md`

---

## File Structure

| 文件 | 责任 | 性质 |
|------|------|------|
| `frontend/src/i18n/locales/zh-CN.json` | 新增 `upload.*` + `common.copySuccess/copyFailed` | 修改 |
| `frontend/src/features/images/copy-format.ts` | 纯函数：buildUrl / formatLine / buildCopyText / baseName / loadPrefs / savePrefs | 新建 |
| `frontend/src/features/images/copy-format.test.ts` | copy-format 单元测试 | 新建 |
| `frontend/src/lib/use-copy.ts` | `useCopy()` hook | 新建 |
| `frontend/src/lib/use-copy.test.ts` | hook 单元测试 | 新建 |
| `frontend/src/features/images/pages/Detail.tsx` | 删除内联 copy()，ShareRow 内部接入 useCopy，处理链接按钮加视觉反馈 | 修改 |
| `frontend/src/features/images/pages/Upload.tsx` | 全页 i18n + DropdownMenu + useCopy + localStorage | 修改 |

---

### Task 1: 新增 i18n keys

**Files:**
- Modify: `frontend/src/i18n/locales/zh-CN.json`

- [ ] **Step 1: 追加 `common.copySuccess` / `common.copyFailed` 和整个 `upload` namespace**

把 `frontend/src/i18n/locales/zh-CN.json` 改为下面完整内容（在 common 末尾加两个 key，errors 之后追加 upload 整块）：

```json
{
  "common": {
    "appName": "月兔图床",
    "loading": "加载中…",
    "error": "出错了",
    "retry": "重试",
    "cancel": "取消",
    "save": "保存",
    "delete": "删除",
    "confirm": "确认",
    "back": "返回",
    "search": "搜索",
    "upload": "上传",
    "logout": "退出",
    "login": "登录",
    "register": "注册",
    "copySuccess": "已复制到剪贴板",
    "copyFailed": "复制失败，请检查浏览器权限"
  },
  "nav": {
    "home": "首页",
    "dashboard": "控制台",
    "images": "我的图片",
    "upload": "上传",
    "profile": "个人资料",
    "admin": "后台",
    "notifications": "通知"
  },
  "errors": {
    "2001": "用户名或密码错误",
    "2008": "操作过于频繁，请稍后再试",
    "2009": "人机验证失败，请重试",
    "3000": "参数校验错误",
    "3002": "文件大小超出限制",
    "3003": "不支持的文件类型",
    "3004": "文件数量超出限制",
    "4010": "未认证或登录已过期",
    "4011": "会话已过期，请重新登录",
    "4012": "存储配额已满",
    "4029": "操作过于频繁",
    "4030": "账号已被禁用",
    "4035": "CSRF 令牌缺失",
    "4036": "CSRF 令牌无效",
    "4037": "私密图片令牌无效或已过期",
    "4042": "私密图片令牌已吊销",
    "1004": "人机验证服务暂时不可用",
    "1003": "图片处理服务错误，图片可能过大（4K+ 需服务器增大 imgproxy 内存）"
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
    "status": {
      "queued": "等待中",
      "uploading": "上传中",
      "done": "完成",
      "failed": "失败"
    },
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

- [ ] **Step 2: 验证 JSON 合法**

Run: `node -e "JSON.parse(require('fs').readFileSync('frontend/src/i18n/locales/zh-CN.json','utf8')); console.log('OK')"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add frontend/src/i18n/locales/zh-CN.json
git commit -m "feat(i18n): add upload namespace and copy toast keys"
```

---

### Task 2: 创建 copy-format 纯函数（TDD）

**Files:**
- Create: `frontend/src/features/images/copy-format.ts`
- Create: `frontend/src/features/images/copy-format.test.ts`

- [ ] **Step 1: 写测试 `frontend/src/features/images/copy-format.test.ts`**

```typescript
// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach } from 'vitest'

import {
  baseName,
  buildUrl,
  formatLine,
  buildCopyText,
  loadPrefs,
  savePrefs,
  DEFAULT_PREFS,
} from './copy-format'

describe('baseName', () => {
  it('strips last extension', () => {
    expect(baseName('photo.jpg')).toBe('photo')
    expect(baseName('a.b.c.png')).toBe('a.b.c')
  })
  it('returns as-is when no extension', () => {
    expect(baseName('README')).toBe('README')
  })
  it('handles dotfiles', () => {
    expect(baseName('.hidden')).toBe('.hidden')
  })
})

describe('buildUrl', () => {
  const origin = 'https://img.example.com'
  it('original: no extension, no query', () => {
    expect(buildUrl(origin, 'abc123', 'original')).toBe(
      'https://img.example.com/i/abc123',
    )
  })
  it('webp: appends .webp?q=85', () => {
    expect(buildUrl(origin, 'abc123', 'webp')).toBe(
      'https://img.example.com/i/abc123.webp?q=85',
    )
  })
  it('avif: appends .avif?q=85', () => {
    expect(buildUrl(origin, 'abc123', 'avif')).toBe(
      'https://img.example.com/i/abc123.avif?q=85',
    )
  })
})

describe('formatLine', () => {
  const url = 'https://img.example.com/i/abc.webp?q=85'
  it('url: passthrough', () => {
    expect(formatLine(url, 'photo', 'url')).toBe(url)
  })
  it('markdown: ![alt](url)', () => {
    expect(formatLine(url, 'photo', 'markdown')).toBe(
      `![photo](${url})`,
    )
  })
  it('bbs: [img]url[/img]', () => {
    expect(formatLine(url, 'photo', 'bbs')).toBe(`[img]${url}[/img]`)
  })
  it('html: <img src=url alt=alt>', () => {
    expect(formatLine(url, 'photo', 'html')).toBe(
      `<img src="${url}" alt="photo">`,
    )
  })
})

describe('buildCopyText', () => {
  const origin = 'https://img.example.com'
  const items = [
    { uniqueLink: 'aaa', fileName: 'cat.jpg' },
    { uniqueLink: 'bbb', fileName: 'dog.png' },
  ]
  it('combines url + webp by default', () => {
    expect(buildCopyText(origin, items, 'url', 'webp')).toBe(
      'https://img.example.com/i/aaa.webp?q=85\nhttps://img.example.com/i/bbb.webp?q=85',
    )
  })
  it('markdown + avif uses baseName as alt', () => {
    expect(buildCopyText(origin, items, 'markdown', 'avif')).toBe(
      '![cat](https://img.example.com/i/aaa.avif?q=85)\n![dog](https://img.example.com/i/bbb.avif?q=85)',
    )
  })
  it('empty list returns empty string', () => {
    expect(buildCopyText(origin, [], 'url', 'webp')).toBe('')
  })
})

describe('prefs persistence', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('DEFAULT_PREFS is url + webp', () => {
    expect(DEFAULT_PREFS).toEqual({ link: 'url', image: 'webp' })
  })

  it('loadPrefs returns DEFAULT_PREFS when nothing stored', () => {
    expect(loadPrefs()).toEqual(DEFAULT_PREFS)
  })

  it('savePrefs then loadPrefs roundtrip', () => {
    savePrefs({ link: 'markdown', image: 'avif' })
    expect(loadPrefs()).toEqual({ link: 'markdown', image: 'avif' })
  })

  it('loadPrefs falls back on malformed JSON', () => {
    localStorage.setItem('upload:copyPrefs', '{not json')
    expect(loadPrefs()).toEqual(DEFAULT_PREFS)
  })

  it('loadPrefs falls back on invalid enum values', () => {
    localStorage.setItem(
      'upload:copyPrefs',
      JSON.stringify({ link: 'bogus', image: 'webp' }),
    )
    expect(loadPrefs()).toEqual(DEFAULT_PREFS)
  })
})
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `npx vitest run src/features/images/copy-format.test.ts` (workdir: `frontend`)
Expected: FAIL — `Failed to resolve import "./copy-format"`

- [ ] **Step 3: 实现 `frontend/src/features/images/copy-format.ts`**

```typescript
// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

export type CopyLinkFormat = 'url' | 'markdown' | 'bbs' | 'html'
export type CopyImageFormat = 'original' | 'webp' | 'avif'

export interface CopyPrefs {
  link: CopyLinkFormat
  image: CopyImageFormat
}

export const DEFAULT_PREFS: CopyPrefs = { link: 'url', image: 'webp' }

const STORAGE_KEY = 'upload:copyPrefs'
const LINK_FORMATS: readonly CopyLinkFormat[] = ['url', 'markdown', 'bbs', 'html']
const IMAGE_FORMATS: readonly CopyImageFormat[] = ['original', 'webp', 'avif']

function isCopyLinkFormat(v: unknown): v is CopyLinkFormat {
  return typeof v === 'string' && (LINK_FORMATS as readonly string[]).includes(v)
}

function isCopyImageFormat(v: unknown): v is CopyImageFormat {
  return typeof v === 'string' && (IMAGE_FORMATS as readonly string[]).includes(v)
}

/** Strip the last extension from a filename. Dotfiles are kept as-is. */
export function baseName(fileName: string): string {
  const dot = fileName.lastIndexOf('.')
  if (dot <= 0) return fileName
  return fileName.slice(0, dot)
}

/** Build the public URL for an image given its link id and target image format. */
export function buildUrl(
  origin: string,
  link: string,
  format: CopyImageFormat,
): string {
  if (format === 'original') return `${origin}/i/${link}`
  return `${origin}/i/${link}.${format}?q=85`
}

/** Format a single line according to the chosen link format. */
export function formatLine(
  url: string,
  alt: string,
  format: CopyLinkFormat,
): string {
  switch (format) {
    case 'markdown':
      return `![${alt}](${url})`
    case 'bbs':
      return `[img]${url}[/img]`
    case 'html':
      return `<img src="${url}" alt="${alt}">`
    case 'url':
    default:
      return url
  }
}

/** Build the full multi-line text to copy. */
export function buildCopyText(
  origin: string,
  items: readonly { uniqueLink: string; fileName: string }[],
  link: CopyLinkFormat,
  image: CopyImageFormat,
): string {
  return items
    .map((it) => formatLine(buildUrl(origin, it.uniqueLink, image), baseName(it.fileName), link))
    .join('\n')
}

/** Read persisted prefs from localStorage; falls back to DEFAULT_PREFS. */
export function loadPrefs(): CopyPrefs {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return DEFAULT_PREFS
    const parsed = JSON.parse(raw) as Partial<CopyPrefs>
    if (isCopyLinkFormat(parsed.link) && isCopyImageFormat(parsed.image)) {
      return { link: parsed.link, image: parsed.image }
    }
    return DEFAULT_PREFS
  } catch {
    return DEFAULT_PREFS
  }
}

/** Persist prefs to localStorage. */
export function savePrefs(prefs: CopyPrefs): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs))
  } catch {
    // localStorage unavailable (private mode, quota); silently ignore
  }
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run: `npx vitest run src/features/images/copy-format.test.ts` (workdir: `frontend`)
Expected: PASS — all tests green

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/images/copy-format.ts frontend/src/features/images/copy-format.test.ts
git commit -m "feat(images): add copy-format pure helpers with tests"
```

---

### Task 3: 创建 useCopy hook（TDD）

**Files:**
- Create: `frontend/src/lib/use-copy.ts`
- Create: `frontend/src/lib/use-copy.test.ts`

- [ ] **Step 1: 写测试 `frontend/src/lib/use-copy.test.ts`**

```typescript
// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

import { toast } from 'sonner'
import { useCopy } from './use-copy'

describe('useCopy', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('navigator', {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    })
    vi.mocked(toast.success).mockClear()
    vi.mocked(toast.error).mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('returns copied=false initially', () => {
    const { result } = renderHook(() => useCopy())
    expect(result.current.copied).toBe(false)
  })

  it('success: awaits clipboard, sets copied=true, toasts default msg', async () => {
    const { result } = renderHook(() => useCopy())
    await act(async () => {
      const ok = await result.current.copy('hello')
      expect(ok).toBe(true)
    })
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('hello')
    expect(result.current.copied).toBe(true)
    expect(toast.success).toHaveBeenCalledWith('已复制到剪贴板')
  })

  it('success: accepts custom success message', async () => {
    const { result } = renderHook(() => useCopy())
    await act(async () => {
      await result.current.copy('x', '令牌已复制')
    })
    expect(toast.success).toHaveBeenCalledWith('令牌已复制')
  })

  it('resets copied to false after timeout', async () => {
    const { result } = renderHook(() => useCopy(1000))
    await act(async () => {
      await result.current.copy('x')
    })
    expect(result.current.copied).toBe(true)
    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(result.current.copied).toBe(false)
  })

  it('failure: rejects -> toast.error, copied stays false, returns false', async () => {
    vi.stubGlobal('navigator', {
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    })
    const { result } = renderHook(() => useCopy())
    await act(async () => {
      const ok = await result.current.copy('x')
      expect(ok).toBe(false)
    })
    expect(toast.error).toHaveBeenCalledWith('复制失败，请检查浏览器权限')
    expect(result.current.copied).toBe(false)
  })

  it('does not setState after unmount', async () => {
    const { result, unmount } = renderHook(() => useCopy(1000))
    await act(async () => {
      await result.current.copy('x')
    })
    unmount()
    expect(() =>
      act(() => {
        vi.advanceTimersByTime(1000)
      }),
    ).not.toThrow()
  })
})
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `npx vitest run src/lib/use-copy.test.ts` (workdir: `frontend`)
Expected: FAIL — `Failed to resolve import "./use-copy"`

- [ ] **Step 3: 实现 `frontend/src/lib/use-copy.ts`**

```typescript
// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

/**
 * Copy-to-clipboard with reliable async + toast feedback + transient
 * `copied` flag for visual indicator (e.g. icon swap).
 *
 * @param timeout how long `copied` stays true after a successful copy, ms
 */
export function useCopy(timeout = 1500): {
  copied: boolean
  copy: (text: string, successMsg?: string) => Promise<boolean>
} {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const copy = useCallback(
    async (text: string, successMsg?: string): Promise<boolean> => {
      try {
        await navigator.clipboard.writeText(text)
        setCopied(true)
        toast.success(successMsg ?? t('common.copySuccess'))
        if (timerRef.current) clearTimeout(timerRef.current)
        timerRef.current = setTimeout(() => setCopied(false), timeout)
        return true
      } catch {
        toast.error(t('common.copyFailed'))
        return false
      }
    },
    [t, timeout],
  )

  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    },
    [],
  )

  return { copied, copy }
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run: `npx vitest run src/lib/use-copy.test.ts` (workdir: `frontend`)
Expected: PASS — all tests green

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/use-copy.ts frontend/src/lib/use-copy.test.ts
git commit -m "feat(lib): add useCopy hook with async reliability and visual feedback"
```

---

### Task 4: 改造 Detail.tsx — 接入 useCopy + ShareRow 视觉反馈

**Files:**
- Modify: `frontend/src/features/images/pages/Detail.tsx`

- [ ] **Step 1: 修改 imports（行 1-19）**

把原本的 import 块（行 4-19）替换为：

```typescript
import {
  IconBan,
  IconCheck,
  IconCopy,
  IconEye,
  IconKey,
  IconLoader2,
  IconLock,
  IconPhotoEdit,
  IconRefresh,
  IconTrash,
  IconWorld,
  IconZoomScan,
} from '@tabler/icons-react'
import { useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { useCopy } from '@/lib/use-copy'
```

（删除 `import { toast } from 'sonner'`；新增 `IconCheck`、`useCopy`）

- [ ] **Step 2: 重写 ShareRow 组件（行 109-141），改为内部 useCopy + 视觉反馈**

把原 ShareRow 整块替换为：

```typescript
function ShareRow({
  label,
  value,
  display,
  successMsg,
}: {
  label: string
  value: string
  display?: string
  successMsg?: string
}) {
  const { copied, copy } = useCopy()
  return (
    <div className="space-y-1">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      <div className="flex gap-2">
        <Input
          readOnly
          value={display ?? value}
          className="font-mono text-xs"
        />
        <Button
          type="button"
          size="icon"
          variant="outline"
          onClick={() => copy(value, successMsg)}
          aria-label={`复制${label}`}
        >
          {copied ? (
            <IconCheck className="text-primary" />
          ) : (
            <IconCopy />
          )}
        </Button>
      </div>
    </div>
  )
}
```

- [ ] **Step 3: 在 Detail 组件顶部加 useCopy，删原 copy helper**

把原本的（行 158-170）：

```typescript
  const toggleVis = useToggleVisibility()
  const del = useDeleteImage()
  const issue = useIssueToken()
  const revoke = useRevokeToken()

  const copy = async (text: string, msg = '已复制到剪贴板') => {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(msg)
    } catch {
      toast.error('复制失败')
    }
  }
```

替换为：

```typescript
  const toggleVis = useToggleVisibility()
  const del = useDeleteImage()
  const issue = useIssueToken()
  const revoke = useRevokeToken()
  const { copy } = useCopy()
```

- [ ] **Step 4: 更新分享链接 3 处 ShareRow（行 308-324）**

把原本的：

```tsx
              <ShareRow
                label="链接"
                value={shareUrl}
                onCopy={() => copy(shareUrl)}
              />
              <ShareRow
                label="Markdown"
                value={`![${title}](${shareUrl})`}
                onCopy={() => copy(`![${title}](${shareUrl})`)}
              />
              <ShareRow
                label="HTML"
                value={`<img src="${shareUrl}" alt="${title}" />`}
                onCopy={() =>
                  copy(`<img src="${shareUrl}" alt="${title}" />`)
                }
              />
```

替换为（移除 onCopy，ShareRow 内部自管）：

```tsx
              <ShareRow label="链接" value={shareUrl} />
              <ShareRow
                label="Markdown"
                value={`![${title}](${shareUrl})`}
              />
              <ShareRow
                label="HTML"
                value={`<img src="${shareUrl}" alt="${title}" />`}
              />
```

- [ ] **Step 5: 更新处理链接按钮（行 425-440），加视觉反馈**

把原本的：

```tsx
                <div className="flex gap-2">
                  <Input
                    readOnly
                    value={processedUrl}
                    className="font-mono text-xs"
                  />
                  <Button
                    type="button"
                    onClick={() => copy(processedUrl, '已复制处理链接')}
                  >
                    <IconCopy />
                    复制
                  </Button>
                </div>
```

替换为（注意：此处复用组件级的 `copied`，仅这一个按钮 + 上面处理链接块没有别的 copy，视觉反馈与 ShareRow 独立；但要避免与 ShareRow 的 copied 状态串扰——所以这里另起一个 useCopy 实例更干净）：

```tsx
                <ProcessedLinkRow value={processedUrl} />
```

然后在文件顶部 ShareRow 下方追加一个新组件（紧跟 ShareRow 之后）：

```typescript
function ProcessedLinkRow({ value }: { value: string }) {
  const { copied, copy } = useCopy()
  return (
    <div className="flex gap-2">
      <Input readOnly value={value} className="font-mono text-xs" />
      <Button type="button" onClick={() => copy(value, '已复制处理链接')}>
        {copied ? (
          <IconCheck className="text-primary" />
        ) : (
          <IconCopy />
        )}
        复制
      </Button>
    </div>
  )
}
```

- [ ] **Step 6: 更新令牌 ShareRow（行 456-466）**

把原本的：

```tsx
                    <ShareRow
                      label="令牌"
                      value={activeToken}
                      display={maskedToken}
                      onCopy={() => copy(activeToken, '令牌已复制')}
                    />
                    <ShareRow
                      label="带令牌链接"
                      value={tokenShare}
                      onCopy={() => copy(tokenShare)}
                    />
```

替换为：

```tsx
                    <ShareRow
                      label="令牌"
                      value={activeToken}
                      display={maskedToken}
                      successMsg="令牌已复制"
                    />
                    <ShareRow label="带令牌链接" value={tokenShare} />
```

- [ ] **Step 7: 验证 typecheck 通过**

Run: `npx tsc -b` (workdir: `frontend`)
Expected: 无报错退出

- [ ] **Step 8: Commit**

```bash
git add frontend/src/features/images/pages/Detail.tsx
git commit -m "refactor(detail): use useCopy hook with visual feedback in ShareRow"
```

---

### Task 5: 改造 Upload.tsx — 全页 i18n + DropdownMenu + useCopy

**Files:**
- Modify: `frontend/src/features/images/pages/Upload.tsx`

- [ ] **Step 1: 重写 imports（行 1-32），加 useTranslation / DropdownMenu / useCopy / copy-format / IconChevronDown**

把原本的 import 块（行 4-32）替换为：

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
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { API_BASE_URL } from '@/config/constants'
import { useCopy } from '@/lib/use-copy'
import { getCsrfToken } from '@/lib/csrf'
import { useAuthStore } from '@/store/auth-store'

import {
  buildCopyText,
  loadPrefs,
  savePrefs,
  type CopyImageFormat,
  type CopyLinkFormat,
} from '../copy-format'
```

- [ ] **Step 2: 替换 STATUS_MAP 为函数式（行 55-60），让标签走 i18n**

删除原本的：

```typescript
const STATUS_MAP: Record<Status, { label: string; variant: 'default' | 'secondary' | 'destructive' | 'outline' }> = {
  queued: { label: '等待中', variant: 'outline' },
  uploading: { label: '上传中', variant: 'secondary' },
  done: { label: '完成', variant: 'default' },
  failed: { label: '失败', variant: 'destructive' },
}
```

替换为：

```typescript
const STATUS_VARIANT: Record<Status, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  queued: 'outline',
  uploading: 'secondary',
  done: 'default',
  failed: 'destructive',
}
```

- [ ] **Step 3: 改 Upload 组件主体顶部（行 62-75），加 useTranslation / prefs state / useCopy**

把原本的（行 62-75）：

```typescript
export default function Upload() {
  const [items, setItems] = useState<QueueItem[]>([])
  const [visibility, setVisibility] = useState<'public' | 'private'>('public')
  const [dragging, setDragging] = useState(false)
  const [uploading, setUploading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const qc = useQueryClient()
  const refreshUser = useAuthStore((s) => s.refreshUser)

  const itemsRef = useRef(items)
  useEffect(() => {
    itemsRef.current = items
  })
```

替换为：

```typescript
export default function Upload() {
  const { t } = useTranslation()
  const [items, setItems] = useState<QueueItem[]>([])
  const [visibility, setVisibility] = useState<'public' | 'private'>('public')
  const [dragging, setDragging] = useState(false)
  const [uploading, setUploading] = useState(false)
  const initialPrefs = useRef<CopyPrefs>(loadPrefs()).current
  const [linkFormat, setLinkFormat] = useState<CopyLinkFormat>(initialPrefs.link)
  const [imageFormat, setImageFormat] = useState<CopyImageFormat>(initialPrefs.image)
  const inputRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const qc = useQueryClient()
  const refreshUser = useAuthStore((s) => s.refreshUser)
  const { copied, copy } = useCopy()

  useEffect(() => {
    savePrefs({ link: linkFormat, image: imageFormat })
  }, [linkFormat, imageFormat])

  const itemsRef = useRef(items)
  useEffect(() => {
    itemsRef.current = items
  })
```

**并在 import 块追加 `type CopyPrefs`：** 把 Step 1 末尾的 `../copy-format` import 改为：

```typescript
import {
  buildCopyText,
  loadPrefs,
  savePrefs,
  type CopyImageFormat,
  type CopyLinkFormat,
  type CopyPrefs,
} from '../copy-format'
```

- [ ] **Step 4: 重写 copyAllLinks（行 186-191）为 buildCopyText + useCopy**

把原本的：

```typescript
  const copyAllLinks = () => {
    const urls = completedLinks.map((i) => `${window.location.origin}/i/${i.uniqueLink}.webp?q=85`)
    if (!urls.length) return
    navigator.clipboard.writeText(urls.join('\n'))
    toast.success(`已复制 ${urls.length} 个链接`)
  }
```

替换为：

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

- [ ] **Step 5: 替换 dropzone JSX（行 195-232）走 i18n**

把原本的（行 195-232）：

```tsx
      <h1 className="text-2xl font-bold">上传图片</h1>

      <div
        role="button"
        tabIndex={0}
        onClick={() => inputRef.current?.click()}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') inputRef.current?.click()
        }}
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={`grid cursor-pointer place-items-center rounded-3xl border-2 border-dashed p-10 text-center transition-colors ${
          dragging
            ? 'border-primary bg-primary/5'
            : 'border-border hover:bg-muted/50'
        }`}
      >
        <input
          ref={inputRef}
          type="file"
          accept="image/*"
          multiple
          className="hidden"
          onChange={(e) => {
            if (e.target.files?.length) addFiles(e.target.files)
            e.target.value = ''
          }}
        />
        <IconUpload className="size-10 text-muted-foreground" />
        <p className="mt-3 font-medium">点击或拖拽图片到此处上传</p>
        <p className="mt-1 text-sm text-muted-foreground">
          支持 JPG / PNG / GIF / WebP 等格式，可多选
        </p>
      </div>
```

替换为：

```tsx
      <h1 className="text-2xl font-bold">{t('upload.title')}</h1>

      <div
        role="button"
        tabIndex={0}
        onClick={() => inputRef.current?.click()}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') inputRef.current?.click()
        }}
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={`grid cursor-pointer place-items-center rounded-3xl border-2 border-dashed p-10 text-center transition-colors ${
          dragging
            ? 'border-primary bg-primary/5'
            : 'border-border hover:bg-muted/50'
        }`}
      >
        <input
          ref={inputRef}
          type="file"
          accept="image/*"
          multiple
          className="hidden"
          onChange={(e) => {
            if (e.target.files?.length) addFiles(e.target.files)
            e.target.value = ''
          }}
        />
        <IconUpload className="size-10 text-muted-foreground" />
        <p className="mt-3 font-medium">{t('upload.dropzone')}</p>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('upload.dropzoneHint')}
        </p>
      </div>
```

- [ ] **Step 6: 替换可见性 Card（行 234-250）走 i18n**

把原本的：

```tsx
      <Card>
        <CardContent className="flex items-center gap-3 p-5">
          <Label className="text-sm">可见性</Label>
          <Select
            value={visibility}
            onValueChange={(v) => setVisibility(v as 'public' | 'private')}
          >
            <SelectTrigger className="h-9 w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="public">公开</SelectItem>
              <SelectItem value="private">私密</SelectItem>
            </SelectContent>
          </Select>
        </CardContent>
      </Card>
```

替换为：

```tsx
      <Card>
        <CardContent className="flex items-center gap-3 p-5">
          <Label className="text-sm">{t('upload.visibility')}</Label>
          <Select
            value={visibility}
            onValueChange={(v) => setVisibility(v as 'public' | 'private')}
          >
            <SelectTrigger className="h-9 w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="public">{t('upload.visibilityPublic')}</SelectItem>
              <SelectItem value="private">{t('upload.visibilityPrivate')}</SelectItem>
            </SelectContent>
          </Select>
        </CardContent>
      </Card>
```

- [ ] **Step 7: 替换"复制全部链接"按钮为 DropdownMenu（行 278-287）**

把原本的：

```tsx
                {completedLinks.length > 0 && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={copyAllLinks}
                  >
                    <IconLink />
                    复制全部链接 ({completedLinks.length})
                  </Button>
                )}
```

替换为：

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

- [ ] **Step 8: 替换队列标题与按钮（行 255-277）走 i18n**

把原本的：

```tsx
            <div className="flex items-center justify-between">
              <p className="font-medium">上传队列（{items.length}）</p>
              <div className="flex gap-2">
                <Button
                  size="sm"
                  disabled={!hasQueued || uploading}
                  onClick={startUpload}
                >
                  {uploading ? (
                    <IconLoader2 className="animate-spin" />
                  ) : (
                    <IconUpload />
                  )}
                  {uploading ? '上传中…' : '开始上传'}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={uploading}
                  onClick={() => navigate('/images')}
                >
                  完成
                </Button>
```

替换为：

```tsx
            <div className="flex items-center justify-between">
              <p className="font-medium">{t('upload.queue', { count: items.length })}</p>
              <div className="flex gap-2">
                <Button
                  size="sm"
                  disabled={!hasQueued || uploading}
                  onClick={startUpload}
                >
                  {uploading ? (
                    <IconLoader2 className="animate-spin" />
                  ) : (
                    <IconUpload />
                  )}
                  {uploading ? t('upload.uploading') : t('upload.startUpload')}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={uploading}
                  onClick={() => navigate('/images')}
                >
                  {t('upload.done')}
                </Button>
```

- [ ] **Step 9: 替换队列项 Badge label（行 292-318）走 STATUS_VARIANT + i18n**

把原本的（行 292-318）：

```tsx
              {items.map((item) => {
                const st = STATUS_MAP[item.status]
                return (
                  <li key={item.id} className="flex items-center gap-3">
                    <img
                      src={item.preview}
                      alt={item.file.name}
                      className="size-14 shrink-0 rounded-xl object-cover ring-1 ring-border"
                    />
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate text-sm font-medium">
                          {item.file.name}
                        </span>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {formatBytes(item.file.size)}
                        </span>
                      </div>
                      <Progress value={item.progress} className="h-2" />
                    </div>
                    <Badge variant={st.variant} className="shrink-0">
                      {item.status === 'done' && <IconCheck />}
                      {item.status === 'uploading' && (
                        <IconLoader2 className="animate-spin" />
                      )}
                      {st.label}
                    </Badge>
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      disabled={uploading}
                      onClick={() => removeItem(item.id)}
                      aria-label="移除"
                    >
                      <IconX />
                    </Button>
                  </li>
                )
              })}
```

替换为：

```tsx
              {items.map((item) => {
                const variant = STATUS_VARIANT[item.status]
                return (
                  <li key={item.id} className="flex items-center gap-3">
                    <img
                      src={item.preview}
                      alt={item.file.name}
                      className="size-14 shrink-0 rounded-xl object-cover ring-1 ring-border"
                    />
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate text-sm font-medium">
                          {item.file.name}
                        </span>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {formatBytes(item.file.size)}
                        </span>
                      </div>
                      <Progress value={item.progress} className="h-2" />
                    </div>
                    <Badge variant={variant} className="shrink-0">
                      {item.status === 'done' && <IconCheck />}
                      {item.status === 'uploading' && (
                        <IconLoader2 className="animate-spin" />
                      )}
                      {t(`upload.status.${item.status}`)}
                    </Badge>
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
                )
              })}
```

- [ ] **Step 10: 替换空态文案（行 337-342）走 i18n**

把原本的：

```tsx
      {items.length === 0 && (
        <div className="grid place-items-center py-6 text-center text-muted-foreground">
          <IconPhoto className="mb-2 size-8 opacity-40" />
          <p className="text-sm">还没有添加任何图片</p>
        </div>
      )}
```

替换为：

```tsx
      {items.length === 0 && (
        <div className="grid place-items-center py-6 text-center text-muted-foreground">
          <IconPhoto className="mb-2 size-8 opacity-40" />
          <p className="text-sm">{t('upload.empty')}</p>
        </div>
      )}
```

- [ ] **Step 11: 替换上传 toast（行 149, 172-174）走 i18n**

行 149 原本是：

```typescript
        if (failed && errMsg) toast.error(`${item.file.name}: ${errMsg}`)
```

改为：

```typescript
        if (failed && errMsg) toast.error(t('upload.toast.itemFailed', { name: item.file.name, msg: errMsg }))
```

行 172-174 原本是：

```typescript
    if (fail === 0) toast.success(`已上传 ${ok} 张图片`)
    else if (ok === 0) toast.error('上传失败')
    else toast.warning(`${ok} 张成功，${fail} 张失败`)
```

改为：

```typescript
    if (fail === 0) toast.success(t('upload.toast.uploadSuccess', { count: ok }))
    else if (ok === 0) toast.error(t('upload.toast.uploadAllFailed'))
    else toast.warning(t('upload.toast.uploadPartial', { ok, fail }))
```

- [ ] **Step 12: 验证 typecheck 通过**

Run: `npx tsc -b` (workdir: `frontend`)
Expected: 无报错退出

- [ ] **Step 13: 验证没有残留硬编码中文（除注释外）**

Run: `npx grep -nE "[一-龥]" frontend/src/features/images/pages/Upload.tsx | grep -v "^[0-9]*:[[:space:]]*//"` — 应该没有结果（或只在注释里）
Expected: 无匹配行

- [ ] **Step 14: Commit**

```bash
git add frontend/src/features/images/pages/Upload.tsx
git commit -m "feat(upload): add copy-format dropdown menu with i18n and useCopy

- Replace hardcoded 'copy all links' button with DropdownMenu offering
  link format (URL/Markdown/BBS/HTML) x image format (original/WebP/AVIF)
- Persist format selection in localStorage
- Migrate all hardcoded Chinese strings to upload.* i18n keys
- Use useCopy hook for reliable async clipboard + visual feedback"
```

---

### Task 6: 最终验证

**Files:** 无修改

- [ ] **Step 1: 跑全部单测**

Run: `npx vitest run` (workdir: `frontend`)
Expected: 全部 PASS（含新增的 copy-format.test.ts、use-copy.test.ts、api.test.ts）

- [ ] **Step 2: 跑生产构建（含 tsc -b）**

Run: `npm run build` (workdir: `frontend`)
Expected: 构建成功，输出到 `../backend/web/`

- [ ] **Step 3: 跑 lint**

Run: `npm run lint` (workdir: `frontend`)
Expected: 无 error（warning 可接受，但不应该新增）

- [ ] **Step 4: 手动验证清单（如可启动 dev server）**

启动 `npm run dev`，在浏览器走一遍：
1. 默认复制：上传两张图，开菜单看到默认选中 URL + WebP，点"复制 N 个链接"，粘贴验证为 `${origin}/i/<link>.webp?q=85` 逐字节一致
2. 切到 Markdown + AVIF，刷新页面，菜单仍显示该选择；复制得到 `![baseName](${origin}/i/<link>.avif?q=85)`
3. 复制后触发器图标短暂变 IconCheck + 主色，1.5s 还原
4. Detail 页点任一 ShareRow 复制按钮，图标变 IconCheck，1.5s 还原
5. DevTools 把 `navigator.clipboard` 删掉再点复制，看到错误 toast，按钮不变绿

- [ ] **Step 5: 最终提交（如有 lint/build 修复）**

如前面 Step 1-3 全过则无需提交；如有修复则：

```bash
git add -A
git commit -m "chore: fix lint/build issues from copy-format feature"
```

---

## Self-Review Notes

**Spec coverage:**
- 决策 1（下拉菜单）→ Task 5 Step 7 ✓
- 决策 2（localStorage）→ Task 2 Step 3 (loadPrefs/savePrefs) + Task 5 Step 3 (useState + useEffect) ✓
- 决策 3（AVIF q=85）→ Task 2 Step 3 (buildUrl) + 测试 ✓
- 决策 4（Upload i18n）→ Task 1 + Task 5 Steps 5-11 ✓
- 决策 5（alt = baseName）→ Task 2 Step 3 (formatLine + baseName) + 测试 ✓
- 决策 6（Upload+Detail 接入）→ Task 3 (hook) + Task 4 (Detail) + Task 5 (Upload) ✓
- 反馈三层（async/视觉/toast）→ Task 3 hook 实现全部覆盖 ✓
- 验收标准 1-10 → Task 6 Step 4 手动验证覆盖 ✓

**Type consistency:**
- `CopyLinkFormat` / `CopyImageFormat` / `CopyPrefs` 在 copy-format.ts 定义，被 useCopy（不直接用）、Upload.tsx 引用 ✓
- `useCopy` 返回 `{ copied, copy }` 在 Upload.tsx 与 Detail.tsx 一致使用 ✓
- `ShareRow` props 从 `{ label, value, display?, onCopy }` 改为 `{ label, value, display?, successMsg? }`，所有 6 处调用同步更新 ✓

**No placeholders:** 所有 step 含完整代码或完整命令，无 TBD/TODO。
