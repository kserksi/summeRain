// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { useEffect } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import {
  IconUsers,
  IconPhoto,
  IconDeviceFloppy,
  IconChartBar,
  IconShieldCheck,
} from '@tabler/icons-react'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '@/lib/errors'
import { useAuthStore } from '@/store/auth-store'
import { useAdminStats } from '../hooks'

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

const STAT_CARDS = [
  { key: 'total_users', labelKey: 'admin.overview.totalUsers', icon: IconUsers, format: (v: number) => v.toLocaleString() },
  { key: 'total_images', labelKey: 'admin.overview.totalImages', icon: IconPhoto, format: (v: number) => v.toLocaleString() },
  { key: 'storage_used', labelKey: 'admin.overview.storageUsed', icon: IconDeviceFloppy, format: formatBytes },
  { key: 'active_users', labelKey: 'admin.overview.activeUsers', icon: IconChartBar, format: (v: number) => v.toLocaleString() },
  { key: 'total_sessions', labelKey: 'admin.overview.totalSessions', icon: IconShieldCheck, format: (v: number) => v.toLocaleString() },
] as const

export default function Overview() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data, isLoading, error } = useAdminStats()

  useEffect(() => {
    if (error instanceof ApiError && (error.code === 4030 || error.code === 4032)) {
      useAuthStore.getState().refreshUser()
      navigate('/dashboard', { replace: true })
    }
  }, [error, navigate])

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-2xl font-semibold">{t('admin.overview.title')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('admin.overview.subtitle')}</p>
      </div>

      <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-5">
        {STAT_CARDS.map((card) => {
          const Icon = card.icon
          const value = data ? card.format(data[card.key]) : null
          return (
            <Card key={card.key} className="rounded-3xl">
              <CardContent className="flex flex-col gap-3">
                <div className="flex size-10 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                  <Icon className="size-5" />
                </div>
                <div>
                  {isLoading || value === null ? (
                    <Skeleton className="h-7 w-16" />
                  ) : (
                    <div className="text-2xl font-bold tabular-nums">{value}</div>
                  )}
                  <div className="mt-0.5 text-sm text-muted-foreground">{t(card.labelKey)}</div>
                </div>
              </CardContent>
            </Card>
          )
        })}
      </div>
    </div>
  )
}
