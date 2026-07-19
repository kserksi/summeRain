// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  canAttemptWasmVips,
  probeWasmVips,
  resetWasmVipsProbe,
  takeWarmedVipsWorker,
} from "./wasm-capability";

const originalDeviceMemory = Object.getOwnPropertyDescriptor(navigator, "deviceMemory");

afterEach(() => {
  resetWasmVipsProbe();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  if (originalDeviceMemory) {
    Object.defineProperty(navigator, "deviceMemory", originalDeviceMemory);
  } else {
    Reflect.deleteProperty(navigator, "deviceMemory");
  }
});

describe("wasm-vips capability probing", () => {
  it("requires cross-origin isolation and shared memory", () => {
    vi.stubGlobal("Worker", class {});
    vi.stubGlobal("SharedArrayBuffer", class {});
    vi.stubGlobal("crossOriginIsolated", false);

    expect(canAttemptWasmVips()).toBe(false);
  });

  it("rejects reported low-memory devices but probes devices with unknown memory", () => {
    enableBaseFeatures();
    setDeviceMemory(2);
    expect(canAttemptWasmVips()).toBe(false);

    Reflect.deleteProperty(navigator, "deviceMemory");
    expect(canAttemptWasmVips()).toBe(true);
  });

  it("probes the real worker protocol once and exposes the warmed worker", async () => {
    enableBaseFeatures();
    const instances: ProbeWorker[] = [];
    vi.stubGlobal(
      "Worker",
      class extends ProbeWorker {
        constructor() {
          super("ready");
          instances.push(this);
        }
      },
    );

    await expect(Promise.all([probeWasmVips(), probeWasmVips()])).resolves.toEqual([true, true]);

    expect(instances).toHaveLength(1);
    expect(instances[0].posted).toMatchObject({ type: "probe" });
    expect(takeWarmedVipsWorker()).toBe(instances[0]);
    expect(instances[0].terminate).not.toHaveBeenCalled();
  });

  it("reports worker initialization errors as unavailable", async () => {
    enableBaseFeatures();
    const instances: ProbeWorker[] = [];
    vi.stubGlobal(
      "Worker",
      class extends ProbeWorker {
        constructor() {
          super("error");
          instances.push(this);
        }
      },
    );

    await expect(probeWasmVips()).resolves.toBe(false);
    await expect(probeWasmVips()).resolves.toBe(false);

    expect(instances).toHaveLength(1);
    expect(instances[0].terminate).toHaveBeenCalledOnce();
  });

  it("lets an aborted caller stop waiting without cancelling the shared probe", async () => {
    enableBaseFeatures();
    const instances: ProbeWorker[] = [];
    vi.stubGlobal(
      "Worker",
      class extends ProbeWorker {
        constructor() {
          super("manual");
          instances.push(this);
        }
      },
    );
    const controller = new AbortController();
    const result = probeWasmVips(controller.signal);

    controller.abort(new DOMException("page left", "AbortError"));

    await expect(result).rejects.toMatchObject({ name: "AbortError", message: "page left" });
    instances[0].replyReady();
    await expect(probeWasmVips()).resolves.toBe(true);
  });
});

class ProbeWorker {
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: ErrorEvent) => void) | null = null;
  posted?: { type: string; id: string };
  readonly terminate = vi.fn();
  private readonly response: "ready" | "error" | "manual";

  constructor(response: "ready" | "error" | "manual") {
    this.response = response;
  }

  postMessage(message: { type: string; id: string }) {
    this.posted = message;
    if (this.response === "ready") queueMicrotask(() => this.replyReady());
    if (this.response === "error") queueMicrotask(() => this.onerror?.(new ErrorEvent("error")));
  }

  replyReady() {
    if (!this.posted) return;
    this.onmessage?.({
      data: { type: "ready", id: this.posted.id },
    } as MessageEvent);
  }
}

function enableBaseFeatures(): void {
  vi.stubGlobal("crossOriginIsolated", true);
  vi.stubGlobal("SharedArrayBuffer", class {});
  vi.stubGlobal("Worker", class {});
  setDeviceMemory(4);
}

function setDeviceMemory(value: number): void {
  Object.defineProperty(navigator, "deviceMemory", {
    configurable: true,
    value,
  });
}
