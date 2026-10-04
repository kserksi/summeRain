// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { reportClientError } from "@/lib/api";

// Mirror the server-side caps so a report stays far inside the 8 KiB body limit.
export const CRASH_REPORT_LIMITS = {
  message: 300,
  stack: 2000,
  componentStack: 2000,
  source: 200,
  path: 200,
} as const;

// A crashing page must not amplify its own failure.
const REPORT_BUDGET = 3;
const BUDGET_WINDOW_MS = 60_000;
const DUPLICATE_WINDOW_MS = 5_000;

export type CrashReportKind = "boundary" | "error" | "unhandledrejection";

export interface CrashReportInput {
  kind: CrashReportKind;
  message?: string;
  stack?: string;
  componentStack?: string;
  source?: string;
}

export interface CrashReportPayload {
  kind: CrashReportKind;
  message: string;
  path: string;
  stack?: string;
  component_stack?: string;
  source?: string;
}

let budgetStartedAt = 0;
let budgetUsed = 0;
let recentFingerprints = new Map<string, number>();

export function sanitizeCrashReport(input: CrashReportInput): CrashReportPayload | undefined {
  const message = truncate(input.message?.trim() ?? "", CRASH_REPORT_LIMITS.message);
  if (!message) return undefined;
  return {
    kind: input.kind,
    message,
    path: currentPath(),
    ...optionalField("stack", input.stack, CRASH_REPORT_LIMITS.stack),
    ...optionalField("component_stack", input.componentStack, CRASH_REPORT_LIMITS.componentStack),
    ...optionalField(
      "source",
      input.source ? stripQueryAndFragment(input.source) : undefined,
      CRASH_REPORT_LIMITS.source,
    ),
  };
}

// reportClientCrash is fire-and-forget and never throws: it runs inside error
// handling, so a second failure must stay contained.
export function reportClientCrash(input: CrashReportInput): void {
  try {
    const payload = sanitizeCrashReport(input);
    if (!payload) return;
    if (!withinBudget(payload, Date.now())) return;
    void reportClientError(payload).catch(() => undefined);
  } catch {
    // Reporting must never mask the original failure.
  }
}

function withinBudget(payload: CrashReportPayload, now: number): boolean {
  const fingerprint = `${payload.kind}:${payload.message}:${payload.source ?? ""}`;
  const lastSeen = recentFingerprints.get(fingerprint);
  if (lastSeen !== undefined && now - lastSeen < DUPLICATE_WINDOW_MS) return false;
  if (now - budgetStartedAt >= BUDGET_WINDOW_MS) {
    budgetStartedAt = now;
    budgetUsed = 0;
    recentFingerprints = new Map();
  }
  if (budgetUsed >= REPORT_BUDGET) return false;
  budgetUsed += 1;
  recentFingerprints.set(fingerprint, now);
  return true;
}

// stripQueryAndFragment keeps secrets that travel in query strings, such as
// private image tokens, out of a report.
export function stripQueryAndFragment(value: string): string {
  return value.split(/[?#]/)[0] ?? "";
}

function currentPath(): string {
  const pathname = typeof window === "undefined" ? "" : (window.location?.pathname ?? "");
  return truncate(stripQueryAndFragment(pathname), CRASH_REPORT_LIMITS.path);
}

function optionalField(
  key: string,
  value: string | undefined,
  limit: number,
): Record<string, string> {
  const trimmed = value?.trim();
  if (!trimmed) return {};
  return { [key]: truncate(trimmed, limit) };
}

function truncate(value: string, limit: number): string {
  return value.length <= limit ? value : value.slice(0, limit);
}
