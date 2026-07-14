// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { api } from '@/lib/api'
import type { Notification } from '@/lib/types'

export function listNotifications(): Promise<{ items: Notification[]; next_cursor: string }> {
  return api.get<{ items: Notification[]; next_cursor: string }>('/notifications/')
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
