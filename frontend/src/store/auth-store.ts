// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { create } from 'zustand'
import type { User } from '@/lib/types'
import { api } from '@/lib/api'

interface AuthStore {
  user: User | null
  isHydrating: boolean
  setUser: (u: User | null) => void
  clear: () => void
  hydrate: () => Promise<void>
  refreshUser: () => Promise<void>
}

let authRequestGeneration = 0

export const useAuthStore = create<AuthStore>((set, get) => ({
  user: null,
  isHydrating: true,
  setUser: (u) => {
    authRequestGeneration += 1
    set({ user: u, isHydrating: false })
  },
  clear: () => {
    authRequestGeneration += 1
    set({ user: null, isHydrating: false })
  },
  // 初始启动：设 hydrating → AuthGuard 等待
  hydrate: async () => {
    const requestGeneration = ++authRequestGeneration
    set({ isHydrating: true })
    try {
      const u = await api.get<User>('/auth/me', { skipAuthRedirect: true })
      if (requestGeneration !== authRequestGeneration) return
      const { clearPersistedUploadsExceptUser } = await import(
        '@/features/images/upload-queue-store'
      )
      await clearPersistedUploadsExceptUser(u.id).catch(() => undefined)
      if (requestGeneration !== authRequestGeneration) return
      get().setUser(u)
    } catch {
      if (requestGeneration === authRequestGeneration) get().clear()
    }
  },
  // 后续静默刷新（上传后/AdminGuard降级等）：不动 hydrating，不触发 AuthGuard 卸载
  refreshUser: async () => {
    try {
      const requestGeneration = authRequestGeneration
      const previousUserId = get().user?.id
      const u = await api.get<User>('/auth/me', { skipAuthRedirect: true })
      if (
        requestGeneration !== authRequestGeneration ||
        get().user?.id !== previousUserId
      ) return
      if (previousUserId !== undefined && previousUserId !== u.id) {
        const [{ stopActiveUploadWork }, { clearPersistedUploadsExceptUser }] =
          await Promise.all([
            import('@/features/images/upload-queue-recovery'),
            import('@/features/images/upload-queue-store'),
          ])
        stopActiveUploadWork()
        await clearPersistedUploadsExceptUser(u.id).catch(() => undefined)
      }
      if (
        requestGeneration !== authRequestGeneration ||
        get().user?.id !== previousUserId
      ) return
      get().setUser(u)
    } catch {
      // 静默失败——不清除已有用户（避免误踢）
    }
  },
}))
