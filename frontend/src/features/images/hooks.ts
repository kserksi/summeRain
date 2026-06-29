// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { toast } from 'sonner'

import { QUERY_KEYS } from '@/config/constants'
import { useAuthStore } from '@/store/auth-store'

import {
  deleteImage,
  getImage,
  issueToken,
  listImages,
  revokeToken,
  toggleVisibility,
  uploadImages,
  type ListImagesParams,
} from './api'

export function useImages(params: ListImagesParams) {
  return useInfiniteQuery({
    queryKey: ['images', params],
    queryFn: ({ pageParam }) =>
      listImages({ ...params, cursor: pageParam as string | undefined }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => (last.has_more ? last.next_cursor : undefined),
  })
}

export function useImage(id: number) {
  return useQuery({
    queryKey: QUERY_KEYS.imageDetail(id),
    queryFn: () => getImage(id),
    enabled: !!id,
  })
}

export function useUpload() {
  const qc = useQueryClient()
  const refreshUser = useAuthStore((s) => s.refreshUser)
  return useMutation({
    mutationFn: (formData: FormData) => uploadImages(formData),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['images'] })
      refreshUser()
      const failed = data.results.filter((r) => !r.success).length
      if (failed === 0) toast.success(`已上传 ${data.total} 张图片`)
      else toast.warning(`${data.total - failed} 张成功，${failed} 张失败`)
    },
    onError: () => toast.error('上传失败'),
  })
}

export function useDeleteImage() {
  const qc = useQueryClient()
  const refreshUser = useAuthStore((s) => s.refreshUser)
  return useMutation({
    mutationFn: (id: number) => deleteImage(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['images'] })
      refreshUser()
      toast.success('图片已删除')
    },
    onError: () => toast.error('删除失败'),
  })
}

export function useToggleVisibility() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (vars: { id: number; visibility: 'public' | 'private' }) =>
      toggleVisibility(vars.id, vars.visibility),
    onSuccess: (img) => {
      qc.invalidateQueries({ queryKey: ['images'] })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.imageDetail(img.id) })
      if (img.visibility === 'private') {
        toast.warning('已设为私密，公开链接将无法访问')
      } else {
        toast.success('已设为公开')
      }
    },
    onError: () => toast.error('切换失败'),
  })
}

export function useIssueToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (vars: { id: number; ttlMs: number }) =>
      issueToken(vars.id, vars.ttlMs),
    onSuccess: (img) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.imageDetail(img.id) })
      toast.success('令牌已签发')
    },
    onError: () => toast.error('签发失败'),
  })
}

export function useRevokeToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => revokeToken(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.imageDetail(id) })
      toast.success('令牌已吊销')
    },
    onError: () => toast.error('吊销失败'),
  })
}
