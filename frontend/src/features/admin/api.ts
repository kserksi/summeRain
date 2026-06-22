import { api } from '@/lib/api'
import type { Image, SystemStats, UserListResult, SystemConfig } from '@/lib/types'

export type AdminImage = Image & { owner_username: string }

export function listUsers(page: number, pageSize: number) {
  return api.get<UserListResult>(`/admin/users?page=${page}&page_size=${pageSize}`)
}

export function setUserStatus(id: number, status: string) {
  return api.patch<void>(`/admin/users/${id}/status`, { status })
}

export function getStats() {
  return api.get<SystemStats>('/admin/stats')
}

export function getConfigs() {
  return api.get<SystemConfig[]>('/admin/configs')
}

export function updateConfigs(items: { key: string; value: string }[]) {
  return api.patch<void>('/admin/configs', { items })
}

export function listAllImages(page: number, pageSize: number = 20) {
  return api.get<{ items: Array<AdminImage & { owner_username: string }>; total: number; page: number }>('/admin/images?page=' + page + '&page_size=' + pageSize)
}
export function requestUserDeletion(id: number, username: string, adminUsername: string) {
  return api.post<void>(`/admin/users/${id}/request-deletion?admin=${adminUsername}`, { username })
}
export function cancelUserDeletion(id: number) {
  return api.post<void>(`/admin/users/${id}/cancel-deletion`)
}
export function updateUserQuota(id: number, storageQuota: number) {
  return api.patch<void>(`/admin/users/${id}/quota`, { storage_quota: storageQuota })
}

export function testR2Connection(params: { endpoint: string; access_key: string; secret_key: string; bucket: string }) {
  return api.post<{ ok: boolean }>('/admin/r2/test', params)
}
