// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { ApiError } from '@/lib/errors'
import type { UploadResponse } from '@/lib/types'

import { uploadImages } from './api'

export interface V1UploadResult {
  uploadId: string
  uniqueLink: string
  pipelineVersion: 1
}

export function buildV1UploadForm(file: File, visibility: 'public' | 'private'): FormData {
  const form = new FormData()
  form.append('images', file, file.name)
  form.append('visibility', visibility)
  return form
}

export function normalizeV1UploadResult(response: UploadResponse): V1UploadResult {
  const result = response.results[0]
  if (response.total !== 1 || response.results.length !== 1 || !result) {
    throw new Error('Legacy upload returned an invalid single-image response')
  }
  if (!result.success) {
    throw new ApiError(result.error_code ?? 1000, result.error || 'Legacy image upload failed')
  }
  if (!result.unique_link) {
    throw new Error('Legacy upload response is missing the image link')
  }
  return {
    uploadId: String(response.upload_id),
    uniqueLink: result.unique_link,
    pipelineVersion: 1,
  }
}

export async function beginV1Upload(
  file: File,
  visibility: 'public' | 'private',
  signal?: AbortSignal,
): Promise<V1UploadResult> {
  const response = await uploadImages(buildV1UploadForm(file, visibility), signal)
  return normalizeV1UploadResult(response)
}
