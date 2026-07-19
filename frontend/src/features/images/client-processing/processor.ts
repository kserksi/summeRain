// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { ClientImageError, isClientImageError } from "./errors";
import { processWithNative } from "./native-processor";
import type { ClientProcessingPlan } from "./preflight";
import type { ProcessedImage, ProcessingProgress, WorkerResponse } from "./types";
import {
  canAttemptWasmVips,
  createVipsWorker,
  takeWarmedVipsWorker,
} from "./wasm-capability";

export { canAttemptWasmVips as canUseWasmVips } from "./wasm-capability";

export async function processClientImage(
  file: File,
  plan: ClientProcessingPlan,
  onProgress: ProcessingProgress,
  signal?: AbortSignal,
): Promise<ProcessedImage> {
  throwIfAborted(signal);
  if (plan.processor === "wasm-vips" && canAttemptWasmVips()) {
    try {
      return await processWithVipsWorker(file, plan.input.mimeType, onProgress, signal);
    } catch (error) {
      if (!(error instanceof VipsUnavailableError)) throw error;
      if (!plan.nativeFallbackSafe) {
        throw new ClientImageError(
          "IMAGE_PROCESSOR_UNAVAILABLE",
          "WASM image processing is required for this image",
          { cause: error },
        );
      }
    }
  }
  if (!plan.nativeFallbackSafe) {
    throw new ClientImageError(
      "IMAGE_PROCESSOR_UNAVAILABLE",
      "WASM image processing is required for this image",
    );
  }
  // Native Canvas encoding cannot be interrupted in every browser. Keep the
  // serial processing slot until its finally blocks release decoded pixels and
  // canvases, then surface the cancellation before the next image starts.
  try {
    const processed = await processWithNative(file, plan.input.mimeType, onProgress, signal);
    throwIfAborted(signal);
    return processed;
  } catch (error) {
    if (isAbortError(error) || isClientImageError(error)) throw error;
    const message = error instanceof Error ? error.message : "Native image processing failed";
    if (/webp|encode/i.test(message)) {
      throw new ClientImageError(
        "IMAGE_WEBP_ENCODE_UNAVAILABLE",
        "This browser cannot encode WebP images",
        { cause: error },
      );
    }
    if (/decode/i.test(message)) {
      throw new ClientImageError("IMAGE_DECODE_FAILED", "This browser cannot decode the image", {
        cause: error,
      });
    }
    throw new ClientImageError("IMAGE_PROCESSING_FAILED", "Native image processing failed", {
      cause: error,
      retryable: true,
    });
  }
}

class VipsUnavailableError extends Error {}

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
      worker = takeWarmedVipsWorker() ?? createVipsWorker();
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
        finish(() =>
          reject(
            new ClientImageError(
              "IMAGE_PROCESSING_TIMEOUT",
              "Client image processing timed out",
              { retryable: true },
            ),
          ),
        );
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
      if (message.type === "ready") return;
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
              : new ClientImageError("IMAGE_PROCESSING_FAILED", message.message),
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
            ? new ClientImageError(
                "IMAGE_PROCESSING_FAILED",
                "WASM image processing worker failed",
                { retryable: true },
              )
            : new VipsUnavailableError("WASM image processing worker is unavailable"),
        ),
      );
    };
    try {
      throwIfAborted(signal);
      worker.postMessage({ type: "process", id, file, mimeType });
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

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
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
