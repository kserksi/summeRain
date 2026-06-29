// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { useMemo } from 'react'
import { Link } from 'react-router'
import {
  IconPhoto,
  IconEye,
  IconDeviceFloppy,
  IconWorld,
  IconLock,
  IconUsers,
  IconChartPie,
  IconShieldCheck,
  IconArrowRight,
  IconUpload,
  IconCloudUpload,
} from '@tabler/icons-react'

import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { useProfile } from '@/features/user/hooks'
import { useImages } from '@/features/images/hooks'
import { useAdminStats } from '@/features/admin/hooks'
import { useAuthStore } from '@/store/auth-store'
import { ROUTES, USER_ROLES } from '@/config/constants'
import type { Image, SystemStats } from '@/lib/types'

type IconType = React.ComponentType<{ className?: string }>

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function StatCard({
  icon: Icon,
  value,
  label,
  loading,
}: {
  icon: IconType
  value: string
  label: string
  loading?: boolean
}) {
  return (
    <Card className="rounded-3xl">
      <CardHeader>
        <div className="flex size-10 items-center justify-center rounded-2xl bg-primary/10 text-primary">
          <Icon className="size-5" />
        </div>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-7 w-20" />
        ) : (
          <div className="text-2xl font-bold tabular-nums">{value}</div>
        )}
        <div className="mt-0.5 text-sm text-muted-foreground">{label}</div>
      </CardContent>
    </Card>
  )
}

const ADMIN_STATS: {
  key: keyof SystemStats
  label: string
  icon: IconType
  format: (v: number) => string
}[] = [
  { key: 'total_users', label: '总用户', icon: IconUsers, format: (v) => v.toLocaleString() },
  { key: 'total_images', label: '图片总数', icon: IconPhoto, format: (v) => v.toLocaleString() },
  { key: 'storage_used', label: '存储用量', icon: IconDeviceFloppy, format: formatBytes },
  { key: 'active_users', label: '活跃用户', icon: IconChartPie, format: (v) => v.toLocaleString() },
  {
    key: 'total_sessions',
    label: '会话总数',
    icon: IconShieldCheck,
    format: (v) => v.toLocaleString(),
  },
]

function AdminSection() {
  const { data, isLoading } = useAdminStats()

  return (
    <Card className="rounded-3xl border-primary/30 bg-primary/5">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          <IconShieldCheck className="size-4 text-primary" />
          系统管理
        </CardTitle>
        <CardDescription>系统运行数据一览</CardDescription>
        <CardAction>
          <Button asChild variant="outline" size="sm">
            <Link to={ROUTES.ADMIN}>
              进入后台
              <IconArrowRight />
            </Link>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
          {ADMIN_STATS.map((stat) => {
            const Icon = stat.icon
            const value = data ? stat.format(data[stat.key]) : null
            return (
              <div key={stat.key} className="flex flex-col gap-2">
                <div className="flex size-9 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                  <Icon className="size-4" />
                </div>
                {isLoading || value === null ? (
                  <Skeleton className="h-6 w-14" />
                ) : (
                  <div className="text-xl font-bold tabular-nums">{value}</div>
                )}
                <div className="text-xs text-muted-foreground">{stat.label}</div>
              </div>
            )
          })}
        </div>
      </CardContent>
    </Card>
  )
}

function RecentImage({ image }: { image: Image }) {
  const isPrivate = image.visibility === 'private'
  return (
    <Link
      to={`/images/${image.id}`}
      className="group relative block overflow-hidden rounded-3xl bg-card ring-1 ring-border transition-all hover:-translate-y-1 hover:shadow-xl"
    >
      <div className="aspect-square w-full overflow-hidden bg-muted">
        <img
          src={`/i/${image.unique_link}.webp?w=400`}
          alt={image.title || image.filename}
          loading="lazy"
          className="size-full object-cover transition-transform duration-300 group-hover:scale-105"
        />
      </div>
      <div className="absolute top-2 left-2">
        <Badge variant={isPrivate ? 'secondary' : 'default'} className="backdrop-blur">
          {isPrivate ? <IconLock /> : <IconWorld />}
          {isPrivate ? '私密' : '公开'}
        </Badge>
      </div>
      <div className="p-2.5">
        <p className="truncate text-xs font-medium">{image.filename}</p>
      </div>
    </Link>
  )
}

function EmptyImages() {
  return (
    <Card className="rounded-3xl">
      <CardContent className="flex flex-col items-center justify-center gap-3 py-12 text-center">
        <div className="flex size-14 items-center justify-center rounded-3xl bg-muted text-muted-foreground">
          <IconCloudUpload className="size-7" />
        </div>
        <div className="space-y-1">
          <p className="font-medium">还没有上传过图片</p>
          <p className="text-sm text-muted-foreground">上传你的第一张图片，开始使用图床服务</p>
        </div>
        <Button asChild>
          <Link to={ROUTES.UPLOAD}>
            <IconUpload />
            立即上传
          </Link>
        </Button>
      </CardContent>
    </Card>
  )
}

export default function DashboardPage() {
  const user = useAuthStore((s) => s.user)
  const { data: profile, isLoading: profileLoading } = useProfile()
  const { data: imagesData, isLoading: imagesLoading } = useImages({ limit: 6 })

  const recentImages = useMemo<Image[]>(
    () => imagesData?.pages?.[0]?.images ?? [],
    [imagesData],
  )
  const totalViews = useMemo(
    () => recentImages.reduce((sum, img) => sum + img.view_count, 0),
    [recentImages],
  )
  const publicCount = useMemo(
    () => recentImages.filter((img) => img.visibility === 'public').length,
    [recentImages],
  )

  const hasImages = recentImages.length > 0

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="font-heading text-2xl font-semibold">
            {profile ? `欢迎回来，${profile.username}` : '仪表盘'}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">查看你的图片与存储概况</p>
        </div>
        <Avatar size="lg">
          {profile?.avatar_url ? (
            <AvatarImage src={profile.avatar_url} alt={profile.username} />
          ) : null}
          <AvatarFallback>{(profile?.username ?? '?').slice(0, 1).toUpperCase()}</AvatarFallback>
        </Avatar>
      </div>

      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <StatCard
          icon={IconPhoto}
          label="我的图片"
          value={(profile?.image_count ?? 0).toLocaleString()}
          loading={profileLoading}
        />
        <StatCard
          icon={IconEye}
          label="总浏览量"
          value={totalViews.toLocaleString()}
          loading={imagesLoading}
        />
        <StatCard
          icon={IconDeviceFloppy}
          label="已用存储"
          value={formatBytes(profile?.storage_used ?? 0)}
          loading={profileLoading}
        />
        <StatCard
          icon={IconWorld}
          label="公开图片"
          value={publicCount.toLocaleString()}
          loading={imagesLoading}
        />
      </div>

      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconDeviceFloppy className="size-4 text-primary" />
            存储用量
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {profileLoading || !profile ? (
            <>
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-4 w-44" />
            </>
          ) : (
            <>
              <Progress value={profile.storage_percent} />
              <div className="flex items-center justify-between text-sm text-muted-foreground">
                <span className="font-medium text-foreground">
                  {profile.storage_percent.toFixed(1)}%
                </span>
                <span>
                  剩余 {formatBytes(Math.max(profile.storage_quota - profile.storage_used, 0))} /{' '}
                  {formatBytes(profile.storage_quota)}
                </span>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <section className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="flex items-center gap-2 font-heading text-lg font-semibold">
            <IconPhoto className="size-5 text-primary" />
            最近上传
          </h2>
          {hasImages && (
            <Button asChild variant="ghost" size="sm">
              <Link to={ROUTES.IMAGES}>
                查看全部
                <IconArrowRight />
              </Link>
            </Button>
          )}
        </div>

        {imagesLoading ? (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-6">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="aspect-square rounded-3xl" />
            ))}
          </div>
        ) : hasImages ? (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-6">
            {recentImages.slice(0, 6).map((img) => (
              <RecentImage key={img.id} image={img} />
            ))}
          </div>
        ) : (
          <EmptyImages />
        )}
      </section>

      {user?.role === USER_ROLES.ADMIN && <AdminSection />}
    </div>
  )
}
