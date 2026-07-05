// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import i18n from '@/i18n'
import { PAGINATION, QUERY_KEYS, USER_STATUS } from '@/config/constants'
import {
  listUsers,
  setUserStatus,
  getStats,
  getConfigs,
  updateConfigs,
  listAllImages,
  requestUserDeletion,
  cancelUserDeletion,
  updateUserQuota,
  testR2Connection,
} from './api'

export function useAdminUsers(page: number, pageSize: number = PAGINATION.DEFAULT_PAGE_SIZE) {
  return useQuery({
    queryKey: ['admin', 'users', { page }],
    queryFn: () => listUsers(page, pageSize),
    placeholderData: (prev) => prev,
  })
}

export function useSetUserStatus() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, status }: { id: number; status: string }) => setUserStatus(id, status),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminUsers })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminStats })
      toast.success(vars.status === USER_STATUS.SUSPENDED ? i18n.t('admin.users.suspendedUser') : i18n.t('admin.users.unsuspendedUser'))
    },
    onError: () => toast.error(i18n.t('admin.shared.actionFailed')),
  })
}

export function useAdminStats() {
  return useQuery({
    queryKey: QUERY_KEYS.adminStats,
    queryFn: getStats,
  })
}

export function useConfigs() {
  return useQuery({
    queryKey: QUERY_KEYS.adminConfigs,
    queryFn: getConfigs,
  })
}

export function useUpdateConfigs() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (items: { key: string; value: string }[]) => updateConfigs(items),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminConfigs })
      toast.success(i18n.t('admin.configs.saved'))
    },
    onError: () => toast.error(i18n.t('admin.configs.saveFailed')),
  })
}

export function useAdminImages(page: number) {
  return useQuery({
    queryKey: ['admin', 'images', { page }],
    queryFn: () => listAllImages(page),
    placeholderData: (prev) => prev,
  })
}

export function useRequestDeletion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, username, adminUsername }: { id: number; username: string; adminUsername: string }) =>
      requestUserDeletion(id, username, adminUsername),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminUsers })
      toast.success(i18n.t('admin.users.deletionRequested'))
    },
    onError: () => toast.error(i18n.t('admin.shared.actionFailed')),
  })
}

export function useCancelDeletion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => cancelUserDeletion(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminUsers })
      toast.success(i18n.t('admin.users.deletionCancelled'))
    },
    onError: () => toast.error(i18n.t('admin.shared.actionFailed')),
  })
}

export function useUpdateQuota() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, storageQuota }: { id: number; storageQuota: number }) =>
      updateUserQuota(id, storageQuota),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminUsers })
      toast.success(i18n.t('admin.users.quotaUpdated'))
    },
    onError: () => toast.error(i18n.t('admin.shared.actionFailed')),
  })
}

export function useTestR2() {
  return useMutation({
    mutationFn: testR2Connection,
    onSuccess: () => toast.success(i18n.t('admin.configs.r2TestSuccess')),
    onError: (e: unknown) => {
      const msg = e instanceof Error ? e.message : i18n.t('admin.configs.r2TestFailed')
      toast.error(msg)
    },
  })
}
