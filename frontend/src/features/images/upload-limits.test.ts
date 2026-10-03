// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  ACTIVE_SESSION_CONCURRENCY_CAP,
  deviceNativeConcurrency,
  MAX_SOURCE_BYTES_CAP,
  NATIVE_CONCURRENCY_CAP,
  PIPELINE_CONCURRENCY_CAP,
  resolveUploadLimits,
} from "./upload-limits";
import type { V2RecipeResponse } from "./v2-upload";

function recipe(hints: Partial<V2RecipeResponse> = {}): V2RecipeResponse {
  return {
    pipeline_version: 2,
    recipe_version: "2.0.0",
    max_part_bytes: 64 * 1024 * 1024,
    max_pixels: 50_000_000,
    session_ttl_ms: 30 * 60 * 1000,
    variants: [],
    ...hints,
  };
}

describe("resolveUploadLimits", () => {
  it("keeps the shipped values when no recipe is available", () => {
    expect(resolveUploadLimits(undefined, 2)).toEqual({
      pipelineConcurrency: PIPELINE_CONCURRENCY_CAP,
      activeSessionConcurrency: ACTIVE_SESSION_CONCURRENCY_CAP,
      nativeConcurrency: 2,
      maxSourceBytes: MAX_SOURCE_BYTES_CAP,
      invalidHints: [],
    });
  });

  it("keeps the shipped values for a server that does not send hints", () => {
    expect(resolveUploadLimits(recipe(), 2)).toEqual({
      pipelineConcurrency: PIPELINE_CONCURRENCY_CAP,
      activeSessionConcurrency: ACTIVE_SESSION_CONCURRENCY_CAP,
      nativeConcurrency: 2,
      maxSourceBytes: MAX_SOURCE_BYTES_CAP,
      invalidHints: [],
    });
  });

  it("adopts smaller server limits", () => {
    const limits = resolveUploadLimits(
      recipe({
        client_pipeline_concurrency: 1,
        client_active_session_concurrency: 2,
        client_max_native_concurrency: 1,
        max_source_bytes: 8 * 1024 * 1024,
      }),
      2,
    );
    expect(limits).toEqual({
      pipelineConcurrency: 1,
      activeSessionConcurrency: 2,
      nativeConcurrency: 1,
      maxSourceBytes: 8 * 1024 * 1024,
      invalidHints: [],
    });
  });

  it("never raises a limit above the frontend caps", () => {
    const limits = resolveUploadLimits(
      recipe({
        client_pipeline_concurrency: 8,
        client_active_session_concurrency: 32,
        client_max_native_concurrency: 4,
        max_source_bytes: 64 * 1024 * 1024,
      }),
      2,
    );
    expect(limits).toEqual({
      pipelineConcurrency: PIPELINE_CONCURRENCY_CAP,
      activeSessionConcurrency: ACTIVE_SESSION_CONCURRENCY_CAP,
      nativeConcurrency: NATIVE_CONCURRENCY_CAP,
      maxSourceBytes: MAX_SOURCE_BYTES_CAP,
      invalidHints: [],
    });
  });

  it("keeps the device concurrency when it is below the server hint", () => {
    expect(resolveUploadLimits(recipe({ client_max_native_concurrency: 4 }), 1)).toMatchObject({
      nativeConcurrency: 1,
    });
    expect(resolveUploadLimits(recipe({ client_max_native_concurrency: 2 }), 2)).toMatchObject({
      nativeConcurrency: 2,
    });
  });

  it("reports unusable hints and falls back to the caps", () => {
    for (const value of [0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1, "2", null, true]) {
      const limits = resolveUploadLimits(
        recipe({
          client_pipeline_concurrency: value,
          client_active_session_concurrency: value,
          client_max_native_concurrency: value,
          max_source_bytes: value,
        }),
        2,
      );
      expect(limits.pipelineConcurrency).toBe(PIPELINE_CONCURRENCY_CAP);
      expect(limits.activeSessionConcurrency).toBe(ACTIVE_SESSION_CONCURRENCY_CAP);
      expect(limits.nativeConcurrency).toBe(2);
      expect(limits.maxSourceBytes).toBe(MAX_SOURCE_BYTES_CAP);
      expect(limits.invalidHints).toEqual([
        "client_pipeline_concurrency",
        "client_active_session_concurrency",
        "client_max_native_concurrency",
        "max_source_bytes",
      ]);
    }
  });

  it("treats a large safe integer as usable and caps it", () => {
    const limits = resolveUploadLimits(recipe({ client_pipeline_concurrency: 1_000_000 }), 2);
    expect(limits.pipelineConcurrency).toBe(PIPELINE_CONCURRENCY_CAP);
    expect(limits.invalidHints).toEqual([]);
  });
});

describe("deviceNativeConcurrency", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("halves the reported core count and stays between one and two", () => {
    vi.stubGlobal("navigator", { hardwareConcurrency: 16 });
    expect(deviceNativeConcurrency()).toBe(2);
    vi.stubGlobal("navigator", { hardwareConcurrency: 3 });
    expect(deviceNativeConcurrency()).toBe(1);
    vi.stubGlobal("navigator", { hardwareConcurrency: 1 });
    expect(deviceNativeConcurrency()).toBe(1);
  });

  it("falls back to one worker when the core count is unknown", () => {
    vi.stubGlobal("navigator", {});
    expect(deviceNativeConcurrency()).toBe(1);
  });
});
