// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { getNativeCanvasCapability } from "./native-capability";
import { processWithNative } from "./native-processor";
import { sniffInput } from "./sniff";
import type { ProcessedImage, ProcessingProgress, WorkerResponse } from "./types";

export async function processClientImage(
  file: File,
  onProgress: ProcessingProgress,
  signal?: AbortSignal,
): Promise<ProcessedImage> {
  throwIfAborted(signal);
  const input = await waitWithSignal(sniffInput(file), signal);
  if (input.animated) throw new Error("Animated images are not supported in V2");
  const nativeCapability = getNativeCanvasCapability(input.width, input.height);

  if (canUseWasmVips()) {
    try {
      return await processWithVipsWorker(file, input.mimeType, onProgress, signal);
    } catch (error) {
      if (!(error instanceof VipsUnavailableError)) throw error;
      if (!nativeCapability.safe) throw new Error(nativeCapability.message);
    }
  }
  if (!nativeCapability.safe) throw new Error(nativeCapability.message);
  // Native Canvas encoding cannot be interrupted in every browser. Keep the
  // serial processing slot until its finally blocks release decoded pixels and
  // canvases, then surface the cancellation before the next image starts.
  const processed = await processWithNative(file, input.mimeType, onProgress, signal);
  throwIfAborted(signal);
  return processed;
}

class VipsUnavailableError extends Error {}

export function canUseWasmVips(): boolean {
  const memory = (navigator as Navigator & { deviceMemory?: number }).deviceMemory ?? 4;
  return (
    window.crossOriginIsolated &&
    typeof SharedArrayBuffer !== "undefined" &&
    typeof Worker !== "undefined" &&
    memory >= 4
  );
}

function processWithVipsWorker(
  file: File,
  mimeType: string,
  onProgress: ProcessingProgress,
  signal?: AbortSignal,
): Promise<ProcessedImage> {
  throwIfAborted(signal);
  return new Promise((resolve, reject) => {
    const id = createRequestID();
    let worker: Worker;
    try {
      worker = new Worker(new URL("./vips-processor.worker.ts", import.meta.url), {
        type: "module",
      });
    } catch (error) {
      reject(
        new VipsUnavailableError(
          error instanceof Error ? error.message : "WASM image processing worker is unavailable",
        ),
      );
      return;
    }
    let initialized = false;
    let settled = false;
    const timeout = window.setTimeout(
      () => {
        finish(() => reject(new Error("Client image processing timed out")));
      },
      8 * 60 * 1000,
    );

    const cleanup = () => {
      window.clearTimeout(timeout);
      signal?.removeEventListener("abort", abort);
      worker.terminate();
    };
    const finish = (settle: () => void) => {
      if (settled) return;
      settled = true;
      cleanup();
      settle();
    };
    const abort = () => finish(() => reject(signal?.reason ?? abortError()));
    signal?.addEventListener("abort", abort, { once: true });

    worker.onmessage = (event: MessageEvent<WorkerResponse>) => {
      if (settled) return;
      const message = event.data;
      if (message.id !== id) return;
      if (message.type === "progress") {
        initialized = true;
        onProgress(message.percent);
        return;
      }
      if (message.type === "error") {
        finish(() =>
          reject(
            message.recoverable
              ? new VipsUnavailableError(message.message)
              : new Error(message.message),
          ),
        );
        return;
      }
      finish(() =>
        resolve({ ...message.result, processor_version: "wasm-vips-0.0.18" }),
      );
    };
    worker.onerror = () => {
      finish(() =>
        reject(
          initialized
            ? new Error("WASM image processing worker failed")
            : new VipsUnavailableError("WASM image processing worker is unavailable"),
        ),
      );
    };
    try {
      throwIfAborted(signal);
      worker.postMessage({ id, file, mimeType });
    } catch (error) {
      finish(() =>
        reject(
          error instanceof DOMException && error.name === "AbortError"
            ? error
            : new VipsUnavailableError(
                error instanceof Error
                  ? error.message
                  : "WASM image processing worker is unavailable",
              ),
        ),
      );
    }
  });
}

function createRequestID(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw signal.reason ?? abortError();
}

function abortError(): DOMException {
  return new DOMException("Image processing aborted", "AbortError");
}

function waitWithSignal<T>(promise: Promise<T>, signal?: AbortSignal): Promise<T> {
  if (!signal) return promise;
  throwIfAborted(signal);
  return new Promise<T>((resolve, reject) => {
    const abort = () => reject(signal.reason ?? abortError());
    signal.addEventListener("abort", abort, { once: true });
    promise.then(
      (value) => {
        signal.removeEventListener("abort", abort);
        resolve(value);
      },
      (error: unknown) => {
        signal.removeEventListener("abort", abort);
        reject(error);
      },
    );
  });
}
