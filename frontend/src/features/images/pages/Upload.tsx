// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import {
  IconCheck,
  IconLink,
  IconLoader2,
  IconPhoto,
  IconRefresh,
  IconUpload,
  IconX,
} from "@tabler/icons-react";
import { useQueryClient } from "@tanstack/react-query";
import type { TFunction } from "i18next";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { useCopy } from "@/lib/use-copy";
import { useAuthStore } from "@/store/auth-store";

import { buildCopyText } from "../copy-format";
import {
  isClientImageError,
  type ClientImageErrorCode,
  type ClientImageErrorDetails,
} from "../client-processing/errors";
import { preflightClientImage } from "../client-processing/preflight";
import { processClientImage } from "../client-processing/processor";
import type { ProcessedImage } from "../client-processing/types";
import { createProcessedPreviewURL } from "../processed-preview";
import {
  recoverPersistedUploadQueue,
  UPLOAD_AUTH_BOUNDARY_EVENT,
  V1_UPLOAD_OUTCOME_UNKNOWN,
} from "../upload-queue-recovery";
import {
  acquireUploadLease,
  clearPersistedUploadsForUser,
  deletePersistedProcessedUpload,
  deletePersistedUpload,
  fingerprintUploadSource,
  hasPersistedUpload,
  persistProcessedUpload,
  preparePersistedUploadRun,
  persistQueuedUpload,
  releaseUploadLease,
  renewUploadLease,
  requestPersistentUploadStorage,
  sourceFileFromEntry,
  updatePersistedUpload,
  UploadQueueLeaseLostError,
  UploadQueueStorageError,
  UPLOAD_QUEUE_RETENTION_MS,
  type PersistedUploadRunPreparation,
  type PersistedUploadTask,
} from "../upload-queue-store";
import { ConcurrencyGate, runWithConcurrency } from "../upload-concurrency";
import { beginV1Upload, type V1UploadResult } from "../v1-upload";
import {
  beginV2Upload,
  cancelV2Upload,
  createV2IdempotencyKey,
  getV2Recipe,
  isV2UploadEnabled,
  nextV2UploadAttempt,
  v2UploadRetryDisposition,
  waitForV2Upload,
  type V2UploadResult,
} from "../v2-upload";

type Status =
  | "saving"
  | "queued"
  | "checking"
  | "processing"
  | "uploading"
  | "serverProcessing"
  | "paused"
  | "done"
  | "failed";
type RetryMode = "resume" | "reuse" | "new";
type CompletedUploadResult = V1UploadResult | V2UploadResult;
type UploadRunResult = "done" | "failed" | "paused" | "pending" | "skipped";

interface QueueFailure {
  code?: ClientImageErrorCode;
  details?: ClientImageErrorDetails;
  fallback?: string;
  retryable: boolean;
}

interface QueueItem {
  id: string;
  ownerUserId: number;
  attempt: number;
  file: File;
  sourceFingerprint?: string;
  durability: "durable" | "volatile";
  restored?: boolean;
  started: boolean;
  processed?: ProcessedImage;
  preview?: string;
  progress: number;
  status: Status;
  uploadId?: string;
  serverExpiresAt?: string;
  recipeVersion?: string;
  retryMode?: RetryMode;
  attemptVisibility?: "public" | "private";
  uniqueLink?: string;
  assetLink?: string;
  pipelineVersion?: number;
  failure?: QueueFailure;
  manualRetry?: boolean;
}

function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

const STATUS_VARIANT: Record<Status, "default" | "secondary" | "destructive" | "outline"> = {
  saving: "secondary",
  queued: "outline",
  checking: "secondary",
  processing: "secondary",
  uploading: "secondary",
  serverProcessing: "secondary",
  paused: "outline",
  done: "default",
  failed: "destructive",
};

const PIPELINE_CONCURRENCY = 2;
// The backend permits eight active sessions per user. Keep half available for
// status recovery or another tab while still keeping the single publish worker busy.
const ACTIVE_SESSION_CONCURRENCY = 4;
const LEASE_RENEW_INTERVAL_MS = 10_000;
const MAX_SOURCE_BYTES = 15 * 1024 * 1024;

const ALLOWED_EXTS = [".png", ".jpg", ".jpeg", ".bmp", ".webp", ".avif"];

function createQueueID(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
}

function isCurrentUploadOwner(ownerUserId: number): boolean {
  return useAuthStore.getState().user?.id === ownerUserId;
}

function persistedTaskFromItem(item: QueueItem, now: number): PersistedUploadTask {
  return {
    ownerUserId: item.ownerUserId,
    queueId: item.id,
    attempt: item.attempt,
    fileName: item.file.name,
    fileType: item.file.type,
    fileSize: item.file.size,
    lastModified: item.file.lastModified,
    sourceFingerprint: item.sourceFingerprint ?? "",
    status: "queued",
    started: false,
    createdAt: now,
    updatedAt: now,
    expiresAt: now + UPLOAD_QUEUE_RETENTION_MS,
  };
}

function assertCurrentUploadOwner(ownerUserId: number): void {
  if (!isCurrentUploadOwner(ownerUserId)) {
    throw abortError("Upload account changed");
  }
}

async function requirePersistedUploadUpdate(
  ownerUserId: number,
  queueId: string,
  patch: Parameters<typeof updatePersistedUpload>[2],
  expectedLeaseOwner: string,
): Promise<void> {
  if (!(await updatePersistedUpload(ownerUserId, queueId, patch, expectedLeaseOwner))) {
    throw abortError("Persisted upload was removed");
  }
}

function createSerialExecutor() {
  let tail = Promise.resolve();
  return async <T,>(task: () => Promise<T>): Promise<T> => {
    const previous = tail;
    let release: () => void = () => {};
    tail = new Promise<void>((resolve) => {
      release = resolve;
    });
    await previous;
    try {
      return await task();
    } finally {
      release();
    }
  };
}

export default function Upload() {
  const { t } = useTranslation();
  const user = useAuthStore((state) => state.user);
  const isHydrating = useAuthStore((state) => state.isHydrating);
  const [items, setItems] = useState<QueueItem[]>([]);
  const [visibility, setVisibility] = useState<"public" | "private">("public");
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const mountedRef = useRef(false);
  const hydratedOwnerRef = useRef<number | undefined>(undefined);
  const uploadingRef = useRef(false);
  const tabOwnerRef = useRef(createQueueID());
  const runUploadsRef = useRef<(items: QueueItem[]) => Promise<void>>(async () => undefined);
  const activeControllersRef = useRef(new Map<string, AbortController>());
  const removedQueueIdsRef = useRef(new Set<string>());
  const previewURLsRef = useRef(new Set<string>());
  const navigate = useNavigate();
  const qc = useQueryClient();
  const refreshUser = useAuthStore((s) => s.refreshUser);
  const { copied, copy } = useCopy();

  const itemsRef = useRef(items);
  useEffect(() => {
    itemsRef.current = items;
  });
  useEffect(() => {
    const activeControllers = activeControllersRef.current;
    const previewURLs = previewURLsRef.current;
    const pauseActiveWork = () => {
      for (const controller of activeControllers.values()) {
        controller.abort(abortError("Upload authentication scope changed"));
      }
      activeControllers.clear();
    };
    const stopForAuthenticationBoundary = (event: Event) => {
      const ownerUserId = useAuthStore.getState().user?.id;
      pauseActiveWork();
      if (ownerUserId) {
        void clearPersistedUploadsForUser(ownerUserId).catch(() => undefined);
      }
      for (const preview of previewURLs) URL.revokeObjectURL(preview);
      previewURLs.clear();
      removedQueueIdsRef.current.clear();
      hydratedOwnerRef.current = undefined;
      uploadingRef.current = false;
      setUploading(false);
      setItems([]);
      if (
        event instanceof CustomEvent &&
        (event.detail as { remote?: boolean } | undefined)?.remote
      ) {
        void useAuthStore.getState().hydrate();
      }
    };
    mountedRef.current = true;
    window.addEventListener(UPLOAD_AUTH_BOUNDARY_EVENT, stopForAuthenticationBoundary);
    return () => {
      mountedRef.current = false;
      window.removeEventListener(UPLOAD_AUTH_BOUNDARY_EVENT, stopForAuthenticationBoundary);
      pauseActiveWork();
      for (const preview of previewURLs) URL.revokeObjectURL(preview);
      previewURLs.clear();
    };
  }, []);

  const addFiles = useCallback((files: FileList | File[]) => {
    const ownerUserId = useAuthStore.getState().user?.id;
    if (!ownerUserId) return;
    const next: QueueItem[] = Array.from(files).map((file) => {
      const dot = file.name.lastIndexOf(".");
      const ext = dot >= 0 ? file.name.slice(dot).toLowerCase() : "";
      let failure: QueueFailure | undefined;
      if (file.size <= 0) {
        failure = { code: "IMAGE_FILE_INVALID", retryable: false };
      } else if (file.size > MAX_SOURCE_BYTES) {
        failure = {
          code: "IMAGE_FILE_SIZE_EXCEEDED",
          details: { maxMB: 15 },
          retryable: false,
        };
      } else if (!ALLOWED_EXTS.includes(ext)) {
        failure = { code: "IMAGE_FORMAT_UNSUPPORTED", retryable: false };
      }
      return {
        id: createQueueID(),
        ownerUserId,
        attempt: 0,
        file,
        durability: "volatile",
        started: false,
        progress: 0,
        status: failure ? "failed" : "saving",
        failure,
      };
    });
    if (next.length === 0) return;
    setItems((previous) => [...previous, ...next]);

    void (async () => {
      let authenticationScopeChanged = false;
      const stopPersistence = () => {
        authenticationScopeChanged = true;
      };
      window.addEventListener(UPLOAD_AUTH_BOUNDARY_EVENT, stopPersistence);
      try {
        await requestPersistentUploadStorage();
        let volatileCount = 0;
        for (const item of next) {
          if (item.failure) continue;
          try {
            const sourceFingerprint = await fingerprintUploadSource(item.file);
            if (removedQueueIdsRef.current.has(item.id)) continue;
            if (authenticationScopeChanged) throw abortError("Upload account changed");
            assertCurrentUploadOwner(ownerUserId);
            const durable = { ...item, sourceFingerprint };
            await persistQueuedUpload(persistedTaskFromItem(durable, Date.now()), item.file);
            if (
              removedQueueIdsRef.current.has(item.id) ||
              authenticationScopeChanged ||
              !isCurrentUploadOwner(ownerUserId)
            ) {
              await deletePersistedUpload(ownerUserId, item.id);
              if (authenticationScopeChanged) return;
              continue;
            }
            if (mountedRef.current) {
              setItems((previous) =>
                previous.map((candidate) =>
                  candidate.id === item.id
                    ? { ...candidate, sourceFingerprint, durability: "durable", status: "queued" }
                    : candidate,
                ),
              );
            }
          } catch (error) {
            if (removedQueueIdsRef.current.has(item.id)) continue;
            if (isAbortError(error)) return;
            volatileCount += 1;
            if (mountedRef.current && isCurrentUploadOwner(ownerUserId)) {
              setItems((previous) =>
                previous.map((candidate) =>
                  candidate.id === item.id
                    ? { ...candidate, durability: "volatile", status: "queued" }
                    : candidate,
                ),
              );
            }
          }
        }
        if (volatileCount > 0 && mountedRef.current && isCurrentUploadOwner(ownerUserId)) {
          toast.warning(t("upload.persistence.volatileWarning", { count: volatileCount }));
        }
      } finally {
        window.removeEventListener(UPLOAD_AUTH_BOUNDARY_EVENT, stopPersistence);
      }
    })();
  }, [t]);

  const discardQueueItem = (id: string) => {
    setItems((previous) =>
      previous.filter((item) => {
        if (item.id === id && item.preview) {
          URL.revokeObjectURL(item.preview);
          previewURLsRef.current.delete(item.preview);
        }
        return item.id !== id;
      }),
    );
  };

  const removeItem = async (id: string) => {
    const item = itemsRef.current.find((candidate) => candidate.id === id);
    if (!item) return;
    removedQueueIdsRef.current.add(id);
    activeControllersRef.current.get(id)?.abort(abortError("Upload removed"));
    discardQueueItem(id);
    try {
      await deletePersistedUpload(item.ownerUserId, item.id);
    } catch {
      // The 24-hour cleanup remains a bounded fallback if local deletion fails.
    }
    if (
      item.uploadId &&
      item.status !== "done" &&
      isCurrentUploadOwner(item.ownerUserId)
    ) {
      await cancelV2Upload(item.uploadId);
    }
  };

  const patchItem = (id: string, value: Partial<QueueItem>) => {
    if (!mountedRef.current) return;
    setItems((previous) =>
      previous.map((candidate) => (candidate.id === id ? { ...candidate, ...value } : candidate)),
    );
  };

  const clearActiveController = (id: string, controller: AbortController) => {
    if (activeControllersRef.current.get(id) === controller) {
      activeControllersRef.current.delete(id);
    }
  };

  const finishItem = async (
    item: QueueItem,
    result: CompletedUploadResult,
    expectedLeaseOwner?: string,
  ): Promise<"done"> => {
    if (item.durability === "durable") {
      try {
        const deleted = await deletePersistedUpload(
          item.ownerUserId,
          item.id,
          expectedLeaseOwner,
        );
        if (expectedLeaseOwner && !deleted) {
          discardQueueItem(item.id);
          return "done";
        }
      } catch {
        // Expiry cleanup will retry local privacy cleanup.
      }
    }
    if (!isCurrentUploadOwner(item.ownerUserId)) return "done";
    patchItem(item.id, {
      status: "done",
      progress: 100,
      uploadId: result.uploadId,
      retryMode: undefined,
      uniqueLink: result.uniqueLink,
      assetLink: "assetLink" in result ? result.assetLink : undefined,
      pipelineVersion: result.pipelineVersion,
      failure: undefined,
    });
    return "done";
  };

  const failItem = async (
    item: QueueItem,
    error: unknown,
    phase: "begin" | "poll" = "begin",
    uploadId?: string,
    expectedLeaseOwner?: string,
  ): Promise<"failed" | "paused"> => {
    if (isAbortError(error) || error instanceof UploadQueueLeaseLostError) {
      if (
        !mountedRef.current ||
        !isCurrentUploadOwner(item.ownerUserId) ||
        removedQueueIdsRef.current.has(item.id)
      ) {
        return "paused";
      }
      if (item.durability === "durable") {
        const stillPersisted = await hasPersistedUpload(item.ownerUserId, item.id).catch(
          () => true,
        );
        if (!stillPersisted) {
          discardQueueItem(item.id);
          return "paused";
        }
      }
      const resumableUploadId = uploadId ?? item.uploadId;
      patchItem(item.id, {
        status: "paused",
        uploadId: resumableUploadId,
        retryMode: phase === "poll" && resumableUploadId ? "resume" : "reuse",
      });
      return "paused";
    }
    const clientError = isClientImageError(error) ? error : undefined;
    const message = error instanceof UploadQueueStorageError
      ? t("upload.persistence.storageUnavailable")
      : clientError
      ? formatQueueFailure(
          {
            code: clientError.code,
            details: clientError.details,
            retryable: clientError.retryable,
          },
          t,
        )
      : error instanceof Error
        ? error.message
        : t("upload.toast.uploadAllFailed");
    const disposition = v2UploadRetryDisposition(error);
    const retryMode: RetryMode =
      phase === "poll" && uploadId
        ? disposition === "new-attempt"
          ? "new"
          : "resume"
        : disposition === "new-attempt"
          ? "new"
          : "reuse";
    let statusRecorded = true;
    if (item.durability === "durable") {
      try {
        statusRecorded = await updatePersistedUpload(
          item.ownerUserId,
          item.id,
          {
            status: "failed",
            uploadId: uploadId ?? item.uploadId,
            retryMode,
            failureCode: clientError?.code,
            failureMessage: message,
            failureRetryable: clientError?.retryable ?? true,
          },
          expectedLeaseOwner,
        );
      } catch {
        // The durable source remains available even if the status write failed.
      }
    }
    if (expectedLeaseOwner && !statusRecorded) {
      const stillPersisted = await hasPersistedUpload(item.ownerUserId, item.id).catch(
        () => true,
      );
      if (!stillPersisted) discardQueueItem(item.id);
      else patchItem(item.id, { status: "paused" });
      return "paused";
    }
    if (!isCurrentUploadOwner(item.ownerUserId)) return "paused";
    if (!clientError) {
      toast.error(t("upload.toast.itemFailed", { name: item.file.name, msg: message }));
    }
    patchItem(item.id, {
      status: "failed",
      uploadId: uploadId ?? item.uploadId,
      retryMode,
      failure: clientError
        ? {
            code: clientError.code,
            details: clientError.details,
            retryable: clientError.retryable,
          }
        : { fallback: message, retryable: true },
    });
    return "failed";
  };

  const attachProcessedPreview = (item: QueueItem, processed: Parameters<typeof createProcessedPreviewURL>[0]) => {
    if (item.preview) return;
    const preview = createProcessedPreviewURL(processed);
    if (!preview) return;
    previewURLsRef.current.add(preview);
    if (!mountedRef.current) {
      URL.revokeObjectURL(preview);
      previewURLsRef.current.delete(preview);
      return;
    }
    setItems((previous) =>
      previous.map((candidate) => {
        if (candidate.id !== item.id) return candidate;
        if (candidate.preview) {
          URL.revokeObjectURL(preview);
          previewURLsRef.current.delete(preview);
          return candidate;
        }
        return { ...candidate, preview };
      }),
    );
  };

  const failLegacyUploadWithUnknownOutcome = async (
    item: QueueItem,
    expectedLeaseOwner: string,
  ): Promise<"failed" | "paused"> => {
    if (
      !mountedRef.current ||
      !isCurrentUploadOwner(item.ownerUserId) ||
      removedQueueIdsRef.current.has(item.id)
    ) {
      return "paused";
    }
    if (item.durability === "durable") {
      const updated = await updatePersistedUpload(item.ownerUserId, item.id, {
        status: "failed",
        retryMode: undefined,
        failureCode: V1_UPLOAD_OUTCOME_UNKNOWN,
        failureMessage: undefined,
        failureRetryable: false,
      }, expectedLeaseOwner);
      if (!updated) {
        discardQueueItem(item.id);
        return "paused";
      }
    }
    const message = t("upload.persistence.v1OutcomeUnknown");
    patchItem(item.id, {
      status: "failed",
      retryMode: undefined,
      failure: { fallback: message, retryable: false },
    });
    toast.error(t("upload.toast.itemFailed", { name: item.file.name, msg: message }));
    return "failed";
  };

  const beginUpload = async (
    item: QueueItem,
    processSerially: <T>(task: () => Promise<T>) => Promise<T>,
    pendingPolls: Array<Promise<UploadRunResult>>,
    controller: AbortController,
    releaseSession: () => Promise<void>,
    leaseOwner: string,
  ): Promise<UploadRunResult> => {
    const patch = (value: Partial<QueueItem>) => patchItem(item.id, value);
    let knownUploadId = item.uploadId;
    let legacyRequestStarted = false;
    const attemptVisibility = item.attemptVisibility ?? visibility;

    try {
      assertCurrentUploadOwner(item.ownerUserId);
      if (item.durability === "durable") {
        await requirePersistedUploadUpdate(item.ownerUserId, item.id, {
          attempt: item.attempt,
          status: "checking",
          started: true,
          visibility: attemptVisibility,
          uploadId: item.uploadId,
          serverExpiresAt: item.serverExpiresAt,
          retryMode: undefined,
        }, leaseOwner);
      }
      assertCurrentUploadOwner(item.ownerUserId);
      patch({ attemptVisibility, started: true, status: "checking", failure: undefined });
      const recipe = await getV2Recipe(controller.signal);
      assertCurrentUploadOwner(item.ownerUserId);
      if (!isV2UploadEnabled(recipe)) {
        if (item.durability === "durable") {
          await requirePersistedUploadUpdate(item.ownerUserId, item.id, {
            status: "uploading",
            pipelineMode: "v1",
          }, leaseOwner);
        }
        assertCurrentUploadOwner(item.ownerUserId);
        patch({ status: "uploading", progress: 10 });
        if (controller.signal.aborted) {
          throw controller.signal.reason ?? abortError("Upload paused");
        }
        legacyRequestStarted = true;
        const result = await beginV1Upload(item.file, attemptVisibility, controller.signal);
        return await finishItem(item, result, leaseOwner);
      }

      let processed = item.processed;
      if (processed?.recipe_version !== recipe.recipe_version) {
        processed = undefined;
        if (item.durability === "durable") {
          if (!(await deletePersistedProcessedUpload(item.ownerUserId, item.id, leaseOwner))) {
            throw abortError("Upload lease moved to another tab");
          }
        }
      }
      if (!processed) {
        if (item.restored && item.sourceFingerprint) {
          const restoredFingerprint = await fingerprintUploadSource(item.file);
          if (restoredFingerprint !== item.sourceFingerprint) {
            throw new Error("The persisted upload source failed integrity validation");
          }
        }
        patch({ status: "checking", progress: 0, failure: undefined, restored: false });
        const plan = await preflightClientImage(item.file, recipe, controller.signal);
        processed = await processSerially(async () => {
          if (item.durability === "durable") {
            await requirePersistedUploadUpdate(item.ownerUserId, item.id, {
              status: "processing",
            }, leaseOwner);
          }
          patch({ status: "processing", progress: 0 });
          return processClientImage(
            item.file,
            plan,
            (percent) => {
              patch({ status: "processing", progress: Math.min(40, Math.round(percent * 0.4)) });
            },
            controller.signal,
          );
        });
        assertCurrentUploadOwner(item.ownerUserId);
        if (item.durability === "durable") {
          await persistProcessedUpload(item.ownerUserId, item.id, processed, leaseOwner);
        }
      }
      attachProcessedPreview(item, processed);
      patch({
        status: "uploading",
        progress: 40,
        processed,
        recipeVersion: recipe.recipe_version,
      });

      assertCurrentUploadOwner(item.ownerUserId);
      const started = await beginV2Upload({
        fileName: item.file.name,
        visibility: attemptVisibility,
        idempotencyKey: createV2IdempotencyKey(item.id, item.attempt),
        processed,
        onSession: async (uploadId, expiresAt) => {
          knownUploadId = uploadId;
          if (removedQueueIdsRef.current.has(item.id)) {
            await cancelV2Upload(uploadId);
            throw abortError("Upload removed");
          }
          if (item.durability === "durable") {
            await requirePersistedUploadUpdate(item.ownerUserId, item.id, {
              status: "uploading",
              uploadId,
              serverExpiresAt: expiresAt,
              recipeVersion: recipe.recipe_version,
              pipelineMode: "v2",
            }, leaseOwner);
          }
          if (removedQueueIdsRef.current.has(item.id)) {
            await deletePersistedUpload(item.ownerUserId, item.id).catch(() => undefined);
            await cancelV2Upload(uploadId);
            throw abortError("Upload removed");
          }
          assertCurrentUploadOwner(item.ownerUserId);
          patch({ uploadId, serverExpiresAt: expiresAt });
        },
        onProgress: (phase, percent) => {
          if (phase === "uploading") {
            patch({ status: "uploading", progress: 40 + Math.round(percent * 0.5) });
          } else {
            patch({
              status: "serverProcessing",
              progress: Math.min(99, 90 + Math.round(percent * 0.09)),
            });
          }
        },
        signal: controller.signal,
      });
      knownUploadId = started.uploadId;
      patch({ uploadId: started.uploadId });
      if (started.completed) return await finishItem(item, started.completed, leaseOwner);

      if (item.durability === "durable") {
        await requirePersistedUploadUpdate(item.ownerUserId, item.id, {
          status: "serverProcessing",
          uploadId: started.uploadId,
          retryMode: "resume",
        }, leaseOwner);
        if (!(await deletePersistedProcessedUpload(item.ownerUserId, item.id, leaseOwner))) {
          throw abortError("Upload lease moved to another tab");
        }
      }
      assertCurrentUploadOwner(item.ownerUserId);
      patch({ status: "serverProcessing", progress: 90 });
      assertCurrentUploadOwner(item.ownerUserId);
      const polling = waitForV2Upload(
        started.uploadId,
        (_phase, percent) => {
          patch({
            status: "serverProcessing",
            progress: Math.min(99, 90 + Math.round(percent * 0.09)),
          });
        },
        controller.signal,
      )
        .then((result) => finishItem(item, result, leaseOwner))
        .catch((error: unknown) =>
          failItem(item, error, "poll", started.uploadId, leaseOwner),
        )
        .finally(async () => {
          clearActiveController(item.id, controller);
          await releaseSession();
        });
      pendingPolls.push(polling);
      return "pending";
    } catch (error) {
      if (legacyRequestStarted) {
        return await failLegacyUploadWithUnknownOutcome(item, leaseOwner);
      }
      return await failItem(item, error, "begin", knownUploadId, leaseOwner);
    }
  };

  const beginStatusResume = async (
    item: QueueItem,
    pendingPolls: Array<Promise<UploadRunResult>>,
    controller: AbortController,
    releaseSession: () => Promise<void>,
    leaseOwner: string,
  ): Promise<UploadRunResult> => {
    const uploadId = item.uploadId;
    if (!uploadId) {
      return await failItem(
        item,
        new Error("Upload status cannot be resumed"),
        "begin",
        undefined,
        leaseOwner,
      );
    }
    try {
      assertCurrentUploadOwner(item.ownerUserId);
      if (item.durability === "durable") {
        await requirePersistedUploadUpdate(item.ownerUserId, item.id, {
          status: "serverProcessing",
          started: true,
          retryMode: "resume",
        }, leaseOwner);
      }
      assertCurrentUploadOwner(item.ownerUserId);
    } catch (error) {
      return await failItem(item, error, "poll", uploadId, leaseOwner);
    }
    patchItem(item.id, {
      status: "serverProcessing",
      progress: Math.max(90, item.progress),
      retryMode: undefined,
    });
    try {
      const polling = waitForV2Upload(
        uploadId,
        (_phase, percent) => {
          patchItem(item.id, {
            status: "serverProcessing",
            progress: Math.min(99, 90 + Math.round(percent * 0.09)),
          });
        },
        controller.signal,
      )
        .then((result) => finishItem(item, result, leaseOwner))
        .catch((error: unknown) => failItem(item, error, "poll", uploadId, leaseOwner))
        .finally(async () => {
          clearActiveController(item.id, controller);
          await releaseSession();
        });
      pendingPolls.push(polling);
      return "pending";
    } catch (error) {
      return await failItem(item, error, "poll", uploadId, leaseOwner);
    }
  };

  const runUploads = async (toUpload: QueueItem[]) => {
    if (!toUpload.length || uploadingRef.current) return;
    uploadingRef.current = true;
    setUploading(true);
    const processSerially = createSerialExecutor();
    const activeSessions = new ConcurrencyGate(ACTIVE_SESSION_CONCURRENCY);
    const pendingPolls: Array<Promise<UploadRunResult>> = [];
    const started = await runWithConcurrency(
      toUpload,
      async (item) => {
        if (!isCurrentUploadOwner(item.ownerUserId)) return "skipped" as const;
        const controller = new AbortController();
        if (!mountedRef.current) {
          controller.abort(abortError("Upload page unmounted"));
        }
        activeControllersRef.current.get(item.id)?.abort(abortError("Upload attempt superseded"));
        activeControllersRef.current.set(item.id, controller);
        const leaseOwner = `${tabOwnerRef.current}:${createQueueID()}`;
        let leaseAcquired = false;
        if (item.durability === "durable") {
          try {
            leaseAcquired = await acquireUploadLease(
              item.ownerUserId,
              item.id,
              leaseOwner,
            );
          } catch {
            clearActiveController(item.id, controller);
            return "skipped" as const;
          }
          if (!leaseAcquired) {
            const stillPersisted = await hasPersistedUpload(item.ownerUserId, item.id).catch(
              () => true,
            );
            if (!stillPersisted) discardQueueItem(item.id);
            else patchItem(item.id, { status: "paused" });
            clearActiveController(item.id, controller);
            return "skipped" as const;
          }
        }
        let released = false;
        let renewTimer: number | undefined;
        const releaseOwnership = async () => {
          if (released) return;
          released = true;
          if (renewTimer !== undefined) window.clearInterval(renewTimer);
          if (leaseAcquired) {
            await releaseUploadLease(item.ownerUserId, item.id, leaseOwner).catch(() => undefined);
          }
        };
        let runnableItem = item;
        if (leaseAcquired) {
          let preparation: PersistedUploadRunPreparation;
          try {
            preparation = await preparePersistedUploadRun(
              item.ownerUserId,
              item.id,
              leaseOwner,
              item.manualRetry === true,
            );
          } catch {
            patchItem(item.id, { status: "paused" });
            await releaseOwnership();
            clearActiveController(item.id, controller);
            return "paused" as const;
          }
          if (preparation.outcome === "missing") {
            await releaseOwnership();
            discardQueueItem(item.id);
            clearActiveController(item.id, controller);
            return "skipped" as const;
          }
          if (preparation.outcome === "lease-lost") {
            patchItem(item.id, { status: "paused" });
            await releaseOwnership();
            clearActiveController(item.id, controller);
            return "paused" as const;
          }
          const persistedTask = preparation.task;
          if (
            persistedTask.pipelineMode === "v1" &&
            persistedTask.status === "uploading"
          ) {
            const result = await failLegacyUploadWithUnknownOutcome(item, leaseOwner);
            await releaseOwnership();
            clearActiveController(item.id, controller);
            return result;
          }
          if (preparation.outcome === "blocked" || persistedTask.status === "failed") {
            const failureMessage = persistedTask.failureCode === V1_UPLOAD_OUTCOME_UNKNOWN
              ? t("upload.persistence.v1OutcomeUnknown")
              : persistedTask.failureMessage ?? t("upload.toast.uploadAllFailed");
            patchItem(item.id, {
              status: "failed",
              attempt: persistedTask.attempt,
              uploadId: persistedTask.uploadId,
              retryMode: persistedTask.retryMode,
              failure: {
                fallback: failureMessage,
                retryable: persistedTask.failureRetryable !== false,
              },
            });
            await releaseOwnership();
            clearActiveController(item.id, controller);
            return "failed" as const;
          }
          const retryMode = persistedTask.status === "serverProcessing"
            ? "resume"
            : persistedTask.uploadId
              ? "reuse"
              : persistedTask.retryMode;
          runnableItem = {
            ...item,
            attempt: persistedTask.attempt,
            started: persistedTask.started,
            uploadId: persistedTask.uploadId,
            serverExpiresAt: persistedTask.serverExpiresAt,
            recipeVersion: persistedTask.recipeVersion,
            retryMode,
            attemptVisibility: persistedTask.visibility,
            manualRetry: false,
          };
          patchItem(item.id, runnableItem);
        }
        if (leaseAcquired) {
          renewTimer = window.setInterval(() => {
            void renewUploadLease(item.ownerUserId, item.id, leaseOwner)
              .then((renewed) => {
                if (!renewed) controller.abort(abortError("Upload lease moved to another tab"));
              })
              .catch(() => controller.abort(abortError("Upload lease could not be renewed")));
          }, LEASE_RENEW_INTERVAL_MS);
        }
        let releaseSession: (() => void) | undefined;
        try {
          releaseSession = await activeSessions.acquire(controller.signal);
        } catch {
          await releaseOwnership();
          clearActiveController(item.id, controller);
          return "paused" as const;
        }
        const releaseResources = async () => {
          releaseSession?.();
          await releaseOwnership();
        };
        const result =
          runnableItem.retryMode === "resume" && runnableItem.uploadId
            ? await beginStatusResume(
                runnableItem,
                pendingPolls,
                controller,
                releaseResources,
                leaseOwner,
              )
            : await beginUpload(
                runnableItem,
                processSerially,
                pendingPolls,
                controller,
                releaseResources,
                leaseOwner,
              );
        if (result !== "pending") {
          await releaseResources();
          clearActiveController(item.id, controller);
        }
        return result;
      },
      PIPELINE_CONCURRENCY,
    );
    const statuses = [...started, ...(await Promise.all(pendingPolls))];
    uploadingRef.current = false;
    if (!mountedRef.current) return;
    setUploading(false);
    qc.invalidateQueries({ queryKey: ["images"] });
    refreshUser();
    const ok = statuses.filter((s) => s === "done").length;
    const fail = statuses.filter((s) => s === "failed").length;
    if (ok + fail === 0) return;
    if (fail === 0) toast.success(t("upload.toast.uploadSuccess", { count: ok }));
    else if (ok === 0) toast.error(t("upload.toast.uploadAllFailed"));
    else toast.warning(t("upload.toast.uploadPartial", { ok, fail }));
  };

  useEffect(() => {
    runUploadsRef.current = runUploads;
  });

  useEffect(() => {
    const ownerUserId = user?.id;
    if (isHydrating || !ownerUserId || hydratedOwnerRef.current === ownerUserId) return;
    hydratedOwnerRef.current = ownerUserId;
    for (const item of itemsRef.current) {
      if (item.ownerUserId !== ownerUserId && item.preview) {
        URL.revokeObjectURL(item.preview);
        previewURLsRef.current.delete(item.preview);
      }
    }
    setItems((current) => current.filter((item) => item.ownerUserId === ownerUserId));
    const controller = new AbortController();
    const stopRecovery = () => {
      controller.abort(abortError("Upload authentication scope changed"));
    };
    window.addEventListener(UPLOAD_AUTH_BOUNDARY_EVENT, stopRecovery);

    void (async () => {
      try {
        const recovered = await recoverPersistedUploadQueue(ownerUserId, controller.signal);
        assertCurrentUploadOwner(ownerUserId);
        const restored: QueueItem[] = [];
        const autoResume = new Set<string>();
        for (const record of recovered) {
          const file = sourceFileFromEntry(record.entry);
          if (!file) continue;
          const task = record.entry.task;
          const result = record.result;
          const failureMessage = record.failureCode === V1_UPLOAD_OUTCOME_UNKNOWN
            ? t("upload.persistence.v1OutcomeUnknown")
            : record.failureMessage ?? task.failureMessage;
          restored.push({
            id: task.queueId,
            ownerUserId,
            attempt: record.attempt,
            file,
            sourceFingerprint: task.sourceFingerprint,
            durability: "durable",
            restored: true,
            started: task.started,
            processed: record.entry.payload?.processed,
            progress:
              record.status === "done"
                ? 100
                : record.status === "serverProcessing"
                  ? 90
                  : task.status === "uploading"
                    ? 40
                    : 0,
            status: record.status,
            uploadId: record.uploadId,
            serverExpiresAt: record.serverExpiresAt,
            recipeVersion: task.recipeVersion,
            retryMode: record.retryMode,
            attemptVisibility: task.visibility,
            uniqueLink: result?.uniqueLink,
            assetLink: result?.assetLink,
            pipelineVersion: result?.pipelineVersion,
            failure: failureMessage
              ? {
                  fallback: failureMessage,
                  retryable: (record.failureRetryable ?? task.failureRetryable) !== false,
                }
              : undefined,
          });
          if (record.autoResume) autoResume.add(task.queueId);
        }
        if (!mountedRef.current || !isCurrentUploadOwner(ownerUserId)) return;
        setItems((current) => {
          const restoredIds = new Set(restored.map((item) => item.id));
          return [
            ...restored,
            ...current.filter(
              (item) => item.ownerUserId === ownerUserId && !restoredIds.has(item.id),
            ),
          ];
        });
        const resumable = restored.filter((item) => autoResume.has(item.id));
        if (resumable.length > 0) await runUploadsRef.current(resumable);
      } catch (error) {
        if (isAbortError(error) || !mountedRef.current || !isCurrentUploadOwner(ownerUserId)) {
          return;
        }
        setItems((current) =>
          current.map((item) =>
            item.ownerUserId === ownerUserId && item.started
              ? {
                  ...item,
                  status: "paused",
                  retryMode: item.status === "serverProcessing" ? "resume" : "reuse",
                }
              : item,
          ),
        );
      }
    })();

    return () => {
      window.removeEventListener(UPLOAD_AUTH_BOUNDARY_EVENT, stopRecovery);
      controller.abort(abortError("Upload recovery page unmounted"));
    };
  }, [isHydrating, user?.id, t]);

  const startUpload = () =>
    runUploads(items.filter((item) => item.status === "queued" || item.status === "paused"));
  const retryFailed = (id?: string) => {
    const retried = items
      .filter(
        (item) =>
          item.status === "failed" && item.failure?.retryable !== false && (!id || item.id === id),
      )
      .map((item) => {
        const startsNewAttempt = item.retryMode === "new";
        return {
          ...item,
          attempt: startsNewAttempt ? nextV2UploadAttempt(item.attempt) : item.attempt,
          status: "queued" as const,
          progress: item.retryMode === "resume" ? Math.max(90, item.progress) : 0,
          uploadId: startsNewAttempt ? undefined : item.uploadId,
          serverExpiresAt: startsNewAttempt ? undefined : item.serverExpiresAt,
          retryMode: item.retryMode === "resume" ? ("resume" as const) : undefined,
          attemptVisibility: startsNewAttempt ? undefined : item.attemptVisibility,
          uniqueLink: undefined,
          assetLink: undefined,
          pipelineVersion: undefined,
          failure: undefined,
          manualRetry: true,
        };
      });
    const replacements = new Map(retried.map((item) => [item.id, item]));
    setItems((current) => current.map((item) => replacements.get(item.id) ?? item));
    void runUploads(retried);
  };
  const retryAllFailed = () => retryFailed();
  const retryOne = (id: string) => retryFailed(id);

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    if (e.dataTransfer.files?.length) addFiles(e.dataTransfer.files);
  };

  const hasQueued = items.some((item) => item.status === "queued" || item.status === "paused");
  const completedLinks = items.filter((i) => i.status === "done" && i.uniqueLink);
  const retryableFailedCount = items.filter(
    (item) => item.status === "failed" && item.failure?.retryable !== false,
  ).length;

  const copyAllLinks = async () => {
    if (!completedLinks.length) return;
    const text = buildCopyText(
      window.location.origin,
      completedLinks.map((i) => ({
        uniqueLink: i.uniqueLink!,
        fileName: i.file.name,
        pipelineVersion: i.pipelineVersion,
        assetLink: i.assetLink,
      })),
      "markdown",
      "webp",
    );
    await copy(
      text,
      t("upload.toast.copied", {
        count: completedLinks.length,
        format: `${t("upload.copy.linkFormats.markdown")}·${t("upload.copy.imageFormats.webp")}`,
      }),
    );
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{t("upload.title")}</h1>

      <div
        role="button"
        tabIndex={0}
        onClick={() => inputRef.current?.click()}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") inputRef.current?.click();
        }}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={`grid cursor-pointer place-items-center rounded-3xl border-2 border-dashed p-10 text-center transition-colors ${
          dragging ? "border-primary bg-primary/5" : "border-border hover:bg-muted/50"
        }`}
      >
        <input
          ref={inputRef}
          type="file"
          accept=".png,.jpg,.jpeg,.bmp,.webp,.avif,image/png,image/jpeg,image/bmp,image/webp,image/avif"
          multiple
          className="hidden"
          onChange={(e) => {
            if (e.target.files?.length) addFiles(e.target.files);
            e.target.value = "";
          }}
        />
        <IconUpload className="size-10 text-muted-foreground" />
        <p className="mt-3 font-medium">{t("upload.dropzone")}</p>
        <p className="mt-1 text-sm text-muted-foreground">{t("upload.dropzoneHint")}</p>
      </div>

      <Card>
        <CardContent className="flex items-center gap-3 p-5">
          <Label className="text-sm">{t("upload.visibility")}</Label>
          <Select
            value={visibility}
            disabled={uploading}
            onValueChange={(v) => setVisibility(v as "public" | "private")}
          >
            <SelectTrigger className="h-9 w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="public">{t("upload.visibilityPublic")}</SelectItem>
              <SelectItem value="private">{t("upload.visibilityPrivate")}</SelectItem>
            </SelectContent>
          </Select>
        </CardContent>
      </Card>

      {items.length > 0 && (
        <Card>
          <CardContent className="space-y-3 p-5">
            <div className="flex items-center justify-between">
              <p className="font-medium">{t("upload.queue", { count: items.length })}</p>
              <div className="flex gap-2">
                <Button size="sm" disabled={!hasQueued || uploading} onClick={startUpload}>
                  {uploading ? <IconLoader2 className="animate-spin" /> : <IconUpload />}
                  {uploading ? t("upload.uploading") : t("upload.startUpload")}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={uploading}
                  onClick={() => navigate("/images")}
                >
                  {t("upload.done")}
                </Button>
                {retryableFailedCount > 0 && (
                  <Button size="sm" variant="outline" disabled={uploading} onClick={retryAllFailed}>
                    <IconRefresh />
                    {t("upload.retryAll", { count: retryableFailedCount })}
                  </Button>
                )}
                {completedLinks.length > 0 && (
                  <Button size="sm" variant="outline" onClick={copyAllLinks}>
                    {copied ? <IconCheck className="text-primary" /> : <IconLink />}
                    {t("upload.copy.button")} ({completedLinks.length})
                  </Button>
                )}
              </div>
            </div>
            <Separator />
            <ul className="space-y-3">
              {items.map((item) => {
                const variant = STATUS_VARIANT[item.status];
                return (
                  <li key={item.id} className="flex items-center gap-3">
                    {item.preview ? (
                      <img
                        src={item.preview}
                        alt={item.file.name}
                        loading="lazy"
                        decoding="async"
                        className="size-14 shrink-0 rounded-xl object-cover ring-1 ring-border"
                      />
                    ) : (
                      <div className="grid size-14 shrink-0 place-items-center rounded-xl bg-muted ring-1 ring-border">
                        <IconPhoto className="size-5 text-muted-foreground" />
                      </div>
                    )}
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate text-sm font-medium">{item.file.name}</span>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {formatBytes(item.file.size)}
                        </span>
                      </div>
                      <Progress value={item.progress} className="h-2" />
                      {item.failure && (
                        <p className="text-xs text-destructive" role="alert">
                          {formatQueueFailure(item.failure, t)}
                          {item.failure.code && (
                            <span className="ml-1 font-mono">
                              {t("upload.toast.errorCode", { code: item.failure.code })}
                            </span>
                          )}
                        </p>
                      )}
                    </div>
                    {!item.failure && item.status !== "done" && (
                      <Badge variant="outline" className="shrink-0">
                        {t(`upload.persistence.${item.durability}`)}
                      </Badge>
                    )}
                    <Badge variant={variant} className="shrink-0">
                      {item.status === "done" && <IconCheck />}
                      {(item.status === "saving" ||
                        item.status === "checking" ||
                        item.status === "processing" ||
                        item.status === "uploading" ||
                        item.status === "serverProcessing") && (
                        <IconLoader2 className="animate-spin" />
                      )}
                      {t(`upload.status.${item.status}`)}
                    </Badge>
                    {item.status === "failed" && item.failure?.retryable !== false && (
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        disabled={uploading}
                        onClick={() => retryOne(item.id)}
                        aria-label={t("upload.retry")}
                      >
                        <IconRefresh />
                      </Button>
                    )}
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      onClick={() => void removeItem(item.id)}
                      aria-label={t("upload.remove")}
                    >
                      <IconX />
                    </Button>
                  </li>
                );
              })}
            </ul>
          </CardContent>
        </Card>
      )}

      {items.length === 0 && (
        <div className="grid place-items-center py-6 text-center text-muted-foreground">
          <IconPhoto className="mb-2 size-8 opacity-40" />
          <p className="text-sm">{t("upload.empty")}</p>
        </div>
      )}
    </div>
  );
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

function abortError(message: string): DOMException {
  return new DOMException(message, "AbortError");
}

function formatQueueFailure(failure: QueueFailure, t: TFunction): string {
  if (failure.code) return t(`upload.errors.${failure.code}`, failure.details);
  return failure.fallback ?? t("upload.toast.uploadAllFailed");
}
