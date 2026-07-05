// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import {
  IconLayoutGrid,
  IconList,
  IconLoader2,
  IconPhoto,
  IconSearch,
  IconTrash,
} from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
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
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { PAGINATION } from '@/config/constants'
import { api } from '@/lib/api'
import { useAuthStore } from '@/store/auth-store'

import { ImageCard } from '../components/ImageCard'
import { useImages } from '../hooks'

type View = 'grid' | 'list'
type Visibility = 'all' | 'public' | 'private'

export default function List() {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [visibility, setVisibility] = useState<Visibility>('all')
  const [view, setView] = useState<View>('grid')
  const [debounced, setDebounced] = useState('')
  const sentinel = useRef<HTMLDivElement>(null)

  const [selectMode, setSelectMode] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)

  const qc = useQueryClient()
  const refreshUser = useAuthStore((s) => s.refreshUser)

  useEffect(() => {
    const id = setTimeout(() => setDebounced(search.trim()), 350)
    return () => clearTimeout(id)
  }, [search])

  const {
    data,
    isLoading,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = useImages({
    limit: PAGINATION.DEFAULT_LIMIT,
    visibility: visibility === 'all' ? undefined : visibility,
    search: debounced || undefined,
  })

  const images = data?.pages.flatMap((p) => p.images) ?? []

  useEffect(() => {
    const el = sentinel.current
    if (!el) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && hasNextPage && !isFetchingNextPage) {
          fetchNextPage()
        }
      },
      { rootMargin: '300px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  const toggleSelect = (id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleSelectMode = () => {
    setSelectMode((prev) => {
      const next = !prev
      if (!next) setSelectedIds(new Set())
      return next
    })
  }

  const selectAll = () => {
    setSelectedIds(new Set(images.map((img) => img.id)))
  }

  const cancelSelection = () => {
    setSelectedIds(new Set())
    setSelectMode(false)
  }

  const handleConfirmDelete = async () => {
    const count = selectedIds.size
    setIsDeleting(true)
    try {
      const results = await Promise.allSettled(
        Array.from(selectedIds).map((id) => api.del(`/images/${id}`)),
      )
      const succeeded = results.filter((r) => r.status === 'fulfilled').length
      const failed = count - succeeded
      if (succeeded > 0) {
        qc.invalidateQueries({ queryKey: ['images'] })
        await refreshUser()
      }
      if (failed === 0) toast.success(t('images.list.toast.deleted', { count: succeeded }))
      else if (succeeded === 0) toast.error(t('images.list.toast.deleteAllFailed'))
      else toast.warning(t('images.list.toast.deletePartial', { ok: succeeded, fail: failed }))
      setSelectedIds(new Set())
      setSelectMode(false)
      const failedErr = results.find((r) => r.status === 'rejected')
      if (failedErr && failedErr.status === 'rejected') {
        console.error('delete error:', failedErr.reason)
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('layout.unknownError')
      toast.error(t('images.list.toast.deleteFailedWithMsg', { msg }))
      console.error('batch delete error:', err)
    } finally {
      setIsDeleting(false)
      setDeleteOpen(false)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-2xl font-bold">{t('nav.images')}</h1>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <div className="relative min-w-60 flex-1">
          <IconSearch className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('images.list.searchPlaceholder')}
            className="pl-9"
          />
        </div>
        <ToggleGroup
          type="single"
          value={visibility}
          onValueChange={(v) => v && setVisibility(v as Visibility)}
          variant="outline"
        >
          <ToggleGroupItem value="all">{t('images.list.filterAll')}</ToggleGroupItem>
          <ToggleGroupItem value="public">{t('images.shared.public')}</ToggleGroupItem>
          <ToggleGroupItem value="private">{t('images.shared.private')}</ToggleGroupItem>
        </ToggleGroup>
        <ToggleGroup
          type="single"
          value={view}
          onValueChange={(v) => v && setView(v as View)}
          variant="outline"
        >
          <ToggleGroupItem value="grid" aria-label={t('images.list.gridView')}>
            <IconLayoutGrid />
          </ToggleGroupItem>
          <ToggleGroupItem value="list" aria-label={t('images.list.listView')}>
            <IconList />
          </ToggleGroupItem>
        </ToggleGroup>
        <Button
          variant={selectMode ? 'default' : 'outline'}
          onClick={toggleSelectMode}
        >
          {t('images.list.select')}
        </Button>
      </div>

      {selectMode && selectedIds.size > 0 && (
        <div className="sticky top-0 z-20 flex flex-wrap items-center gap-3 rounded-2xl bg-card/95 p-3 ring-1 ring-border backdrop-blur">
          <span className="text-sm font-medium">
            {t('images.list.selectedCount', { count: selectedIds.size })}
          </span>
          <div className="ml-auto flex items-center gap-2">
            <Button size="sm" variant="outline" onClick={selectAll}>
              {t('images.list.selectAll')}
            </Button>
            <Button
              size="sm"
              variant="destructive"
              onClick={() => setDeleteOpen(true)}
            >
              <IconTrash />
              {t('images.list.batchDelete')}
            </Button>
            <Button size="sm" variant="ghost" onClick={cancelSelection}>
              {t('common.cancel')}
            </Button>
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-4">
          {Array.from({ length: 12 }).map((_, i) => (
            <Skeleton key={i} className="aspect-square rounded-3xl" />
          ))}
        </div>
      ) : images.length === 0 ? (
        <div className="grid min-h-[40vh] place-items-center text-center text-muted-foreground">
          <div className="space-y-2">
            <IconPhoto className="mx-auto size-10 opacity-40" />
            <p>{t('images.list.empty')}</p>
          </div>
        </div>
      ) : (
        <div
          className={
            view === 'grid'
              ? 'grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-4'
              : 'grid grid-cols-1 gap-4 sm:grid-cols-[repeat(auto-fill,minmax(220px,1fr))]'
          }
        >
          {images.map((img) => (
            <ImageCard
              key={img.id}
              image={img}
              selectMode={selectMode}
              selected={selectedIds.has(img.id)}
              onToggleSelect={toggleSelect}
            />
          ))}
        </div>
      )}

      <div ref={sentinel} className="h-4" />
      {isFetchingNextPage && (
        <div className="flex justify-center py-4 text-muted-foreground">
          <IconLoader2 className="size-5 animate-spin" />
        </div>
      )}

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('images.list.confirmBatchDeleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('images.list.confirmBatchDeleteDesc', { count: selectedIds.size })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={isDeleting}
              onClick={(e) => {
                e.preventDefault()
                void handleConfirmDelete()
              }}
            >
              {isDeleting && <IconLoader2 className="animate-spin" />}
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
