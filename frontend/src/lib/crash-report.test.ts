// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { reportClientError } = vi.hoisted(() => ({ reportClientError: vi.fn() }));

vi.mock("@/lib/api", () => ({ reportClientError }));

async function loadCrashReport() {
  vi.resetModules();
  return import("./crash-report");
}

describe("sanitizeCrashReport", () => {
  beforeEach(() => {
    reportClientError.mockReset();
    reportClientError.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("truncates every field and uses only the current pathname", async () => {
    const { CRASH_REPORT_LIMITS, sanitizeCrashReport } = await loadCrashReport();

    const payload = sanitizeCrashReport({
      kind: "boundary",
      message: `  ${"m".repeat(400)}  `,
      stack: "s".repeat(3000),
      componentStack: "c".repeat(3000),
      source: "s".repeat(400),
    });

    expect(payload?.message).toHaveLength(CRASH_REPORT_LIMITS.message);
    expect(payload?.stack).toHaveLength(CRASH_REPORT_LIMITS.stack);
    expect(payload?.component_stack).toHaveLength(CRASH_REPORT_LIMITS.componentStack);
    expect(payload?.source).toHaveLength(CRASH_REPORT_LIMITS.source);
    expect(payload?.path).toBe(window.location.pathname);
  });

  it("drops empty optional fields and refuses an empty message", async () => {
    const { sanitizeCrashReport } = await loadCrashReport();

    expect(sanitizeCrashReport({ kind: "error", message: " boom ", stack: "   " })).toEqual({
      kind: "error",
      message: "boom",
      path: window.location.pathname,
    });
    expect(sanitizeCrashReport({ kind: "error", message: "   " })).toBeUndefined();
  });

  it("strips query strings and fragments from the source location", async () => {
    const { sanitizeCrashReport } = await loadCrashReport();

    const payload = sanitizeCrashReport({
      kind: "error",
      message: "boom",
      source: "https://host/assets/app.js?token=secret#frag",
    });

    expect(payload?.source).toBe("https://host/assets/app.js");
  });

  it("strips query strings and fragments", async () => {
    const { stripQueryAndFragment } = await loadCrashReport();
    expect(stripQueryAndFragment("/i/abc.webp?token=secret#frag")).toBe("/i/abc.webp");
  });
});

describe("reportClientCrash", () => {
  beforeEach(() => {
    reportClientError.mockReset();
    reportClientError.mockResolvedValue(undefined);
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-03T00:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("reports at most three times per minute", async () => {
    const { reportClientCrash } = await loadCrashReport();

    for (let index = 0; index < 5; index += 1) {
      reportClientCrash({ kind: "error", message: `failure ${index}` });
    }
    expect(reportClientError).toHaveBeenCalledTimes(3);

    vi.advanceTimersByTime(61_000);
    reportClientCrash({ kind: "error", message: "failure later" });
    expect(reportClientError).toHaveBeenCalledTimes(4);
  });

  it("suppresses the same fingerprint within five seconds", async () => {
    const { reportClientCrash } = await loadCrashReport();

    reportClientCrash({ kind: "error", message: "same" });
    reportClientCrash({ kind: "error", message: "same" });
    expect(reportClientError).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(6_000);
    reportClientCrash({ kind: "error", message: "same" });
    expect(reportClientError).toHaveBeenCalledTimes(2);
  });

  it("never throws when the transport fails", async () => {
    reportClientError.mockRejectedValueOnce(new Error("offline"));
    const { reportClientCrash } = await loadCrashReport();

    expect(() => reportClientCrash({ kind: "error", message: "boom" })).not.toThrow();
    await Promise.resolve();
  });
});
