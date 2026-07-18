// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ProcessedImage } from "./types";

const nativeResult: ProcessedImage = {
  source: { mime_type: "image/jpeg", width: 1, height: 1, animated: false },
  processor_version: "native-test",
  recipe_version: "2.0.0",
  parts: [],
};

const { processWithNative, sniffInput } = vi.hoisted(() => ({
  processWithNative: vi.fn(),
  sniffInput: vi.fn(),
}));

vi.mock("./native-processor", () => ({ processWithNative }));
vi.mock("./sniff", () => ({ sniffInput }));

import { processClientImage } from "./processor";

describe("processClientImage", () => {
  beforeEach(() => {
    processWithNative.mockResolvedValue(nativeResult);
    sniffInput.mockResolvedValue({
      mimeType: "image/jpeg",
      animated: false,
      width: 1,
      height: 1,
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    processWithNative.mockClear();
    sniffInput.mockReset();
  });

  it("does not read the source when processing is already aborted", async () => {
    const controller = new AbortController();
    controller.abort(new DOMException("page left", "AbortError"));

    await expect(
      processClientImage(testFile(), vi.fn(), controller.signal),
    ).rejects.toMatchObject({ name: "AbortError", message: "page left" });
    expect(sniffInput).not.toHaveBeenCalled();
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

    await expect(processClientImage(testFile(), vi.fn())).resolves.toBe(nativeResult);
    expect(processWithNative).toHaveBeenCalledOnce();
  });

  it("falls back to pica when the worker fails before wasm-vips initializes", async () => {
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

    await expect(processClientImage(testFile(), vi.fn())).resolves.toBe(nativeResult);
    expect(processWithNative).toHaveBeenCalledOnce();
  });

  it("cleans up and falls back when worker.postMessage throws synchronously", async () => {
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

    await expect(processClientImage(testFile(), vi.fn())).resolves.toBe(nativeResult);
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

    await expect(processClientImage(testFile(), vi.fn())).resolves.toMatchObject({
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

    await expect(processClientImage(testFile(), vi.fn())).rejects.toThrow(
      "WASM image processing worker failed",
    );
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("requires wasm-vips instead of allocating an unsafe native canvas", async () => {
    vi.stubGlobal("crossOriginIsolated", false);
    sniffInput.mockResolvedValue({
      mimeType: "image/jpeg",
      animated: false,
      width: 5_000,
      height: 5_000,
    });

    await expect(processClientImage(testFile(), vi.fn())).rejects.toThrow(
      "too large for safe Canvas processing on this device; WASM image processing is required",
    );
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("does not fall back to Canvas for an unsafe image when wasm-vips is unavailable", async () => {
    enableWasmPath();
    sniffInput.mockResolvedValue({
      mimeType: "image/jpeg",
      animated: false,
      width: 5_000,
      height: 5_000,
    });
    vi.stubGlobal(
      "Worker",
      class {
        constructor() {
          throw new TypeError("Module workers are unsupported");
        }
      },
    );

    await expect(processClientImage(testFile(), vi.fn())).rejects.toThrow(
      "WASM image processing is required",
    );
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("routes a 50 MP image through wasm-vips without allocating a native canvas", async () => {
    enableWasmPath();
    sniffInput.mockResolvedValue({
      mimeType: "image/jpeg",
      animated: false,
      width: 8_000,
      height: 6_250,
    });
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

    await expect(processClientImage(testFile(), vi.fn())).resolves.toMatchObject({
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
    const result = processClientImage(testFile(), vi.fn(), controller.signal);
    await posted;

    controller.abort(new DOMException("page left", "AbortError"));

    await expect(result).rejects.toMatchObject({ name: "AbortError", message: "page left" });
    expect(terminate).toHaveBeenCalledOnce();
    expect(processWithNative).not.toHaveBeenCalled();
  });

  it("does not release native processing before its cleanup finishes on abort", async () => {
    let resolveNative: (value: ProcessedImage) => void = () => {};
    processWithNative.mockReturnValue(
      new Promise<ProcessedImage>((resolve) => {
        resolveNative = resolve;
      }),
    );
    const controller = new AbortController();
    const reason = new DOMException("page left", "AbortError");
    const result = processClientImage(testFile(), vi.fn(), controller.signal);
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

function testFile(): File {
  return new File([new Uint8Array([0xff, 0xd8, 0xff])], "photo.jpg", {
    type: "image/jpeg",
  });
}
