// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import {
  reconcileV2UploadStatuses,
  resetV2UploadObservers,
  nextV2UploadAttempt,
  type UploadSessionResponse,
  type V2UploadResult,
} from "./v2-upload";
import {
  acquireUploadLease,
  clearPersistedUploadsExceptUser,
  deletePersistedProcessedUpload,
  deletePersistedUpload,
  hasPersistedUpload,
  listPersistedUploads,
  releaseUploadLease,
  updatePersistedUpload,
  type PersistedUploadEntry,
  type PersistedUploadStatus,
  type PersistedRetryMode,
} from "./upload-queue-store";

export const UPLOAD_AUTH_BOUNDARY_EVENT = "summerain:upload-auth-boundary";
export const V1_UPLOAD_OUTCOME_UNKNOWN = "UPLOAD_V1_OUTCOME_UNKNOWN";
const UPLOAD_AUTH_BOUNDARY_STORAGE_KEY = "summerain:upload-auth-boundary-notice";
let authenticationGeneration = 0;

function applyAuthenticationBoundary(remote: boolean): void {
  authenticationGeneration += 1;
  resetV2UploadObservers();
  window.dispatchEvent(
    new CustomEvent(UPLOAD_AUTH_BOUNDARY_EVENT, { detail: { remote } }),
  );
}

if (typeof window !== "undefined") {
  window.addEventListener("storage", (event) => {
    if (event.key === UPLOAD_AUTH_BOUNDARY_STORAGE_KEY && event.newValue) {
      applyAuthenticationBoundary(true);
    }
  });
}

export function stopActiveUploadWork(): void {
  applyAuthenticationBoundary(false);
  try {
    localStorage.setItem(
      UPLOAD_AUTH_BOUNDARY_STORAGE_KEY,
      `${Date.now()}:${Math.random().toString(36).slice(2)}`,
    );
  } catch {
    // The current tab is already stopped; storage events are best-effort hardening.
  }
}

function assertRecoveryActive(generation: number, signal?: AbortSignal): void {
  if (signal?.aborted) {
    throw signal.reason ?? new DOMException("Upload recovery stopped", "AbortError");
  }
  if (generation !== authenticationGeneration) {
    throw new DOMException("Upload authentication scope changed", "AbortError");
  }
}

export type UploadRecoveryPlan =
  | { action: "completed"; result: V2UploadResult }
  | { action: "poll" }
  | { action: "resume" }
  | { action: "restart" }
  | { action: "failed"; message: string }
  | { action: "paused" };

export interface RecoveredUploadEntry {
  entry: PersistedUploadEntry;
  status: PersistedUploadStatus | "paused" | "done";
  attempt: number;
  uploadId?: string;
  serverExpiresAt?: string;
  retryMode?: PersistedRetryMode;
  result?: V2UploadResult;
  failureCode?: string;
  failureMessage?: string;
  failureRetryable?: boolean;
  autoResume: boolean;
}

export async function recoverPersistedUploadQueue(
  ownerUserId: number,
  signal?: AbortSignal,
): Promise<RecoveredUploadEntry[]> {
  const generation = authenticationGeneration;
  assertRecoveryActive(generation, signal);
  await clearPersistedUploadsExceptUser(ownerUserId);
  assertRecoveryActive(generation, signal);
  const entries = await listPersistedUploads(ownerUserId);
  assertRecoveryActive(generation, signal);
  const validEntries: PersistedUploadEntry[] = [];
  for (const entry of entries) {
    assertRecoveryActive(generation, signal);
    if (!entry.payload) {
      if (
        !entry.task.leaseOwner ||
        (entry.task.leaseExpiresAt ?? 0) <= Date.now()
      ) {
        await deletePersistedUpload(ownerUserId, entry.task.queueId);
      }
      continue;
    }
    validEntries.push(entry);
  }

  const activeIds = validEntries
    .filter(({ task }) => task.started && task.uploadId)
    .map(({ task }) => task.uploadId!);
  let sessions = new Map<string, UploadSessionResponse>();
  let missing = new Set<string>();
  let statusUnavailable = false;
  if (activeIds.length > 0) {
    try {
      const snapshot = await reconcileV2UploadStatuses(activeIds, signal);
      assertRecoveryActive(generation, signal);
      sessions = new Map(snapshot.uploads.map((session) => [session.upload_id, session]));
      missing = new Set(snapshot.missingUploadIds);
    } catch {
      assertRecoveryActive(generation, signal);
      statusUnavailable = true;
    }
  }

  const recovered: RecoveredUploadEntry[] = [];
  for (const entry of validEntries) {
    const recoveredEntry = await recoverPersistedEntry(
      ownerUserId,
      entry,
      sessions,
      missing,
      statusUnavailable,
      generation,
      signal,
    );
    if (recoveredEntry) recovered.push(recoveredEntry);
  }
  return recovered;
}

async function recoverPersistedEntry(
  ownerUserId: number,
  entry: PersistedUploadEntry,
  sessions: Map<string, UploadSessionResponse>,
  missing: Set<string>,
  statusUnavailable: boolean,
  generation: number,
  signal?: AbortSignal,
): Promise<RecoveredUploadEntry | undefined> {
  const task = entry.task;
  const leaseOwner = `recovery:${Date.now()}:${Math.random().toString(36).slice(2)}`;
  assertRecoveryActive(generation, signal);
  if (!(await acquireUploadLease(ownerUserId, task.queueId, leaseOwner))) {
    if (!(await hasPersistedUpload(ownerUserId, task.queueId))) return undefined;
    return {
      entry,
      status: "paused",
      attempt: task.attempt,
      uploadId: task.uploadId,
      serverExpiresAt: task.serverExpiresAt,
      retryMode: task.status === "serverProcessing" ? "resume" : "reuse",
      autoResume: false,
    };
  }

  try {
    assertRecoveryActive(generation, signal);
    if (!task.started) {
      return {
        entry,
        status: "queued",
        attempt: task.attempt,
        uploadId: task.uploadId,
        serverExpiresAt: task.serverExpiresAt,
        retryMode: task.retryMode,
        autoResume: false,
      };
    }
    if (!task.uploadId) {
      if (task.pipelineMode === "v1" && task.status === "uploading") {
        const updated = await updatePersistedUpload(ownerUserId, task.queueId, {
          status: "failed",
          retryMode: undefined,
          failureCode: V1_UPLOAD_OUTCOME_UNKNOWN,
          failureMessage: undefined,
          failureRetryable: false,
        }, leaseOwner);
        if (!updated) return undefined;
        return {
          entry,
          status: "failed",
          attempt: task.attempt,
          failureCode: V1_UPLOAD_OUTCOME_UNKNOWN,
          failureRetryable: false,
          autoResume: false,
        };
      }
      const failed = task.status === "failed";
      return {
        entry,
        status: failed ? "failed" : "queued",
        attempt: task.attempt,
        retryMode: task.retryMode,
        failureCode: task.failureCode,
        failureMessage: task.failureMessage,
        failureRetryable: task.failureRetryable,
        autoResume: !failed,
      };
    }
    if (statusUnavailable) {
      return {
        entry,
        status: "paused",
        attempt: task.attempt,
        uploadId: task.uploadId,
        serverExpiresAt: task.serverExpiresAt,
        retryMode: task.status === "serverProcessing" ? "resume" : "reuse",
        autoResume: false,
      };
    }

    const plan = planUploadRecovery(sessions.get(task.uploadId), {
      missing: missing.has(task.uploadId),
    });
    if (plan.action === "completed") {
      if (!(await deletePersistedUpload(ownerUserId, task.queueId, leaseOwner))) {
        return undefined;
      }
      assertRecoveryActive(generation, signal);
      return {
        entry,
        status: "done",
        attempt: task.attempt,
        uploadId: task.uploadId,
        result: plan.result,
        autoResume: false,
      };
    }
    if (plan.action === "poll") {
      if (!(await updatePersistedUpload(ownerUserId, task.queueId, {
        status: "serverProcessing",
        retryMode: "resume",
      }, leaseOwner))) return undefined;
      if (!(await deletePersistedProcessedUpload(ownerUserId, task.queueId, leaseOwner))) {
        return undefined;
      }
      assertRecoveryActive(generation, signal);
      return {
        entry: { ...entry, payload: { ...entry.payload!, processed: undefined } },
        status: "serverProcessing",
        attempt: task.attempt,
        uploadId: task.uploadId,
        serverExpiresAt: task.serverExpiresAt,
        retryMode: "resume",
        autoResume: true,
      };
    }
    if (plan.action === "resume") {
      return {
        entry,
        status: "queued",
        attempt: task.attempt,
        uploadId: task.uploadId,
        serverExpiresAt: task.serverExpiresAt,
        retryMode: "reuse",
        autoResume: true,
      };
    }
    if (plan.action === "restart") {
      const attempt = nextV2UploadAttempt(task.attempt);
      if (!(await updatePersistedUpload(ownerUserId, task.queueId, {
        attempt,
        status: "queued",
        uploadId: undefined,
        serverExpiresAt: undefined,
        retryMode: undefined,
        failureCode: undefined,
        failureMessage: undefined,
        failureRetryable: undefined,
      }, leaseOwner))) return undefined;
      assertRecoveryActive(generation, signal);
      return { entry, status: "queued", attempt, autoResume: true };
    }
    if (plan.action === "failed") {
      if (!(await updatePersistedUpload(ownerUserId, task.queueId, {
        status: "failed",
        retryMode: "new",
        failureMessage: plan.message,
        failureRetryable: true,
      }, leaseOwner))) return undefined;
      assertRecoveryActive(generation, signal);
      return {
        entry,
        status: "failed",
        attempt: task.attempt,
        uploadId: task.uploadId,
        serverExpiresAt: task.serverExpiresAt,
        retryMode: "new",
        failureMessage: plan.message,
        failureRetryable: true,
        autoResume: false,
      };
    }
    return {
      entry,
      status: "paused",
      attempt: task.attempt,
      uploadId: task.uploadId,
      serverExpiresAt: task.serverExpiresAt,
      retryMode: task.status === "serverProcessing" ? "resume" : "reuse",
      autoResume: false,
    };
  } finally {
    await releaseUploadLease(ownerUserId, task.queueId, leaseOwner).catch(() => undefined);
  }
}

export function planUploadRecovery(
  session: UploadSessionResponse | undefined,
  options: { missing: boolean; now?: number },
): UploadRecoveryPlan {
  if (options.missing) return { action: "restart" };
  if (!session) return { action: "paused" };

  if (
    session.status === "completed" ||
    (session.status === "cleanup_pending" && session.image_id !== undefined)
  ) {
    if (!session.unique_link || !session.asset_link) {
      return { action: "failed", message: "Completed upload is missing its image link" };
    }
    return {
      action: "completed",
      result: {
        uploadId: session.upload_id,
        uniqueLink: session.unique_link,
        assetLink: session.asset_link,
        pipelineVersion: 2,
      },
    };
  }

  if (session.status === "processing") return { action: "poll" };
  if (session.status === "initiated" || session.status === "uploading") {
    const expiresAt = Date.parse(session.expires_at);
    if (!Number.isFinite(expiresAt) || expiresAt <= (options.now ?? Date.now())) {
      return { action: "restart" };
    }
    return { action: "resume" };
  }
  if (session.status === "cleanup_pending") {
    return { action: "failed", message: "Upload cleanup is pending" };
  }
  if (session.status === "failed") {
    return { action: "failed", message: "Server image processing failed" };
  }
  return { action: "failed", message: "Upload session was cancelled" };
}
