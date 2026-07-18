// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

/**
 * Run an async function over a list of items with at most `limit` concurrent
 * invocations. Results are returned in the same order as the input items,
 * regardless of completion timing.
 *
 * Classic "N workers draining a shared queue" pattern. `index++` is atomic
 * under JavaScript's single-threaded execution model.
 *
 * If `fn` rejects, the whole promise rejects (error propagation is NOT
 * isolated — callers must handle if they want to continue on partial failure).
 */
export async function runWithConcurrency<T, R>(
  items: readonly T[],
  fn: (item: T) => Promise<R>,
  limit: number,
): Promise<R[]> {
  const results: R[] = new Array(items.length)
  let index = 0
  await Promise.all(
    Array.from({ length: Math.min(limit, items.length) }, async () => {
      while (index < items.length) {
        const i = index++
        results[i] = await fn(items[i])
      }
    }),
  )
  return results
}

interface ConcurrencyWaiter {
  grant: () => void
  signal?: AbortSignal
  abort?: () => void
}

// Bounds work whose lifetime extends beyond a pipeline task, such as a server
// upload session that remains active while its publish job is being polled.
export class ConcurrencyGate {
  private active = 0
  private readonly limit: number
  private readonly waiters: ConcurrencyWaiter[] = []

  constructor(limit: number) {
    if (!Number.isSafeInteger(limit) || limit < 1) {
      throw new Error('Concurrency limit must be a positive integer')
    }
    this.limit = limit
  }

  acquire(signal?: AbortSignal): Promise<() => void> {
    if (signal?.aborted) return Promise.reject(signal.reason ?? abortError())

    return new Promise<() => void>((resolve, reject) => {
      const waiter: ConcurrencyWaiter = {
        signal,
        grant: () => {
          signal?.removeEventListener('abort', waiter.abort!)
          this.active += 1
          let released = false
          resolve(() => {
            if (released) return
            released = true
            this.active -= 1
            this.pump()
          })
        },
      }
      waiter.abort = () => {
        const index = this.waiters.indexOf(waiter)
        if (index >= 0) this.waiters.splice(index, 1)
        reject(signal?.reason ?? abortError())
      }

      if (this.active < this.limit) {
        waiter.grant()
        return
      }
      signal?.addEventListener('abort', waiter.abort, { once: true })
      this.waiters.push(waiter)
    })
  }

  private pump(): void {
    while (this.active < this.limit && this.waiters.length > 0) {
      this.waiters.shift()?.grant()
    }
  }
}

function abortError(): DOMException {
  return new DOMException('Concurrency wait aborted', 'AbortError')
}
