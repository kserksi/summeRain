import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  IconSearch,
  IconTrash,
  IconEye,
  IconChevronLeft,
  IconChevronRight,
} from '@tabler/icons-react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Lightbox } from '@/features/images/components/Lightbox'
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
import { PAGINATION } from '@/config/constants'
import { api } from '@/lib/api'
import { ApiError } from '@/lib/errors'
import { useAuthStore } from '@/store/auth-store'
import { useAdminImages } from '../hooks'
import type { AdminImage } from '../api'

const PAGE_SIZE = PAGINATION.DEFAULT_PAGE_SIZE

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function DeleteImageButton({ image }: { image: AdminImage }) {
  const qc = useQueryClient()
  const mutation = useMutation({
    mutationFn: () => api.del<void>(`/admin/images/${image.id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin', 'images'] })
      toast.success('图片已删除')
    },
    onError: () => toast.error('删除失败，请重试'),
  })

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant="destructive">
          <IconTrash className="size-4" />
          删除
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>确认删除该图片？</AlertDialogTitle>
          <AlertDialogDescription>
            删除后该图片将无法恢复，相关数据将被清除。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            确认
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

export default function Images() {
  const [page, setPage] = useState(1)
  const [lightboxSrc, setLightboxSrc] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const { data, isLoading, error } = useAdminImages(page)

  const filtered = useMemo(() => {
    const items = data?.items ?? []
    const q = search.trim().toLowerCase()
    if (!q) return items
    return items.filter((img) => img.filename.toLowerCase().includes(q))
  }, [data, search])

  if (error instanceof ApiError && (error.code === 4030 || error.code === 4032)) {
    useAuthStore.getState().refreshUser()
    window.location.assign('/dashboard')
  }

  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="font-heading text-2xl font-semibold">图片管理</h1>
          <p className="mt-1 text-sm text-muted-foreground">共 {total} 张图片</p>
        </div>
        <div className="relative w-full max-w-xs">
          <IconSearch className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="搜索文件名"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>
      </div>

      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[100px]">预览</TableHead>
              <TableHead>文件名</TableHead>
              <TableHead>所有者</TableHead>
              <TableHead>可见性</TableHead>
              <TableHead className="text-right">大小</TableHead>
              <TableHead className="text-right">浏览</TableHead>
              <TableHead>上传时间</TableHead>
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
                  {search ? '没有匹配的图片' : '暂无图片'}
                </TableCell>
              </TableRow>
            ) : (
              filtered.map((img) => (
                <TableRow key={img.id}>
                  <TableCell>
                    <img
                      src={`/i/${img.unique_link}.webp?w=80&h=60`}
                      alt={img.filename}
                      loading="lazy"
                      className="h-12 w-16 cursor-zoom-in rounded border border-border object-cover transition hover:ring-2 hover:ring-primary"
                      onClick={() => setLightboxSrc(`/i/${img.unique_link}`)}
                    />
                  </TableCell>
                  <TableCell className="max-w-[200px] truncate font-medium" title={img.filename}>
                    {img.filename}
                  </TableCell>
                  <TableCell className="text-muted-foreground">{img.owner_username}</TableCell>
                  <TableCell>
                    {img.visibility === 'public' ? (
                      <Badge className="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                        公开
                      </Badge>
                    ) : (
                      <Badge variant="secondary">私有</Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatBytes(img.file_size)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    <span className="inline-flex items-center justify-end gap-1">
                      <IconEye className="size-4 text-muted-foreground" />
                      {img.view_count}
                    </span>
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-muted-foreground tabular-nums">
                    {new Date(img.created_at).toLocaleDateString('zh-CN')}
                  </TableCell>
                  <TableCell className="text-right">
                    <DeleteImageButton image={img} />
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

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
        <span className="text-sm text-muted-foreground">
          {page} / {totalPages || 1}
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

      {lightboxSrc && (
        <Lightbox src={lightboxSrc} onClose={() => setLightboxSrc(null)} />
      )}
    </div>
  )
}
