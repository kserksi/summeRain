// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import "fake-indexeddb/auto";

import { Blob as NodeBlob } from "node:buffer";
import { afterEach, describe, expect, it } from "vitest";

import type { ProcessedImage } from "./client-processing/types";
import {
  acquireUploadLease,
  cleanupExpiredUploads,
  deletePersistedUpload,
  deletePersistedProcessedUpload,
  listPersistedUploads,
  persistProcessedUpload,
  persistQueuedUpload,
  preparePersistedUploadRun,
  releaseUploadLease,
  renewUploadLease,
  resetUploadQueueDatabaseForTests,
  sourceFileFromEntry,
  UPLOAD_LEASE_MS,
  UPLOAD_QUEUE_RETENTION_MS,
  updatePersistedUpload,
  UploadQueueLeaseLostError,
  type PersistedUploadTask,
} from "./upload-queue-store";

afterEach(async () => {
  await resetUploadQueueDatabaseForTests();
});

describe("persisted upload queue", () => {
  it("round-trips source blobs and isolates identical queue IDs by user", async () => {
    await persistQueuedUpload(task(11, "shared", 1_000), testBlob("first-user", "image/jpeg"));
    await persistQueuedUpload(task(22, "shared", 1_000), testBlob("second-user", "image/png"));

    const first = await listPersistedUploads(11, 1_001);
    const second = await listPersistedUploads(22, 1_001);

    expect(first).toHaveLength(1);
    expect(second).toHaveLength(1);
    expect(await blobText(first[0].payload!.sourceBlob)).toBe("first-user");
    expect(await blobText(second[0].payload!.sourceBlob)).toBe("second-user");
    expect(sourceFileFromEntry(first[0])).toMatchObject({
      name: "photo-11.jpg",
      type: "image/jpeg",
      lastModified: 123,
    });
  });

  it("removes tasks and payloads after the 24-hour retention window", async () => {
    const now = 5_000;
    await persistQueuedUpload(task(11, "expired", now - UPLOAD_QUEUE_RETENTION_MS), blob());
    await persistQueuedUpload(task(11, "fresh", now), blob());

    await expect(cleanupExpiredUploads(now)).resolves.toBe(1);
    await expect(listPersistedUploads(11, now)).resolves.toMatchObject([
      { task: { queueId: "fresh" } },
    ]);
  });

  it("atomically retains processed parts with their recipe and can discard only derivatives", async () => {
    await persistQueuedUpload(task(11, "processed", 1_000), testBlob("source"));
    const processed = processedImage();

    await persistProcessedUpload(11, "processed", processed);

    let [entry] = await listPersistedUploads(11, 1_001);
    expect(entry.task).toMatchObject({ status: "uploading", recipeVersion: "2.0.0" });
    expect(entry.payload?.processed).toMatchObject({
      processor_version: "test-processor",
      recipe_version: "2.0.0",
      parts: [{ kind: "master", size: 6 }],
    });
    expect(await blobText(entry.payload!.processed!.parts[0].blob)).toBe("master");

    await deletePersistedProcessedUpload(11, "processed");

    [entry] = await listPersistedUploads(11, 1_001);
    expect(await blobText(entry.payload!.sourceBlob)).toBe("source");
    expect(entry.payload?.processed).toBeUndefined();
  });

  it("allows only one live tab lease and permits takeover after expiry", async () => {
    const now = 10_000;
    await persistQueuedUpload(task(11, "leased", now), blob());

    await expect(acquireUploadLease(11, "leased", "tab-a", now)).resolves.toBe(true);
    await expect(acquireUploadLease(11, "leased", "tab-b", now + 1)).resolves.toBe(false);
    await expect(renewUploadLease(11, "leased", "tab-b", now + 2)).resolves.toBe(false);
    await expect(renewUploadLease(11, "leased", "tab-a", now + 3)).resolves.toBe(true);

    await expect(
      acquireUploadLease(11, "leased", "tab-b", now + 3 + UPLOAD_LEASE_MS),
    ).resolves.toBe(true);
    await releaseUploadLease(11, "leased", "tab-a");
    await expect(acquireUploadLease(11, "leased", "tab-a", now + 4)).resolves.toBe(false);

    await releaseUploadLease(11, "leased", "tab-b");
    await expect(acquireUploadLease(11, "leased", "tab-a", now + 5)).resolves.toBe(true);
  });

  it("fences stale writers after another tab takes over an expired lease", async () => {
    const now = 20_000;
    await persistQueuedUpload(task(11, "fenced", now), blob());
    await acquireUploadLease(11, "fenced", "tab-a", now);
    await acquireUploadLease(11, "fenced", "tab-b", now + UPLOAD_LEASE_MS);

    await expect(
      preparePersistedUploadRun(11, "fenced", "tab-a", true),
    ).resolves.toEqual({ outcome: "lease-lost" });
    await expect(
      updatePersistedUpload(11, "fenced", { status: "processing" }, "tab-a"),
    ).resolves.toBe(false);
    await expect(
      persistProcessedUpload(11, "fenced", processedImage(), "tab-a"),
    ).rejects.toBeInstanceOf(UploadQueueLeaseLostError);
    await expect(deletePersistedUpload(11, "fenced", "tab-a")).resolves.toBe(false);

    const [entry] = await listPersistedUploads(11, now + UPLOAD_LEASE_MS + 1);
    expect(entry.task).toMatchObject({
      status: "queued",
      leaseOwner: "tab-b",
    });
  });

  it("distinguishes a missing task from a lease handoff", async () => {
    await expect(
      preparePersistedUploadRun(11, "not-persisted", "retry-token", true),
    ).resolves.toEqual({ outcome: "missing" });
  });

  it("derives a new manual attempt from the authoritative failed task", async () => {
    const now = Date.now();
    await persistQueuedUpload(
      {
        ...task(11, "manual-new", now),
        attempt: 1,
        status: "failed",
        started: true,
        retryMode: "new",
        uploadId: "authoritative-upload",
        failureRetryable: true,
      },
      blob(),
    );
    await acquireUploadLease(11, "manual-new", "retry-token", now);

    const prepared = await preparePersistedUploadRun(
      11,
      "manual-new",
      "retry-token",
      true,
    );

    expect(prepared).toMatchObject({
      outcome: "ready",
      manualRetryApplied: true,
      task: {
        attempt: 2,
        status: "queued",
        uploadId: undefined,
        serverExpiresAt: undefined,
        retryMode: undefined,
      },
    });
  });

  it("uses the authoritative upload ID for a manual status resume", async () => {
    const now = Date.now();
    await persistQueuedUpload(
      {
        ...task(11, "manual-resume", now),
        attempt: 1,
        status: "failed",
        started: true,
        retryMode: "resume",
        uploadId: "authoritative-upload",
        serverExpiresAt: new Date(now + 60_000).toISOString(),
        failureRetryable: true,
      },
      blob(),
    );
    await acquireUploadLease(11, "manual-resume", "retry-token", now);

    const prepared = await preparePersistedUploadRun(
      11,
      "manual-resume",
      "retry-token",
      true,
    );

    expect(prepared).toMatchObject({
      outcome: "ready",
      manualRetryApplied: true,
      task: {
        attempt: 1,
        status: "serverProcessing",
        uploadId: "authoritative-upload",
        retryMode: "resume",
      },
    });
  });

  it("blocks a stale manual retry of an authoritative V1 unknown outcome", async () => {
    const now = Date.now();
    await persistQueuedUpload(
      {
        ...task(11, "manual-blocked", now),
        status: "failed",
        started: true,
        pipelineMode: "v1",
        failureCode: "UPLOAD_V1_OUTCOME_UNKNOWN",
        failureRetryable: false,
      },
      blob(),
    );
    await acquireUploadLease(11, "manual-blocked", "retry-token", now);

    const prepared = await preparePersistedUploadRun(
      11,
      "manual-blocked",
      "retry-token",
      true,
    );

    expect(prepared).toMatchObject({
      outcome: "blocked",
      task: {
        status: "failed",
        pipelineMode: "v1",
        failureRetryable: false,
      },
    });
  });

  it("ignores stale manual intent when the authoritative task is processing", async () => {
    const now = Date.now();
    await persistQueuedUpload(
      {
        ...task(11, "manual-processing", now),
        attempt: 1,
        status: "serverProcessing",
        started: true,
        retryMode: "resume",
        uploadId: "authoritative-upload",
      },
      blob(),
    );
    await acquireUploadLease(11, "manual-processing", "retry-token", now);

    const prepared = await preparePersistedUploadRun(
      11,
      "manual-processing",
      "retry-token",
      true,
    );

    expect(prepared).toMatchObject({
      outcome: "ready",
      manualRetryApplied: false,
      task: {
        attempt: 1,
        status: "serverProcessing",
        uploadId: "authoritative-upload",
      },
    });
  });
});

function task(ownerUserId: number, queueId: string, createdAt: number): PersistedUploadTask {
  return {
    ownerUserId,
    queueId,
    attempt: 0,
    fileName: `photo-${ownerUserId}.jpg`,
    fileType: "image/jpeg",
    fileSize: 6,
    lastModified: 123,
    sourceFingerprint: `${ownerUserId}-${queueId}`,
    status: "queued",
    started: false,
    createdAt,
    updatedAt: createdAt,
    expiresAt: createdAt + UPLOAD_QUEUE_RETENTION_MS,
  };
}

function blob(): Blob {
  return testBlob("source", "image/jpeg");
}

function blobText(value: Blob): Promise<string> {
  return (value as unknown as NodeBlob).text();
}

function testBlob(contents: string, type = "application/octet-stream"): Blob {
  return new NodeBlob([contents], { type }) as unknown as Blob;
}

function processedImage(): ProcessedImage {
  const master = testBlob("master", "image/webp");
  return {
    source: { mime_type: "image/jpeg", width: 1, height: 1, animated: false },
    processor_version: "test-processor",
    recipe_version: "2.0.0",
    parts: [
      {
        kind: "master",
        blob: master,
        size: master.size,
        sha256: "a".repeat(64),
        mime_type: "image/webp",
        width: 1,
        height: 1,
        quality: 80,
      },
    ],
  };
}
