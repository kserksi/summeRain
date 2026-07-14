// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

export const API_BASE_URL = '/api/v1'

export const STORAGE_KEYS = {
  THEME: 'ic_theme',
} as const

export const COOKIE_NAMES = {
  SESSION: '__Host-session_token',
  CSRF: '__Host-csrf_token',
} as const

export const ROUTES = {
  HOME: '/',
  LOGIN: '/login',
  REGISTER: '/register',
  DASHBOARD: '/dashboard',
  IMAGES: '/images',
  IMAGE_DETAIL: '/images/:id',
  UPLOAD: '/upload',
  PROFILE: '/profile',
  ADMIN: '/admin',
  ADMIN_USERS: '/admin/users',
  ADMIN_CONFIGS: '/admin/configs',
} as const

export const PAGINATION = {
  DEFAULT_LIMIT: 20,
  DEFAULT_PAGE_SIZE: 20,
  MAX_PAGE_SIZE: 100,
} as const

export const IMAGE_TOKEN = {
  DEFAULT_TTL_MS: 3_600_000,
  MIN_TTL_MS: 600_000,
  MAX_TTL_MS: 259_200_000,
} as const

export const RECAPTCHA_ACTIONS = {
  LOGIN: 'login',
  REGISTER: 'register',
} as const

export const CAPTCHA_PROVIDERS = ['none', 'recaptcha', 'turnstile', 'geetest_v4'] as const
export type CaptchaProvider = (typeof CAPTCHA_PROVIDERS)[number]

export const USER_ROLES = {
  USER: 'user',
  ADMIN: 'admin',
} as const

export const USER_STATUS = {
  ACTIVE: 'active',
  SUSPENDED: 'suspended',
  PENDING: 'pending',
} as const

export const QUERY_KEYS = {
  images: ['images'] as const,
  imageDetail: (id: string | number) => ['images', id] as const,
  adminUsers: ['admin', 'users'] as const,
  adminStats: ['admin', 'stats'] as const,
  adminConfigs: ['admin', 'configs'] as const,
  notifications: ['notifications'] as const,
  profile: ['profile'] as const,
  publicConfig: ['public-config'] as const,
} as const
