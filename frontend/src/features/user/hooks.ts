// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import i18n from '@/i18n'
import { QUERY_KEYS } from '@/config/constants'
import { useAuthStore } from '@/store/auth-store'
import { getProfile, changePassword } from './api'

export function useProfile() {
  return useQuery({
    queryKey: QUERY_KEYS.profile,
    queryFn: getProfile,
  })
}

export function useChangePassword() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  return useMutation({
    mutationFn: changePassword,
    onSuccess: () => {
      queryClient.clear()
      useAuthStore.getState().clear()
      toast.success(i18n.t('profile.password.changed'))
      navigate('/login', { replace: true })
    },
  })
}
