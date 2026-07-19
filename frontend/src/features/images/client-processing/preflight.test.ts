// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { V2RecipeResponse } from "../v2-upload";

const { getNativeCanvasCapability, probeWasmVips, sniffInput } = vi.hoisted(() => ({
  getNativeCanvasCapability: vi.fn(),
  probeWasmVips: vi.fn(),
  sniffInput: vi.fn(),
}));

vi.mock("./native-capability", () => ({ getNativeCanvasCapability }));
vi.mock("./sniff", () => ({ sniffInput }));
vi.mock("./wasm-capability", () => ({ probeWasmVips }));

import { preflightClientImage } from "./preflight";

describe("preflightClientImage", () => {
  beforeEach(() => {
    sniffInput.mockResolvedValue({
      mimeType: "image/jpeg",
      animated: false,
      width: 4_032,
      height: 3_024,
    });
    getNativeCanvasCapability.mockReturnValue({ safe: true, maxPixels: 13_000_000 });
    probeWasmVips.mockResolvedValue(true);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    sniffInput.mockReset();
    getNativeCanvasCapability.mockReset();
    probeWasmVips.mockReset();
  });

  it("stops before reading the source when already aborted", async () => {
    const controller = new AbortController();
    controller.abort(new DOMException("page left", "AbortError"));

    await expect(
      preflightClientImage(testFile(), recipe(), controller.signal),
    ).rejects.toMatchObject({ name: "AbortError", message: "page left" });
    expect(sniffInput).not.toHaveBeenCalled();
  });

  it("rejects animation before probing a processor", async () => {
    sniffInput.mockResolvedValue({
      mimeType: "image/webp",
      animated: true,
      width: 400,
      height: 400,
    });

    await expect(preflightClientImage(testFile(), recipe())).rejects.toMatchObject({
      code: "IMAGE_ANIMATION_UNSUPPORTED",
    });
    expect(probeWasmVips).not.toHaveBeenCalled();
  });

  it("applies a lower server pixel limit before probing wasm", async () => {
    sniffInput.mockResolvedValue({
      mimeType: "image/jpeg",
      animated: false,
      width: 6_000,
      height: 5_000,
    });

    await expect(
      preflightClientImage(testFile(), { ...recipe(), max_pixels: 25_000_000 }),
    ).rejects.toMatchObject({
      code: "IMAGE_DIMENSION_EXCEEDED",
      details: { maxMP: 25 },
    });
    expect(probeWasmVips).not.toHaveBeenCalled();
  });

  it("rejects a source edge that cannot be encoded as a WebP master", async () => {
    sniffInput.mockResolvedValue({
      mimeType: "image/png",
      animated: false,
      width: 20_000,
      height: 1_000,
    });

    await expect(preflightClientImage(testFile(), recipe())).rejects.toMatchObject({
      code: "IMAGE_DIMENSION_EXCEEDED",
      details: { maxDimension: 16_383 },
    });
    expect(probeWasmVips).not.toHaveBeenCalled();
  });

  it("negotiates a 50 MP image onto a probed wasm processor", async () => {
    sniffInput.mockResolvedValue({
      mimeType: "image/jpeg",
      animated: false,
      width: 8_000,
      height: 6_250,
    });
    getNativeCanvasCapability.mockReturnValue({
      safe: false,
      maxPixels: 13_000_000,
      message: "unsafe",
    });

    await expect(preflightClientImage(testFile(), recipe())).resolves.toMatchObject({
      processor: "wasm-vips",
      nativeFallbackSafe: false,
      input: { width: 8_000, height: 6_250 },
    });
  });

  it("uses the native processor only for a source inside its safety budget", async () => {
    probeWasmVips.mockResolvedValue(false);

    await expect(preflightClientImage(testFile(), recipe())).resolves.toMatchObject({
      processor: "native-pica",
      nativeFallbackSafe: true,
    });
  });

  it("fails closed when wasm is unavailable and Canvas would be unsafe", async () => {
    probeWasmVips.mockResolvedValue(false);
    getNativeCanvasCapability.mockReturnValue({
      safe: false,
      maxPixels: 13_000_000,
      message: "unsafe",
    });

    await expect(preflightClientImage(testFile(), recipe())).rejects.toMatchObject({
      code: "IMAGE_PROCESSOR_UNAVAILABLE",
      details: { maxMP: 13 },
    });
  });

  it("does not let an aborted caller wait on shared inspection work", async () => {
    let resolveInspection: (value: unknown) => void = () => {};
    sniffInput.mockReturnValue(
      new Promise((resolve) => {
        resolveInspection = resolve;
      }),
    );
    const controller = new AbortController();
    const result = preflightClientImage(testFile(), recipe(), controller.signal);

    controller.abort(new DOMException("page left", "AbortError"));

    await expect(result).rejects.toMatchObject({ name: "AbortError", message: "page left" });
    resolveInspection({ mimeType: "image/jpeg", animated: false, width: 1, height: 1 });
  });
});

function recipe(): V2RecipeResponse {
  return {
    pipeline_version: 2,
    recipe_version: "2.0.0",
    max_part_bytes: 64 * 1024 * 1024,
    max_pixels: 50_000_000,
    session_ttl_ms: 30 * 60 * 1000,
    variants: [
      { kind: "master", quality: 80, fit: "original" },
      { kind: "gallery", width: 400, height: 400, quality: 60, fit: "cover" },
      { kind: "admin", width: 120, height: 160, quality: 60, fit: "cover" },
      { kind: "publish_source", long_edge: 2048, quality: 80, fit: "contain" },
    ],
  };
}

function testFile(): File {
  return new File([new Uint8Array([0xff, 0xd8, 0xff])], "photo.jpg", {
    type: "image/jpeg",
  });
}

