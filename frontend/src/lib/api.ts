// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { API_BASE_URL } from '@/config/constants'
import i18n from '@/i18n'
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
    // 网络错误统一映射为 networkError,避免暴露原始 fetch 错误
    // TODO: 区分 timeout 与 offline 两种场景
    throw new ApiError(0, i18n.t('api.networkError'))
  }

  if (resp.status === 401 && !skipAuthRedirect) {
    window.location.assign('/login')
    throw new ApiError(4010, i18n.t('api.sessionExpired'))
  }

  let json: { code: number; message: string; data?: T }
  try {
    json = await resp.json()
  } catch {
    throw new ApiError(0, i18n.t('api.parseFailed'))
  }

  if (json.code !== 0) {
    if (json.code === 4030 && !skipAuthRedirect) {
      window.location.assign('/login?reason=banned')
    }
    throw new ApiError(json.code, json.message || i18n.t('api.unknownError'))
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
    // 注意: upload 不要全局设 Content-Type,浏览器会自动加 boundary
    // (手写 multipart/form-data 会缺少 boundary 参数,导致后端解析失败)
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
