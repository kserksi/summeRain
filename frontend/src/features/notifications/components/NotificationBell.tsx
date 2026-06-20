import {
  IconBell,
  IconBellOff,
  IconCheck,
  IconTrash,
} from '@tabler/icons-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import type { Notification } from '@/lib/types'

import {
  useClearNotifications,
  useDeleteNotification,
  useMarkAllRead,
  useMarkRead,
  useNotifications,
} from '../hooks'

function timeAgo(iso: string): string {
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const diff = Math.max(0, Date.now() - then)
  const sec = Math.floor(diff / 1000)
  if (sec < 60) return '刚刚'
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}分钟前`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr}小时前`
  return `${Math.floor(hr / 24)}天前`
}

type NotificationRowProps = {
  notification: Notification
  onMarkRead: (id: number) => void
  onDelete: (id: number) => void
  markReadPending: boolean
  deletePending: boolean
}

function NotificationRow({
  notification,
  onMarkRead,
  onDelete,
  markReadPending,
  deletePending,
}: NotificationRowProps) {
  return (
    <DropdownMenuItem
      onSelect={(e) => e.preventDefault()}
      className="flex-col items-start gap-1 px-3 py-2.5"
    >
      <div className="flex w-full items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <p
            className={cn(
              'truncate text-sm',
              notification.is_read ? 'font-normal' : 'font-semibold',
            )}
          >
            {notification.title}
          </p>
          {notification.message ? (
            <p className="line-clamp-2 text-xs text-muted-foreground">
              {notification.message}
            </p>
          ) : null}
          <p className="mt-0.5 text-xs text-muted-foreground">
            {timeAgo(notification.created_at)}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-0.5">
          {!notification.is_read ? (
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="标记为已读"
              disabled={markReadPending}
              onClick={(e) => {
                e.stopPropagation()
                onMarkRead(notification.id)
              }}
            >
              <IconCheck className="size-4" />
            </Button>
          ) : null}
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="删除通知"
            disabled={deletePending}
            onClick={(e) => {
              e.stopPropagation()
              onDelete(notification.id)
            }}
          >
            <IconTrash className="size-4" />
          </Button>
        </div>
      </div>
    </DropdownMenuItem>
  )
}

export function NotificationBell() {
  const { data, isLoading } = useNotifications()
  const markReadMut = useMarkRead()
  const markAllReadMut = useMarkAllRead()
  const deleteMut = useDeleteNotification()
  const clearMut = useClearNotifications()

  const notifications = Array.isArray(data) ? data : []
  const unreadCount = notifications.filter((n) => !n.is_read).length
  const hasUnread = unreadCount > 0

  const handleMarkRead = (id: number) => {
    markReadMut.mutate(id, {
      onError: () => toast.error('标记已读失败'),
    })
  }

  const handleMarkAllRead = () => {
    markAllReadMut.mutate(undefined, {
      onSuccess: () => toast.success('已全部标记为已读'),
      onError: () => toast.error('操作失败'),
    })
  }

  const handleDelete = (id: number) => {
    deleteMut.mutate(id, {
      onError: () => toast.error('删除失败'),
    })
  }

  const handleClearAll = () => {
    if (notifications.length === 0) return
    if (!window.confirm('确定要清空全部通知吗？')) return
    clearMut.mutate(undefined, {
      onSuccess: () => toast.success('已清空全部通知'),
      onError: () => toast.error('清空失败'),
    })
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="通知"
          className="relative grid size-11 place-items-center rounded-xl border border-border bg-card text-foreground transition hover:border-primary hover:text-primary"
        >
          <IconBell className="size-5" />
          {hasUnread ? (
            <span className="absolute right-2 top-2 size-2 rounded-full bg-destructive ring-2 ring-card" />
          ) : null}
        </button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="end" className="w-80 p-0">
        <div className="flex items-center justify-between px-3 py-2">
          <span className="text-sm font-medium">通知</span>
          <Button
            variant="ghost"
            size="xs"
            disabled={!hasUnread || markAllReadMut.isPending}
            onClick={handleMarkAllRead}
          >
            <IconCheck className="size-3.5" />
            全部已读
          </Button>
        </div>
        <DropdownMenuSeparator className="my-0" />

        {isLoading ? (
          <div className="flex flex-col gap-2 p-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="flex flex-col gap-1.5">
                <Skeleton className="h-3.5 w-2/3" />
                <Skeleton className="h-3 w-full" />
                <Skeleton className="h-3 w-1/3" />
              </div>
            ))}
          </div>
        ) : notifications.length === 0 ? (
          <div className="flex flex-col items-center gap-2 px-3 py-10 text-muted-foreground">
            <IconBellOff className="size-6" />
            <span className="text-sm">暂无通知</span>
          </div>
        ) : (
          <ScrollArea className="h-80">
            <div className="flex flex-col">
              {notifications.map((n) => (
                <NotificationRow
                  key={n.id}
                  notification={n}
                  onMarkRead={handleMarkRead}
                  onDelete={handleDelete}
                  markReadPending={markReadMut.isPending}
                  deletePending={deleteMut.isPending}
                />
              ))}
            </div>
          </ScrollArea>
        )}

        <DropdownMenuSeparator className="my-0" />
        <div className="p-2">
          <Button
            variant="ghost"
            size="sm"
            className="w-full text-destructive hover:bg-destructive/10 hover:text-destructive"
            disabled={notifications.length === 0 || clearMut.isPending}
            onClick={handleClearAll}
          >
            <IconTrash className="size-4" />
            清空全部
          </Button>
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
