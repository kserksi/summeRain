// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import type { WorkerResponse } from "./types";

const PROBE_TIMEOUT_MS = 20_000;
const WARM_WORKER_IDLE_MS = 30_000;

let probePromise: Promise<boolean> | undefined;
let probeWorker: Worker | undefined;
let warmedWorker: Worker | undefined;
let warmedWorkerTimeout: number | undefined;

export function canAttemptWasmVips(): boolean {
  const memory = (navigator as Navigator & { deviceMemory?: number }).deviceMemory;
  return (
    window.crossOriginIsolated &&
    typeof SharedArrayBuffer !== "undefined" &&
    typeof Worker !== "undefined" &&
    (memory === undefined || memory >= 4)
  );
}

export function probeWasmVips(signal?: AbortSignal): Promise<boolean> {
  throwIfAborted(signal);
  if (!canAttemptWasmVips()) return Promise.resolve(false);
  probePromise ??= performProbe();
  return waitWithSignal(probePromise, signal);
}

export function createVipsWorker(): Worker {
  return new Worker(new URL("./vips-processor.worker.ts", import.meta.url), {
    type: "module",
  });
}

export function takeWarmedVipsWorker(): Worker | undefined {
  const worker = warmedWorker;
  warmedWorker = undefined;
  if (warmedWorkerTimeout !== undefined) window.clearTimeout(warmedWorkerTimeout);
  warmedWorkerTimeout = undefined;
  return worker;
}

export function resetWasmVipsProbe(): void {
  probeWorker?.terminate();
  probeWorker = undefined;
  warmedWorker?.terminate();
  warmedWorker = undefined;
  if (warmedWorkerTimeout !== undefined) window.clearTimeout(warmedWorkerTimeout);
  warmedWorkerTimeout = undefined;
  probePromise = undefined;
}

function performProbe(): Promise<boolean> {
  return new Promise((resolve) => {
    let worker: Worker;
    try {
      worker = createVipsWorker();
    } catch {
      resolve(false);
      return;
    }
    probeWorker = worker;
    const id = createRequestID();
    let settled = false;
    const timeout = window.setTimeout(() => finish(false), PROBE_TIMEOUT_MS);

    const cleanup = (keepWarm: boolean) => {
      window.clearTimeout(timeout);
      worker.onmessage = null;
      worker.onerror = null;
      if (probeWorker === worker) probeWorker = undefined;
      if (!keepWarm) worker.terminate();
    };
    const finish = (available: boolean) => {
      if (settled) return;
      settled = true;
      cleanup(available);
      if (available) keepWorkerWarm(worker);
      resolve(available);
    };

    worker.onmessage = (event: MessageEvent<WorkerResponse>) => {
      const message = event.data;
      if (message.id !== id) return;
      finish(message.type === "ready");
    };
    worker.onerror = () => finish(false);
    try {
      worker.postMessage({ type: "probe", id });
    } catch {
      finish(false);
    }
  });
}

function keepWorkerWarm(worker: Worker): void {
  warmedWorker?.terminate();
  if (warmedWorkerTimeout !== undefined) window.clearTimeout(warmedWorkerTimeout);
  warmedWorker = worker;
  warmedWorkerTimeout = window.setTimeout(() => {
    if (warmedWorker === worker) {
      warmedWorker.terminate();
      warmedWorker = undefined;
    }
    warmedWorkerTimeout = undefined;
  }, WARM_WORKER_IDLE_MS);
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

