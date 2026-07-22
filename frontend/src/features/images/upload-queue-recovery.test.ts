// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import "fake-indexeddb/auto";

import { Blob as NodeBlob } from "node:buffer";
import { afterEach, describe, expect, it } from "vitest";

import {
  planUploadRecovery,
  recoverPersistedUploadQueue,
  V1_UPLOAD_OUTCOME_UNKNOWN,
} from "./upload-queue-recovery";
import {
  acquireUploadLease,
  listPersistedUploads,
  persistQueuedUpload,
  resetUploadQueueDatabaseForTests,
  UPLOAD_QUEUE_RETENTION_MS,
  type PersistedUploadTask,
} from "./upload-queue-store";
import type { UploadSessionResponse } from "./v2-upload";

afterEach(async () => {
  await resetUploadQueueDatabaseForTests();
});

describe("planUploadRecovery", () => {
  it("finishes a published session without replaying the upload", () => {
    expect(planUploadRecovery(session("completed"), { missing: false })).toEqual({
      action: "completed",
      result: {
        uploadId: "upload-1",
        uniqueLink: "image-link",
        assetLink: "/assets/image.webp",
        pipelineVersion: 2,
      },
    });
  });

  it("polls a session already owned by server processing", () => {
    expect(planUploadRecovery(session("processing"), { missing: false })).toEqual({
      action: "poll",
    });
  });

  it("resumes a non-expired initiated or uploading session", () => {
    const now = Date.parse("2026-07-19T00:00:00.000Z");

    expect(planUploadRecovery(session("initiated", now + 1), { missing: false, now })).toEqual({
      action: "resume",
    });
    expect(planUploadRecovery(session("uploading", now + 1), { missing: false, now })).toEqual({
      action: "resume",
    });
  });

  it("restarts a missing or expired server session", () => {
    const now = Date.parse("2026-07-19T00:00:00.000Z");

    expect(planUploadRecovery(undefined, { missing: true, now })).toEqual({
      action: "restart",
    });
    expect(planUploadRecovery(session("uploading", now), { missing: false, now })).toEqual({
      action: "restart",
    });
  });

  it("surfaces terminal and malformed completion states as failures", () => {
    expect(planUploadRecovery(session("failed"), { missing: false })).toEqual({
      action: "failed",
      message: "Server image processing failed",
    });
    expect(
      planUploadRecovery({ ...session("completed"), unique_link: undefined }, { missing: false }),
    ).toEqual({
      action: "failed",
      message: "Completed upload is missing its image link",
    });
  });

  it("pauses when status reconciliation is unavailable", () => {
    expect(planUploadRecovery(undefined, { missing: false })).toEqual({ action: "paused" });
  });
});

describe("recoverPersistedUploadQueue", () => {
  it("marks an interrupted V1 request as an unknown, non-retryable outcome", async () => {
    const task = persistedTask("legacy-upload", {
      status: "uploading",
      started: true,
      pipelineMode: "v1",
    });
    await persistQueuedUpload(task, sourceBlob());

    const [recovered] = await recoverPersistedUploadQueue(task.ownerUserId);

    expect(recovered).toMatchObject({
      status: "failed",
      failureCode: V1_UPLOAD_OUTCOME_UNKNOWN,
      failureRetryable: false,
      autoResume: false,
    });
    const [stored] = await listPersistedUploads(task.ownerUserId);
    expect(stored.task).toMatchObject({
      status: "failed",
      failureCode: V1_UPLOAD_OUTCOME_UNKNOWN,
      failureRetryable: false,
    });
  });

  it("does not mutate a queue entry leased by another live tab", async () => {
    const now = Date.now();
    const task = persistedTask("other-tab", {
      status: "processing",
      started: true,
      pipelineMode: "v2",
    });
    await persistQueuedUpload(task, sourceBlob());
    await acquireUploadLease(task.ownerUserId, task.queueId, "other-tab-token", now);

    const [recovered] = await recoverPersistedUploadQueue(task.ownerUserId);

    expect(recovered).toMatchObject({ status: "paused", autoResume: false });
    const [stored] = await listPersistedUploads(task.ownerUserId);
    expect(stored.task).toMatchObject({
      status: "processing",
      leaseOwner: "other-tab-token",
    });
  });
});

function persistedTask(
  queueId: string,
  patch: Partial<PersistedUploadTask>,
): PersistedUploadTask {
  const now = Date.now();
  return {
    ownerUserId: 7,
    queueId,
    attempt: 0,
    fileName: `${queueId}.jpg`,
    fileType: "image/jpeg",
    fileSize: 6,
    lastModified: now,
    sourceFingerprint: "fingerprint",
    status: "queued",
    started: false,
    createdAt: now,
    updatedAt: now,
    expiresAt: now + UPLOAD_QUEUE_RETENTION_MS,
    ...patch,
  };
}

function sourceBlob(): Blob {
  return new NodeBlob(["source"], { type: "image/jpeg" }) as unknown as Blob;
}

function session(
  status: UploadSessionResponse["status"],
  expiresAt = Date.now() + 60_000,
): UploadSessionResponse {
  return {
    upload_id: "upload-1",
    status,
    unique_link: "image-link",
    asset_link: "/assets/image.webp",
    expires_at: new Date(expiresAt).toISOString(),
    parts: [],
  };
}
