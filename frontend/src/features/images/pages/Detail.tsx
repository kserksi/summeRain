import {
  IconBan,
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
import { toast } from 'sonner'

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

const FORMAT_OPTIONS: { label: string; value: ProcFormat }[] = [
  { label: '原图', value: 'original' },
  { label: 'WebP', value: 'webp' },
  { label: 'AVIF', value: 'avif' },
  { label: 'JPEG', value: 'jpg' },
  { label: 'PNG', value: 'png' },
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

const TTL_OPTIONS = [
  { label: '10 分钟', value: 10 * 60 * 1000 },
  { label: '1 小时', value: IMAGE_TOKEN.DEFAULT_TTL_MS },
  { label: '24 小时', value: 24 * 60 * 60 * 1000 },
  { label: '72 小时', value: IMAGE_TOKEN.MAX_TTL_MS },
]

function ShareRow({
  label,
  value,
  display,
  onCopy,
}: {
  label: string
  value: string
  display?: string
  onCopy: () => void
}) {
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
          onClick={onCopy}
          aria-label={`复制${label}`}
        >
          <IconCopy />
        </Button>
      </div>
    </div>
  )
}

export default function Detail() {
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

  const imgUrl = `/i/${image.unique_link}.webp?q=85`
  const shareUrl = `${window.location.origin}/i/${image.unique_link}.webp?q=85`
  const isPrivate = image?.visibility === 'private'
  const title = image?.title || image?.filename
  const activeToken = issuedToken || image?.access_token
  const maskedToken = activeToken
    ? `${activeToken.slice(0, 6)}••••••`
    : ''
  const tokenShare = activeToken ? `${shareUrl}&token=${activeToken}` : ''

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
        ← 返回
      </Button>

      <div className="grid gap-6 md:grid-cols-[1fr_360px]">
        <div className="overflow-hidden rounded-3xl bg-card ring-1 ring-border">
          <button
            type="button"
            onClick={() => setLightbox(true)}
            className="group relative block size-full"
            aria-label="放大查看"
          >
            <img
              src={imgUrl}
              alt={image.filename}
              className="mx-auto max-h-[70vh] w-full object-contain"
            />
            <span className="absolute right-3 bottom-3 flex items-center gap-1 rounded-full bg-black/60 px-2.5 py-1 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100">
              <IconZoomScan className="size-3.5" /> 放大
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
                  {isPrivate ? '私密' : '公开'}
                </Badge>
              </div>
              <Separator />
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
                <dt className="text-muted-foreground">尺寸</dt>
                <dd>
                  {image.width} × {image.height}
                </dd>
                <dt className="text-muted-foreground">大小</dt>
                <dd>{formatBytes(image.file_size)}</dd>
                <dt className="flex items-center gap-1 text-muted-foreground">
                  <IconEye className="size-3.5" /> 浏览
                </dt>
                <dd>{image.view_count}</dd>
                <dt className="text-muted-foreground">上传时间</dt>
                <dd>{formatDate(image.created_at)}</dd>
              </dl>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="flex items-center justify-between p-5">
              <div>
                <p className="font-medium">可见性</p>
                <p className="text-sm text-muted-foreground">
                  {isPrivate ? '仅通过令牌链接可访问' : '所有人可见'}
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
              <p className="font-medium">分享链接</p>
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
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-4 p-5">
              <div className="space-y-0.5">
                <p className="flex items-center gap-1.5 font-medium">
                  <IconPhotoEdit className="size-4" /> 图片处理与转换
                </p>
                <p className="text-sm text-muted-foreground">
                  基于 imgproxy 实时转换格式、尺寸与质量。
                </p>
              </div>

              <div className="space-y-1.5">
                <Label className="text-xs text-muted-foreground">格式</Label>
                <div className="flex flex-wrap gap-2">
                  {FORMAT_OPTIONS.map((f) => (
                    <Button
                      key={f.value}
                      size="sm"
                      variant={procFormat === f.value ? 'default' : 'outline'}
                      onClick={() => setProcFormat(f.value)}
                    >
                      {f.label}
                    </Button>
                  ))}
                </div>
              </div>

              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <Label className="text-xs text-muted-foreground">
                    尺寸 (px)
                  </Label>
                  <div className="flex items-center gap-1.5">
                    <span className="text-xs text-muted-foreground">
                      锁定比例
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
                    aria-label="宽度"
                  />
                  <span className="text-muted-foreground">×</span>
                  <Input
                    type="number"
                    min={1}
                    placeholder={String(image.height)}
                    value={procHeight}
                    onChange={(e) => onProcHeightChange(e.target.value)}
                    className="h-8"
                    aria-label="高度"
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setProcWidth('')
                      setProcHeight('')
                    }}
                  >
                    重置
                  </Button>
                </div>
              </div>

              {showQuality && (
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between">
                    <Label className="text-xs text-muted-foreground">
                      质量
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
                <Label className="text-xs text-muted-foreground">链接</Label>
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
              </div>
            </CardContent>
          </Card>

          {isPrivate && (
            <Card>
              <CardContent className="space-y-3 p-5">
                <p className="flex items-center gap-1.5 font-medium">
                  <IconKey className="size-4" /> 访问令牌
                </p>
                <p className="text-sm text-muted-foreground">
                  私密图片需要带令牌的链接才能访问。
                </p>

                {activeToken ? (
                  <>
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
                    {image?.token_expires_at && (
                      <p className="text-xs text-muted-foreground">
                        过期时间：{formatDate(image?.token_expires_at ?? '')}
                      </p>
                    )}
                  </>
                ) : (
                  <p className="text-sm text-muted-foreground">无活跃令牌</p>
                )}

                <div className="flex flex-wrap items-center gap-2">
                  <Label className="text-sm">有效期</Label>
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
                          {o.label}
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
                            toast.success('令牌已签发')
                            const warning = (
                              data as { warning?: string }
                            ).warning
                            if (warning) toast.warning(warning)
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
                    {image?.access_token ? '重新签发' : '签发令牌'}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={revoke.isPending || !image?.access_token}
                    onClick={() => revoke.mutate(image.id)}
                  >
                    <IconBan />
                    吊销
                  </Button>
                </div>
              </CardContent>
            </Card>
          )}

          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="destructive" className="w-full">
                <IconTrash />
                删除图片
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>确认删除？</AlertDialogTitle>
                <AlertDialogDescription>
                  此操作不可撤销，图片将被永久删除。
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>取消</AlertDialogCancel>
                <AlertDialogAction
                  onClick={onDelete}
                  disabled={del.isPending}
                >
                  {del.isPending ? '删除中…' : '确认删除'}
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
