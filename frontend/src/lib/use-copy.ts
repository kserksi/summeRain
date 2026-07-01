// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

/**
 * Copy-to-clipboard with reliable async + toast feedback + transient
 * `copied` flag for visual indicator (e.g. icon swap).
 *
 * @param timeout how long `copied` stays true after a successful copy, ms
 */
export function useCopy(timeout = 1500): {
  copied: boolean
  copy: (text: string, successMsg?: string) => Promise<boolean>
} {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const copy = useCallback(
    async (text: string, successMsg?: string): Promise<boolean> => {
      try {
        await navigator.clipboard.writeText(text)
        setCopied(true)
        toast.success(successMsg ?? t('common.copySuccess'))
        if (timerRef.current) clearTimeout(timerRef.current)
        timerRef.current = setTimeout(() => setCopied(false), timeout)
        return true
      } catch {
        toast.error(t('common.copyFailed'))
        return false
      }
    },
    [t, timeout],
  )

  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    },
    [],
  )

  return { copied, copy }
}
