import {
  IconCheck,
  IconLink,
  IconLoader2,
  IconPhoto,
  IconUpload,
  IconX,
} from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
import { getCsrfToken } from '@/lib/csrf'
import { useAuthStore } from '@/store/auth-store'

type Status = 'queued' | 'uploading' | 'done' | 'failed'

interface QueueItem {
  id: string
  file: File
  preview: string
  progress: number
  status: Status
  uniqueLink?: string
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  )
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

const STATUS_MAP: Record<Status, { label: string; variant: 'default' | 'secondary' | 'destructive' | 'outline' }> = {
  queued: { label: '等待中', variant: 'outline' },
  uploading: { label: '上传中', variant: 'secondary' },
  done: { label: '完成', variant: 'default' },
  failed: { label: '失败', variant: 'destructive' },
}

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
  useEffect(
    () => () => {
      itemsRef.current.forEach((i) => URL.revokeObjectURL(i.preview))
    },
    [],
  )

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

  const removeItem = (id: string) => {
    setItems((prev) =>
      prev.filter((i) => {
        if (i.id === id) URL.revokeObjectURL(i.preview)
        return i.id !== id
      }),
    )
  }

  const uploadOne = (item: QueueItem) =>
    new Promise<'done' | 'failed'>((resolve) => {
      const fd = new FormData()
      fd.append('images', item.file)
      fd.append('visibility', visibility)

      const xhr = new XMLHttpRequest()
      xhr.open('POST', `${API_BASE_URL}/images/`)
      xhr.withCredentials = true
      const csrf = getCsrfToken()
      if (csrf) xhr.setRequestHeader('X-CSRF-Token', csrf)

      const patch = (p: Partial<QueueItem>) =>
        setItems((prev) =>
          prev.map((i) => (i.id === item.id ? { ...i, ...p } : i)),
        )

      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          patch({
            progress: Math.min(99, Math.round((e.loaded / e.total) * 100)),
            status: 'uploading',
          })
        }
      }
      xhr.onload = () => {
        let failed = true
        let errMsg = ''
        let link = ''
        try {
          const json = JSON.parse(xhr.responseText)
          if (json.code === 0 && json.data?.results?.length) {
            const r = json.data.results[0]
            failed = !r.success
            if (failed) errMsg = r.error || `错误码 ${r.error_code || '?'}`
            else link = r.unique_link || ''
          } else if (json.code !== 0) {
            failed = true
            errMsg = json.message || `错误码 ${json.code}`
          }
        } catch {
          failed = true
          errMsg = '响应解析失败'
        }
        if (failed && errMsg) toast.error(`${item.file.name}: ${errMsg}`)
        patch({ status: failed ? 'failed' : 'done', progress: 100, uniqueLink: link || undefined })
        resolve(failed ? 'failed' : 'done')
      }
      xhr.onerror = () => {
        patch({ status: 'failed' })
        resolve('failed')
      }

      patch({ status: 'uploading', progress: 0 })
      xhr.send(fd)
    })

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
    if (fail === 0) toast.success(`已上传 ${ok} 张图片`)
    else if (ok === 0) toast.error('上传失败')
    else toast.warning(`${ok} 张成功，${fail} 张失败`)
  }

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setDragging(false)
    if (e.dataTransfer.files?.length) addFiles(e.dataTransfer.files)
  }

  const hasQueued = items.some((i) => i.status === 'queued')
  const completedLinks = items.filter((i) => i.status === 'done' && i.uniqueLink)

  const copyAllLinks = () => {
    const urls = completedLinks.map((i) => `${window.location.origin}/i/${i.uniqueLink}.webp?q=85`)
    if (!urls.length) return
    navigator.clipboard.writeText(urls.join('\n'))
    toast.success(`已复制 ${urls.length} 个链接`)
  }

  return (
    <div className="space-y-6">
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

      {items.length > 0 && (
        <Card>
          <CardContent className="space-y-3 p-5">
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
              </div>
            </div>
            <Separator />
            <ul className="space-y-3">
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
            </ul>
          </CardContent>
        </Card>
      )}

      {items.length === 0 && (
        <div className="grid place-items-center py-6 text-center text-muted-foreground">
          <IconPhoto className="mb-2 size-8 opacity-40" />
          <p className="text-sm">还没有添加任何图片</p>
        </div>
      )}
    </div>
  )
}
