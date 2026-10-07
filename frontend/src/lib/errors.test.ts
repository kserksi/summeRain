// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'

import enUS from '@/i18n/locales/en-US.json'
import { ApiError, apiErrorMessage } from './errors'

describe('apiErrorMessage', () => {
  it('prefers the localized message for a mapped error code', () => {
    const error = new ApiError(2001, 'server fallback')
    expect(apiErrorMessage(error, 'fallback')).toBe(enUS.errors['2001'])
  })

  it('falls back to the server message for an unmapped code', () => {
    const error = new ApiError(9999, 'server fallback')
    expect(apiErrorMessage(error, 'fallback')).toBe('server fallback')
  })

  it('returns the provided fallback for non-API errors', () => {
    expect(apiErrorMessage(new Error('boom'), 'fallback')).toBe('fallback')
    expect(apiErrorMessage(undefined, 'fallback')).toBe('fallback')
  })
})
