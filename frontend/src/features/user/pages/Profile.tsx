import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { toast } from 'sonner'
import {
  IconUser,
  IconMail,
  IconCalendar,
  IconShieldCheck,
  IconLock,
  IconDeviceFloppy,
} from '@tabler/icons-react'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { USER_ROLES, USER_STATUS } from '@/config/constants'
import { ApiError } from '@/lib/errors'
import type { UserProfile } from '@/lib/types'
import { useProfile, useChangePassword } from '../hooks'

const passwordSchema = z
  .object({
    old_password: z.string().min(1, '请输入当前密码'),
    new_password: z.string().min(8, '密码至少需要 8 个字符'),
    confirm_password: z.string().min(1, '请再次输入新密码'),
  })
  .refine((data) => data.new_password === data.confirm_password, {
    message: '两次输入的密码不一致',
    path: ['confirm_password'],
  })

type PasswordValues = z.infer<typeof passwordSchema>

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 2)} ${units[i]}`
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
}

function RoleBadge({ role }: { role: UserProfile['role'] }) {
  if (role === USER_ROLES.ADMIN) {
    return (
      <Badge className="bg-blue-500/10 text-blue-600 dark:bg-blue-500/15 dark:text-blue-400">
        管理员
      </Badge>
    )
  }
  return <Badge variant="default">普通用户</Badge>
}

function StatusBadge({ status }: { status: UserProfile['status'] }) {
  if (status === USER_STATUS.SUSPENDED) {
    return <Badge variant="destructive">已封禁</Badge>
  }
  if (status === USER_STATUS.PENDING) {
    return <Badge variant="outline">待激活</Badge>
  }
  return (
    <Badge className="bg-emerald-500/10 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-400">
      正常
    </Badge>
  )
}

function InfoRow({
  icon,
  label,
  children,
}: {
  icon: React.ReactNode
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-3">
      <div className="flex items-center gap-2.5 text-muted-foreground">
        <span className="text-foreground/70">{icon}</span>
        <span className="text-sm">{label}</span>
      </div>
      <div className="text-sm font-medium text-foreground">{children}</div>
    </div>
  )
}

function AccountCard() {
  const { data: profile, isLoading, isError } = useProfile()

  return (
    <Card className="rounded-3xl">
      <CardHeader>
        <CardTitle className="text-lg">账户信息</CardTitle>
        <CardDescription>查看你的账户与存储使用情况</CardDescription>
      </CardHeader>
      <CardContent className="space-y-1">
        {isLoading ? (
          <div className="space-y-4 py-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="flex items-center justify-between">
                <Skeleton className="h-4 w-24" />
                <Skeleton className="h-4 w-40" />
              </div>
            ))}
          </div>
        ) : isError || !profile ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            加载账户信息失败，请稍后重试
          </p>
        ) : (
          <>
            <InfoRow icon={<IconUser className="size-4" />} label="用户名">
              {profile.username}
            </InfoRow>
            <Separator />
            <InfoRow icon={<IconMail className="size-4" />} label="邮箱">
              {profile.email}
            </InfoRow>
            <Separator />
            <InfoRow icon={<IconShieldCheck className="size-4" />} label="角色">
              <RoleBadge role={profile.role} />
            </InfoRow>
            <Separator />
            <InfoRow icon={<IconShieldCheck className="size-4" />} label="状态">
              <StatusBadge status={profile.status} />
            </InfoRow>
            <Separator />
            <InfoRow icon={<IconCalendar className="size-4" />} label="注册时间">
              {formatDate(profile.created_at)}
            </InfoRow>
            <Separator />
            <div className="space-y-2 py-3">
              <div className="flex items-center justify-between text-sm">
                <span className="flex items-center gap-2.5 text-muted-foreground">
                  <IconDeviceFloppy className="size-4 text-foreground/70" />
                  存储用量
                </span>
                <span className="font-medium">
                  {formatBytes(profile.storage_used)} / {formatBytes(profile.storage_quota)}
                </span>
              </div>
              <Progress value={profile.storage_percent} />
              <p className="text-right text-xs text-muted-foreground">
                已使用 {profile.storage_percent.toFixed(1)}%
              </p>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function PasswordCard() {
  const { mutateAsync, isPending } = useChangePassword()
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<PasswordValues>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { old_password: '', new_password: '', confirm_password: '' },
  })

  const onSubmit = async (values: PasswordValues) => {
    try {
      await mutateAsync({
        old_password: values.old_password,
        new_password: values.new_password,
      })
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : '修改密码失败，请稍后重试')
      reset({ ...values, old_password: '' })
    }
  }

  return (
    <Card className="rounded-3xl">
      <CardHeader>
        <CardTitle className="text-lg">修改密码</CardTitle>
        <CardDescription>
          修改成功后将自动退出登录，请使用新密码重新登录
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="old_password">当前密码</Label>
            <div className="relative">
              <IconLock className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id="old_password"
                type="password"
                autoComplete="current-password"
                className="pl-9"
                {...register('old_password')}
              />
            </div>
            {errors.old_password && (
              <p className="text-xs text-destructive">{errors.old_password.message}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="new_password">新密码</Label>
            <div className="relative">
              <IconLock className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id="new_password"
                type="password"
                autoComplete="new-password"
                className="pl-9"
                {...register('new_password')}
              />
            </div>
            {errors.new_password ? (
              <p className="text-xs text-destructive">{errors.new_password.message}</p>
            ) : (
              <p className="text-xs text-muted-foreground">至少 8 个字符</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="confirm_password">确认新密码</Label>
            <div className="relative">
              <IconLock className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id="confirm_password"
                type="password"
                autoComplete="new-password"
                className="pl-9"
                {...register('confirm_password')}
              />
            </div>
            {errors.confirm_password && (
              <p className="text-xs text-destructive">{errors.confirm_password.message}</p>
            )}
          </div>

          <Button type="submit" className="w-full" disabled={isPending}>
            <IconDeviceFloppy className="size-4" />
            {isPending ? '保存中…' : '保存新密码'}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}

export default function ProfilePage() {
  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="font-heading text-2xl font-bold">个人资料</h1>
        <p className="mt-1 text-sm text-muted-foreground">管理你的账户信息与安全设置</p>
      </div>
      <AccountCard />
      <PasswordCard />
    </div>
  )
}
