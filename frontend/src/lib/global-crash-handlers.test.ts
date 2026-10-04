// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";

const { reportClientCrash, stripQueryAndFragment } = vi.hoisted(() => ({
  reportClientCrash: vi.fn(),
  stripQueryAndFragment: vi.fn((value: string) => value.split(/[?#]/)[0] ?? ""),
}));

vi.mock("@/lib/crash-report", () => ({ reportClientCrash, stripQueryAndFragment }));

import { installGlobalCrashHandlers } from "./global-crash-handlers";

describe("installGlobalCrashHandlers", () => {
  it("reports uncaught errors and unhandled rejections once installed", () => {
    installGlobalCrashHandlers();
    installGlobalCrashHandlers();

    window.dispatchEvent(
      new ErrorEvent("error", {
        message: "boom",
        filename: "https://example.test/assets/app.js?v=1",
        lineno: 10,
        colno: 5,
        error: new Error("boom"),
      }),
    );
    expect(reportClientCrash).toHaveBeenCalledTimes(1);
    expect(reportClientCrash).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "error",
        message: "boom",
        source: "https://example.test/assets/app.js:10:5",
      }),
    );

    window.dispatchEvent(
      Object.assign(new Event("unhandledrejection"), { reason: new Error("async boom") }),
    );
    expect(reportClientCrash).toHaveBeenCalledTimes(2);
    expect(reportClientCrash).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "unhandledrejection", message: "async boom" }),
    );
  });
});
