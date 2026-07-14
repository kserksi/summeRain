// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { IconSun, IconMoon } from '@tabler/icons-react'
import { useThemeStore } from '@/store/theme-store'
import { transitionTheme } from '@/lib/theme-transition'

export function ThemeToggle() {
  const { t } = useTranslation()
  const theme = useThemeStore((s) => s.theme)
  const toggle = useThemeStore((s) => s.toggle)
  const ref = useRef<HTMLButtonElement>(null)

  const handleClick = () => {
    const next = theme === 'dark' ? 'light' : 'dark'
    const rect = ref.current?.getBoundingClientRect()
    transitionTheme(next, rect ? rect.left + rect.width / 2 : 0, rect ? rect.top + rect.height / 2 : 0, toggle)
  }

  return (
    <button
      ref={ref}
      onClick={handleClick}
      aria-label={t('layout.toggleTheme')}
      className="grid size-11 place-items-center rounded-xl border border-border bg-card text-foreground transition hover:border-primary hover:text-primary"
    >
      {theme === 'dark' ? <IconSun className="size-5" /> : <IconMoon className="size-5" />}
    </button>
  )
}
