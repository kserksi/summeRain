// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from 'react'
import { IconAlertTriangle, IconPackageImport } from '@tabler/icons-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/store/auth-store'

const MAX_DOWNLOADS = 10

function computeRemaining(iso: string | null) {
  if (!iso) return { hours: 0, minutes: 0 }
  const diff = new Date(iso).getTime() - Date.now()
  if (diff <= 0) return { hours: 0, minutes: 0 }
  const totalMinutes = Math.floor(diff / 60000)
  return { hours: Math.floor(totalMinutes / 60), minutes: totalMinutes % 60 }
}

export function PendingDeletionBanner() {
  const user = useAuthStore((s) => s.user)
  const scheduledAt = user?.deletion_scheduled_at ?? null
  const [, setTick] = useState(0)

  useEffect(() => {
    if (user?.status !== 'pending_deletion') return
    const timer = setInterval(() => setTick((t) => t + 1), 60000)
    return () => clearInterval(timer)
  }, [user?.status])

  if (user?.status !== 'pending_deletion') return null

  const { hours, minutes } = computeRemaining(scheduledAt)
  const used = user.batch_download_count ?? 0
  const downloadDisabled = used >= MAX_DOWNLOADS

  return (
    <div className="sticky top-[64px] z-40 flex flex-wrap items-center gap-x-4 gap-y-2 border-b-2 border-destructive/30 bg-destructive/10 px-6 py-3">
      <IconAlertTriangle className="size-5 shrink-0 text-destructive" />
      <div className="min-w-0 flex-1">
        <p className="font-semibold text-destructive">账号即将删除</p>
        <p className="text-sm text-muted-foreground">
          您的账号将在 {hours} 小时 {minutes} 分钟后永久删除，请尽快下载您需要的数据。
        </p>
      </div>
      <Badge variant="destructive">
        剩余下载 {MAX_DOWNLOADS - used}/{MAX_DOWNLOADS}
      </Badge>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={downloadDisabled}
        onClick={() => window.open('/api/v1/images/batch-download', '_blank')}
      >
        <IconPackageImport className="size-4" />
        打包下载
      </Button>
    </div>
  )
}
