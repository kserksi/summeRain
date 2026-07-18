// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

const MAX_NATIVE_DIMENSION = 8_192;

export interface NativeCanvasCapability {
  safe: boolean;
  maxPixels: number;
  message?: string;
}

export function getNativeCanvasCapability(
  width: number,
  height: number,
): NativeCanvasCapability {
  const memory = (navigator as Navigator & { deviceMemory?: number }).deviceMemory ?? 4;
  const maxPixels = memory <= 2 ? 8_000_000 : memory >= 8 ? 16_000_000 : 13_000_000;
  const pixels = width * height;

  if (
    !Number.isSafeInteger(pixels) ||
    width <= 0 ||
    height <= 0 ||
    width > MAX_NATIVE_DIMENSION ||
    height > MAX_NATIVE_DIMENSION ||
    pixels > maxPixels
  ) {
    return {
      safe: false,
      maxPixels,
      message:
        "This image is too large for safe Canvas processing on this device; WASM image processing is required",
    };
  }

  return { safe: true, maxPixels };
}

export function assertNativeCanvasCapability(width: number, height: number): void {
  const capability = getNativeCanvasCapability(width, height);
  if (!capability.safe) throw new Error(capability.message);
}
