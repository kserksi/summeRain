// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ApiError } from '@/lib/errors'

import { beginV1Upload, buildV1UploadForm, normalizeV1UploadResult } from './v1-upload'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('V1 multipart fallback', () => {
  it('uses the V1 field names and preserves the source file', async () => {
    const file = new File(['jpeg'], 'photo.jpg', { type: 'image/jpeg' })
    const form = buildV1UploadForm(file, 'private')

    const uploaded = form.get('images') as File
    expect(uploaded).toMatchObject({ name: 'photo.jpg', size: 4, type: 'image/jpeg' })
    expect(await uploaded.text()).toBe('jpeg')
    expect(form.get('visibility')).toBe('private')
  })

  it('normalizes a successful legacy response for the shared upload queue', () => {
    expect(
      normalizeV1UploadResult({
        upload_id: 27,
        total: 1,
        results: [{ filename: 'photo.jpg', success: true, image_id: 4, unique_link: 'legacy' }],
        storage_used: 4,
        storage_quota: 100,
        storage_percent: 4,
      }),
    ).toEqual({ uploadId: '27', uniqueLink: 'legacy', pipelineVersion: 1 })
  })

  it('surfaces a per-file V1 rejection as an API error', () => {
    expect(() =>
      normalizeV1UploadResult({
        upload_id: 28,
        total: 1,
        results: [
          {
            filename: 'large.jpg',
            success: false,
            error: 'file too large',
            error_code: 3002,
          },
        ],
        storage_used: 0,
        storage_quota: 100,
        storage_percent: 0,
      }),
    ).toThrowError(new ApiError(3002, 'file too large'))
  })

  it('posts the multipart form with the caller abort signal', async () => {
    const controller = new AbortController()
    const upload = vi.spyOn(api, 'upload').mockResolvedValue({
      upload_id: 29,
      total: 1,
      results: [{ filename: 'photo.jpg', success: true, unique_link: 'legacy-signal' }],
      storage_used: 4,
      storage_quota: 100,
      storage_percent: 4,
    })

    await expect(
      beginV1Upload(new File(['jpeg'], 'photo.jpg', { type: 'image/jpeg' }), 'public', controller.signal),
    ).resolves.toMatchObject({ uniqueLink: 'legacy-signal', pipelineVersion: 1 })
    expect(upload).toHaveBeenCalledWith('/images/', expect.any(FormData), {
      signal: controller.signal,
    })
  })
})
