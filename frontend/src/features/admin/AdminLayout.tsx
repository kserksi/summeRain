// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { NavLink, Outlet } from 'react-router'
import { useTranslation } from 'react-i18next'
import { IconAdjustments, IconChartBar, IconPhoto, IconUsers } from '@tabler/icons-react'

const TABS = [
  { to: '/admin', labelKey: 'admin.shared.nav.overview', icon: IconChartBar, end: true },
  { to: '/admin/users', labelKey: 'admin.shared.nav.users', icon: IconUsers, end: false },
  { to: '/admin/images', labelKey: 'admin.shared.nav.images', icon: IconPhoto, end: false },
  { to: '/admin/configs', labelKey: 'admin.shared.nav.configs', icon: IconAdjustments, end: false },
] as const

export function AdminLayout() {
  const { t } = useTranslation()
  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-2xl font-semibold">{t('admin.shared.title')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('admin.shared.subtitle')}</p>
      </div>

      <nav className="flex gap-1 rounded-xl border border-border bg-card p-1">
        {TABS.map(({ to, labelKey, icon: Icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={({ isActive }) =>
              `flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition ${
                isActive
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              }`
            }
          >
            <Icon className="size-4" />
            {t(labelKey)}
          </NavLink>
        ))}
      </nav>

      <Outlet />
    </div>
  )
}
