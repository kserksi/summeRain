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

export const useAuthStore = create<AuthStore>((set) => ({
  user: null,
  isHydrating: true,
  setUser: (u) => set({ user: u }),
  clear: () => set({ user: null }),
  // 初始启动：设 hydrating → AuthGuard 等待
  hydrate: async () => {
    set({ isHydrating: true })
    try {
      const u = await api.get<User>('/auth/me', { skipAuthRedirect: true })
      set({ user: u, isHydrating: false })
    } catch {
      set({ user: null, isHydrating: false })
    }
  },
  // 后续静默刷新（上传后/AdminGuard降级等）：不动 hydrating，不触发 AuthGuard 卸载
  refreshUser: async () => {
    try {
      const u = await api.get<User>('/auth/me', { skipAuthRedirect: true })
      set({ user: u })
    } catch {
      // 静默失败——不清除已有用户（避免误踢）
    }
  },
}))
