// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'

import { runWithConcurrency } from './upload-concurrency'

describe('runWithConcurrency', () => {
  it('returns empty array for empty input', async () => {
    const result = await runWithConcurrency([], async (x) => x, 3)
    expect(result).toEqual([])
  })

  it('preserves order in results array regardless of completion timing', async () => {
    // Make later items complete faster to scramble natural completion order
    const fn = async (x: number) => {
      await new Promise((r) => setTimeout(r, 50 - x * 10))
      return x * 10
    }
    const items = [1, 2, 3, 4]
    const results = await runWithConcurrency(items, fn, 2)
    expect(results).toEqual([10, 20, 30, 40])
  })

  it('limits peak concurrency to the specified limit', async () => {
    let active = 0
    let peak = 0
    const fn = async (x: number) => {
      active++
      peak = Math.max(peak, active)
      await new Promise((r) => setTimeout(r, 10))
      active--
      return x
    }
    const items = Array.from({ length: 20 }, (_, i) => i)
    await runWithConcurrency(items, fn, 5)
    expect(peak).toBeLessThanOrEqual(5)
    expect(peak).toBeGreaterThanOrEqual(5) // should actually hit the limit with 20 items
  })

  it('limit larger than items length processes all items', async () => {
    const fn = async (x: number) => x + 1
    const items = [1, 2, 3]
    const results = await runWithConcurrency(items, fn, 10)
    expect(results).toEqual([2, 3, 4])
  })

  it('propagates rejection when fn throws', async () => {
    const fn = async (x: number) => {
      if (x === 2) throw new Error('boom')
      return x
    }
    await expect(runWithConcurrency([1, 2, 3], fn, 2)).rejects.toThrow('boom')
  })
})
