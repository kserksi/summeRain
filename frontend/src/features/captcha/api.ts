// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { api } from '@/lib/api'
import type { PublicConfig } from '@/lib/types'

export function getPublicConfig(): Promise<PublicConfig> {
  return api.get<PublicConfig>('/public/config', { skipAuthRedirect: true })
}
