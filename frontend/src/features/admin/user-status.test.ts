// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'
import { USER_STATUS } from '@/config/constants'
import { getAdminUserActionPolicy } from './user-status'

describe('admin user action policy', () => {
  it('makes deleting an informational-only state', () => {
    expect(getAdminUserActionPolicy(USER_STATUS.DELETING)).toEqual({
      canChangeStatus: false,
      canRequestDeletion: false,
      canCancelDeletion: false,
      canEditQuota: false,
    })
  })

  it('keeps pending deletion cancellable', () => {
    expect(getAdminUserActionPolicy(USER_STATUS.PENDING_DELETION)).toEqual({
      canChangeStatus: false,
      canRequestDeletion: false,
      canCancelDeletion: true,
      canEditQuota: true,
    })
  })

  it('only exposes status changes for active and suspended users', () => {
    expect(getAdminUserActionPolicy(USER_STATUS.ACTIVE).canChangeStatus).toBe(true)
    expect(getAdminUserActionPolicy(USER_STATUS.SUSPENDED).canChangeStatus).toBe(true)
    expect(getAdminUserActionPolicy('unknown').canChangeStatus).toBe(false)
  })
})
