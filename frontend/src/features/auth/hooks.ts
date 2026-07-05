// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { useCallback } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import i18n from '@/i18n'
import { queryClient } from '@/lib/query-client'
import { ApiError } from '@/lib/errors'
import { ROUTES } from '@/config/constants'
import { useAuthStore } from '@/store/auth-store'
import { login, logout, register } from './api'
import type { LoginData, RegisterData } from './api'

export function useErrorMessage() {
  const { t } = useTranslation()
  return useCallback(
    (err: unknown): string => {
      if (err instanceof ApiError) {
        const key = `errors.${err.code}`
        const msg = t(key)
        if (msg && msg !== key) return msg
        return err.message
      }
      return t('common.error')
    },
    [t],
  )
}

export function useLogin() {
  const navigate = useNavigate()
  const setUser = useAuthStore((s) => s.setUser)
  const resolveError = useErrorMessage()

  return useMutation({
    mutationFn: (data: LoginData) => login(data),
    onSuccess: (result) => {
      queryClient.clear()
      setUser(result.user)
      navigate(ROUTES.DASHBOARD, { replace: true })
    },
    onError: (err) => {
      toast.error(resolveError(err))
    },
  })
}

export function useRegister() {
  const navigate = useNavigate()
  const resolveError = useErrorMessage()

  return useMutation({
    mutationFn: (data: RegisterData) => register(data),
    onSuccess: () => {
      toast.success(i18n.t('auth.registerSuccess'))
      navigate(ROUTES.LOGIN, { replace: true })
    },
    onError: (err) => {
      toast.error(resolveError(err))
    },
  })
}

export function useLogout() {
  const clear = useAuthStore((s) => s.clear)

  return useMutation({
    mutationFn: () => logout(),
    onSettled: () => {
      clear()
      window.location.assign(ROUTES.HOME)
    },
  })
}
