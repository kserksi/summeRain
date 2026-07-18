// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { api } from '@/lib/api'
import type { Image, ImageListResponse, UploadResponse } from '@/lib/types'

export interface ListImagesParams {
  cursor?: string
  limit?: number
  visibility?: string
  search?: string
}

export function listImages(params: ListImagesParams): Promise<ImageListResponse> {
  const sp = new URLSearchParams()
  if (params.cursor) sp.set('cursor', params.cursor)
  if (params.limit != null) sp.set('limit', String(params.limit))
  if (params.visibility) sp.set('visibility', params.visibility)
  if (params.search) sp.set('search', params.search)
  const qs = sp.toString()
  return api.get<ImageListResponse>(`/images/${qs ? `?${qs}` : ''}`)
}

export function getImage(id: number): Promise<Image> {
  return api.get<Image>(`/images/${id}`)
}

export function deleteImage(id: number): Promise<void> {
  return api.del<void>(`/images/${id}`)
}

export function toggleVisibility(
  id: number,
  visibility: 'public' | 'private',
): Promise<Image> {
  return api.patch<Image>(`/images/${id}/visibility`, { visibility })
}

interface IssueTokenResponse {
  token_id: number
  token: string
  expires_at: string
  warning?: string
}

export interface IssuedImageToken {
  token_id: number
  access_token: string
  token_expires_at: string
  warning?: string
}

export function normalizeIssuedImageToken(result: IssueTokenResponse): IssuedImageToken {
  return {
    token_id: result.token_id,
    access_token: result.token,
    token_expires_at: result.expires_at,
    warning: result.warning,
  }
}

export function uploadImages(formData: FormData, signal?: AbortSignal): Promise<UploadResponse> {
  return api.upload<UploadResponse>('/images/', formData, { signal })
}

export async function issueToken(id: number, ttlMs: number): Promise<IssuedImageToken> {
  const result = await api.post<IssueTokenResponse>(`/images/${id}/tokens`, { ttl_ms: ttlMs })
  return normalizeIssuedImageToken(result)
}

export function revokeToken(id: number): Promise<void> {
  return api.del<void>(`/images/${id}/tokens`)
}
