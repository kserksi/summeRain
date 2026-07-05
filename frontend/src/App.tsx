// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { lazy, Suspense, useEffect } from 'react'
import { Routes, Route, Navigate, Outlet } from 'react-router'
import { useTranslation } from 'react-i18next'
import { Navbar } from '@/components/layout/Navbar'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { useAuthStore } from '@/store/auth-store'
import { IconLoader2 } from '@tabler/icons-react'
import { PendingDeletionBanner } from '@/components/PendingDeletionBanner'

const Login = lazy(() => import('@/features/auth/pages/Login'))
const Register = lazy(() => import('@/features/auth/pages/Register'))
const LandingPage = lazy(() => import('@/pages/Landing'))
const DashboardPage = lazy(() => import('@/pages/Dashboard'))
const ImageList = lazy(() => import('@/features/images/pages/List'))
const ImageDetail = lazy(() => import('@/features/images/pages/Detail'))
const Upload = lazy(() => import('@/features/images/pages/Upload'))
const Profile = lazy(() => import('@/features/user/pages/Profile'))
const AdminOverview = lazy(() => import('@/features/admin/pages/Overview'))
const AdminUsers = lazy(() => import('@/features/admin/pages/Users'))
const AdminImages = lazy(() => import('@/features/admin/pages/Images'))
const AdminConfigs = lazy(() => import('@/features/admin/pages/Configs'))
const AdminLayout = lazy(() => import('@/features/admin/AdminLayout').then((m) => ({ default: m.AdminLayout })))

function Loading() {
  return (
    <div className="grid min-h-[50vh] place-items-center">
      <IconLoader2 className="size-8 animate-spin text-primary" />
    </div>
  )
}

function NotFound() {
  const { t } = useTranslation()
  return (
    <div className="grid min-h-[60vh] place-items-center text-center">
      <div>
        <p className="text-6xl font-extrabold text-primary">404</p>
        <p className="mt-3 text-muted-foreground">{t('layout.notFound')}</p>
      </div>
    </div>
  )
}

function Layout() {
  return (
    <div className="min-h-screen bg-background">
      <Navbar />
      <PendingDeletionBanner />
      <main className="mx-auto max-w-7xl px-6 py-8">
        <Outlet />
      </main>
    </div>
  )
}

function AuthGuard() {
  const user = useAuthStore((s) => s.user)
  const isHydrating = useAuthStore((s) => s.isHydrating)
  if (isHydrating) return <Loading />
  if (!user) return <Navigate to="/login" replace />
  return <Outlet />
}

function AdminGuard() {
  const user = useAuthStore((s) => s.user)
  if (user?.role !== 'admin') return <Navigate to="/dashboard" replace />
  return <Outlet />
}

export default function App() {
  const hydrate = useAuthStore((s) => s.hydrate)

  useEffect(() => {
    hydrate()
  }, [hydrate])

  return (
    <ErrorBoundary>
      <Suspense fallback={<Loading />}>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<LandingPage />} />
          <Route path="login" element={<Login />} />
          <Route path="register" element={<Register />} />
          <Route element={<AuthGuard />}>
            <Route path="dashboard" element={<DashboardPage />} />
            <Route path="images" element={<ImageList />} />
            <Route path="images/:id" element={<ImageDetail />} />
            <Route path="upload" element={<Upload />} />
            <Route path="profile" element={<Profile />} />
            <Route element={<AdminGuard />}>
              <Route path="admin" element={<AdminLayout />}>
                <Route index element={<AdminOverview />} />
                <Route path="users" element={<AdminUsers />} />
                <Route path="images" element={<AdminImages />} />
                <Route path="configs" element={<AdminConfigs />} />
              </Route>
            </Route>
          </Route>
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
      </Suspense>
    </ErrorBoundary>
  )
}
