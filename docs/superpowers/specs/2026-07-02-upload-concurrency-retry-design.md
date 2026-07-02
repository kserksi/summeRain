# 上传并发限流 + 重试机制 + 格式收紧设计

- 日期：2026-07-02
- 状态：已通过 brainstorming，待 writing-plans 拆解
- 范围：仅前端，1 个文件修改 + 1 个 i18n 文件追加
- 前序 spec：`2026-07-01-upload-copy-popover-design.md`（已上线）

## 背景

线上反馈两个问题：

1. **大批量上传易失败**：单次上传 50 张图时失败率高，每次只传 20 张失败率显著下降。
2. **无重试机制**：上传失败的图片只能移除后重新添加，导致原本的队列顺序被打乱。

systematic-debugging 根因分析定位：

- **问题 1 根因**：`Upload.tsx:188` 的 `Promise.all(pending.map(uploadOne))` **无并发上限**，N 个文件 = N 个并发 XHR。后端 `image_service.go` 每次上传同步调 imgproxy 两次，imgproxy 客户端 10 秒超时（`imgproxy_service.go:27`）。并发一高，imgproxy 队列塞满 → 大面积超时。与单文件大小无关。
- **问题 2 根因**：`Upload.tsx:185` 的 `items.filter((i) => i.status === 'queued')` 只挑 queued 项；`xhr.onerror` 直接标记 failed，零自动重试。队列里 failed 项没有任何重试入口，移除后重新添加必然把新项排到队尾。

附带发现（独立审计）：前端 `addFiles` 过滤用 `f.type.startsWith('image/')`（过松，接受 svg/avif/bmp 等后端会拒的文件），`<input accept="image/*">` 同样过松，导致用户能选上后端会拒的文件，UX 差。

## 目标

1. 前端并发上限 = 5，消除大批量上传的 imgproxy 超时
2. 提供"单项重试"与"重试全部失败"两个入口，**保留原队列顺序**（id 不变、数组位置不变）
3. 前端格式过滤与后端白名单精确对齐，选择阶段就拒收不支持的文件

## 非目标

- 不动后端（imgproxy 同步调用、`io.ReadAll` 内存读、10s 超时——前端限流已足以解决问题 1）
- 不加自动重试（不掩盖真实错误，由用户决定何时重试）
- 不动 nginx/docker 资源限制
- 不开放 AVIF 上传（Go 1.24 虽然支持 AVIF 嗅探了，但这是独立决策，不在本 spec 范围）
- 不动 Detail.tsx、use-copy.ts、copy-format.ts、popover.tsx（不引入回归）

## 决策记录

| # | 项 | 选择 |
|---|----|----|
| 1 | 并发数 | **5**（用户选） |
| 2 | 重试范围 | **单项重试 + 批量重试都加**（用户选） |
| 3 | 自动重试 | **不加**（用户未选；不掩盖真实错误） |
| 4 | 后端改动 | **不动**（前端限流已解决） |
| 5 | 前端格式过滤 | **收紧到与后端精确对齐**（用户选"顺手修"） |
| 6 | AVIF 上传支持 | **不开放**（用户未选） |

## 设计

### 改动 1：并发限流

**模块级新增常量与 helper（可单测）：**

```ts
const CONCURRENCY = 5

async function runWithConcurrency<T, R>(
  items: T[],
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

经典的 "N 个 worker 抢队列" 模式。`results[i]` 按输入顺序排列（保证顺序可预测），`index++` 在 JS 单线程下原子。

**`startUpload` 改造**（`Upload.tsx:184-197`）：把 `Promise.all(pending.map(uploadOne))` 换成 `runWithConcurrency(pending, uploadOne, CONCURRENCY)`。其余逻辑（toast 统计、`qc.invalidateQueries`、`refreshUser`）不变。

### 改动 2：重试机制

**抽 `runUploads` 公共函数**（供 `startUpload` / `retryAllFailed` / `retryOne` 共用）：

```ts
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

**顺序保持的关键**：`uploadOne` 内部 `patch({ status, ... })` 通过 `setItems((prev) => prev.map(...))` 原地更新，**不改 id、不改数组位置**。重试只是把 status 从 failed 重新走一遍，队列顺序完全不变。

**UI 改动 A：单项重试按钮**（在队列 `<li>` 内，"移除"按钮左侧）：

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
```

**UI 改动 B：批量重试按钮**（队列标题栏，"复制全部链接"右侧）：

```tsx
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
```

需要计算 `const failedCount = items.filter((i) => i.status === 'failed').length`。

**新增 imports**：`IconRefresh` from `@tabler/icons-react`。

### 改动 3：前端格式收紧

**模块级常量**：

```ts
const ALLOWED_EXTS = ['.png', '.jpg', '.jpeg', '.webp', '.gif']
const ALLOWED_TYPES = ['image/png', 'image/jpeg', 'image/webp', 'image/gif']
```

与后端 `image_service.go:247`（扩展名）和 `image_service.go:452`（MIME）**精确对齐**。

**`addFiles` 过滤改造**（`Upload.tsx:105-116`）：

```ts
const addFiles = useCallback((files: FileList | File[]) => {
  const next = Array.from(files)
    .filter((f) => {
      const ext = f.name.toLowerCase().slice(f.name.lastIndexOf('.'))
      return ALLOWED_EXTS.includes(ext) && ALLOWED_TYPES.includes(f.type)
    })
    .map((f) => ({ /* ...unchanged... */ }))
  if (next.length) setItems((prev) => [...prev, ...next])
}, [])
```

**`<input>` 改造**（`Upload.tsx:247`）：

```tsx
<input
  accept=".png,.jpg,.jpeg,.webp,.gif,image/png,image/jpeg,image/webp,image/gif"
  ...
/>
```

双轨（扩展名 + MIME）为了兼顾不同 OS 文件选择器行为：扩展名优先（Windows），MIME 作为兜底（macOS/Linux）。

### i18n 新增（`zh-CN.json` 的 `upload` namespace 下）

```jsonc
"upload": {
  // ...existing keys...
  "retry": "重试",
  "retryAll": "重试全部失败（{{count}}）"
}
```

### 不改的方面

- `uploadOne` 内部逻辑完全不动（XHR、onprogress、onload、onerror、patch 全保留）
- `copyAllLinks`、popover、useCopy、localStorage 持久化——全部不动
- 默认 URL+WebP 复制行为不变
- 后端零改动

## 验收标准

1. 队列 50 张图：浏览器 Network 面板观察同时进行的 XHR **不超过 5 个**
2. 模拟失败（DevTools throttle 到 Slow 3G 或 offline）→ 项标记 failed
3. 点单项"重试"图标 → **仅该项**重新上传，**位置不变**
4. 多项失败时点"重试全部失败 (N)"→ 所有 failed 项并发 ≤5 地重跑，**各自位置不变**
5. uploading 进行中所有按钮（开始上传/重试/重试全部/移除）正确禁用
6. 选 `.svg`/`.avif`/`.bmp`/`.heic` → **不进队列**（前端拒收）；选 `.png/.jpg/.jpeg/.webp/.gif` → 正常进队列
7. 浏览器文件选择器对 `.svg` 等不支持的类型直接灰掉（OS 级 accept 生效）
8. `runWithConcurrency` 单测通过（顺序保证、并发上限、空数组、错误传播）
9. typecheck / lint / build 干净
10. 默认 URL+WebP 复制、popover 行为不受影响（不回归）

## 不改的东西

- 后端、API
- `i18n/index.ts`、locale 文件结构
- Detail.tsx、use-copy.ts、copy-format.ts、popover.tsx
- 其他 feature 页面

## 测试策略

`runWithConcurrency` 抽到模块级（不依赖 React）→ 单测可直接 import。覆盖：

- **顺序**：`results[i]` 严格对应 `items[i]`
- **并发上限**：mock fn 计数同时 in-flight 实例，断言峰值 ≤ limit
- **空数组**：返回空数组、不报错
- **错误传播**：`fn` reject 时 `runWithConcurrency` 也 reject（让调用方决定如何处理）

UI 部分（重试按钮、format filter）不加单测——保留与现有 Upload.tsx 一致的"无组件测试"基线（业务逻辑在 `runWithConcurrency` 已覆盖）。
