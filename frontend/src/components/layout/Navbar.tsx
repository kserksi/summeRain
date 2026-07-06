// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { Link, NavLink } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ThemeToggle } from '@/components/layout/ThemeToggle'
import { NotificationBell } from '@/features/notifications/components/NotificationBell'
import { useAuthStore } from '@/store/auth-store'

const navItems = [
  { to: '/', labelKey: 'nav.home' },
  { to: '/dashboard', labelKey: 'nav.dashboard', auth: true },
  { to: '/images', labelKey: 'nav.images', auth: true },
  { to: '/upload', labelKey: 'nav.upload', auth: true },
]

export function Navbar() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const items = navItems.filter((i) => !i.auth || user)

  return (
    <header className="sticky top-0 z-50 border-b border-border bg-card/80 backdrop-blur-lg">
      <div className="mx-auto flex h-16 max-w-7xl items-center gap-4 px-6">
        <Link to="/" className="flex items-center gap-2 text-lg font-extrabold">
          <span className="text-primary text-2xl">⬢</span>
          <span>{t('common.appName')}</span>
          <span className="text-accent">.</span>
        </Link>
        <nav className="hidden flex-1 items-center gap-1 md:flex">
          {items.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                `rounded-full px-4 py-2 text-sm font-medium transition ${isActive ? 'bg-primary text-white' : 'text-muted-foreground hover:bg-accent hover:text-foreground'}`
              }
            >
              {t(item.labelKey)}
            </NavLink>
          ))}
          {user?.role === 'admin' && (
            <NavLink to="/admin" className={({ isActive }) => `rounded-full px-4 py-2 text-sm font-medium transition ${isActive ? 'bg-primary text-white' : 'text-muted-foreground hover:bg-accent hover:text-foreground'}`}>
              {t('nav.admin')}
            </NavLink>
          )}
        </nav>
        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle />
          {user ? (
            <div className="flex items-center gap-2">
              <NotificationBell />
              <Link to="/profile" className="size-10 overflow-hidden rounded-full bg-gradient-to-br from-primary to-accent transition hover:opacity-80">
                {user.avatar_url ? (
                  <img src={user.avatar_url} alt={user.username} className="size-full object-cover" />
                ) : (
                  <span className="grid size-full place-items-center font-bold text-white">
                    {user.username.charAt(0).toUpperCase()}
                  </span>
                )}
              </Link>
            </div>
          ) : (
            <Link to="/login" className="text-sm font-medium text-muted-foreground hover:text-primary">{t('common.login')}</Link>
          )}
        </div>
      </div>
    </header>
  )
}
