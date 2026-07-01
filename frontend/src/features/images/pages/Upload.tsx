// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

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

const STATUS_VARIANT: Record<Status, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  queued: 'outline',
  uploading: 'secondary',
  done: 'default',
  failed: 'destructive',
}

export default function Upload() {
  const { t } = useTranslation()
  const [items, setItems] = useState<QueueItem[]>([])
  const [visibility, setVisibility] = useState<'public' | 'private'>('public')
  const [dragging, setDragging] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [linkFormat, setLinkFormat] = useState<CopyLinkFormat>(() => loadPrefs().link)
  const [imageFormat, setImageFormat] = useState<CopyImageFormat>(() => loadPrefs().image)
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
            if (failed) errMsg = r.error || t('upload.toast.errorCode', { code: r.error_code || '?' })
            else link = r.unique_link || ''
          } else if (json.code !== 0) {
            failed = true
            errMsg = json.message || t('upload.toast.errorCode', { code: json.code })
          }
        } catch {
          failed = true
          errMsg = t('upload.toast.parseFailed')
        }
        if (failed && errMsg) toast.error(t('upload.toast.itemFailed', { name: item.file.name, msg: errMsg }))
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
    if (fail === 0) toast.success(t('upload.toast.uploadSuccess', { count: ok }))
    else if (ok === 0) toast.error(t('upload.toast.uploadAllFailed'))
    else toast.warning(t('upload.toast.uploadPartial', { ok, fail }))
  }

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setDragging(false)
    if (e.dataTransfer.files?.length) addFiles(e.dataTransfer.files)
  }

  const hasQueued = items.some((i) => i.status === 'queued')
  const completedLinks = items.filter((i) => i.status === 'done' && i.uniqueLink)

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

  return (
    <div className="space-y-6">
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

      {items.length > 0 && (
        <Card>
          <CardContent className="space-y-3 p-5">
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
              </div>
            </div>
            <Separator />
            <ul className="space-y-3">
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
            </ul>
          </CardContent>
        </Card>
      )}

      {items.length === 0 && (
        <div className="grid place-items-center py-6 text-center text-muted-foreground">
          <IconPhoto className="mb-2 size-8 opacity-40" />
          <p className="text-sm">{t('upload.empty')}</p>
        </div>
      )}
    </div>
  )
}
