import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './api'
import { ApiError } from './errors'

const mockResponse = (status: number, body: unknown) =>
  ({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  }) as Response

describe('api wrapper', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
    vi.spyOn(window, 'location', 'get').mockReturnValue({
      ...window.location,
      assign: vi.fn(),
    } as Location)
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      get: () => '',
      set: () => {},
    })
  })

  it('returns data on code=0', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(mockResponse(200, { code: 0, message: 'ok', data: { id: 1 } }))
    const result = await api.get('/test')
    expect(result).toEqual({ id: 1 })
  })

  it('throws ApiError on code!=0', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(mockResponse(200, { code: 2001, message: '凭证错误' }))
    const err = (await api.get('/test').catch((e: unknown) => e)) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.message).toBe('凭证错误')
    expect(err.code).toBe(2001)
  })

  it('does NOT inject CSRF on GET', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(mockResponse(200, { code: 0, message: 'ok', data: null }))
    await api.get('/test')
    const opts = vi.mocked(fetch).mock.calls[0][1] as RequestInit
    expect(opts.headers).not.toHaveProperty('X-CSRF-Token')
  })

  it('sends credentials: include', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(mockResponse(200, { code: 0, message: 'ok', data: null }))
    await api.get('/test')
    const opts = vi.mocked(fetch).mock.calls[0][1] as RequestInit
    expect(opts.credentials).toBe('include')
  })

  it('handles network error', async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new TypeError('Failed to fetch'))
    const err = (await api.get('/test').catch((e: unknown) => e)) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.message).toBe('网络错误，请检查连接')
  })

  it('does NOT redirect on 401 when skipAuthRedirect=true', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(mockResponse(401, { code: 4010, message: '未认证' }))
    await api.get('/test', { skipAuthRedirect: true }).catch(() => {})
    expect(window.location.assign).not.toHaveBeenCalled()
  })

  it('ApiError preserves code', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(mockResponse(200, { code: 4030, message: '账户已被禁用' }))
    try {
      await api.get('/test', { skipAuthRedirect: true })
      expect.fail('should have thrown')
    } catch (err: unknown) {
      expect(err).toBeInstanceOf(ApiError)
      expect((err as ApiError).code).toBe(4030)
    }
  })
})
