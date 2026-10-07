// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import i18n from '@/i18n'

export class ApiError extends Error {
  code: number
  constructor(code: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}

export function apiErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    const key = `errors.${error.code}`
    const translated = i18n.t(key)
    if (translated && translated !== key) return translated
    return error.message
  }
  return fallback
}
