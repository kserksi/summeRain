// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { STORAGE_KEYS } from '@/config/constants'

type Theme = 'light' | 'dark'

interface ThemeStore {
  theme: Theme
  setTheme: (t: Theme) => void
  toggle: () => void
}

function applyDom(t: Theme) {
  document.documentElement.setAttribute('data-theme', t)
  document.documentElement.classList.toggle('dark', t === 'dark')
}

export const useThemeStore = create<ThemeStore>()(
  persist(
    (set) => ({
      theme: 'light',
      setTheme: (t) => { applyDom(t); set({ theme: t }) },
      toggle: () => set((s) => {
        const next = s.theme === 'dark' ? 'light' : 'dark'
        applyDom(next)
        return { theme: next }
      }),
    }),
    {
      name: STORAGE_KEYS.THEME,
      onRehydrateStorage: () => (state) => { if (state) applyDom(state.theme) },
    },
  ),
)
