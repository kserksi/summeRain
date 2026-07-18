// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useCopy } from '@/lib/use-copy'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Slider } from '@/components/ui/slider'
import { Switch } from '@/components/ui/switch'
import { IMAGE_TOKEN } from '@/config/constants'

import { Lightbox } from '../components/Lightbox'
import {
  appendAccessToken,
  hasV2Assets,
  resolveAbsoluteImageAssetUrl,
  resolveDetailPreviewUrl,
} from '../asset-url'
import {
  useDeleteImage,
  useImage,
  useIssueToken,
  useRevokeToken,
  useToggleVisibility,
} from '../hooks'

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  )
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

type ProcFormat = 'original' | 'webp' | 'avif' | 'jpg' | 'png'

const FORMAT_OPTIONS: { labelKey: string; value: ProcFormat }[] = [
  { labelKey: 'images.shared.formatOriginal', value: 'original' },
  { labelKey: 'images.shared.formatWebp', value: 'webp' },
  { labelKey: 'images.shared.formatAvif', value: 'avif' },
  { labelKey: 'images.shared.formatJpeg', value: 'jpg' },
  { labelKey: 'images.shared.formatPng', value: 'png' },
]

function buildImageUrl(
  link: string,
  format: ProcFormat,
  width?: number,
  height?: number,
  quality?: number,
): string {
  const ext = format === 'original' ? '' : `.${format}`
  const params = new URLSearchParams()
  if (width) params.set('w', String(width))
  if (height) params.set('h', String(height))
  if (quality && format !== 'original' && format !== 'png')
    params.set('q', String(quality))
  const qs = params.toString()
  return `/i/${link}${ext}${qs ? `?${qs}` : ''}`
}

const TTL_OPTIONS: { labelKey: string; value: number }[] = [
  { labelKey: 'images.detail.ttl10m', value: 10 * 60 * 1000 },
  { labelKey: 'images.detail.ttl1h', value: IMAGE_TOKEN.DEFAULT_TTL_MS },
  { labelKey: 'images.detail.ttl24h', value: 24 * 60 * 60 * 1000 },
  { labelKey: 'images.detail.ttl72h', value: IMAGE_TOKEN.MAX_TTL_MS },
]

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
  const { t } = useTranslation()
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
          aria-label={t('images.detail.copyAriaLabel', { label })}
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

function ProcessedLinkRow({ value }: { value: string }) {
  const { t } = useTranslation()
  const { copied, copy } = useCopy()
  return (
    <div className="flex gap-2">
      <Input readOnly value={value} className="font-mono text-xs" />
      <Button type="button" onClick={() => copy(value, t('images.detail.processedLinkCopied'))}>
        {copied ? (
          <IconCheck className="text-primary" />
        ) : (
          <IconCopy />
        )}
        {t('images.shared.copy')}
      </Button>
    </div>
  )
}

export default function Detail() {
  const { t } = useTranslation()
  const { id } = useParams()
  const navigate = useNavigate()
  const numericId = Number(id)

  const { data: image, isLoading } = useImage(numericId)
  const [lightbox, setLightbox] = useState(false)
  const [ttl, setTtl] = useState<number>(IMAGE_TOKEN.DEFAULT_TTL_MS)
  const [procFormat, setProcFormat] = useState<ProcFormat>('avif')
  const [procWidth, setProcWidth] = useState('')
  const [procHeight, setProcHeight] = useState('')
  const [quality, setQuality] = useState(80)
  const [lockAspect, setLockAspect] = useState(false)
  const [issuedToken, setIssuedToken] = useState<string | null>(null)
  const [issuedTokenExpiresAt, setIssuedTokenExpiresAt] = useState<string | null>(null)

  const toggleVis = useToggleVisibility()
  const del = useDeleteImage()
  const issue = useIssueToken()
  const revoke = useRevokeToken()

  const processedPath = buildImageUrl(
    image?.unique_link ?? '',
    procFormat,
    procWidth ? Number(procWidth) : undefined,
    procHeight ? Number(procHeight) : undefined,
    quality,
  )

  if (isLoading || !image) {
    return (
      <div className="grid gap-6 md:grid-cols-[1fr_360px]">
        <Skeleton className="aspect-video rounded-3xl" />
        <Skeleton className="h-96 rounded-3xl" />
      </div>
    )
  }

  const isV2 = hasV2Assets(image)
  const imgUrl = resolveDetailPreviewUrl(image)
  const shareUrl = resolveAbsoluteImageAssetUrl(
    window.location.origin,
    image,
    'publish',
  )
  const fixedAssetUrls = {
    publish: shareUrl,
    master: resolveAbsoluteImageAssetUrl(window.location.origin, image, 'master'),
    gallery: resolveAbsoluteImageAssetUrl(window.location.origin, image, 'gallery'),
    admin: resolveAbsoluteImageAssetUrl(window.location.origin, image, 'admin'),
  }
  const isPrivate = image?.visibility === 'private'
  const title = image?.title || image?.filename
  const activeToken = issuedToken || image?.access_token
  const activeTokenExpiresAt = issuedTokenExpiresAt || image?.token_expires_at
  const maskedToken = activeToken
    ? `${activeToken.slice(0, 6)}••••••`
    : ''
  const tokenShare = activeToken ? appendAccessToken(shareUrl, activeToken) : ''

  const aspectRatio = image.height / image.width
  const onProcWidthChange = (val: string) => {
    setProcWidth(val)
    if (lockAspect && val) {
      const w = Number(val)
      if (w > 0) setProcHeight(String(Math.round(w * aspectRatio)))
    }
  }
  const onProcHeightChange = (val: string) => {
    setProcHeight(val)
    if (lockAspect && val) {
      const h = Number(val)
      if (h > 0) setProcWidth(String(Math.round(h / aspectRatio)))
    }
  }
  const showQuality = procFormat === 'webp' || procFormat === 'jpg' || procFormat === 'avif'
  const processedUrl = `${window.location.origin}${processedPath}`

  const onDelete = async () => {
    try {
      await del.mutateAsync(image.id)
      navigate('/images')
    } catch {
      // handled by mutation toast
    }
  }

  return (
    <div className="space-y-6">
      <Button variant="ghost" size="sm" onClick={() => navigate('/images')}>
        ← {t('common.back')}
      </Button>

      <div className="grid gap-6 md:grid-cols-[1fr_360px]">
        <div className="overflow-hidden rounded-3xl bg-card ring-1 ring-border">
          <button
            type="button"
            onClick={() => setLightbox(true)}
            className="group relative block size-full"
            aria-label={t('images.detail.zoomAria')}
          >
            <img
              src={imgUrl}
              alt={image.filename}
              className="mx-auto max-h-[70vh] w-full object-contain"
            />
            <span className="absolute right-3 bottom-3 flex items-center gap-1 rounded-full bg-black/60 px-2.5 py-1 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100">
              <IconZoomScan className="size-3.5" /> {t('images.detail.zoom')}
            </span>
          </button>
        </div>

        <div className="space-y-4">
          <Card>
            <CardContent className="space-y-3 p-5">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <h1 className="truncate text-lg font-semibold">{title}</h1>
                  <p className="truncate text-sm text-muted-foreground">
                    {image.filename}
                  </p>
                </div>
                <Badge variant={isPrivate ? 'secondary' : 'default'}>
                  {isPrivate ? <IconLock /> : <IconWorld />}
                  {isPrivate ? t('images.shared.private') : t('images.shared.public')}
                </Badge>
              </div>
              <Separator />
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
                <dt className="text-muted-foreground">{t('images.shared.dimensions')}</dt>
                <dd>
                  {image.width} × {image.height}
                </dd>
                <dt className="text-muted-foreground">{t('images.shared.size')}</dt>
                <dd>{formatBytes(image.file_size)}</dd>
                <dt className="flex items-center gap-1 text-muted-foreground">
                  <IconEye className="size-3.5" /> {t('images.shared.views')}
                </dt>
                <dd>{image.view_count}</dd>
                <dt className="text-muted-foreground">{t('images.shared.uploadedAt')}</dt>
                <dd>{formatDate(image.created_at)}</dd>
              </dl>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="flex items-center justify-between p-5">
              <div>
                <p className="font-medium">{t('upload.visibility')}</p>
                <p className="text-sm text-muted-foreground">
                  {isPrivate ? t('images.detail.privateHint') : t('images.detail.publicHint')}
                </p>
              </div>
              <Switch
                checked={isPrivate}
                onCheckedChange={(checked) =>
                  toggleVis.mutate({
                    id: image.id,
                    visibility: checked ? 'private' : 'public',
                  })
                }
                disabled={toggleVis.isPending}
              />
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-3 p-5">
              <p className="font-medium">{t('images.detail.shareLinks')}</p>
              <ShareRow label={t('images.shared.link')} value={shareUrl} />
              <ShareRow
                label="Markdown"
                value={`![${title}](${shareUrl})`}
              />
              <ShareRow
                label="HTML"
                value={`<img src="${shareUrl}" alt="${title}" />`}
              />
            </CardContent>
          </Card>

          {isV2 ? (
            <Card>
              <CardContent className="space-y-3 p-5">
                <p className="font-medium">{t('images.detail.shareLinks')}</p>
                <ShareRow label="Publish" value={fixedAssetUrls.publish} />
                <ShareRow label="Master" value={fixedAssetUrls.master} />
                <ShareRow label="Gallery" value={fixedAssetUrls.gallery} />
                <ShareRow label="Admin" value={fixedAssetUrls.admin} />
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardContent className="space-y-4 p-5">
              <div className="space-y-0.5">
                <p className="flex items-center gap-1.5 font-medium">
                  <IconPhotoEdit className="size-4" /> {t('images.detail.processingTitle')}
                </p>
                <p className="text-sm text-muted-foreground">
                  {t('images.detail.processingHint')}
                </p>
              </div>

              <div className="space-y-1.5">
                <Label className="text-xs text-muted-foreground">{t('images.shared.format')}</Label>
                <div className="flex flex-wrap gap-2">
                  {FORMAT_OPTIONS.map((f) => (
                    <Button
                      key={f.value}
                      size="sm"
                      variant={procFormat === f.value ? 'default' : 'outline'}
                      onClick={() => setProcFormat(f.value)}
                    >
                      {t(f.labelKey)}
                    </Button>
                  ))}
                </div>
              </div>

              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <Label className="text-xs text-muted-foreground">
                    {t('images.shared.dimensions')} (px)
                  </Label>
                  <div className="flex items-center gap-1.5">
                    <span className="text-xs text-muted-foreground">
                      {t('images.detail.lockAspect')}
                    </span>
                    <Switch
                      checked={lockAspect}
                      onCheckedChange={setLockAspect}
                    />
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Input
                    type="number"
                    min={1}
                    placeholder={String(image.width)}
                    value={procWidth}
                    onChange={(e) => onProcWidthChange(e.target.value)}
                    className="h-8"
                    aria-label={t('images.detail.widthAria')}
                  />
                  <span className="text-muted-foreground">×</span>
                  <Input
                    type="number"
                    min={1}
                    placeholder={String(image.height)}
                    value={procHeight}
                    onChange={(e) => onProcHeightChange(e.target.value)}
                    className="h-8"
                    aria-label={t('images.detail.heightAria')}
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setProcWidth('')
                      setProcHeight('')
                    }}
                  >
                    {t('images.detail.reset')}
                  </Button>
                </div>
              </div>

              {showQuality && (
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between">
                    <Label className="text-xs text-muted-foreground">
                      {t('images.detail.quality')}
                    </Label>
                    <span className="text-xs tabular-nums text-muted-foreground">
                      {quality}
                    </span>
                  </div>
                  <Slider
                    value={[quality]}
                    onValueChange={(v) => setQuality(v[0])}
                    min={10}
                    max={100}
                    step={1}
                    className="w-full"
                  />
                </div>
              )}

              <div className="space-y-1.5">
                <Label className="text-xs text-muted-foreground">{t('images.shared.link')}</Label>
                <ProcessedLinkRow value={processedUrl} />
              </div>
              </CardContent>
            </Card>
          )}

          {isPrivate && (
            <Card>
              <CardContent className="space-y-3 p-5">
                <p className="flex items-center gap-1.5 font-medium">
                  <IconKey className="size-4" /> {t('images.detail.accessToken')}
                </p>
                <p className="text-sm text-muted-foreground">
                  {t('images.detail.tokenHelp')}
                </p>

                {activeToken ? (
                  <>
                    <ShareRow
                      label={t('images.shared.token')}
                      value={activeToken}
                      display={maskedToken}
                      successMsg={t('images.detail.tokenCopied')}
                    />
                    <ShareRow label={t('images.detail.tokenLink')} value={tokenShare} />
                    {activeTokenExpiresAt && (
                      <p className="text-xs text-muted-foreground">
                        {t('images.detail.expiresAt', { date: formatDate(activeTokenExpiresAt) })}
                      </p>
                    )}
                  </>
                ) : (
                  <p className="text-sm text-muted-foreground">{t('images.detail.noActiveToken')}</p>
                )}

                <div className="flex flex-wrap items-center gap-2">
                  <Label className="text-sm">{t('images.detail.validity')}</Label>
                  <Select
                    value={String(ttl)}
                    onValueChange={(v) => setTtl(Number(v))}
                  >
                    <SelectTrigger className="h-8 w-36">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {TTL_OPTIONS.map((o) => (
                        <SelectItem key={o.value} value={String(o.value)}>
                          {t(o.labelKey)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    disabled={issue.isPending}
                    onClick={() =>
                      issue.mutate(
                        { id: image.id, ttlMs: ttl },
                        {
                          onSuccess: (data) => {
                            setIssuedToken(data.access_token || '')
                            setIssuedTokenExpiresAt(data.token_expires_at || null)
                            if (data.warning) toast.warning(data.warning)
                          },
                        },
                      )
                    }
                  >
                    {issue.isPending ? (
                      <IconLoader2 className="animate-spin" />
                    ) : (
                      <IconRefresh />
                    )}
                    {activeToken ? t('images.detail.reissueToken') : t('images.detail.issueToken')}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={revoke.isPending || !activeToken}
                    onClick={() =>
                      revoke.mutate(image.id, {
                        onSuccess: () => {
                          setIssuedToken(null)
                          setIssuedTokenExpiresAt(null)
                        },
                      })
                    }
                  >
                    <IconBan />
                    {t('images.detail.revoke')}
                  </Button>
                </div>
              </CardContent>
            </Card>
          )}

          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="destructive" className="w-full">
                <IconTrash />
                {t('images.detail.deleteImage')}
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>{t('images.detail.confirmDeleteTitle')}</AlertDialogTitle>
                <AlertDialogDescription>
                  {t('images.detail.confirmDeleteDesc')}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
                <AlertDialogAction
                  onClick={onDelete}
                  disabled={del.isPending}
                >
                  {del.isPending ? t('images.detail.deleting') : t('images.detail.confirmDelete')}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </div>

      {lightbox && <Lightbox src={imgUrl} onClose={() => setLightbox(false)} />}
    </div>
  )
}
