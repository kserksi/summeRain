// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { api } from '@/lib/api'
import type { CaptchaPayload, User } from '@/lib/types'

export interface LoginData {
  username: string
  password: string
  captcha?: CaptchaPayload
}

export interface RegisterData {
  username: string
  email: string
  password: string
  captcha?: CaptchaPayload
}

export interface LoginResponse {
  user: User
}

export function login(data: LoginData): Promise<LoginResponse> {
  return api.post<LoginResponse>('/auth/login', data, { skipAuthRedirect: true })
}

export function register(data: RegisterData): Promise<void> {
  return api.post<void>('/auth/register', data, { skipAuthRedirect: true })
}

export function logout(): Promise<void> {
  return api.post<void>('/auth/logout', undefined, { skipAuthRedirect: true })
}
