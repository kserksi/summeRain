// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { deleteDB, openDB, type DBSchema, type IDBPDatabase } from "idb";

import type { ProcessedImage } from "./client-processing/types";

const DATABASE_NAME = "summerain-upload-queue";
const DATABASE_VERSION = 1;
const STORAGE_RESERVE_BYTES = 32 * 1024 * 1024;

export const UPLOAD_QUEUE_RETENTION_MS = 24 * 60 * 60 * 1000;
export const UPLOAD_LEASE_MS = 30_000;

export type PersistedUploadStatus =
  | "queued"
  | "checking"
  | "processing"
  | "uploading"
  | "serverProcessing"
  | "failed";

export type PersistedRetryMode = "resume" | "reuse" | "new";

export interface PersistedUploadTask {
  ownerUserId: number;
  queueId: string;
  attempt: number;
  fileName: string;
  fileType: string;
  fileSize: number;
  lastModified: number;
  sourceFingerprint: string;
  status: PersistedUploadStatus;
  started: boolean;
  visibility?: "public" | "private";
  uploadId?: string;
  serverExpiresAt?: string;
  recipeVersion?: string;
  pipelineMode?: "v1" | "v2";
  retryMode?: PersistedRetryMode;
  failureCode?: string;
  failureMessage?: string;
  failureRetryable?: boolean;
  leaseOwner?: string;
  leaseExpiresAt?: number;
  createdAt: number;
  updatedAt: number;
  expiresAt: number;
}

export interface PersistedUploadPayload {
  ownerUserId: number;
  queueId: string;
  sourceBlob: Blob;
  processed?: ProcessedImage;
}

export interface PersistedUploadEntry {
  task: PersistedUploadTask;
  payload?: PersistedUploadPayload;
}

export type PersistedUploadRunPreparation =
  | { outcome: "ready"; task: PersistedUploadTask; manualRetryApplied: boolean }
  | { outcome: "blocked"; task: PersistedUploadTask }
  | { outcome: "missing" }
  | { outcome: "lease-lost" };

interface UploadQueueDatabase extends DBSchema {
  tasks: {
    key: [number, string];
    value: PersistedUploadTask;
    indexes: {
      "by-owner": number;
      "by-expiry": number;
    };
  };
  payloads: {
    key: [number, string];
    value: PersistedUploadPayload;
    indexes: {
      "by-owner": number;
    };
  };
}

export class UploadQueueStorageError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "UploadQueueStorageError";
  }
}

export class UploadQueueLeaseLostError extends Error {
  constructor() {
    super("Upload lease moved to another tab");
    this.name = "UploadQueueLeaseLostError";
  }
}

let databasePromise: Promise<IDBPDatabase<UploadQueueDatabase>> | undefined;

function uploadKey(ownerUserId: number, queueId: string): [number, string] {
  return [ownerUserId, queueId];
}

function getDatabase(): Promise<IDBPDatabase<UploadQueueDatabase>> {
  if (!databasePromise) {
    databasePromise = openDB<UploadQueueDatabase>(DATABASE_NAME, DATABASE_VERSION, {
      upgrade(database) {
        const tasks = database.createObjectStore("tasks", {
          keyPath: ["ownerUserId", "queueId"],
        });
        tasks.createIndex("by-owner", "ownerUserId");
        tasks.createIndex("by-expiry", "expiresAt");

        const payloads = database.createObjectStore("payloads", {
          keyPath: ["ownerUserId", "queueId"],
        });
        payloads.createIndex("by-owner", "ownerUserId");
      },
      terminated() {
        databasePromise = undefined;
      },
    });
    databasePromise.catch(() => {
      databasePromise = undefined;
    });
  }
  return databasePromise;
}

export async function requestPersistentUploadStorage(): Promise<boolean> {
  try {
    if (!navigator.storage?.persist) return false;
    return await navigator.storage.persist();
  } catch {
    return false;
  }
}

export async function hasUploadStorageCapacity(additionalBytes: number): Promise<boolean> {
  if (!Number.isFinite(additionalBytes) || additionalBytes < 0) return false;
  try {
    const estimate = await navigator.storage?.estimate?.();
    if (!estimate?.quota || estimate.usage == null) return true;
    const reserve = Math.max(STORAGE_RESERVE_BYTES, Math.floor(estimate.quota * 0.1));
    return estimate.quota - estimate.usage - reserve >= additionalBytes;
  } catch {
    return true;
  }
}

export async function fingerprintUploadSource(file: Blob): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", await file.arrayBuffer());
  return Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, "0")).join(
    "",
  );
}

export async function persistQueuedUpload(
  task: PersistedUploadTask,
  sourceBlob: Blob,
): Promise<void> {
  if (!(await hasUploadStorageCapacity(sourceBlob.size))) {
    throw new UploadQueueStorageError("Browser storage does not have enough free space");
  }
  try {
    const database = await getDatabase();
    const transaction = database.transaction(["tasks", "payloads"], "readwrite");
    await Promise.all([
      transaction.objectStore("tasks").put(task),
      transaction.objectStore("payloads").put({
        ownerUserId: task.ownerUserId,
        queueId: task.queueId,
        sourceBlob,
      }),
    ]);
    await transaction.done;
  } catch (error) {
    throw new UploadQueueStorageError("Could not persist the upload queue", { cause: error });
  }
}

export async function listPersistedUploads(
  ownerUserId: number,
  now = Date.now(),
): Promise<PersistedUploadEntry[]> {
  await cleanupExpiredUploads(now);
  const database = await getDatabase();
  const tasks = await database.getAllFromIndex("tasks", "by-owner", ownerUserId);
  const payloads = await Promise.all(
    tasks.map((task) => database.get("payloads", uploadKey(ownerUserId, task.queueId))),
  );
  return tasks.map((task, index) => ({ task, payload: payloads[index] }));
}

export async function updatePersistedUpload(
  ownerUserId: number,
  queueId: string,
  patch: Partial<Omit<PersistedUploadTask, "ownerUserId" | "queueId" | "createdAt" | "expiresAt">>,
  expectedLeaseOwner?: string,
): Promise<boolean> {
  const database = await getDatabase();
  const transaction = database.transaction("tasks", "readwrite");
  const store = transaction.objectStore("tasks");
  const task = await store.get(uploadKey(ownerUserId, queueId));
  if (!task || (expectedLeaseOwner && task.leaseOwner !== expectedLeaseOwner)) {
    await transaction.done;
    return false;
  }
  await store.put({ ...task, ...patch, updatedAt: Date.now() });
  await transaction.done;
  return true;
}

export async function preparePersistedUploadRun(
  ownerUserId: number,
  queueId: string,
  expectedLeaseOwner: string,
  manualRetry: boolean,
): Promise<PersistedUploadRunPreparation> {
  const database = await getDatabase();
  const transaction = database.transaction("tasks", "readwrite");
  const store = transaction.objectStore("tasks");
  const task = await store.get(uploadKey(ownerUserId, queueId));
  if (!task) {
    await transaction.done;
    return { outcome: "missing" };
  }
  if (task.leaseOwner !== expectedLeaseOwner) {
    await transaction.done;
    return { outcome: "lease-lost" };
  }
  if (!manualRetry || task.status !== "failed") {
    await transaction.done;
    return { outcome: "ready", task, manualRetryApplied: false };
  }
  if (task.failureRetryable === false) {
    await transaction.done;
    return { outcome: "blocked", task };
  }

  const retryMode = task.retryMode;
  const prepared: PersistedUploadTask = {
    ...task,
    status: retryMode === "resume" && task.uploadId ? "serverProcessing" : "queued",
    retryMode: retryMode === "resume" && task.uploadId ? "resume" : undefined,
    failureCode: undefined,
    failureMessage: undefined,
    failureRetryable: undefined,
    updatedAt: Date.now(),
  };
  if (retryMode === "new") {
    if (
      !Number.isSafeInteger(task.attempt) ||
      task.attempt < 0 ||
      task.attempt === Number.MAX_SAFE_INTEGER
    ) {
      transaction.abort();
      await transaction.done.catch(() => undefined);
      throw new Error("Invalid upload attempt");
    }
    prepared.attempt = task.attempt + 1;
    prepared.uploadId = undefined;
    prepared.serverExpiresAt = undefined;
    prepared.pipelineMode = undefined;
  }
  await store.put(prepared);
  await transaction.done;
  return { outcome: "ready", task: prepared, manualRetryApplied: true };
}

export async function persistProcessedUpload(
  ownerUserId: number,
  queueId: string,
  processed: ProcessedImage,
  expectedLeaseOwner?: string,
): Promise<void> {
  const additionalBytes = processed.parts.reduce((total, part) => total + part.size, 0);
  if (!(await hasUploadStorageCapacity(additionalBytes))) {
    throw new UploadQueueStorageError("Browser storage cannot persist the processed upload");
  }

  try {
    const database = await getDatabase();
    const transaction = database.transaction(["tasks", "payloads"], "readwrite");
    const tasks = transaction.objectStore("tasks");
    const payloads = transaction.objectStore("payloads");
    const key = uploadKey(ownerUserId, queueId);
    const [task, payload] = await Promise.all([tasks.get(key), payloads.get(key)]);
    if (!task || !payload) {
      throw new UploadQueueStorageError("The persisted upload source is missing");
    }
    if (expectedLeaseOwner && task.leaseOwner !== expectedLeaseOwner) {
      throw new UploadQueueLeaseLostError();
    }
    await Promise.all([
      payloads.put({ ...payload, processed }),
      tasks.put({
        ...task,
        recipeVersion: processed.recipe_version,
        pipelineMode: "v2",
        status: "uploading",
        updatedAt: Date.now(),
      }),
    ]);
    await transaction.done;
  } catch (error) {
    if (error instanceof UploadQueueStorageError || error instanceof UploadQueueLeaseLostError) {
      throw error;
    }
    throw new UploadQueueStorageError("Could not persist processed upload data", { cause: error });
  }
}

export async function deletePersistedProcessedUpload(
  ownerUserId: number,
  queueId: string,
  expectedLeaseOwner?: string,
): Promise<boolean> {
  const database = await getDatabase();
  const transaction = database.transaction(["tasks", "payloads"], "readwrite");
  const tasks = transaction.objectStore("tasks");
  const store = transaction.objectStore("payloads");
  const key = uploadKey(ownerUserId, queueId);
  if (expectedLeaseOwner) {
    const task = await tasks.get(key);
    if (!task || task.leaseOwner !== expectedLeaseOwner) {
      await transaction.done;
      return false;
    }
  }
  const payload = await store.get(key);
  if (payload?.processed) {
    const sourceOnly = { ...payload };
    delete sourceOnly.processed;
    await store.put(sourceOnly);
  }
  await transaction.done;
  return true;
}

export async function deletePersistedUpload(
  ownerUserId: number,
  queueId: string,
  expectedLeaseOwner?: string,
): Promise<boolean> {
  const database = await getDatabase();
  const transaction = database.transaction(["tasks", "payloads"], "readwrite");
  const key = uploadKey(ownerUserId, queueId);
  if (expectedLeaseOwner) {
    const task = await transaction.objectStore("tasks").get(key);
    if (!task || task.leaseOwner !== expectedLeaseOwner) {
      await transaction.done;
      return false;
    }
  }
  await Promise.all([
    transaction.objectStore("tasks").delete(key),
    transaction.objectStore("payloads").delete(key),
  ]);
  await transaction.done;
  return true;
}

export async function clearPersistedUploadsForUser(ownerUserId: number): Promise<void> {
  const database = await getDatabase();
  const [taskKeys, payloadKeys] = await Promise.all([
    database.getAllKeysFromIndex("tasks", "by-owner", ownerUserId),
    database.getAllKeysFromIndex("payloads", "by-owner", ownerUserId),
  ]);
  if (taskKeys.length === 0 && payloadKeys.length === 0) return;
  const transaction = database.transaction(["tasks", "payloads"], "readwrite");
  await Promise.all([
    ...taskKeys.map((key) => transaction.objectStore("tasks").delete(key)),
    ...payloadKeys.map((key) => transaction.objectStore("payloads").delete(key)),
  ]);
  await transaction.done;
}

export async function clearPersistedUploadsExceptUser(ownerUserId: number): Promise<void> {
  const database = await getDatabase();
  const now = Date.now();
  const [tasks, payloadKeys] = await Promise.all([
    database.getAll("tasks"),
    database.getAllKeys("payloads"),
  ]);
  const staleTasks = tasks.filter(
    (task) => task.ownerUserId !== ownerUserId || task.expiresAt <= now,
  );
  const stalePayloadKeys = payloadKeys.filter(([owner]) => owner !== ownerUserId);
  if (staleTasks.length === 0 && stalePayloadKeys.length === 0) return;
  const transaction = database.transaction(["tasks", "payloads"], "readwrite");
  await Promise.all([
    ...staleTasks.flatMap((task) => {
      const key = uploadKey(task.ownerUserId, task.queueId);
      return [
        transaction.objectStore("tasks").delete(key),
        transaction.objectStore("payloads").delete(key),
      ];
    }),
    ...stalePayloadKeys.map((key) => transaction.objectStore("payloads").delete(key)),
  ]);
  await transaction.done;
}

export async function cleanupExpiredUploads(now = Date.now()): Promise<number> {
  const database = await getDatabase();
  const tasks = await database.getAll("tasks");
  const expired = tasks.filter((task) => task.expiresAt <= now);
  if (expired.length === 0) return 0;
  const transaction = database.transaction(["tasks", "payloads"], "readwrite");
  await Promise.all(
    expired.flatMap((task) => {
      const key = uploadKey(task.ownerUserId, task.queueId);
      return [
        transaction.objectStore("tasks").delete(key),
        transaction.objectStore("payloads").delete(key),
      ];
    }),
  );
  await transaction.done;
  return expired.length;
}

export async function hasPersistedUpload(
  ownerUserId: number,
  queueId: string,
): Promise<boolean> {
  const database = await getDatabase();
  return (await database.getKey("tasks", uploadKey(ownerUserId, queueId))) !== undefined;
}

export async function acquireUploadLease(
  ownerUserId: number,
  queueId: string,
  leaseOwner: string,
  now = Date.now(),
): Promise<boolean> {
  const database = await getDatabase();
  const transaction = database.transaction("tasks", "readwrite");
  const store = transaction.objectStore("tasks");
  const task = await store.get(uploadKey(ownerUserId, queueId));
  if (!task) {
    await transaction.done;
    return false;
  }
  if (task.leaseOwner && task.leaseOwner !== leaseOwner && (task.leaseExpiresAt ?? 0) > now) {
    await transaction.done;
    return false;
  }
  await store.put({
    ...task,
    leaseOwner,
    leaseExpiresAt: now + UPLOAD_LEASE_MS,
    updatedAt: now,
  });
  await transaction.done;
  return true;
}

export async function renewUploadLease(
  ownerUserId: number,
  queueId: string,
  leaseOwner: string,
  now = Date.now(),
): Promise<boolean> {
  const database = await getDatabase();
  const transaction = database.transaction("tasks", "readwrite");
  const store = transaction.objectStore("tasks");
  const task = await store.get(uploadKey(ownerUserId, queueId));
  if (!task || task.leaseOwner !== leaseOwner) {
    await transaction.done;
    return false;
  }
  await store.put({
    ...task,
    leaseExpiresAt: now + UPLOAD_LEASE_MS,
    updatedAt: now,
  });
  await transaction.done;
  return true;
}

export async function releaseUploadLease(
  ownerUserId: number,
  queueId: string,
  leaseOwner: string,
): Promise<void> {
  const database = await getDatabase();
  const transaction = database.transaction("tasks", "readwrite");
  const store = transaction.objectStore("tasks");
  const task = await store.get(uploadKey(ownerUserId, queueId));
  if (task?.leaseOwner === leaseOwner) {
    const released = { ...task };
    delete released.leaseOwner;
    delete released.leaseExpiresAt;
    await store.put({ ...released, updatedAt: Date.now() });
  }
  await transaction.done;
}

export function sourceFileFromEntry(entry: PersistedUploadEntry): File | undefined {
  if (!entry.payload) return undefined;
  return new File([entry.payload.sourceBlob], entry.task.fileName, {
    type: entry.task.fileType,
    lastModified: entry.task.lastModified,
  });
}

export async function resetUploadQueueDatabaseForTests(): Promise<void> {
  const existing = databasePromise;
  databasePromise = undefined;
  if (existing) (await existing).close();
  await deleteDB(DATABASE_NAME);
}
