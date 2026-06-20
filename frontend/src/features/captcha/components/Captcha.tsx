import { useEffect, useRef } from 'react'
import { IconShield, IconShieldCheckFilled } from '@tabler/icons-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { CaptchaPayload } from '@/lib/types'
import { useCaptcha, loadScript, RECAPTCHA_URL, TURNSTILE_URL, GEETEST_URL } from '@/features/captcha/hooks'

export interface CaptchaProps {
  action: string
  onVerified: (payload: CaptchaPayload) => void
}

export function Captcha({ action, onVerified }: CaptchaProps) {
  const { provider, enabled, siteKey, getCaptchaPayload, containerRef } = useCaptcha(action)
  const verifiedRef = useRef(false)

  useEffect(() => {
    if (!enabled || !siteKey) return
    if (provider === 'recaptcha') void loadScript(RECAPTCHA_URL(siteKey))
    else if (provider === 'turnstile') void loadScript(TURNSTILE_URL)
    else if (provider === 'geetest_v4') void loadScript(GEETEST_URL)
  }, [enabled, provider, siteKey])

  useEffect(() => {
    if (!enabled || verifiedRef.current) return
    if (provider !== 'recaptcha' && provider !== 'turnstile') return
    let cancelled = false
    void getCaptchaPayload().then((payload) => {
      if (cancelled || verifiedRef.current || !payload) return
      verifiedRef.current = true
      onVerified(payload)
    })
    return () => {
      cancelled = true
    }
  }, [enabled, provider, getCaptchaPayload, onVerified])

  if (!enabled) return null

  if (provider === 'turnstile') {
    return <div ref={containerRef} className="min-h-[65px]" />
  }

  if (provider === 'geetest_v4') {
    return (
      <Button
        type="button"
        variant="outline"
        onClick={() => {
          void getCaptchaPayload().then((payload) => {
            if (payload) onVerified(payload)
          })
        }}
      >
        <IconShieldCheckFilled className="size-4" />
        点击验证
      </Button>
    )
  }

  return (
    <Badge variant="secondary" className={cn('gap-1 text-xs')}>
      <IconShield className="size-3" />
      受 reCAPTCHA 保护
    </Badge>
  )
}
