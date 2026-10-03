// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import type { V2RecipeResponse } from "./v2-upload";

// Build-time safety caps. A server policy can only lower these values, never
// raise them: a slow, metered, or memory-constrained browser must stay
// responsive even when the server is sized for a larger host.
export const PIPELINE_CONCURRENCY_CAP = 2;
export const ACTIVE_SESSION_CONCURRENCY_CAP = 4;
export const NATIVE_CONCURRENCY_CAP = 2;
export const MAX_SOURCE_BYTES_CAP = 15 * 1024 * 1024;

export interface UploadLimits {
  pipelineConcurrency: number;
  activeSessionConcurrency: number;
  nativeConcurrency: number;
  maxSourceBytes: number;
  // Hint names the server sent but this client could not use. Callers surface
  // them so a broken deployment is noticed instead of silently ignored.
  invalidHints: string[];
}

export function deviceNativeConcurrency(): number {
  const cores =
    (navigator as Navigator & { hardwareConcurrency?: number }).hardwareConcurrency ?? 2;
  return Math.max(1, Math.min(NATIVE_CONCURRENCY_CAP, Math.floor(cores / 2)));
}

// resolveUploadLimits combines server policy, device capability, and the
// build-time caps: effective = min(server hint, device capability, cap).
// A missing hint keeps the value this client shipped with, which is what an
// older server that does not send the hint expects; an unusable hint falls back
// to the cap and is reported through invalidHints.
export function resolveUploadLimits(
  recipe: V2RecipeResponse | undefined,
  deviceConcurrency = deviceNativeConcurrency(),
): UploadLimits {
  const invalidHints: string[] = [];
  return {
    pipelineConcurrency: boundedHint(
      recipe?.client_pipeline_concurrency,
      PIPELINE_CONCURRENCY_CAP,
      "client_pipeline_concurrency",
      invalidHints,
    ),
    activeSessionConcurrency: boundedHint(
      recipe?.client_active_session_concurrency,
      ACTIVE_SESSION_CONCURRENCY_CAP,
      "client_active_session_concurrency",
      invalidHints,
    ),
    nativeConcurrency: Math.min(
      deviceConcurrency,
      boundedHint(
        recipe?.client_max_native_concurrency,
        NATIVE_CONCURRENCY_CAP,
        "client_max_native_concurrency",
        invalidHints,
      ),
    ),
    maxSourceBytes: boundedHint(
      recipe?.max_source_bytes,
      MAX_SOURCE_BYTES_CAP,
      "max_source_bytes",
      invalidHints,
    ),
    invalidHints,
  };
}

function boundedHint(
  value: unknown,
  maximum: number,
  name: string,
  invalidHints: string[],
): number {
  if (value === undefined) return maximum;
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 1) {
    invalidHints.push(name);
    return maximum;
  }
  return Math.min(value, maximum);
}
