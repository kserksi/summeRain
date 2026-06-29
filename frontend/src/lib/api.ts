// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { API_BASE_URL } from '@/config/constants'
import { getCsrfToken } from '@/lib/csrf'
import { ApiError } from '@/lib/errors'

type RequestOptions = {
  method?: string
  body?: unknown
  headers?: Record<string, string>
  skipAuthRedirect?: boolean
  signal?: AbortSignal
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, headers = {}, skipAuthRedirect = false, signal } = opts
  const url = `${API_BASE_URL}${path}`
  const isWrite = method !== 'GET' && method !== 'HEAD'

  const finalHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
    ...headers,
  }
  if (isWrite) {
    const csrf = getCsrfToken()
    if (csrf) finalHeaders['X-CSRF-Token'] = csrf
  }

  let resp: Response
  try {
    resp = await fetch(url, {
      method,
      headers: finalHeaders,
      credentials: 'include',
      body: body ? JSON.stringify(body) : undefined,
      signal,
    })
  } catch {
    throw new ApiError(0, '网络错误，请检查连接')
  }

  if (resp.status === 401 && !skipAuthRedirect) {
    window.location.assign('/login')
    throw new ApiError(4010, '会话已过期')
  }

  let json: { code: number; message: string; data?: T }
  try {
    json = await resp.json()
  } catch {
    throw new ApiError(0, '响应解析失败')
  }

  if (json.code !== 0) {
    if (json.code === 4030 && !skipAuthRedirect) {
      window.location.assign('/login?reason=banned')
    }
    throw new ApiError(json.code, json.message || '未知错误')
  }
  return json.data as T
}

export const api = {
  get: <T>(path: string, opts?: RequestOptions) => request<T>(path, { ...opts, method: 'GET' }),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>(path, { ...opts, method: 'POST', body }),
  patch: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>(path, { ...opts, method: 'PATCH', body }),
  del: <T>(path: string, opts?: RequestOptions) => request<T>(path, { ...opts, method: 'DELETE' }),
  upload: async <T>(path: string, formData: FormData, opts?: RequestOptions): Promise<T> => {
    const csrf = getCsrfToken()
    const headers: Record<string, string> = {}
    if (csrf) headers['X-CSRF-Token'] = csrf
    if (opts?.headers) Object.assign(headers, opts.headers)
    const resp = await fetch(`${API_BASE_URL}${path}`, {
      method: 'POST',
      headers,
      credentials: 'include',
      body: formData,
      signal: opts?.signal,
    })
    const json = await resp.json()
    if (json.code !== 0) throw new ApiError(json.code, json.message)
    return json.data as T
  },
}
