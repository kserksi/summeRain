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

export function uploadImages(formData: FormData): Promise<UploadResponse> {
  return api.upload<UploadResponse>('/images/', formData)
}

export function issueToken(id: number, ttlMs: number): Promise<Image> {
  return api.post<Image>(`/images/${id}/tokens`, { ttl_ms: ttlMs })
}

export function revokeToken(id: number): Promise<void> {
  return api.del<void>(`/images/${id}/tokens`)
}
