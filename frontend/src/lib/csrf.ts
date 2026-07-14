// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { COOKIE_NAMES } from '@/config/constants'

export function getCsrfToken(): string {
  const match = document.cookie
    .split('; ')
    .find((row) => row.startsWith(`${COOKIE_NAMES.CSRF}=`))
  return match ? decodeURIComponent(match.split('=')[1]) : ''
}
