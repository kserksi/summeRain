// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

import { toast } from 'sonner'
import { useCopy } from './use-copy'

describe('useCopy', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('navigator', {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    })
    vi.mocked(toast.success).mockClear()
    vi.mocked(toast.error).mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('returns copied=false initially', () => {
    const { result } = renderHook(() => useCopy())
    expect(result.current.copied).toBe(false)
  })

  it('success: awaits clipboard, sets copied=true, toasts default msg', async () => {
    const { result } = renderHook(() => useCopy())
    await act(async () => {
      const ok = await result.current.copy('hello')
      expect(ok).toBe(true)
    })
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('hello')
    expect(result.current.copied).toBe(true)
    expect(toast.success).toHaveBeenCalledWith('Copied to clipboard')
  })

  it('success: accepts custom success message', async () => {
    const { result } = renderHook(() => useCopy())
    await act(async () => {
      await result.current.copy('x', 'Token copied')
    })
    expect(toast.success).toHaveBeenCalledWith('Token copied')
  })

  it('resets copied to false after timeout', async () => {
    const { result } = renderHook(() => useCopy(1000))
    await act(async () => {
      await result.current.copy('x')
    })
    expect(result.current.copied).toBe(true)
    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(result.current.copied).toBe(false)
  })

  it('failure: rejects -> toast.error, copied stays false, returns false', async () => {
    vi.stubGlobal('navigator', {
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    })
    const { result } = renderHook(() => useCopy())
    await act(async () => {
      const ok = await result.current.copy('x')
      expect(ok).toBe(false)
    })
    expect(toast.error).toHaveBeenCalledWith('Could not copy. Check your browser permissions.')
    expect(result.current.copied).toBe(false)
  })

  it('does not setState after unmount', async () => {
    const { result, unmount } = renderHook(() => useCopy(1000))
    await act(async () => {
      await result.current.copy('x')
    })
    unmount()
    expect(() =>
      act(() => {
        vi.advanceTimersByTime(1000)
      }),
    ).not.toThrow()
  })
})
