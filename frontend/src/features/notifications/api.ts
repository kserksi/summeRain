import { api } from '@/lib/api'
import type { Notification } from '@/lib/types'

export function listNotifications(): Promise<Notification[]> {
  return api.get<Notification[]>('/notifications/')
}

export function markRead(id: number): Promise<void> {
  return api.patch<void>(`/notifications/${id}/read`)
}

export function markAllRead(): Promise<void> {
  return api.patch<void>('/notifications/batch-read')
}

export function deleteNotification(id: number): Promise<void> {
  return api.del<void>(`/notifications/${id}`)
}

export function clearAll(): Promise<void> {
  return api.del<void>('/notifications/clear')
}
