// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import { normalizeIssuedImageToken } from './api'

describe('normalizeIssuedImageToken', () => {
  it('maps the token endpoint contract to image-detail fields', () => {
    expect(
      normalizeIssuedImageToken({
        token_id: 7,
        token: 'private-secret',
        expires_at: '2026-07-16T12:00:00Z',
        warning: 'save now',
      }),
    ).toEqual({
      token_id: 7,
      access_token: 'private-secret',
      token_expires_at: '2026-07-16T12:00:00Z',
      warning: 'save now',
    })
  })
})
