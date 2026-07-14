// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { IconX } from '@tabler/icons-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export interface LightboxProps {
  src: string
  onClose: () => void
}

export function Lightbox({ src, onClose }: LightboxProps) {
  const { t } = useTranslation()
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prev
    }
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
    >
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute top-4 right-4 text-white hover:bg-white/10"
        onClick={onClose}
        aria-label={t('images.shared.close')}
      >
        <IconX />
      </Button>
      <img
        src={src}
        alt=""
        onClick={(e) => e.stopPropagation()}
        className="max-h-[90vh] max-w-[90vw] scale-95 object-contain duration-200 animate-in fade-in-0 zoom-in-90"
      />
    </div>
  )
}
