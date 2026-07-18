// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { API_BASE_URL } from "@/config/constants";
import { api, refreshCsrfToken } from "@/lib/api";
import { getCsrfToken } from "@/lib/csrf";
import { ApiError } from "@/lib/errors";

import type { ProcessedImage, ProcessedPart, V2VariantKind } from "./client-processing/types";

type UploadSessionStatus =
  | "initiated"
  | "uploading"
  | "processing"
  | "completed"
  | "failed"
  | "cancelled"
  | "cleanup_pending";

interface UploadPartResponse {
  kind: V2VariantKind;
  status: "pending" | "received" | "finalized" | "cleaned";
  put_url?: string;
  size: number;
  sha256: string;
  width: number;
  height: number;
}

interface UploadSessionResponse {
  upload_id: string;
  status: UploadSessionStatus;
  image_id?: number;
  unique_link?: string;
  asset_link?: string;
  expires_at: string;
  parts: UploadPartResponse[];
}

export interface V2RecipeResponse {
  // Optional so a new frontend can still talk to an older V2-only server.
  v2_enabled?: boolean;
  pipeline_version: number;
  recipe_version: string;
  max_part_bytes: number;
  max_pixels: number;
  session_ttl_ms: number;
}

interface V2BatchStatusResponse {
  uploads: UploadSessionResponse[];
}

export interface V2UploadResult {
  uploadId: string;
  uniqueLink: string;
  assetLink: string;
  pipelineVersion: 2;
}

export interface V2UploadStartResult {
  uploadId: string;
  completed?: V2UploadResult;
}

export class V2UploadPendingError extends Error {
  readonly uploadId: string;

  constructor(uploadId: string) {
    super("Server image processing is still pending; retry status recovery");
    this.name = "V2UploadPendingError";
    this.uploadId = uploadId;
  }
}

class V2UploadTerminalError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "V2UploadTerminalError";
  }
}

export type V2UploadRetryDisposition = "new-attempt" | "reuse-attempt";

export function v2UploadRetryDisposition(error: unknown): V2UploadRetryDisposition {
  if (error instanceof V2UploadTerminalError) return "new-attempt";
  if (error instanceof ApiError && (error.code === 4043 || error.code === 4091)) {
    return "new-attempt";
  }
  return "reuse-attempt";
}

export function createV2IdempotencyKey(queueId: string, attempt: number): string {
  if (!Number.isSafeInteger(attempt) || attempt < 0) throw new Error("Invalid upload attempt");
  return `v2-${queueId}-attempt-${attempt}`;
}

export function nextV2UploadAttempt(attempt: number): number {
  if (!Number.isSafeInteger(attempt) || attempt < 0 || attempt === Number.MAX_SAFE_INTEGER) {
    throw new Error("Invalid upload attempt");
  }
  return attempt + 1;
}

export type V2UploadProgress = (phase: "uploading" | "server-processing", percent: number) => void;

export interface V2UploadOptions {
  fileName: string;
  visibility: "public" | "private";
  idempotencyKey: string;
  processed: ProcessedImage;
  onProgress: V2UploadProgress;
  onSession?: (uploadId: string) => void;
  signal?: AbortSignal;
}

const POLL_DEADLINE_MS = 10 * 60 * 1000;
const FAST_POLL_WINDOW_MS = 30_000;
const MEDIUM_POLL_WINDOW_MS = 2 * 60 * 1000;
const SESSION_RECOVERY_TIMEOUT_MS = 5_000;
const TRANSIENT_CODES = new Set([0, 1000, 1001, 1002, 1003, 4291, 5030]);
const STATUS_BATCH_SIZE = 100;
let recipePromise: Promise<V2RecipeResponse> | undefined;

export function isV2UploadEnabled(recipe: V2RecipeResponse): boolean {
  return recipe.v2_enabled !== false;
}

export function getV2Recipe(signal?: AbortSignal): Promise<V2RecipeResponse> {
  throwIfAborted(signal);
  if (!recipePromise) {
    recipePromise = api.get<V2RecipeResponse>("/uploads/recipe").then((recipe) => {
      if (
        isV2UploadEnabled(recipe) &&
        (recipe.pipeline_version !== 2 || recipe.recipe_version !== "2.0.0")
      ) {
        throw new Error("This client does not support the server image recipe");
      }
      return recipe;
    });
    recipePromise.catch(() => {
      recipePromise = undefined;
    });
  }
  return waitWithSignal(recipePromise, signal);
}

export async function beginV2Upload(options: V2UploadOptions): Promise<V2UploadStartResult> {
  const { fileName, visibility, idempotencyKey, processed, onProgress, onSession, signal } =
    options;
  throwIfAborted(signal);

  const recipe = await getV2Recipe(signal);
  throwIfAborted(signal);
  if (!isV2UploadEnabled(recipe)) {
    throw new Error("V2 image uploads are disabled by the server");
  }
  const pixels = processed.source.width * processed.source.height;
  if (pixels > recipe.max_pixels) throw new Error("Image exceeds the server pixel limit");
  if (processed.parts.some((part) => part.size > recipe.max_part_bytes)) {
    throw new Error("A processed image part exceeds the server size limit");
  }

  const manifest = buildManifest(fileName, visibility, processed);
  const session = await retryTransientRequest(
    () =>
      api.post<UploadSessionResponse>("/uploads/", manifest, {
        headers: { "Idempotency-Key": idempotencyKey },
        signal,
        csrfRetry: "idempotent",
      }),
    signal,
  );
  assertSessionMatches(session, processed);
  onSession?.(session.upload_id);

  if (isPublishedSession(session)) {
    return { uploadId: session.upload_id, completed: completedResult(session) };
  }
  assertActiveSession(session);
  if (session.status === "processing") {
    onProgress("server-processing", 0);
    return { uploadId: session.upload_id };
  }

  try {
    await uploadPendingParts(session, processed.parts, onProgress, signal);
    throwIfAborted(signal);
  } catch (error) {
    const cancelled = await bestEffortCancelSession(session.upload_id);
    if (!signal?.aborted && cancelled?.status === "cancelled") {
      throw new V2UploadTerminalError(errorMessage(error, "Upload session was cancelled"));
    }
    throw error;
  }

  let completed: UploadSessionResponse;
  try {
    completed = await requestCompletion(session.upload_id, signal);
  } catch (error) {
    if (signal?.aborted) {
      await bestEffortCancelSession(session.upload_id);
      throw signal.reason ?? abortError();
    }
    const recovered = await recoverCompletion(session.upload_id, onProgress, signal);
    if (recovered) return recovered;
    throw error;
  }
  if (isPublishedSession(completed)) {
    return { uploadId: completed.upload_id, completed: completedResult(completed) };
  }
  assertActiveSession(completed);
  if (completed.status !== "processing") {
    await bestEffortCancelSession(session.upload_id);
    throw new Error("Upload did not enter server processing");
  }
  onProgress("server-processing", 0);
  return { uploadId: completed.upload_id };
}

export function waitForV2Upload(
  uploadId: string,
  onProgress: V2UploadProgress,
  signal?: AbortSignal,
): Promise<V2UploadResult> {
  return statusCoordinator.wait(uploadId, onProgress, signal);
}

export function buildManifest(
  fileName: string,
  visibility: "public" | "private",
  processed: ProcessedImage,
) {
  return {
    filename: fileName,
    visibility,
    processor_version: processed.processor_version,
    recipe_version: processed.recipe_version,
    source: processed.source,
    parts: processed.parts.map((part) => ({
      kind: part.kind,
      size: part.size,
      sha256: part.sha256,
      mime_type: part.mime_type,
      width: part.width,
      height: part.height,
      quality: part.quality,
    })),
  };
}

async function uploadPendingParts(
  session: UploadSessionResponse,
  localParts: ProcessedPart[],
  onProgress: V2UploadProgress,
  signal?: AbortSignal,
): Promise<void> {
  const localByKind = new Map(localParts.map((part) => [part.kind, part]));
  const loaded = new Map<V2VariantKind, number>();
  const total = localParts.reduce((sum, part) => sum + part.size, 0);

  for (const remote of session.parts) {
    const local = localByKind.get(remote.kind);
    if (!local) throw new Error(`Upload session contains an unknown part: ${remote.kind}`);
    if (remote.status !== "pending") loaded.set(remote.kind, local.size);
  }

  const report = () => {
    const transferred = Array.from(loaded.values()).reduce((sum, value) => sum + value, 0);
    onProgress(
      "uploading",
      total > 0 ? Math.min(100, Math.round((transferred / total) * 100)) : 100,
    );
  };
  report();

  const queue = session.parts
    .filter((part) => part.status === "pending")
    .map((remote) => {
      const local = localByKind.get(remote.kind);
      if (!local || local.sha256 !== remote.sha256 || local.size !== remote.size) {
        throw new Error(`Upload session manifest mismatch for ${remote.kind}`);
      }
      return { remote, local };
    })
    .sort((a, b) => a.local.size - b.local.size);

  if (queue.length === 0) return;

  const controller = new AbortController();
  const abortFromParent = () => controller.abort(signal?.reason);
  signal?.addEventListener("abort", abortFromParent, { once: true });
  try {
    await Promise.all(
      queue.map(({ remote, local }) =>
        partUploadScheduler.run(async () => {
          throwIfAborted(controller.signal);
          const putURL = remote.put_url;
          if (!putURL) throw new Error(`Upload URL is missing for ${remote.kind}`);
          await putPartWithRetry(
            putURL,
            local,
            (bytes) => {
              loaded.set(local.kind, bytes);
              report();
            },
            controller.signal,
          );
          loaded.set(local.kind, local.size);
          report();
        }),
      ),
    );
  } catch (error) {
    controller.abort(error);
    throw error;
  } finally {
    signal?.removeEventListener("abort", abortFromParent);
  }
}

class PartUploadScheduler {
  private active = 0;
  private limit = 2;
  private completed = 0;
  private readonly queue: Array<{
    task: () => Promise<void>;
    resolve: () => void;
    reject: (error: unknown) => void;
  }> = [];

  run(task: () => Promise<void>): Promise<void> {
    return new Promise((resolve, reject) => {
      this.queue.push({ task, resolve, reject });
      this.pump();
    });
  }

  private pump(): void {
    while (this.active < this.limit && this.queue.length > 0) {
      const entry = this.queue.shift();
      if (!entry) return;
      this.active += 1;
      entry
        .task()
        .then(() => {
          this.completed += 1;
          if (this.completed === 1 && shouldRampUploadConcurrency()) this.limit = 3;
          entry.resolve();
        })
        .catch(entry.reject)
        .finally(() => {
          this.active -= 1;
          this.pump();
        });
    }
  }
}

const partUploadScheduler = new PartUploadScheduler();

async function putPartWithRetry(
  putURL: string,
  part: ProcessedPart,
  onLoaded: (bytes: number) => void,
  signal: AbortSignal,
): Promise<void> {
  let csrfRefreshed = false;
  for (let attempt = 0; attempt < 5; attempt += 1) {
    throwIfAborted(signal);
    try {
      await putPart(putURL, part, onLoaded, signal);
      return;
    } catch (error) {
      onLoaded(0);
      if (error instanceof ApiError && error.code === 4036 && !csrfRefreshed) {
        await waitWithSignal(refreshCsrfToken(), signal);
        csrfRefreshed = true;
        attempt -= 1;
        continue;
      }
      if (signal.aborted || !isTransient(error) || attempt === 4) throw error;
      await delay(Math.min(8_000, 500 * 2 ** attempt) + Math.floor(Math.random() * 250), signal);
    }
  }
}

function putPart(
  putURL: string,
  part: ProcessedPart,
  onLoaded: (bytes: number) => void,
  signal: AbortSignal,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const url = putURL.startsWith("/") ? putURL : `${API_BASE_URL}/${putURL.replace(/^\/+/, "")}`;
    xhr.open("PUT", url);
    xhr.withCredentials = true;
    xhr.timeout = 2 * 60 * 1000;
    xhr.setRequestHeader("Content-Type", "image/webp");
    const csrf = getCsrfToken();
    if (csrf) xhr.setRequestHeader("X-CSRF-Token", csrf);

    const abort = () => xhr.abort();
    signal.addEventListener("abort", abort, { once: true });
    const finish = (fn: () => void) => {
      signal.removeEventListener("abort", abort);
      fn();
    };

    xhr.upload.onprogress = (event) => onLoaded(Math.min(part.size, event.loaded));
    xhr.onload = () => {
      let body: { code?: number; message?: string };
      try {
        body = JSON.parse(xhr.responseText) as { code?: number; message?: string };
      } catch {
        finish(() => reject(new ApiError(0, "Could not parse the upload response")));
        return;
      }
      if (body.code !== 0) {
        finish(() =>
          reject(new ApiError(body.code ?? xhr.status, body.message || "Upload failed")),
        );
        return;
      }
      finish(resolve);
    };
    xhr.onerror = () => finish(() => reject(new ApiError(0, "Network error while uploading")));
    xhr.ontimeout = () => finish(() => reject(new ApiError(0, "Upload timed out")));
    xhr.onabort = () => finish(() => reject(new DOMException("Upload aborted", "AbortError")));
    try {
      xhr.send(part.blob);
    } catch (error) {
      finish(() => reject(error));
    }
  });
}

interface StatusWaiter {
  startedAt: number;
  deadline: number;
  onProgress: V2UploadProgress;
  resolve: (result: V2UploadResult) => void;
  reject: (error: unknown) => void;
  signal?: AbortSignal;
  abort?: () => void;
}

class V2StatusCoordinator {
  private readonly waiters = new Map<string, Set<StatusWaiter>>();
  private timer: number | undefined;
  private running = false;

  wait(
    uploadId: string,
    onProgress: V2UploadProgress,
    signal?: AbortSignal,
  ): Promise<V2UploadResult> {
    throwIfAborted(signal);
    return new Promise((resolve, reject) => {
      const startedAt = Date.now();
      const waiter: StatusWaiter = {
        startedAt,
        deadline: startedAt + POLL_DEADLINE_MS,
        onProgress,
        resolve,
        reject,
        signal,
      };
      waiter.abort = () => this.remove(uploadId, waiter, signal?.reason ?? abortError());
      signal?.addEventListener("abort", waiter.abort, { once: true });
      const group = this.waiters.get(uploadId) ?? new Set<StatusWaiter>();
      group.add(waiter);
      this.waiters.set(uploadId, group);
      this.schedule(250);
    });
  }

  private schedule(delayMs: number): void {
    if (this.running || this.timer !== undefined || this.waiters.size === 0) return;
    this.timer = window.setTimeout(() => {
      this.timer = undefined;
      void this.tick();
    }, delayMs);
  }

  private async tick(): Promise<void> {
    if (this.running || this.waiters.size === 0) return;
    this.running = true;
    const now = Date.now();
    for (const [uploadId, group] of this.waiters) {
      for (const waiter of group) {
        if (waiter.deadline <= now) {
          this.remove(
            uploadId,
            waiter,
            new V2UploadPendingError(uploadId),
          );
        }
      }
    }

    const uploadIds = Array.from(this.waiters.keys());
    try {
      for (let offset = 0; offset < uploadIds.length; offset += STATUS_BATCH_SIZE) {
        await this.processStatusChunk(uploadIds.slice(offset, offset + STATUS_BATCH_SIZE));
      }
    } finally {
      this.running = false;
      this.schedule(this.nextPollDelay(Date.now()));
    }
  }

  private nextPollDelay(now: number): number {
    let delay = v2StatusPollDelay(MEDIUM_POLL_WINDOW_MS);
    for (const group of this.waiters.values()) {
      for (const waiter of group) {
        delay = Math.min(delay, v2StatusPollDelay(Math.max(0, now - waiter.startedAt)));
      }
    }
    return delay;
  }

  private async processStatusChunk(uploadIds: string[]): Promise<void> {
    if (uploadIds.length === 0) return;

    let response: V2BatchStatusResponse;
    try {
      response = await requestStatusBatch(uploadIds);
    } catch (error) {
      if (error instanceof ApiError && error.code === 4043 && uploadIds.length > 1) {
        const middle = Math.floor(uploadIds.length / 2);
        await this.processStatusChunk(uploadIds.slice(0, middle));
        await this.processStatusChunk(uploadIds.slice(middle));
        return;
      }
      for (const uploadId of uploadIds) this.recordStatusFailure(uploadId, error);
      return;
    }

    const sessions = new Map(response.uploads.map((session) => [session.upload_id, session]));
    for (const uploadId of uploadIds) {
      const session = sessions.get(uploadId);
      if (!session) {
        // Batch aggregation can briefly lag behind an accepted upload. Keep the
        // waiter attached and let the next poll reconcile the session.
        continue;
      }
      this.handleSession(session);
    }
  }

  private handleSession(session: UploadSessionResponse): void {
    const group = this.waiters.get(session.upload_id);
    if (!group) return;
    if (isPublishedSession(session)) {
      let result: V2UploadResult;
      try {
        result = completedResult(session);
      } catch (error) {
        for (const waiter of Array.from(group)) this.remove(session.upload_id, waiter, error);
        return;
      }
      for (const waiter of Array.from(group)) this.finish(session.upload_id, waiter, result);
      return;
    }
    try {
      assertActiveSession(session);
    } catch (error) {
      for (const waiter of Array.from(group)) this.remove(session.upload_id, waiter, error);
      return;
    }
    const now = Date.now();
    for (const waiter of group) {
      const elapsed = now - waiter.startedAt;
      waiter.onProgress(
        "server-processing",
        Math.min(99, Math.round((elapsed / POLL_DEADLINE_MS) * 100)),
      );
    }
  }

  private recordStatusFailure(uploadId: string, error: unknown): void {
    if (isTransient(error)) return;
    const group = this.waiters.get(uploadId);
    if (!group) return;
    for (const waiter of Array.from(group)) {
      this.remove(uploadId, waiter, error);
    }
  }

  private finish(uploadId: string, waiter: StatusWaiter, result: V2UploadResult): void {
    this.detach(uploadId, waiter);
    waiter.resolve(result);
  }

  private remove(uploadId: string, waiter: StatusWaiter, error: unknown): void {
    this.detach(uploadId, waiter);
    waiter.reject(error);
  }

  private detach(uploadId: string, waiter: StatusWaiter): void {
    waiter.signal?.removeEventListener("abort", waiter.abort!);
    const group = this.waiters.get(uploadId);
    group?.delete(waiter);
    if (group?.size === 0) this.waiters.delete(uploadId);
    if (this.waiters.size === 0 && this.timer !== undefined) {
      window.clearTimeout(this.timer);
      this.timer = undefined;
    }
  }
}

async function requestStatusBatch(uploadIds: string[]): Promise<V2BatchStatusResponse> {
  const controller = new AbortController();
  const timeout = window.setTimeout(
    () => controller.abort(abortError("Status request timed out")),
    15_000,
  );
  try {
    return await api.post<V2BatchStatusResponse>(
      "/uploads/status",
      { upload_ids: uploadIds },
      { signal: controller.signal, csrfRetry: "idempotent" },
    );
  } finally {
    window.clearTimeout(timeout);
  }
}

const statusCoordinator = new V2StatusCoordinator();

export function v2StatusPollDelay(elapsedMs: number): number {
  if (!Number.isFinite(elapsedMs) || elapsedMs < 0) return 2_000;
  if (elapsedMs < FAST_POLL_WINDOW_MS) return 2_000;
  if (elapsedMs < MEDIUM_POLL_WINDOW_MS) return 5_000;
  return 10_000;
}

function completedResult(session: UploadSessionResponse): V2UploadResult {
  if (!session.unique_link || !session.asset_link) {
    throw new Error("Completed upload is missing its image link");
  }
  return {
    uploadId: session.upload_id,
    uniqueLink: session.unique_link,
    assetLink: session.asset_link,
    pipelineVersion: 2,
  };
}

function isPublishedSession(session: UploadSessionResponse): boolean {
  return (
    session.status === "completed" ||
    (session.status === "cleanup_pending" && session.image_id !== undefined)
  );
}

async function recoverCompletion(
  uploadId: string,
  onProgress: V2UploadProgress,
  signal?: AbortSignal,
): Promise<V2UploadStartResult | undefined> {
  const controller = new AbortController();
  const timeout = window.setTimeout(
    () => controller.abort(abortError("Upload status recovery timed out")),
    SESSION_RECOVERY_TIMEOUT_MS,
  );
  const abortFromParent = () => controller.abort(signal?.reason ?? abortError());
  signal?.addEventListener("abort", abortFromParent, { once: true });
  let current: UploadSessionResponse | undefined;
  try {
    throwIfAborted(signal);
    try {
      current = await requestUploadStatus(uploadId, controller.signal);
    } catch {
      current = undefined;
    }

    const currentResult = recoveredStartResult(current, onProgress);
    if (currentResult) return currentResult;
    if (current && current.status !== "initiated" && current.status !== "uploading") {
      return undefined;
    }

    try {
      throwIfAborted(signal);
      const completed = await requestCompletion(uploadId, controller.signal, 2);
      const completedResult = recoveredStartResult(completed, onProgress);
      if (completedResult) return completedResult;
      current = completed;
    } catch {
      try {
        current = await requestUploadStatus(uploadId, controller.signal);
      } catch {
        current = undefined;
      }
      const retriedResult = recoveredStartResult(current, onProgress);
      if (retriedResult) return retriedResult;
    }
  } catch {
    current = undefined;
  } finally {
    window.clearTimeout(timeout);
    signal?.removeEventListener("abort", abortFromParent);
  }

  if (signal?.aborted) {
    await bestEffortCancelSession(uploadId);
    throw signal.reason ?? abortError();
  }
  if (!current || current.status === "initiated" || current.status === "uploading") {
    const cancelled = await bestEffortCancelSession(uploadId);
    if (cancelled?.status === "cancelled") {
      throw new V2UploadTerminalError("Upload session was cancelled during recovery");
    }
  }
  return undefined;
}

function recoveredStartResult(
  session: UploadSessionResponse | undefined,
  onProgress: V2UploadProgress,
): V2UploadStartResult | undefined {
  if (!session) return undefined;
  if (isPublishedSession(session)) {
    return { uploadId: session.upload_id, completed: completedResult(session) };
  }
  if (session.status === "processing") {
    onProgress("server-processing", 0);
    return { uploadId: session.upload_id };
  }
  return undefined;
}

function requestUploadStatus(
  uploadId: string,
  signal: AbortSignal,
): Promise<UploadSessionResponse> {
  return api.get<UploadSessionResponse>(`/uploads/${encodeURIComponent(uploadId)}`, {
    signal,
    skipAuthRedirect: true,
  });
}

function requestCompletion(
  uploadId: string,
  signal?: AbortSignal,
  maxAttempts = 3,
): Promise<UploadSessionResponse> {
  return retryTransientRequest(
    () =>
      api.post<UploadSessionResponse>(
        `/uploads/${encodeURIComponent(uploadId)}/complete`,
        undefined,
        { signal, csrfRetry: "idempotent" },
      ),
    signal,
    maxAttempts,
  );
}

async function bestEffortCancelSession(
  uploadId: string,
): Promise<UploadSessionResponse | undefined> {
  const controller = new AbortController();
  const timeout = window.setTimeout(
    () => controller.abort(abortError("Upload cancellation timed out")),
    SESSION_RECOVERY_TIMEOUT_MS,
  );
  try {
    return await api.del<UploadSessionResponse>(`/uploads/${encodeURIComponent(uploadId)}`, {
      signal: controller.signal,
      skipAuthRedirect: true,
      csrfRetry: "idempotent",
    });
  } catch {
    // A processing/completed session deliberately rejects cancellation. Network
    // failures are left for the server-side expiry cleanup to reconcile.
  } finally {
    window.clearTimeout(timeout);
  }
  return undefined;
}

function assertActiveSession(session: UploadSessionResponse): void {
  if (session.status === "failed") {
    throw new V2UploadTerminalError("Server image processing failed");
  }
  if (session.status === "cancelled") {
    throw new V2UploadTerminalError("Upload session was cancelled");
  }
  if (session.status === "cleanup_pending") {
    throw new V2UploadTerminalError(
      "Upload cleanup is pending; retry with a new upload attempt",
    );
  }
}

function assertSessionMatches(session: UploadSessionResponse, processed: ProcessedImage): void {
  const local = new Map(processed.parts.map((part) => [part.kind, part]));
  if (session.parts.length !== local.size) throw new Error("Upload session manifest mismatch");
  for (const remote of session.parts) {
    const part = local.get(remote.kind);
    if (!part || part.sha256 !== remote.sha256 || part.size !== remote.size) {
      throw new Error(`Upload session manifest mismatch for ${remote.kind}`);
    }
  }
}

function shouldRampUploadConcurrency(): boolean {
  const navigatorWithHints = navigator as Navigator & {
    connection?: { effectiveType?: string; saveData?: boolean };
    deviceMemory?: number;
  };
  const connection = navigatorWithHints.connection;
  return (
    !connection?.saveData &&
    (!connection?.effectiveType || connection.effectiveType === "4g") &&
    (navigator.hardwareConcurrency || 2) >= 6 &&
    (navigatorWithHints.deviceMemory ?? 4) >= 4
  );
}

async function retryTransientRequest<T>(
  request: () => Promise<T>,
  signal?: AbortSignal,
  maxAttempts = 3,
): Promise<T> {
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    throwIfAborted(signal);
    try {
      return await waitWithSignal(request(), signal);
    } catch (error) {
      if (signal?.aborted) throw signal.reason ?? abortError();
      if (!isTransient(error) || attempt === maxAttempts - 1) throw error;
      await delay(250 * 2 ** attempt, signal);
    }
  }
  throw new Error("Transient request retry exhausted");
}

function isTransient(error: unknown): boolean {
  return error instanceof ApiError && TRANSIENT_CODES.has(error.code);
}

function errorMessage(error: unknown, fallback: string): string {
  if (
    error &&
    typeof error === "object" &&
    "message" in error &&
    typeof error.message === "string" &&
    error.message
  ) {
    return error.message;
  }
  return fallback;
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw signal.reason ?? abortError();
}

function abortError(message = "Operation aborted"): DOMException {
  return new DOMException(message, "AbortError");
}

function delay(milliseconds: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => {
      signal?.removeEventListener("abort", abort);
      resolve();
    }, milliseconds);
    const abort = () => {
      window.clearTimeout(timer);
      reject(signal?.reason ?? new DOMException("Operation aborted", "AbortError"));
    };
    signal?.addEventListener("abort", abort, { once: true });
  });
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
