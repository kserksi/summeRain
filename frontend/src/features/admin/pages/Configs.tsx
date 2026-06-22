import { useMemo, useState } from 'react'
import {
  IconAdjustments,
  IconShieldCheck,
  IconKey,
  IconDeviceFloppy,
  IconDroplet,
  IconCloud,
  IconPlugConnected,
} from '@tabler/icons-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { CAPTCHA_PROVIDERS, IMAGE_TOKEN } from '@/config/constants'
import type { SystemConfig } from '@/lib/types'
import { useConfigs, useUpdateConfigs, useTestR2 } from '../hooks'

const CFG = {
  CAPTCHA_PROVIDER: 'captcha_provider',
  CAPTCHA_SITE_KEY: 'captcha_site_key',
  CAPTCHA_SECRET: 'captcha_secret',
  IMAGE_TOKEN_TTL: 'image_token_default_ttl',
  WATERMARK_ENABLED: 'watermark_enabled',
  WATERMARK_TEXT: 'watermark_text',
  WATERMARK_OPACITY: 'watermark_opacity',
  WATERMARK_POSITION: 'watermark_position',
  WATERMARK_SIZE: 'watermark_size',
  WATERMARK_COLOR: 'watermark_color',
  R2_ENABLED: 'r2_enabled',
  R2_ENDPOINT: 'r2_endpoint',
  R2_ACCESS_KEY: 'r2_access_key',
  R2_SECRET_KEY: 'r2_secret_key',
  R2_BUCKET: 'r2_bucket',
  R2_PUBLIC_URL: 'r2_public_url',
} as const

const PROVIDER_LABELS: Record<string, string> = {
  none: '不启用',
  recaptcha: 'reCAPTCHA v3',
  turnstile: 'Cloudflare Turnstile',
  geetest_v4: 'GeeTest v4',
}

const EXTERNAL_SCRIPT_HINTS: Record<string, string> = {
  recaptcha: '需在页面引入 https://www.google.com/recaptcha/api.js',
  turnstile: '需在页面引入 https://challenges.cloudflare.com/turnstile/api.js',
  geetest_v4: '需在页面引入 GeeTest v4 SDK 脚本',
}

const WATERMARK_POSITIONS = [
  { value: 'ce', label: '居中' },
  { value: 'soea', label: '右下' },
  { value: 'sowe', label: '左下' },
  { value: 'noea', label: '右上' },
  { value: 'nowe', label: '左上' },
] as const

type FormState = {
  provider: string
  siteKey: string
  secret: string
  ttl: string
  watermarkEnabled: boolean
  watermarkText: string
  watermarkOpacity: string
  watermarkPosition: string
  watermarkSize: string
  watermarkColor: string
  r2Enabled: boolean
  r2Endpoint: string
  r2AccessKey: string
  r2SecretKey: string
  r2Bucket: string
  r2PublicURL: string
}

const DEFAULTS: FormState = {
  provider: 'none',
  siteKey: '',
  secret: '',
  ttl: String(IMAGE_TOKEN.DEFAULT_TTL_MS),
  watermarkEnabled: false,
  watermarkText: '',
  watermarkOpacity: '0.5',
  watermarkPosition: 'soea',
  watermarkSize: '64',
  watermarkColor: 'ffffff',
  r2Enabled: false,
  r2Endpoint: '',
  r2AccessKey: '',
  r2SecretKey: '',
  r2Bucket: '',
  r2PublicURL: '',
}

function fromConfigs(configs: SystemConfig[]): FormState {
  const map = new Map(configs.map((c) => [c.config_key, c.config_value]))
  return {
    provider: map.get(CFG.CAPTCHA_PROVIDER) ?? DEFAULTS.provider,
    siteKey: map.get(CFG.CAPTCHA_SITE_KEY) ?? '',
    secret: map.get(CFG.CAPTCHA_SECRET) ?? '',
    ttl: map.get(CFG.IMAGE_TOKEN_TTL) ?? DEFAULTS.ttl,
    watermarkEnabled: (map.get(CFG.WATERMARK_ENABLED) ?? 'false') === 'true',
    watermarkText: map.get(CFG.WATERMARK_TEXT) ?? '',
    watermarkOpacity: map.get(CFG.WATERMARK_OPACITY) ?? DEFAULTS.watermarkOpacity,
    watermarkPosition: map.get(CFG.WATERMARK_POSITION) ?? DEFAULTS.watermarkPosition,
    watermarkSize: map.get(CFG.WATERMARK_SIZE) ?? DEFAULTS.watermarkSize,
    watermarkColor: map.get(CFG.WATERMARK_COLOR) ?? DEFAULTS.watermarkColor,
    r2Enabled: (map.get(CFG.R2_ENABLED) ?? 'false') === 'true',
    r2Endpoint: map.get(CFG.R2_ENDPOINT) ?? '',
    r2AccessKey: map.get(CFG.R2_ACCESS_KEY) ?? '',
    r2SecretKey: map.get(CFG.R2_SECRET_KEY) ?? '',
    r2Bucket: map.get(CFG.R2_BUCKET) ?? '',
    r2PublicURL: map.get(CFG.R2_PUBLIC_URL) ?? '',
  }
}

const CFG_MAP: Record<keyof FormState, string> = {
  provider: CFG.CAPTCHA_PROVIDER,
  siteKey: CFG.CAPTCHA_SITE_KEY,
  secret: CFG.CAPTCHA_SECRET,
  ttl: CFG.IMAGE_TOKEN_TTL,
  watermarkEnabled: CFG.WATERMARK_ENABLED,
  watermarkText: CFG.WATERMARK_TEXT,
  watermarkOpacity: CFG.WATERMARK_OPACITY,
  watermarkPosition: CFG.WATERMARK_POSITION,
  watermarkSize: CFG.WATERMARK_SIZE,
  watermarkColor: CFG.WATERMARK_COLOR,
  r2Enabled: CFG.R2_ENABLED,
  r2Endpoint: CFG.R2_ENDPOINT,
  r2AccessKey: CFG.R2_ACCESS_KEY,
  r2SecretKey: CFG.R2_SECRET_KEY,
  r2Bucket: CFG.R2_BUCKET,
  r2PublicURL: CFG.R2_PUBLIC_URL,
}

const FORM_KEYS = Object.keys(CFG_MAP) as (keyof FormState)[]

export default function Configs() {
  const { data, isLoading } = useConfigs()
  const updateConfigs = useUpdateConfigs()
  const testR2 = useTestR2()

  const server = useMemo<FormState>(() => (data ? fromConfigs(data) : DEFAULTS), [data])
  const [edits, setEdits] = useState<Partial<FormState>>({})

  const form = useMemo<FormState>(() => ({ ...server, ...edits }), [server, edits])

  const changedItems = useMemo<{ key: string; value: string }[]>(() => {
    return FORM_KEYS.filter((k) => {
      const fv = form[k]
      const sv = server[k]
      if (typeof fv === 'boolean' || typeof sv === 'boolean') {
        return String(fv) !== String(sv)
      }
      return fv !== sv
    }).map((k) => ({
      key: CFG_MAP[k],
      value: String(form[k]),
    }))
  }, [form, server])

  const hasChanges = changedItems.length > 0

  function setField<K extends keyof FormState>(key: K, value: FormState[K]) {
    setEdits((prev) => ({ ...prev, [key]: value }))
  }

  function handleSave() {
    if (!hasChanges) return
    updateConfigs.mutate(changedItems, {
      onSuccess: () => {
        // 不立即 setEdits({}) —— 等 query refetch 后 server 自然追上 form，changedItems 归零
      },
    })
  }

  function handleCancel() {
    setEdits({})
  }

  function handleTestR2() {
    testR2.mutate({
      endpoint: form.r2Endpoint,
      access_key: form.r2AccessKey,
      secret_key: form.r2SecretKey,
      bucket: form.r2Bucket,
    })
  }

  const r2FormComplete = form.r2Endpoint && form.r2AccessKey && form.r2SecretKey && form.r2Bucket

  const scriptHint = EXTERNAL_SCRIPT_HINTS[form.provider]

  return (
    <div className="flex min-h-[calc(100vh-8rem)] flex-col gap-6">
      <div>
        <h1 className="font-heading text-2xl font-semibold">系统配置</h1>
        <p className="mt-1 text-sm text-muted-foreground">人机验证、图片访问令牌与水印设置</p>
      </div>

      <div className="flex-1 space-y-6">
        <Card className="rounded-3xl">
          <CardHeader>
            <div className="flex items-center gap-2">
              <IconShieldCheck className="size-5 text-primary" />
              <CardTitle>人机验证</CardTitle>
            </div>
            <CardDescription>配置注册与登录流程的验证码服务</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>验证服务提供商</Label>
              {isLoading ? (
                <Skeleton className="h-9 w-full max-w-xs" />
              ) : (
                <Select value={form.provider} onValueChange={(v) => setField('provider', v)}>
                  <SelectTrigger className="w-full max-w-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {CAPTCHA_PROVIDERS.map((p) => (
                      <SelectItem key={p} value={p}>
                        {PROVIDER_LABELS[p] ?? p}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              {scriptHint && (
                <p className="text-xs text-muted-foreground">{scriptHint}</p>
              )}
            </div>

            <Separator />

            <div className="space-y-2">
              <Label htmlFor="site-key">Site Key</Label>
              <Input
                id="site-key"
                placeholder="公钥，用于前端渲染"
                value={form.siteKey}
                disabled={form.provider === 'none' || isLoading}
                onChange={(e) => setField('siteKey', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="captcha-secret" className="flex items-center gap-1.5">
                <IconKey className="size-3.5" />
                Secret
              </Label>
              <Input
                id="captcha-secret"
                type="password"
                placeholder="私钥，用于服务端校验"
                value={form.secret}
                disabled={form.provider === 'none' || isLoading}
                onChange={(e) => setField('secret', e.target.value)}
              />
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-3xl">
          <CardHeader>
            <div className="flex items-center gap-2">
              <IconAdjustments className="size-5 text-primary" />
              <CardTitle>私密图片令牌有效期</CardTitle>
            </div>
            <CardDescription>访问私密图片所需的临时令牌默认有效时长</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            <Label htmlFor="ttl">默认有效期（毫秒）</Label>
            <Input
              id="ttl"
              type="number"
              inputMode="numeric"
              min={IMAGE_TOKEN.MIN_TTL_MS}
              max={IMAGE_TOKEN.MAX_TTL_MS}
              value={form.ttl}
              disabled={isLoading}
              onChange={(e) => setField('ttl', e.target.value)}
            />
            <p className="text-xs text-muted-foreground tabular-nums">
              允许范围：{IMAGE_TOKEN.MIN_TTL_MS.toLocaleString()} –{' '}
              {IMAGE_TOKEN.MAX_TTL_MS.toLocaleString()} ms（默认{' '}
              {IMAGE_TOKEN.DEFAULT_TTL_MS.toLocaleString()} ms）
            </p>
          </CardContent>
        </Card>

        <Card className="rounded-3xl">
          <CardHeader>
            <div className="flex items-center gap-2">
              <IconDroplet className="size-5 text-primary" />
              <CardTitle>图片水印</CardTitle>
            </div>
            <CardDescription>启用后，所有经 imgproxy 处理的图片将自动添加水印</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <Label htmlFor="watermark-enabled">启用水印</Label>
              {isLoading ? (
                <Skeleton className="h-6 w-11" />
              ) : (
                <Switch
                  id="watermark-enabled"
                  checked={form.watermarkEnabled}
                  onCheckedChange={(v) => setField('watermarkEnabled', v)}
                />
              )}
            </div>

            <Separator />

            <div className="space-y-2">
              <Label htmlFor="watermark-text">水印文字</Label>
              <Input
                id="watermark-text"
                placeholder="例如 © kserks"
                value={form.watermarkText}
                disabled={!form.watermarkEnabled || isLoading}
                onChange={(e) => setField('watermarkText', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="watermark-opacity">不透明度（0.0–1.0）</Label>
              <Input
                id="watermark-opacity"
                type="number"
                inputMode="decimal"
                step="0.1"
                min={0}
                max={1}
                value={form.watermarkOpacity}
                disabled={!form.watermarkEnabled || isLoading}
                onChange={(e) => setField('watermarkOpacity', e.target.value)}
              />
              <p className="text-xs text-muted-foreground">推荐 0.3–0.7，值越大越不透明</p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="watermark-size">字体大小（px）</Label>
              <Input
                id="watermark-size"
                type="number"
                inputMode="numeric"
                min={8}
                max={200}
                value={form.watermarkSize}
                disabled={!form.watermarkEnabled || isLoading}
                onChange={(e) => setField('watermarkSize', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="watermark-color">颜色（十六进制，不含 #）</Label>
              <div className="flex items-center gap-2">
                <Input
                  id="watermark-color"
                  placeholder="ffffff"
                  value={form.watermarkColor}
                  disabled={!form.watermarkEnabled || isLoading}
                  onChange={(e) => setField('watermarkColor', e.target.value.replace(/[^0-9a-fA-F]/g, ''))}
                  className="font-mono"
                  maxLength={6}
                />
                <div
                  className="size-9 shrink-0 rounded-lg border border-border"
                  style={{ backgroundColor: `#${form.watermarkColor || 'ffffff'}` }}
                />
              </div>
              <p className="text-xs text-muted-foreground">如 ffffff（白）、000000（黑）、ff6600（橙）</p>
            </div>

            <div className="space-y-2">
              <Label>水印位置</Label>
              {isLoading ? (
                <Skeleton className="h-9 w-full max-w-xs" />
              ) : (
                <Select
                  value={form.watermarkPosition}
                  onValueChange={(v) => setField('watermarkPosition', v)}
                  disabled={!form.watermarkEnabled}
                >
                  <SelectTrigger className="w-full max-w-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {WATERMARK_POSITIONS.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        {p.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-3xl">
          <CardHeader>
            <div className="flex items-center gap-2">
              <IconCloud className="size-5 text-primary" />
              <CardTitle>Cloudflare R2 存储</CardTitle>
            </div>
            <CardDescription>启用后新上传图片仅存储到 R2，通过 CDN 加速分发，不占用本地磁盘。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <Label htmlFor="r2-enabled">启用 R2 存储</Label>
              {isLoading ? (
                <Skeleton className="h-6 w-11" />
              ) : (
                <Switch
                  id="r2-enabled"
                  checked={form.r2Enabled}
                  onCheckedChange={(v) => setField('r2Enabled', v)}
                />
              )}
            </div>

            <Separator />

            <div className="space-y-2">
              <Label htmlFor="r2-endpoint">S3 Endpoint</Label>
              <Input
                id="r2-endpoint"
                placeholder="https://xxx.r2.cloudflarestorage.com"
                value={form.r2Endpoint}
                disabled={!form.r2Enabled || isLoading}
                onChange={(e) => setField('r2Endpoint', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="r2-bucket">Bucket 名称</Label>
              <Input
                id="r2-bucket"
                placeholder="imgcloud"
                value={form.r2Bucket}
                disabled={!form.r2Enabled || isLoading}
                onChange={(e) => setField('r2Bucket', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="r2-public">公开访问域名</Label>
              <Input
                id="r2-public"
                placeholder="https://r2.kserks.org"
                value={form.r2PublicURL}
                disabled={!form.r2Enabled || isLoading}
                onChange={(e) => setField('r2PublicURL', e.target.value)}
              />
              <p className="text-xs text-muted-foreground">绑定 R2 Bucket 的自定义域名（需在 Cloudflare 后台配置）</p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="r2-access-key">Access Key ID</Label>
              <Input
                id="r2-access-key"
                placeholder="R2 API Token Access Key ID"
                value={form.r2AccessKey}
                disabled={!form.r2Enabled || isLoading}
                onChange={(e) => setField('r2AccessKey', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="r2-secret-key" className="flex items-center gap-1.5">
                <IconKey className="size-3.5" />
                Secret Access Key
              </Label>
              <Input
                id="r2-secret-key"
                type="password"
                placeholder="Secret Key"
                value={form.r2SecretKey}
                disabled={!form.r2Enabled || isLoading}
                onChange={(e) => setField('r2SecretKey', e.target.value)}
              />
            </div>

            <p className="text-xs text-muted-foreground">
              启用后新上传图片仅存储在 R2。配置前上传的图片保留在本地，可点击"迁移到 R2"同步。
            </p>

            <Button
              variant="outline"
              size="sm"
              className="w-full"
              disabled={!form.r2Enabled || !r2FormComplete || testR2.isPending}
              onClick={handleTestR2}
            >
              <IconPlugConnected className="size-4" />
              {testR2.isPending ? '测试中...' : '测试连接'}
            </Button>
          </CardContent>
        </Card>
      </div>

      <div className="sticky bottom-0 z-10 -mx-6 flex items-center justify-between gap-4 border-t border-border/60 bg-background/80 px-6 py-4 backdrop-blur-md">
        <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
          <IconDeviceFloppy className="size-4" />
          修改将影响全站
        </span>
        <div className="flex gap-2">
          <Button
            variant="outline"
            onClick={handleCancel}
            disabled={!hasChanges || updateConfigs.isPending}
          >
            取消
          </Button>
          <Button onClick={handleSave} disabled={!hasChanges || updateConfigs.isPending}>
            保存
          </Button>
        </div>
      </div>
    </div>
  )
}
