// Copyright 2026 kserks
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
