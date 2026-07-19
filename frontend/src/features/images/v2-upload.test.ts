// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/errors";

import type { ProcessedImage } from "./client-processing/types";
import {
  buildManifest,
  beginV2Upload,
  createV2IdempotencyKey,
  invalidateV2RecipeCache,
  isV2UploadEnabled,
  nextV2UploadAttempt,
  parseV2Recipe,
  V2UploadPendingError,
  v2StatusPollDelay,
  v2UploadRetryDisposition,
  waitForV2Upload,
} from "./v2-upload";

describe("v2StatusPollDelay", () => {
  it("backs off long-running publish queues without changing the fast path", () => {
    expect(v2StatusPollDelay(0)).toBe(2_000);
    expect(v2StatusPollDelay(29_999)).toBe(2_000);
    expect(v2StatusPollDelay(30_000)).toBe(5_000);
    expect(v2StatusPollDelay(120_000)).toBe(10_000);
  });
});

afterEach(() => {
  invalidateV2RecipeCache();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("buildManifest", () => {
  it("keeps fixed metadata and excludes in-memory blobs", () => {
    const processed: ProcessedImage = {
      source: { mime_type: "image/jpeg", width: 10, height: 20, animated: false },
      processor_version: "test",
      recipe_version: "2.0.0",
      parts: [
        {
          kind: "master",
          blob: new Blob(["webp"]),
          size: 4,
          sha256: "a".repeat(64),
          mime_type: "image/webp",
          width: 10,
          height: 20,
          quality: 80,
        },
      ],
    };

    const manifest = buildManifest("photo.jpg", "private", processed);

    expect(manifest).toMatchObject({
      filename: "photo.jpg",
      visibility: "private",
      processor_version: "test",
      recipe_version: "2.0.0",
      parts: [
        {
          kind: "master",
          size: 4,
          sha256: "a".repeat(64),
          mime_type: "image/webp",
          width: 10,
          height: 20,
          quality: 80,
        },
      ],
    });
    expect(manifest.parts[0]).not.toHaveProperty("blob");
  });
});

describe("upload attempts", () => {
  it("rotates the idempotency key for a manual retry", () => {
    const first = createV2IdempotencyKey("queue-1", 0);
    const retriedAttempt = nextV2UploadAttempt(0);
    const retried = createV2IdempotencyKey("queue-1", retriedAttempt);

    expect(retriedAttempt).toBe(1);
    expect(retried).not.toBe(first);
    expect(retried).toBe("v2-queue-1-attempt-1");
  });

  it("rotates the idempotency key after a server manifest conflict", () => {
    expect(v2UploadRetryDisposition(new ApiError(4091, "manifest conflict"))).toBe(
      "new-attempt",
    );
  });
});

describe("upload pipeline capability", () => {
  const recipe = {
    pipeline_version: 2,
    recipe_version: "2.0.0",
    max_part_bytes: 64 * 1024 * 1024,
    max_pixels: 50_000_000,
    session_ttl_ms: 30 * 60 * 1000,
    variants: recipeVariants(),
  };

  it("uses V1 when the server explicitly disables new V2 sessions", () => {
    expect(isV2UploadEnabled({ ...recipe, v2_enabled: false })).toBe(false);
  });

  it("keeps compatibility with recipes from older V2-only servers", () => {
    expect(isV2UploadEnabled(recipe)).toBe(true);
  });

  it("validates the complete fixed recipe contract", () => {
    expect(parseV2Recipe(recipe)).toEqual(recipe);
  });

  it("rejects missing or changed fixed variants with a stable client error", () => {
    expect(() => parseV2Recipe({ ...recipe, variants: undefined })).toThrowError(
      expect.objectContaining({ code: "IMAGE_RECIPE_UNSUPPORTED" }),
    );
    expect(() =>
      parseV2Recipe({
        ...recipe,
        variants: recipeVariants().map((variant) =>
          variant.kind === "gallery" ? { ...variant, width: 401 } : variant,
        ),
      }),
    ).toThrowError(expect.objectContaining({ code: "IMAGE_RECIPE_UNSUPPORTED" }));
  });

  it("rejects unsafe numeric recipe limits", () => {
    expect(() => parseV2Recipe({ ...recipe, max_pixels: Number.MAX_SAFE_INTEGER + 1 })).toThrowError(
      expect.objectContaining({ code: "IMAGE_RECIPE_UNSUPPORTED" }),
    );
  });
});

describe("beginV2Upload cancellation", () => {
  it("stops while a shared recipe request is still pending", async () => {
    let resolveFetch: (value: Response) => void = () => {};
    const pendingFetch = new Promise<Response>((resolve) => {
      resolveFetch = resolve;
    });
    const fetchMock = vi.fn(() => pendingFetch);
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    const result = beginV2Upload({
      fileName: "photo.jpg",
      visibility: "public",
      idempotencyKey: "attempt-1",
      processed: processedImage(),
      onProgress: vi.fn(),
      signal: controller.signal,
    }).catch((error: unknown) => error);
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());

    controller.abort(new DOMException("page left", "AbortError"));

    await expect(result).resolves.toMatchObject({ name: "AbortError", message: "page left" });
    resolveFetch(
      response(200, {
        code: 0,
        message: "success",
        data: {
          pipeline_version: 2,
          recipe_version: "2.0.0",
          max_part_bytes: 15 * 1024 * 1024,
          max_pixels: 50_000_000,
          session_ttl_ms: 60_000,
          variants: recipeVariants(),
        },
      }),
    );
    await Promise.resolve();
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("cancels the server session and detaches XHR abort handling when send throws", async () => {
    const xhrAbort = vi.fn();
    vi.stubGlobal(
      "XMLHttpRequest",
      class {
        upload: { onprogress: ((event: ProgressEvent) => void) | null } = { onprogress: null };
        onload: (() => void) | null = null;
        onerror: (() => void) | null = null;
        ontimeout: (() => void) | null = null;
        onabort: (() => void) | null = null;
        responseText = "";
        status = 0;
        timeout = 0;
        withCredentials = false;

        open() {}
        setRequestHeader() {}
        abort = xhrAbort;
        send() {
          throw new DOMException("could not send", "DataCloneError");
        }
      },
    );
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/uploads/recipe")) return response(200, recipeEnvelope());
      if (init?.method === "DELETE") {
        return response(200, {
          code: 0,
          message: "success",
          data: activeSession("send-failure-upload", "cancelled", []),
        });
      }
      return response(200, {
        code: 0,
        message: "success",
        data: activeSession("send-failure-upload", "initiated", [
          {
            kind: "master",
            status: "pending",
            put_url: "/api/v1/uploads/send-failure-upload/parts/master",
            size: 4,
            sha256: "a".repeat(64),
            width: 1,
            height: 1,
          },
        ]),
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();

    await expect(
      beginV2Upload({
        fileName: "photo.jpg",
        visibility: "public",
        idempotencyKey: "send-failure-attempt",
        processed: processedImageWithMaster(),
        onProgress: vi.fn(),
        signal: controller.signal,
      }),
    ).rejects.toThrow("could not send");

    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(true);
    controller.abort();
    expect(xhrAbort).not.toHaveBeenCalled();
  });

  it("retries complete on the same upload when recovery still sees an uploadable session", async () => {
    let completeAttempts = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/uploads/recipe")) return response(200, recipeEnvelope());
      if (url.endsWith("/complete")) {
        completeAttempts += 1;
        if (completeAttempts <= 3) throw new TypeError("connection reset");
        return response(200, {
          code: 0,
          message: "success",
          data: activeSession("recovery-upload", "processing", []),
        });
      }
      if (init?.method === "GET") {
        return response(200, {
          code: 0,
          message: "success",
          data: activeSession("recovery-upload", "uploading", []),
        });
      }
      return response(200, {
        code: 0,
        message: "success",
        data: activeSession("recovery-upload", "uploading", []),
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const onProgress = vi.fn();

    await expect(
      beginV2Upload({
        fileName: "photo.jpg",
        visibility: "public",
        idempotencyKey: "recovery-attempt",
        processed: processedImage(),
        onProgress,
      }),
    ).resolves.toEqual({ uploadId: "recovery-upload" });

    expect(onProgress).toHaveBeenCalledWith("server-processing", 0);
    expect(completeAttempts).toBe(4);
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
  });

  it("retries init with the same idempotency key after a transient database error", async () => {
    let initAttempts = 0;
    const seenKeys: string[] = [];
    const onSession = vi.fn();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/uploads/recipe")) return response(200, recipeEnvelope());
        if (url.endsWith("/uploads/")) {
          initAttempts += 1;
          seenKeys.push((init?.headers as Record<string, string>)["Idempotency-Key"]);
          if (initAttempts === 1) {
            return response(500, { code: 1001, message: "database temporarily unavailable" });
          }
          return response(200, {
            code: 0,
            message: "success",
            data: activeSession("init-retry-upload", "uploading", []),
          });
        }
        return response(200, {
          code: 0,
          message: "success",
          data: activeSession("init-retry-upload", "processing", []),
        });
      }),
    );

    await expect(
      beginV2Upload({
        fileName: "photo.jpg",
        visibility: "public",
        idempotencyKey: "stable-init-key",
        processed: processedImage(),
        onProgress: vi.fn(),
        onSession,
      }),
    ).resolves.toEqual({ uploadId: "init-retry-upload" });

    expect(initAttempts).toBe(2);
    expect(seenKeys).toEqual(["stable-init-key", "stable-init-key"]);
    expect(onSession).toHaveBeenCalledWith("init-retry-upload");
  });
});

describe("V2 status coordination", () => {
  it("bisects a 4043 response and only rejects the missing upload waiter", async () => {
    vi.useFakeTimers();
    const missing = "missing-upload";
    const requestedBatches: string[][] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        const { upload_ids: uploadIds } = JSON.parse(String(init?.body)) as {
          upload_ids: string[];
        };
        requestedBatches.push(uploadIds);
        if (uploadIds.includes(missing)) {
          return response(404, { code: 4043, message: "Upload session is missing" });
        }
        return response(200, {
          code: 0,
          message: "success",
          data: { uploads: uploadIds.map(completedSession) },
        });
      }),
    );

    const first = waitForV2Upload("first-upload", vi.fn());
    const bad = waitForV2Upload(missing, vi.fn()).catch((error: unknown) => error);
    const second = waitForV2Upload("second-upload", vi.fn());

    await vi.advanceTimersByTimeAsync(250);

    await expect(first).resolves.toMatchObject({ uploadId: "first-upload" });
    await expect(second).resolves.toMatchObject({ uploadId: "second-upload" });
    await expect(bad).resolves.toMatchObject({ code: 4043 });
    expect(requestedBatches).toEqual([
      ["first-upload", missing, "second-upload"],
      ["first-upload"],
      [missing, "second-upload"],
      [missing],
      ["second-upload"],
    ]);
  });

  it("treats cleanup_pending as a terminal status", async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        response(200, {
          code: 0,
          message: "success",
          data: {
            uploads: [
              {
                upload_id: "cleanup-upload",
                status: "cleanup_pending",
                expires_at: new Date(Date.now() + 60_000).toISOString(),
                parts: [],
              },
            ],
          },
        }),
      ),
    );

    const result = waitForV2Upload("cleanup-upload", vi.fn()).catch(
      (error: unknown) => error,
    );
    await vi.advanceTimersByTimeAsync(250);

    await expect(result).resolves.toMatchObject({
      message: "Upload cleanup is pending; retry with a new upload attempt",
    });
  });

  it("treats cleanup_pending with a published image as completed", async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        response(200, {
          code: 0,
          message: "success",
          data: {
            uploads: [
              {
                upload_id: "published-cleanup-upload",
                status: "cleanup_pending",
                image_id: 42,
                unique_link: "published-link",
                asset_link: "/assets/published.webp",
                expires_at: new Date(Date.now() + 60_000).toISOString(),
                parts: [],
              },
            ],
          },
        }),
      ),
    );

    const result = waitForV2Upload("published-cleanup-upload", vi.fn());
    await vi.advanceTimersByTimeAsync(250);

    await expect(result).resolves.toEqual({
      uploadId: "published-cleanup-upload",
      uniqueLink: "published-link",
      assetLink: "/assets/published.webp",
      pipelineVersion: 2,
    });
  });

  it("removes an aborted waiter before polling", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    const result = waitForV2Upload("aborted-upload", vi.fn(), controller.signal).catch(
      (error: unknown) => error,
    );

    controller.abort(new DOMException("page left", "AbortError"));
    await vi.advanceTimersByTimeAsync(250);

    await expect(result).resolves.toMatchObject({ name: "AbortError", message: "page left" });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("keeps a later status chunk healthy when an earlier chunk has a transient failure", async () => {
    vi.useFakeTimers();
    const requestedBatches: string[][] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        const { upload_ids: uploadIds } = JSON.parse(String(init?.body)) as {
          upload_ids: string[];
        };
        requestedBatches.push(uploadIds);
        if (uploadIds.length === 100) throw new TypeError("network unavailable");
        return response(200, {
          code: 0,
          message: "success",
          data: { uploads: uploadIds.map(completedSession) },
        });
      }),
    );
    const controllers = Array.from({ length: 100 }, () => new AbortController());
    const earlier = controllers.map((controller, index) =>
      waitForV2Upload(`earlier-${index}`, vi.fn(), controller.signal).catch(
        (error: unknown) => error,
      ),
    );
    const later = waitForV2Upload("later-upload", vi.fn());

    await vi.advanceTimersByTimeAsync(250);

    await expect(later).resolves.toMatchObject({ uploadId: "later-upload" });
    expect(requestedBatches.map((batch) => batch.length)).toEqual([100, 1]);
    for (const controller of controllers) controller.abort();
    await Promise.all(earlier);
  });

  it("retries a transient backend system error while polling", async () => {
    vi.useFakeTimers();
    let attempts = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        attempts += 1;
        if (attempts === 1) {
          return response(500, { code: 1001, message: "database temporarily unavailable" });
        }
        return response(200, {
          code: 0,
          message: "success",
          data: { uploads: [completedSession("system-retry-upload")] },
        });
      }),
    );

    const result = waitForV2Upload("system-retry-upload", vi.fn());
    await vi.advanceTimersByTimeAsync(2_250);

    await expect(result).resolves.toMatchObject({ uploadId: "system-retry-upload" });
    expect(attempts).toBe(2);
  });

  it("keeps polling after more than five consecutive transient failures", async () => {
    vi.useFakeTimers();
    let attempts = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        attempts += 1;
        if (attempts <= 6) {
          return response(500, { code: 1001, message: "database temporarily unavailable" });
        }
        return response(200, {
          code: 0,
          message: "success",
          data: { uploads: [completedSession("long-recovery-upload")] },
        });
      }),
    );
    const settled = vi.fn();
    const result = waitForV2Upload("long-recovery-upload", vi.fn());
    void result.then(settled, settled);

    await vi.advanceTimersByTimeAsync(10_250);
    expect(attempts).toBe(6);
    expect(settled).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(2_000);
    await expect(result).resolves.toMatchObject({ uploadId: "long-recovery-upload" });
    expect(attempts).toBe(7);
  });

  it("keeps polling when a successful batch temporarily omits the upload", async () => {
    vi.useFakeTimers();
    let attempts = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        attempts += 1;
        return response(200, {
          code: 0,
          message: "success",
          data: {
            uploads: attempts === 1 ? [] : [completedSession("temporarily-missing-upload")],
          },
        });
      }),
    );

    const result = waitForV2Upload("temporarily-missing-upload", vi.fn());
    await vi.advanceTimersByTimeAsync(2_250);

    await expect(result).resolves.toMatchObject({ uploadId: "temporarily-missing-upload" });
    expect(attempts).toBe(2);
  });

  it("classifies pending status recovery as the same upload attempt", () => {
    expect(v2UploadRetryDisposition(new V2UploadPendingError("pending-upload"))).toBe(
      "reuse-attempt",
    );
  });
});

function completedSession(uploadId: string) {
  return {
    upload_id: uploadId,
    status: "completed",
    unique_link: `${uploadId}-link`,
    asset_link: `/assets/${uploadId}.webp`,
    expires_at: new Date(Date.now() + 60_000).toISOString(),
    parts: [],
  };
}

function response(status: number, body: unknown): Response {
  return {
    status,
    json: async () => body,
  } as Response;
}

function processedImage(): ProcessedImage {
  return {
    source: { mime_type: "image/jpeg", width: 1, height: 1, animated: false },
    processor_version: "test",
    recipe_version: "2.0.0",
    parts: [],
  };
}

function processedImageWithMaster(): ProcessedImage {
  return {
    ...processedImage(),
    parts: [
      {
        kind: "master",
        blob: new Blob(["webp"]),
        size: 4,
        sha256: "a".repeat(64),
        mime_type: "image/webp",
        width: 1,
        height: 1,
        quality: 80,
      },
    ],
  };
}

function activeSession(uploadId: string, status: string, parts: unknown[]) {
  return {
    upload_id: uploadId,
    status,
    expires_at: new Date(Date.now() + 60_000).toISOString(),
    parts,
  };
}

function recipeEnvelope() {
  return {
    code: 0,
    message: "success",
    data: {
      pipeline_version: 2,
      recipe_version: "2.0.0",
      max_part_bytes: 15 * 1024 * 1024,
      max_pixels: 50_000_000,
      session_ttl_ms: 60_000,
      variants: recipeVariants(),
    },
  };
}

function recipeVariants() {
  return [
    { kind: "master" as const, quality: 80 as const, fit: "original" as const },
    {
      kind: "gallery" as const,
      width: 400,
      height: 400,
      quality: 60 as const,
      fit: "cover" as const,
    },
    {
      kind: "admin" as const,
      width: 120,
      height: 160,
      quality: 60 as const,
      fit: "cover" as const,
    },
    {
      kind: "publish_source" as const,
      long_edge: 2048,
      quality: 80 as const,
      fit: "contain" as const,
    },
  ];
}
