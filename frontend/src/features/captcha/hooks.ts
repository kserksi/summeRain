import { useCallback, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { QUERY_KEYS } from '@/config/constants'
import type { CaptchaPayload, PublicConfig } from '@/lib/types'
import { getPublicConfig } from '@/features/captcha/api'

interface GeetestValidate {
  lot_number?: string
  captcha_output?: string
  pass_token?: string
  gen_time?: string
}

interface GeetestCaptchaObject {
  onSuccess: (cb: () => void) => void
  onError: (cb: (err: unknown) => void) => void
  showCaptcha: () => void
  getValidate: () => GeetestValidate
}

declare global {
  interface Window {
    grecaptcha?: any
    turnstile?: any
    initGeetest4?: (
      config: Record<string, unknown>,
      cb: (obj: GeetestCaptchaObject) => void,
    ) => void
  }
}

export function usePublicConfig() {
  return useQuery<PublicConfig>({
    queryKey: QUERY_KEYS.publicConfig,
    queryFn: getPublicConfig,
    staleTime: Infinity,
  })
}

export const RECAPTCHA_URL = (siteKey: string) =>
  `https://www.google.com/recaptcha/api.js?render=${siteKey}`
export const TURNSTILE_URL = 'https://challenges.cloudflare.com/turnstile/v0/api.js'
export const GEETEST_URL = 'https://static.geetest.com/v4/gt4.js'

const scriptPromises = new Map<string, Promise<void>>()

export function loadScript(src: string): Promise<void> {
  const existing = scriptPromises.get(src)
  if (existing) return existing
  const promise = new Promise<void>((resolve, reject) => {
    const el = document.createElement('script')
    el.src = src
    el.async = true
    el.defer = true
    el.onload = () => resolve()
    el.onerror = () => reject(new Error(`Failed to load ${src}`))
    document.head.appendChild(el)
  })
  scriptPromises.set(src, promise)
  return promise
}

function waitForGlobal(getter: () => unknown, timeoutMs = 6000): Promise<void> {
  return new Promise((resolve, reject) => {
    if (getter()) return resolve()
    const start = Date.now()
    const timer = setInterval(() => {
      if (getter()) {
        clearInterval(timer)
        resolve()
      } else if (Date.now() - start > timeoutMs) {
        clearInterval(timer)
        reject(new Error('captcha library load timeout'))
      }
    }, 50)
  })
}

export function useCaptcha(action: string) {
  const { data } = usePublicConfig()
  const provider = data?.captcha_provider ?? 'none'
  const siteKey = data?.captcha_site_key ?? ''
  const enabled = provider !== 'none'

  const containerRef = useRef<HTMLDivElement | null>(null)
  const geetestObjRef = useRef<GeetestCaptchaObject | null>(null)
  const geetestResolveRef = useRef<((payload: CaptchaPayload) => void) | null>(null)
  const geetestRejectRef = useRef<((err: unknown) => void) | null>(null)

  const getCaptchaPayload = useCallback(async (): Promise<CaptchaPayload | undefined> => {
    if (provider === 'recaptcha' && siteKey) {
      await loadScript(RECAPTCHA_URL(siteKey))
      await waitForGlobal(() => window.grecaptcha?.execute)
      const token: string = await window.grecaptcha.execute(siteKey, { action })
      return { provider: 'recaptcha', token, action }
    }

    if (provider === 'turnstile' && siteKey) {
      const container = containerRef.current
      if (!container) return undefined
      await loadScript(TURNSTILE_URL)
      await waitForGlobal(() => window.turnstile?.render)
      return await new Promise<CaptchaPayload>((resolve, reject) => {
        window.turnstile.render(container, {
          sitekey: siteKey,
          callback: (token: string) => resolve({ provider: 'turnstile', token }),
          'error-callback': (err: unknown) => reject(err),
        })
      })
    }

    if (provider === 'geetest_v4' && siteKey) {
      await loadScript(GEETEST_URL)
      await waitForGlobal(() => window.initGeetest4)
      if (!geetestObjRef.current) {
        await new Promise<void>((resolve) => {
          window.initGeetest4!({ captchaId: siteKey, product: 'bind' }, (obj) => {
            obj.onSuccess(() => {
              const validate = obj.getValidate()
              geetestResolveRef.current?.({
                provider: 'geetest_v4',
                lot_number: validate.lot_number,
                captcha_output: validate.captcha_output,
                pass_token: validate.pass_token,
                gen_time: validate.gen_time,
              })
            })
            obj.onError((err) => {
              geetestRejectRef.current?.(err)
            })
            geetestObjRef.current = obj
            resolve()
          })
        })
      }
      const obj = geetestObjRef.current
      return await new Promise<CaptchaPayload>((resolve, reject) => {
        geetestResolveRef.current = resolve
        geetestRejectRef.current = reject
        obj?.showCaptcha()
      })
    }

    return undefined
  }, [provider, siteKey, action])

  return { provider, enabled, siteKey, getCaptchaPayload, containerRef }
}
