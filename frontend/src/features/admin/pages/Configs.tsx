// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
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
  SITE_LANGUAGE: 'site_language',
} as const

const PROVIDER_LABELS: Record<string, string> = {
  recaptcha: 'reCAPTCHA v3',
  turnstile: 'Cloudflare Turnstile',
  geetest_v4: 'GeeTest v4',
}

const SCRIPT_HINT_KEYS: Record<string, string> = {
  recaptcha: 'admin.configs.scriptHintRecaptcha',
  turnstile: 'admin.configs.scriptHintTurnstile',
  geetest_v4: 'admin.configs.scriptHintGeetest',
}

const WATERMARK_POSITIONS = [
  { value: 'ce', labelKey: 'admin.configs.posCenter' },
  { value: 'soea', labelKey: 'admin.configs.posBottomRight' },
  { value: 'sowe', labelKey: 'admin.configs.posBottomLeft' },
  { value: 'noea', labelKey: 'admin.configs.posTopRight' },
  { value: 'nowe', labelKey: 'admin.configs.posTopLeft' },
] as const

type FormState = {
  siteLanguage: string
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
  siteLanguage: 'zh-CN',
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
    siteLanguage: map.get(CFG.SITE_LANGUAGE) ?? DEFAULTS.siteLanguage,
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
  siteLanguage: CFG.SITE_LANGUAGE,
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
  const { t } = useTranslation()
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

  const scriptHintKey = SCRIPT_HINT_KEYS[form.provider]

  return (
    <div className="flex min-h-[calc(100vh-8rem)] flex-col gap-6">
      <div>
        <h1 className="font-heading text-2xl font-semibold">{t('admin.configs.title')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('admin.configs.subtitle')}</p>
      </div>

      <div className="flex-1 space-y-6">
        <Card className="rounded-3xl">
          <CardHeader>
            <div className="flex items-center gap-2">
              <CardTitle>{t('admin.configs.siteLanguageTitle')}</CardTitle>
            </div>
            <CardDescription>{t('admin.configs.siteLanguageDesc')}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              <Label>{t('admin.configs.siteLanguageLabel')}</Label>
              {isLoading ? (
                <Skeleton className="h-9 w-full max-w-xs" />
              ) : (
                <Select value={form.siteLanguage} onValueChange={(v) => setField('siteLanguage', v)}>
                  <SelectTrigger className="w-full max-w-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="zh-CN">中文</SelectItem>
                    <SelectItem value="ja-JP">日本語</SelectItem>
                  </SelectContent>
                </Select>
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-3xl">
          <CardHeader>
            <div className="flex items-center gap-2">
              <IconShieldCheck className="size-5 text-primary" />
              <CardTitle>{t('admin.configs.captchaTitle')}</CardTitle>
            </div>
            <CardDescription>{t('admin.configs.captchaDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>{t('admin.configs.providerLabel')}</Label>
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
                        {p === 'none' ? t('admin.configs.providerNone') : (PROVIDER_LABELS[p] ?? p)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              {scriptHintKey && (
                <p className="text-xs text-muted-foreground">{t(scriptHintKey)}</p>
              )}
            </div>

            <Separator />

            <div className="space-y-2">
              <Label htmlFor="site-key">Site Key</Label>
              <Input
                id="site-key"
                placeholder={t('admin.configs.siteKeyPlaceholder')}
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
                placeholder={t('admin.configs.secretPlaceholder')}
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
              <CardTitle>{t('admin.configs.tokenTtlTitle')}</CardTitle>
            </div>
            <CardDescription>{t('admin.configs.tokenTtlDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            <Label htmlFor="ttl">{t('admin.configs.defaultTtlLabel')}</Label>
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
              {t('admin.configs.ttlRange', {
                min: IMAGE_TOKEN.MIN_TTL_MS.toLocaleString(),
                max: IMAGE_TOKEN.MAX_TTL_MS.toLocaleString(),
                default: IMAGE_TOKEN.DEFAULT_TTL_MS.toLocaleString(),
              })}
            </p>
          </CardContent>
        </Card>

        <Card className="rounded-3xl">
          <CardHeader>
            <div className="flex items-center gap-2">
              <IconDroplet className="size-5 text-primary" />
              <CardTitle>{t('admin.configs.watermarkTitle')}</CardTitle>
            </div>
            <CardDescription>{t('admin.configs.watermarkDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <Label htmlFor="watermark-enabled">{t('admin.configs.enableWatermark')}</Label>
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
              <Label htmlFor="watermark-text">{t('admin.configs.watermarkText')}</Label>
              <Input
                id="watermark-text"
                placeholder={t('admin.configs.watermarkTextPlaceholder')}
                value={form.watermarkText}
                disabled={!form.watermarkEnabled || isLoading}
                onChange={(e) => setField('watermarkText', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="watermark-opacity">{t('admin.configs.opacityLabel')}</Label>
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
              <p className="text-xs text-muted-foreground">{t('admin.configs.opacityHint')}</p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="watermark-size">{t('admin.configs.sizeLabel')}</Label>
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
              <Label htmlFor="watermark-color">{t('admin.configs.colorLabel')}</Label>
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
              <p className="text-xs text-muted-foreground">{t('admin.configs.colorHint')}</p>
            </div>

            <div className="space-y-2">
              <Label>{t('admin.configs.positionLabel')}</Label>
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
                        {t(p.labelKey)}
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
              <CardTitle>{t('admin.configs.r2Title')}</CardTitle>
            </div>
            <CardDescription>{t('admin.configs.r2Desc')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <Label htmlFor="r2-enabled">{t('admin.configs.enableR2')}</Label>
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
              <Label htmlFor="r2-bucket">{t('admin.configs.bucketLabel')}</Label>
              <Input
                id="r2-bucket"
                placeholder="imgcloud"
                value={form.r2Bucket}
                disabled={!form.r2Enabled || isLoading}
                onChange={(e) => setField('r2Bucket', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="r2-public">{t('admin.configs.publicUrlLabel')}</Label>
              <Input
                id="r2-public"
                placeholder="https://r2.example.com"
                value={form.r2PublicURL}
                disabled={!form.r2Enabled || isLoading}
                onChange={(e) => setField('r2PublicURL', e.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t('admin.configs.publicUrlHint')}</p>
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
              {t('admin.configs.r2MigrateHint')}
            </p>

            <Button
              variant="outline"
              size="sm"
              className="w-full"
              disabled={!form.r2Enabled || !r2FormComplete || testR2.isPending}
              onClick={handleTestR2}
            >
              <IconPlugConnected className="size-4" />
              {testR2.isPending ? t('admin.configs.testing') : t('admin.configs.testConnection')}
            </Button>
          </CardContent>
        </Card>
      </div>

      <div className="sticky bottom-0 z-10 -mx-6 flex items-center justify-between gap-4 border-t border-border/60 bg-background/80 px-6 py-4 backdrop-blur-md">
        <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
          <IconDeviceFloppy className="size-4" />
          {t('admin.configs.affectsAll')}
        </span>
        <div className="flex gap-2">
          <Button
            variant="outline"
            onClick={handleCancel}
            disabled={!hasChanges || updateConfigs.isPending}
          >
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={!hasChanges || updateConfigs.isPending}>
            {t('common.save')}
          </Button>
        </div>
      </div>
    </div>
  )
}
