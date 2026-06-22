import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
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
      toast.success(vars.status === USER_STATUS.SUSPENDED ? '已封禁该用户' : '已解封该用户')
    },
    onError: () => toast.error('操作失败，请重试'),
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
      toast.success('配置已保存')
    },
    onError: () => toast.error('保存失败，请重试'),
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
      toast.success('已发送删除请求')
    },
    onError: () => toast.error('操作失败，请重试'),
  })
}

export function useCancelDeletion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => cancelUserDeletion(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminUsers })
      toast.success('已取消删除请求')
    },
    onError: () => toast.error('操作失败，请重试'),
  })
}

export function useUpdateQuota() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, storageQuota }: { id: number; storageQuota: number }) =>
      updateUserQuota(id, storageQuota),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.adminUsers })
      toast.success('配额已更新')
    },
    onError: () => toast.error('操作失败，请重试'),
  })
}

export function useTestR2() {
  return useMutation({
    mutationFn: testR2Connection,
    onSuccess: () => toast.success('R2 连接测试成功'),
    onError: (e: unknown) => {
      const msg = e instanceof Error ? e.message : '连接测试失败'
      toast.error(msg)
    },
  })
}
