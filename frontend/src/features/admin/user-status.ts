// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { USER_STATUS } from '@/config/constants'

export interface AdminUserActionPolicy {
  canChangeStatus: boolean
  canRequestDeletion: boolean
  canCancelDeletion: boolean
  canEditQuota: boolean
}

const NO_ADMIN_USER_ACTIONS: AdminUserActionPolicy = {
  canChangeStatus: false,
  canRequestDeletion: false,
  canCancelDeletion: false,
  canEditQuota: false,
}

export function getAdminUserActionPolicy(status: string): AdminUserActionPolicy {
  switch (status) {
    case USER_STATUS.ACTIVE:
      return {
        canChangeStatus: true,
        canRequestDeletion: true,
        canCancelDeletion: false,
        canEditQuota: true,
      }
    case USER_STATUS.SUSPENDED:
      return {
        canChangeStatus: true,
        canRequestDeletion: false,
        canCancelDeletion: false,
        canEditQuota: true,
      }
    case USER_STATUS.PENDING_DELETION:
      return {
        canChangeStatus: false,
        canRequestDeletion: false,
        canCancelDeletion: true,
        canEditQuota: true,
      }
    default:
      return NO_ADMIN_USER_ACTIONS
  }
}
