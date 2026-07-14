// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

export interface User {
  id: number
  username: string
  email: string
  role: string
  status: string
  avatar_url: string | null
  storage_used: number
  storage_quota: number
  image_count: number
  deletion_scheduled_at: string | null
  batch_download_count: number
  created_at: string
  updated_at: string
}

export interface UserProfile extends Omit<User, 'created_at' | 'updated_at'> {
  storage_percent: number
  created_at: string
}

export interface Image {
  id: number
  user_id: number
  image_file_id: number
  unique_link: string
  title: string
  filename: string
  description?: string
  visibility: 'public' | 'private'
  view_count: number
  width: number
  height: number
  file_size: number
  created_at: string
  updated_at: string
  access_token?: string
  token_expires_at?: string
}

export interface ImageListResponse {
  images: Image[]
  next_cursor: string
  has_more: boolean
}

export interface UploadResult {
  filename: string
  success: boolean
  image_id?: number
  unique_link?: string
  thumbnail_url?: string
  processed_url?: string
  error?: string
  error_code?: number
}

export interface UploadResponse {
  upload_id: number
  total: number
  results: UploadResult[]
  storage_used: number
  storage_quota: number
  storage_percent: number
}

export interface SystemStats {
  total_users: number
  total_images: number
  storage_used: number
  active_users: number
  total_sessions: number
}

export interface UserListResult {
  items: User[]
  total: number
  page: number
}

export interface Notification {
  id: number
  user_id: number
  type: string
  title: string
  message: string
  is_read: boolean
  metadata?: string
  created_at: string
}

export interface PublicConfig {
  captcha_provider: string
  captcha_site_key: string
  site_language: string
}

export interface SystemConfig {
  config_key: string
  config_value: string
}

export interface CaptchaPayload {
  provider: string
  token?: string
  action?: string
  lot_number?: string
  captcha_output?: string
  pass_token?: string
  gen_time?: string
}

export interface ApiError extends Error {
  code: number
}
