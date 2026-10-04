// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { reportClientCrash, stripQueryAndFragment } from "@/lib/crash-report";

let installed = false;

// installGlobalCrashHandlers reports uncaught errors and unhandled promise
// rejections that never reach a React error boundary. It is idempotent so a
// double-invoked boot cannot add duplicate listeners.
export function installGlobalCrashHandlers(): void {
  if (installed || typeof window === "undefined") return;
  installed = true;

  window.addEventListener("error", (event) => {
    reportClientCrash({
      kind: "error",
      message: errorMessage(event.error) || event.message,
      stack: errorStack(event.error),
      source: sourceOf(event),
    });
  });

  window.addEventListener("unhandledrejection", (event) => {
    reportClientCrash({
      kind: "unhandledrejection",
      message: errorMessage(event.reason),
      stack: errorStack(event.reason),
    });
  });
}

function errorMessage(value: unknown): string {
  if (value instanceof Error) return value.message;
  if (typeof value === "string") return value;
  if (value === undefined || value === null) return "";
  // String() keeps unknown objects, which may carry credentials, out of the
  // report.
  return String(value);
}

function errorStack(value: unknown): string | undefined {
  return value instanceof Error && typeof value.stack === "string" ? value.stack : undefined;
}

function sourceOf(event: ErrorEvent): string | undefined {
  if (!event.filename) return undefined;
  return `${stripQueryAndFragment(event.filename)}:${event.lineno}:${event.colno}`;
}
