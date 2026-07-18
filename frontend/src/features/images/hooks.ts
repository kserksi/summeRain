// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { toast } from 'sonner'

import i18n from '@/i18n'
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
      if (failed === 0) toast.success(i18n.t('upload.toast.uploadSuccess', { count: data.total }))
      else toast.warning(i18n.t('upload.toast.uploadPartial', { ok: data.total - failed, fail: failed }))
    },
    onError: () => toast.error(i18n.t('upload.toast.uploadAllFailed')),
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
      toast.success(i18n.t('images.toast.deleted'))
    },
    onError: () => toast.error(i18n.t('images.toast.deleteFailed')),
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
        toast.warning(i18n.t('images.toast.setToPrivate'))
      } else {
        toast.success(i18n.t('images.toast.setToPublic'))
      }
    },
    onError: () => toast.error(i18n.t('images.toast.toggleFailed')),
  })
}

export function useIssueToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (vars: { id: number; ttlMs: number }) =>
      issueToken(vars.id, vars.ttlMs),
    onSuccess: (_token, vars) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.imageDetail(vars.id) })
      toast.success(i18n.t('images.toast.tokenIssued'))
    },
    onError: () => toast.error(i18n.t('images.toast.issueFailed')),
  })
}

export function useRevokeToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => revokeToken(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.imageDetail(id) })
      toast.success(i18n.t('images.toast.tokenRevoked'))
    },
    onError: () => toast.error(i18n.t('images.toast.revokeFailed')),
  })
}
