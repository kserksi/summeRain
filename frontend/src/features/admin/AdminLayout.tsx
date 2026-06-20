import { NavLink, Outlet } from 'react-router'
import { IconAdjustments, IconChartBar, IconPhoto, IconUsers } from '@tabler/icons-react'

const TABS = [
  { to: '/admin', label: '概览', icon: IconChartBar, end: true },
  { to: '/admin/users', label: '用户管理', icon: IconUsers, end: false },
  { to: '/admin/images', label: '图片管理', icon: IconPhoto, end: false },
  { to: '/admin/configs', label: '系统配置', icon: IconAdjustments, end: false },
] as const

export function AdminLayout() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-2xl font-semibold">管理后台</h1>
        <p className="mt-1 text-sm text-muted-foreground">系统管理面板</p>
      </div>

      <nav className="flex gap-1 rounded-xl border border-border bg-card p-1">
        {TABS.map(({ to, label, icon: Icon, end }) => (
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
            {label}
          </NavLink>
        ))}
      </nav>

      <Outlet />
    </div>
  )
}
