// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ClientProcessingPlan } from "./preflight";
import type { ProcessedImage } from "./types";
import { resetWasmVipsProbe } from "./wasm-capability";

const nativeResult: ProcessedImage = {
  source: { mime_type: "image/jpeg", width: 1, height: 1, animated: false },
  processor_version: "native-test",
  recipe_version: "2.0.0",
  parts: [],
};

const { processWithNative } = vi.hoisted(() => ({
  processWithNative: vi.fn(),
}));

vi.mock("./native-processor", () => ({ processWithNative }));

import { processClientImage } from "./processor";

describe("processClientImage", () => {
  beforeEach(() => {
    processWithNative.mockResolvedValue(nativeResult);
  });

  afterEach(() => {
    resetWasmVipsProbe();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    processWithNative.mockReset();
  });

  it("does not process the source when already aborted", async () => {
    const controller = new AbortController();
    controller.abort(new DOMException("page left", "AbortError"));

    await expect(
      processClientImage(testFile(), nativePlan(), vi.fn(), controller.signal),
    ).rejects.toMatchObject({ name: "AbortError", message: "page left" });
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("falls back to pica when a module worker cannot be constructed", async () => {
    enableWasmPath();
    vi.stubGlobal(
      "Worker",
      class {
        constructor() {
          throw new TypeError("Module workers are unsupported");
        }
      },
    );

    await expect(processClientImage(testFile(), wasmPlan(true), vi.fn())).resolves.toBe(
      nativeResult,
    );
    expect(processWithNative).toHaveBeenCalledOnce();
  });

  it("falls back to pica when the worker fails before processing initializes", async () => {
    enableWasmPath();
    vi.stubGlobal(
      "Worker",
      class {
        onmessage: ((event: MessageEvent) => void) | null = null;
        onerror: ((event: ErrorEvent) => void) | null = null;

        postMessage() {
          queueMicrotask(() => this.onerror?.(new ErrorEvent("error")));
        }

        terminate() {}
      },
    );

    await expect(processClientImage(testFile(), wasmPlan(true), vi.fn())).resolves.toBe(
      nativeResult,
    );
    expect(processWithNative).toHaveBeenCalledOnce();
  });

  it("cleans up and falls back when worker.postMessage throws", async () => {
    enableWasmPath();
    const terminate = vi.fn();
    vi.stubGlobal(
      "Worker",
      class {
        onmessage: ((event: MessageEvent) => void) | null = null;
        onerror: ((event: ErrorEvent) => void) | null = null;

        postMessage() {
          throw new DOMException("could not clone", "DataCloneError");
        }

        terminate = terminate;
      },
    );

    await expect(processClientImage(testFile(), wasmPlan(true), vi.fn())).resolves.toBe(
      nativeResult,
    );
    expect(processWithNative).toHaveBeenCalledOnce();
    expect(terminate).toHaveBeenCalledOnce();
  });

  it("uses a random-value request ID when randomUUID is unavailable", async () => {
    enableWasmPath();
    const getRandomValues = globalThis.crypto.getRandomValues.bind(globalThis.crypto);
    vi.stubGlobal("crypto", { getRandomValues });
    vi.stubGlobal(
      "Worker",
      class {
        onmessage: ((event: MessageEvent) => void) | null = null;
        onerror: ((event: ErrorEvent) => void) | null = null;

        postMessage(message: { id: string }) {
          queueMicrotask(() =>
            this.onmessage?.({
              data: {
                type: "result",
                id: message.id,
                result: {
                  source: {
                    mime_type: "image/jpeg",
                    width: 1,
                    height: 1,
                    animated: false,
                  },
                  recipe_version: "2.0.0",
                  parts: [],
                },
              },
            } as MessageEvent),
          );
        }

        terminate() {}
      },
    );

    await expect(processClientImage(testFile(), wasmPlan(true), vi.fn())).resolves.toMatchObject({
      processor_version: "wasm-vips-0.0.18",
    });
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("does not replay processing after wasm-vips has initialized", async () => {
    enableWasmPath();
    vi.stubGlobal(
      "Worker",
      class {
        onmessage: ((event: MessageEvent) => void) | null = null;
        onerror: ((event: ErrorEvent) => void) | null = null;

        postMessage(message: { id: string }) {
          queueMicrotask(() => {
            this.onmessage?.({
              data: { type: "progress", id: message.id, percent: 5 },
            } as MessageEvent);
            this.onerror?.(new ErrorEvent("error"));
          });
        }

        terminate() {}
      },
    );

    await expect(processClientImage(testFile(), wasmPlan(true), vi.fn())).rejects.toMatchObject({
      code: "IMAGE_PROCESSING_FAILED",
    });
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("rejects an unsafe native fallback when wasm-vips is unavailable", async () => {
    vi.stubGlobal("crossOriginIsolated", false);

    await expect(processClientImage(testFile(), wasmPlan(false), vi.fn())).rejects.toMatchObject({
      code: "IMAGE_PROCESSOR_UNAVAILABLE",
    });
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("does not fall back to Canvas for an unsafe image after worker startup fails", async () => {
    enableWasmPath();
    vi.stubGlobal(
      "Worker",
      class {
        constructor() {
          throw new TypeError("Module workers are unsupported");
        }
      },
    );

    await expect(processClientImage(testFile(), wasmPlan(false), vi.fn())).rejects.toMatchObject({
      code: "IMAGE_PROCESSOR_UNAVAILABLE",
    });
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("routes a 50 MP plan through wasm-vips without allocating a native canvas", async () => {
    enableWasmPath();
    vi.stubGlobal(
      "Worker",
      class {
        onmessage: ((event: MessageEvent) => void) | null = null;
        onerror: ((event: ErrorEvent) => void) | null = null;

        postMessage(message: { id: string }) {
          queueMicrotask(() =>
            this.onmessage?.({
              data: {
                type: "result",
                id: message.id,
                result: {
                  source: {
                    mime_type: "image/jpeg",
                    width: 8_000,
                    height: 6_250,
                    animated: false,
                  },
                  recipe_version: "2.0.0",
                  parts: [],
                },
              },
            } as MessageEvent),
          );
        }

        terminate() {}
      },
    );

    await expect(
      processClientImage(testFile(), wasmPlan(false, 8_000, 6_250), vi.fn()),
    ).resolves.toMatchObject({
      processor_version: "wasm-vips-0.0.18",
      source: { width: 8_000, height: 6_250 },
    });
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("terminates the wasm worker when processing is aborted", async () => {
    enableWasmPath();
    const terminate = vi.fn();
    let markPosted: () => void = () => {};
    const posted = new Promise<void>((resolve) => {
      markPosted = resolve;
    });
    vi.stubGlobal(
      "Worker",
      class {
        onmessage: ((event: MessageEvent) => void) | null = null;
        onerror: ((event: ErrorEvent) => void) | null = null;

        postMessage() {
          markPosted();
        }

        terminate = terminate;
      },
    );
    const controller = new AbortController();
    const result = processClientImage(testFile(), wasmPlan(false), vi.fn(), controller.signal);
    await posted;

    controller.abort(new DOMException("page left", "AbortError"));

    await expect(result).rejects.toMatchObject({ name: "AbortError", message: "page left" });
    expect(terminate).toHaveBeenCalledOnce();
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("does not release native processing before cleanup finishes on abort", async () => {
    let resolveNative: (value: ProcessedImage) => void = () => {};
    processWithNative.mockReturnValue(
      new Promise<ProcessedImage>((resolve) => {
        resolveNative = resolve;
      }),
    );
    const controller = new AbortController();
    const reason = new DOMException("page left", "AbortError");
    const result = processClientImage(testFile(), nativePlan(), vi.fn(), controller.signal);
    await vi.waitFor(() => expect(processWithNative).toHaveBeenCalledOnce());
    let settled = false;
    void result.then(
      () => {
        settled = true;
      },
      () => {
        settled = true;
      },
    );

    controller.abort(reason);
    await Promise.resolve();
    expect(settled).toBe(false);

    resolveNative(nativeResult);
    await expect(result).rejects.toBe(reason);
  });
});

function enableWasmPath(): void {
  vi.stubGlobal("crossOriginIsolated", true);
  vi.stubGlobal("SharedArrayBuffer", class {});
}

function nativePlan(): ClientProcessingPlan {
  return {
    input: { mimeType: "image/jpeg", animated: false, width: 1, height: 1 },
    processor: "native-pica",
    nativeFallbackSafe: true,
  };
}

function wasmPlan(
  nativeFallbackSafe: boolean,
  width = 1,
  height = 1,
): ClientProcessingPlan {
  return {
    input: { mimeType: "image/jpeg", animated: false, width, height },
    processor: "wasm-vips",
    nativeFallbackSafe,
  };
}

function testFile(): File {
  return new File([new Uint8Array([0xff, 0xd8, 0xff])], "photo.jpg", {
    type: "image/jpeg",
  });
}
