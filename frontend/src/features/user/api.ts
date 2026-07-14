// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { api } from '@/lib/api'
import type { UserProfile } from '@/lib/types'

export function getProfile(): Promise<UserProfile> {
  return api.get<UserProfile>('/user/profile')
}

export function changePassword(data: {
  old_password: string
  new_password: string
}): Promise<void> {
  return api.patch<void>('/user/password', data)
}

export function updateAvatar(avatarUrl: string): Promise<{ avatar_url: string }> {
  return api.patch<{ avatar_url: string }>('/user/avatar', { avatar_url: avatarUrl })
}
