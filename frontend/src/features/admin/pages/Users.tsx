import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import {
  IconSearch,
  IconBan,
  IconCheck,
  IconChevronLeft,
  IconChevronRight,
  IconTrash,
  IconDatabaseEdit,
  IconArrowBackUp,
  IconAlertTriangle,
  IconClockCancel,
} from '@tabler/icons-react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
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
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { PAGINATION, QUERY_KEYS, USER_STATUS, USER_ROLES } from '@/config/constants'
import { api } from '@/lib/api'
import { ApiError } from '@/lib/errors'
import { useAuthStore } from '@/store/auth-store'
import { toast } from 'sonner'
import type { User } from '@/lib/types'
import { useAdminUsers, useSetUserStatus } from '../hooks'

const PAGE_SIZE = PAGINATION.DEFAULT_PAGE_SIZE
const PENDING_DELETION = 'pending_deletion'

type AdminUser = User & { deletion_scheduled_at?: string | null }

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function getRemainingHours(deletionScheduledAt?: string | null): number | null {
  if (!deletionScheduledAt) return null
  const diffMs = new Date(deletionScheduledAt).getTime() - Date.now()
  if (diffMs <= 0) return 0
  return Math.ceil(diffMs / (1000 * 60 * 60))
}

function StatusBadge({ user }: { user: AdminUser }) {
  if (user.status === USER_STATUS.ACTIVE) {
    return (
      <Badge className="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">正常</Badge>
    )
  }
  if (user.status === PENDING_DELETION) {
    const hours = getRemainingHours(user.deletion_scheduled_at)
    return (
      <Badge className="gap-1 bg-amber-500/10 text-amber-600 dark:text-amber-400">
        <IconClockCancel className="size-3.5" />
        注销中
        {hours !== null && hours > 0 ? `（剩 ${hours}h）` : ''}
      </Badge>
    )
  }
  return <Badge variant="destructive">封禁</Badge>
}

function BanUnbanActions({ user }: { user: AdminUser }) {
  const setStatus = useSetUserStatus()
  const isSuspended = user.status === USER_STATUS.SUSPENDED

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant={isSuspended ? 'outline' : 'destructive'}>
          {isSuspended ? <IconCheck className="size-4" /> : <IconBan className="size-4" />}
          {isSuspended ? '解封' : '封禁'}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {isSuspended ? '确认解封该用户？' : '确认封禁该用户？'}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {isSuspended
              ? '解封后该用户将恢复正常登录与使用。'
              : '封禁后该用户将无法登录，已上传的图片不受影响。'}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction
            variant={isSuspended ? 'default' : 'destructive'}
            disabled={setStatus.isPending}
            onClick={() =>
              setStatus.mutate({
                id: user.id,
                status: isSuspended ? USER_STATUS.ACTIVE : USER_STATUS.SUSPENDED,
              })
            }
          >
            确认
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function QuotaEditDialog({ user }: { user: AdminUser }) {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [mb, setMb] = useState<string>(() => String(Math.round((user.storage_quota ?? 0) / 1024 / 1024)))
  const [submitting, setSubmitting] = useState(false)

  const numMb = Number(mb)
  const tooLow = mb !== '' && Number.isFinite(numMb) && numMb < 500

  const presets = [
    { label: '500 MB', value: 500 },
    { label: '1 GB', value: 1024 },
    { label: '5 GB', value: 5120 },
    { label: '10 GB', value: 10240 },
  ]

  async function handleSave() {
    if (!mb || !Number.isFinite(numMb) || numMb < 500) return
    setSubmitting(true)
    try {
      await api.patch(`/admin/users/${user.id}/quota`, { storage_quota: numMb * 1024 * 1024 })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminUsers })
      toast.success('配额已更新')
      setOpen(false)
    } catch {
      toast.error('更新配额失败，请重试')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="ghost">
          <IconDatabaseEdit className="size-4" />
          配额
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>调整存储配额</DialogTitle>
          <DialogDescription>
            {`当前配额：${formatBytes(user.storage_quota)} · 已用：${formatBytes(user.storage_used)}`}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-2">
            <Label htmlFor={`quota-mb-${user.id}`}>新配额（MB）</Label>
            <Input
              id={`quota-mb-${user.id}`}
              type="number"
              min={500}
              value={mb}
              onChange={(e) => setMb(e.target.value)}
              aria-invalid={tooLow}
            />
            {tooLow && (
              <p className="text-xs text-destructive">最小配额为 500 MB</p>
            )}
          </div>
          <div className="flex flex-wrap gap-2">
            {presets.map((p) => (
              <Button
                key={p.value}
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setMb(String(p.value))}
              >
                {p.label}
              </Button>
            ))}
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">取消</Button>
          </DialogClose>
          <Button
            variant="default"
            disabled={submitting || !mb || tooLow || !Number.isFinite(numMb)}
            onClick={handleSave}
          >
            {submitting ? '保存中…' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function DeleteDialog({ user }: { user: AdminUser }) {
  const qc = useQueryClient()
  const adminUsername = useAuthStore((s) => s.user?.username) ?? ''
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const matched = typed.trim() === user.username

  async function handleConfirm() {
    if (!matched) return
    setSubmitting(true)
    try {
      await api.post(
        `/admin/users/${user.id}/request-deletion?admin=${encodeURIComponent(adminUsername)}`,
        { username: typed.trim() },
      )
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminUsers })
      toast.success('已发起注销')
      setOpen(false)
      setTyped('')
    } catch {
      toast.error('发起注销失败，请重试')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        setOpen(v)
        if (!v) setTyped('')
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm" variant="outline" className="text-destructive hover:text-destructive">
          <IconTrash className="size-4" />
          注销
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <IconAlertTriangle className="size-5" />
            注销用户 {user.username}
          </DialogTitle>
          <DialogDescription>
            此操作将立即锁定该用户，24小时后永久删除所有数据。
          </DialogDescription>
        </DialogHeader>
        <ul className="space-y-1.5 text-sm">
          <li className="flex items-start gap-2">
            <IconBan className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            <span>禁止上传 / 删除 / 转换</span>
          </li>
          <li className="flex items-start gap-2">
            <IconCheck className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            <span>允许打包下载原图（最多 10 次）</span>
          </li>
          <li className="flex items-start gap-2">
            <IconTrash className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            <span>24 小时后永久删除</span>
          </li>
          <li className="flex items-start gap-2">
            <IconArrowBackUp className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            <span>24 小时内可撤销</span>
          </li>
        </ul>
        <Separator />
        <div className="space-y-2">
          <Label htmlFor={`del-confirm-${user.id}`}>请输入用户名确认</Label>
          <Input
            id={`del-confirm-${user.id}`}
            placeholder={user.username}
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
          />
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">取消</Button>
          </DialogClose>
          <Button
            variant="destructive"
            disabled={!matched || submitting}
            onClick={handleConfirm}
          >
            {submitting ? '处理中…' : '确认注销'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function CancelDeletionButton({ user }: { user: AdminUser }) {
  const qc = useQueryClient()
  const [submitting, setSubmitting] = useState(false)

  async function handleCancel() {
    setSubmitting(true)
    try {
      await api.post(`/admin/users/${user.id}/cancel-deletion`)
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminUsers })
      toast.success('已撤销注销')
    } catch {
      toast.error('撤销失败，请重试')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Button
      size="sm"
      variant="outline"
      disabled={submitting}
      onClick={handleCancel}
    >
      <IconArrowBackUp className="size-4" />
      {submitting ? '撤销中…' : '撤销'}
    </Button>
  )
}

function UserActions({ user }: { user: AdminUser }) {
  if (user.role === USER_ROLES.ADMIN) {
    return <span className="text-muted-foreground">—</span>
  }

  return (
    <div className="flex items-center justify-end gap-1.5">
      {user.status === PENDING_DELETION ? (
        <CancelDeletionButton user={user} />
      ) : (
        <BanUnbanActions user={user} />
      )}
      {user.status === USER_STATUS.ACTIVE && <DeleteDialog user={user} />}
      <QuotaEditDialog user={user} />
    </div>
  )
}

export default function Users() {
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const { data, isLoading, error } = useAdminUsers(page)

  const filtered = useMemo(() => {
    const items = data?.items ?? []
    const q = search.trim().toLowerCase()
    if (!q) return items
    return items.filter(
      (u) => u.username.toLowerCase().includes(q) || u.email.toLowerCase().includes(q),
    )
  }, [data, search])

  if (error instanceof ApiError && (error.code === 4030 || error.code === 4032)) {
    useAuthStore.getState().refreshUser()
    navigate('/dashboard', { replace: true })
  }

  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="font-heading text-2xl font-semibold">用户管理</h1>
          <p className="mt-1 text-sm text-muted-foreground">共 {total} 位用户</p>
        </div>
        <div className="relative w-full max-w-xs">
          <IconSearch className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="搜索用户名或邮箱"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>用户</TableHead>
            <TableHead>邮箱</TableHead>
            <TableHead>角色</TableHead>
            <TableHead>状态</TableHead>
            <TableHead className="text-right">图片</TableHead>
            <TableHead className="min-w-[160px]">存储</TableHead>
            <TableHead>注册时间</TableHead>
            <TableHead className="text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading ? (
            Array.from({ length: 6 }).map((_, i) => (
              <TableRow key={i}>
                {Array.from({ length: 8 }).map((__, j) => (
                  <TableCell key={j}>
                    <Skeleton className="h-5 w-full max-w-[120px]" />
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : filtered.length === 0 ? (
            <TableRow>
              <TableCell colSpan={8} className="py-12 text-center text-muted-foreground">
                {search ? '没有匹配的用户' : '暂无用户'}
              </TableCell>
            </TableRow>
          ) : (
            filtered.map((u) => {
              const pct = u.storage_quota
                ? Math.min(100, Math.round((u.storage_used / u.storage_quota) * 100))
                : 0
              return (
                <TableRow key={u.id}>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Avatar size="sm">
                        {u.avatar_url && <AvatarImage src={u.avatar_url} alt={u.username} />}
                        <AvatarFallback>{u.username.charAt(0).toUpperCase()}</AvatarFallback>
                      </Avatar>
                      <span className="font-medium">{u.username}</span>
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{u.email}</TableCell>
                  <TableCell>
                    <Badge variant={u.role === USER_ROLES.ADMIN ? 'default' : 'secondary'}>
                      {u.role === USER_ROLES.ADMIN ? '管理员' : '用户'}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <StatusBadge user={u} />
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{u.image_count}</TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-1">
                      <Progress value={pct} />
                      <span className="text-xs text-muted-foreground tabular-nums">
                        {formatBytes(u.storage_used)}
                        {u.storage_quota > 0 && ` / ${formatBytes(u.storage_quota)}`}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-muted-foreground tabular-nums">
                    {new Date(u.created_at).toLocaleDateString('zh-CN')}
                  </TableCell>
                  <TableCell className="text-right">
                    <UserActions user={u} />
                  </TableCell>
                </TableRow>
              )
            })
          )}
        </TableBody>
      </Table>

      <div className="flex items-center justify-center gap-4">
        <Button
          variant="outline"
          size="icon-sm"
          onClick={() => setPage((p) => Math.max(1, p - 1))}
          disabled={page <= 1 || isLoading}
          aria-label="上一页"
        >
          <IconChevronLeft className="size-4" />
        </Button>
        <span className="text-sm tabular-nums text-muted-foreground">
          {page} / {totalPages}
        </span>
        <Button
          variant="outline"
          size="icon-sm"
          onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
          disabled={page >= totalPages || isLoading}
          aria-label="下一页"
        >
          <IconChevronRight className="size-4" />
        </Button>
      </div>
    </div>
  )
}
