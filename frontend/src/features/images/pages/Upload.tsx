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
import { createProcessedPreviewURL } from "../processed-preview";
import { ConcurrencyGate, runWithConcurrency } from "../upload-concurrency";
import { beginV1Upload, type V1UploadResult } from "../v1-upload";
import {
  beginV2Upload,
  createV2IdempotencyKey,
  getV2Recipe,
  isV2UploadEnabled,
  nextV2UploadAttempt,
  v2UploadRetryDisposition,
  waitForV2Upload,
  type V2UploadResult,
} from "../v2-upload";

type Status =
  | "queued"
  | "checking"
  | "processing"
  | "uploading"
  | "serverProcessing"
  | "done"
  | "failed";
type RetryMode = "resume" | "reuse" | "new";
type CompletedUploadResult = V1UploadResult | V2UploadResult;

interface QueueFailure {
  code?: ClientImageErrorCode;
  details?: ClientImageErrorDetails;
  fallback?: string;
  retryable: boolean;
}

interface QueueItem {
  id: string;
  attempt: number;
  file: File;
  preview?: string;
  progress: number;
  status: Status;
  uploadId?: string;
  retryMode?: RetryMode;
  attemptVisibility?: "public" | "private";
  uniqueLink?: string;
  assetLink?: string;
  pipelineVersion?: number;
  failure?: QueueFailure;
}

function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

const STATUS_VARIANT: Record<Status, "default" | "secondary" | "destructive" | "outline"> = {
  queued: "outline",
  checking: "secondary",
  processing: "secondary",
  uploading: "secondary",
  serverProcessing: "secondary",
  done: "default",
  failed: "destructive",
};

const PIPELINE_CONCURRENCY = 2;
// The backend permits eight active sessions per user. Keep half available for
// status recovery or another tab while still keeping the single publish worker busy.
const ACTIVE_SESSION_CONCURRENCY = 4;
const MAX_SOURCE_BYTES = 15 * 1024 * 1024;

const ALLOWED_EXTS = [".png", ".jpg", ".jpeg", ".bmp", ".webp", ".avif"];

function createQueueID(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
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
  const [items, setItems] = useState<QueueItem[]>([]);
  const [visibility, setVisibility] = useState<"public" | "private">("public");
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const mountedRef = useRef(false);
  const uploadingRef = useRef(false);
  const activeControllersRef = useRef(new Map<string, AbortController>());
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
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      for (const controller of activeControllers.values()) {
        controller.abort(new DOMException("Upload page unmounted", "AbortError"));
      }
      activeControllers.clear();
      for (const preview of previewURLs) URL.revokeObjectURL(preview);
      previewURLs.clear();
    };
  }, []);

  const addFiles = useCallback((files: FileList | File[]) => {
    const next = Array.from(files)
      .map((f) => {
        const dot = f.name.lastIndexOf(".");
        const ext = dot >= 0 ? f.name.slice(dot).toLowerCase() : "";
        let failure: QueueFailure | undefined;
        if (f.size <= 0) {
          failure = { code: "IMAGE_FILE_INVALID", retryable: false };
        } else if (f.size > MAX_SOURCE_BYTES) {
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
          attempt: 0,
          file: f,
          progress: 0,
          status: (failure ? "failed" : "queued") as Status,
          failure,
        };
      });
    if (next.length) setItems((prev) => [...prev, ...next]);
  }, []);

  const removeItem = (id: string) => {
    setItems((prev) =>
      prev.filter((i) => {
        if (i.id === id && i.preview) {
          URL.revokeObjectURL(i.preview);
          previewURLsRef.current.delete(i.preview);
        }
        return i.id !== id;
      }),
    );
  };

  const patchItem = (id: string, value: Partial<QueueItem>) => {
    if (!mountedRef.current) return;
    setItems((previous) =>
      previous.map((candidate) => (candidate.id === id ? { ...candidate, ...value } : candidate)),
    );
  };

  const finishItem = (item: QueueItem, result: CompletedUploadResult): "done" => {
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

  const failItem = (
    item: QueueItem,
    error: unknown,
    phase: "begin" | "poll" = "begin",
    uploadId?: string,
  ): "failed" => {
    if (isAbortError(error)) return "failed";
    const clientError = isClientImageError(error) ? error : undefined;
    const message = clientError
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
    if (!clientError) {
      toast.error(t("upload.toast.itemFailed", { name: item.file.name, msg: message }));
    }
    const disposition = v2UploadRetryDisposition(error);
    patchItem(item.id, {
      status: "failed",
      uploadId: uploadId ?? item.uploadId,
      retryMode:
        phase === "poll" && uploadId
          ? disposition === "new-attempt"
            ? "new"
            : "resume"
          : disposition === "new-attempt"
            ? "new"
            : "reuse",
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

  const beginUpload = async (
    item: QueueItem,
    processSerially: <T>(task: () => Promise<T>) => Promise<T>,
    pendingPolls: Array<Promise<"done" | "failed">>,
    controller: AbortController,
    releaseSession: () => void,
  ): Promise<"done" | "failed" | "pending"> => {
    const patch = (value: Partial<QueueItem>) => patchItem(item.id, value);
    let knownUploadId = item.uploadId;
    const attemptVisibility = item.attemptVisibility ?? visibility;

    try {
      patch({ attemptVisibility });
      const recipe = await getV2Recipe(controller.signal);
      if (!isV2UploadEnabled(recipe)) {
        patch({ status: "uploading", progress: 10 });
        const result = await beginV1Upload(item.file, attemptVisibility, controller.signal);
        return finishItem(item, result);
      }
      patch({ status: "checking", progress: 0, failure: undefined });
      const plan = await preflightClientImage(item.file, recipe, controller.signal);
      const processed = await processSerially(async () => {
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
      attachProcessedPreview(item, processed);
      patch({ status: "uploading", progress: 40 });

      const started = await beginV2Upload({
        fileName: item.file.name,
        visibility: attemptVisibility,
        idempotencyKey: createV2IdempotencyKey(item.id, item.attempt),
        processed,
        onSession: (uploadId) => {
          knownUploadId = uploadId;
          patch({ uploadId });
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
      if (started.completed) return finishItem(item, started.completed);

      patch({ status: "serverProcessing", progress: 90 });
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
        .then((result) => finishItem(item, result))
        .catch((error: unknown) => failItem(item, error, "poll", started.uploadId))
        .finally(() => {
          clearActiveController(item.id, controller);
          releaseSession();
        });
      pendingPolls.push(polling);
      return "pending";
    } catch (error) {
      return failItem(item, error, "begin", knownUploadId);
    }
  };

  const beginStatusResume = (
    item: QueueItem,
    pendingPolls: Array<Promise<"done" | "failed">>,
    controller: AbortController,
    releaseSession: () => void,
  ): "pending" | "failed" => {
    const uploadId = item.uploadId;
    if (!uploadId) return failItem(item, new Error("Upload status cannot be resumed"));
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
        .then((result) => finishItem(item, result))
        .catch((error: unknown) => failItem(item, error, "poll", uploadId))
        .finally(() => {
          clearActiveController(item.id, controller);
          releaseSession();
        });
      pendingPolls.push(polling);
      return "pending";
    } catch (error) {
      return failItem(item, error, "poll", uploadId);
    }
  };

  const runUploads = async (toUpload: QueueItem[]) => {
    if (!toUpload.length || uploadingRef.current) return;
    uploadingRef.current = true;
    setUploading(true);
    const processSerially = createSerialExecutor();
    const activeSessions = new ConcurrencyGate(ACTIVE_SESSION_CONCURRENCY);
    const pendingPolls: Array<Promise<"done" | "failed">> = [];
    const started = await runWithConcurrency(
      toUpload,
      async (item) => {
        const controller = new AbortController();
        if (!mountedRef.current) {
          controller.abort(abortError("Upload page unmounted"));
        }
        activeControllersRef.current.get(item.id)?.abort(abortError("Upload attempt superseded"));
        activeControllersRef.current.set(item.id, controller);
        let releaseSession: (() => void) | undefined;
        try {
          releaseSession = await activeSessions.acquire(controller.signal);
        } catch {
          clearActiveController(item.id, controller);
          return "failed";
        }
        const result =
          item.retryMode === "resume" && item.uploadId
            ? beginStatusResume(item, pendingPolls, controller, releaseSession)
            : await beginUpload(
                item,
                processSerially,
                pendingPolls,
                controller,
                releaseSession,
              );
        if (result !== "pending") {
          releaseSession();
          clearActiveController(item.id, controller);
        }
        return result;
      },
      PIPELINE_CONCURRENCY,
    );
    const statuses = [
      ...started.filter((status): status is "done" | "failed" => status !== "pending"),
      ...(await Promise.all(pendingPolls)),
    ];
    uploadingRef.current = false;
    if (!mountedRef.current) return;
    setUploading(false);
    qc.invalidateQueries({ queryKey: ["images"] });
    refreshUser();
    const ok = statuses.filter((s) => s === "done").length;
    const fail = statuses.length - ok;
    if (fail === 0) toast.success(t("upload.toast.uploadSuccess", { count: ok }));
    else if (ok === 0) toast.error(t("upload.toast.uploadAllFailed"));
    else toast.warning(t("upload.toast.uploadPartial", { ok, fail }));
  };

  const startUpload = () => runUploads(items.filter((i) => i.status === "queued"));
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
          retryMode: item.retryMode === "resume" ? ("resume" as const) : undefined,
          attemptVisibility: startsNewAttempt ? undefined : item.attemptVisibility,
          uniqueLink: undefined,
          assetLink: undefined,
          pipelineVersion: undefined,
          failure: undefined,
        };
      });
    const replacements = new Map(retried.map((item) => [item.id, item]));
    setItems((current) => current.map((item) => replacements.get(item.id) ?? item));
    void runUploads(retried);
  };
  const retryAllFailed = () => retryFailed();
  const retryOne = (id: string) => retryFailed(id);

  const clearActiveController = (id: string, controller: AbortController) => {
    if (activeControllersRef.current.get(id) === controller) {
      activeControllersRef.current.delete(id);
    }
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    if (e.dataTransfer.files?.length) addFiles(e.dataTransfer.files);
  };

  const hasQueued = items.some((i) => i.status === "queued");
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
                    <Badge variant={variant} className="shrink-0">
                      {item.status === "done" && <IconCheck />}
                      {(item.status === "checking" ||
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
                      disabled={uploading}
                      onClick={() => removeItem(item.id)}
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
