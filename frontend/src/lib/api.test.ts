// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { api } from "./api";
import { ApiError } from "./errors";

const mockResponse = (status: number, body: unknown) =>
  ({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  }) as Response;

describe("api wrapper", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    vi.spyOn(window, "location", "get").mockReturnValue({
      ...window.location,
      assign: vi.fn(),
    } as Location);
    Object.defineProperty(document, "cookie", {
      configurable: true,
      get: () => "",
      set: () => {},
    });
  });

  it("returns data on code=0", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      mockResponse(200, { code: 0, message: "ok", data: { id: 1 } }),
    );
    const result = await api.get("/test");
    expect(result).toEqual({ id: 1 });
  });

  it("throws ApiError on code!=0", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(mockResponse(200, { code: 2001, message: "凭证错误" }));
    const err = (await api.get("/test").catch((e: unknown) => e)) as ApiError;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.message).toBe("凭证错误");
    expect(err.code).toBe(2001);
  });

  it("does NOT inject CSRF on GET", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      mockResponse(200, { code: 0, message: "ok", data: null }),
    );
    await api.get("/test");
    const opts = vi.mocked(fetch).mock.calls[0][1] as RequestInit;
    expect(opts.headers).not.toHaveProperty("X-CSRF-Token");
  });

  it("sends credentials: include", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      mockResponse(200, { code: 0, message: "ok", data: null }),
    );
    await api.get("/test");
    const opts = vi.mocked(fetch).mock.calls[0][1] as RequestInit;
    expect(opts.credentials).toBe("include");
  });

  it("handles network error", async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new TypeError("Failed to fetch"));
    const err = (await api.get("/test").catch((e: unknown) => e)) as ApiError;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.message).toBe("Network error. Check your connection.");
  });

  it("does NOT redirect on 401 when skipAuthRedirect=true", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(mockResponse(401, { code: 4010, message: "未认证" }));
    await api.get("/test", { skipAuthRedirect: true }).catch(() => {});
    expect(window.location.assign).not.toHaveBeenCalled();
  });

  it("ApiError preserves code", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      mockResponse(200, { code: 4030, message: "账户已被禁用" }),
    );
    try {
      await api.get("/test", { skipAuthRedirect: true });
      expect.fail("should have thrown");
    } catch (err: unknown) {
      expect(err).toBeInstanceOf(ApiError);
      expect((err as ApiError).code).toBe(4030);
    }
  });

  it("does not refresh or replay an ordinary non-idempotent write", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      mockResponse(403, { code: 4036, message: "Invalid CSRF token" }),
    );

    const err = (await api.post("/images", { title: "once" }).catch((e: unknown) => e)) as ApiError;

    expect(err.code).toBe(4036);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("refreshes once and replays an explicitly idempotent request with the new cookie", async () => {
    let cookie = "__Host-csrf_token=old-token";
    Object.defineProperty(document, "cookie", {
      configurable: true,
      get: () => cookie,
      set: () => {},
    });
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      if (String(input).endsWith("/auth/csrf/refresh")) {
        cookie = "__Host-csrf_token=new-token";
        return mockResponse(200, { code: 0, message: "success" });
      }
      const headers = init?.headers as Record<string, string>;
      if (headers["X-CSRF-Token"] === "old-token") {
        return mockResponse(403, { code: 4036, message: "Invalid CSRF token" });
      }
      return mockResponse(200, { code: 0, message: "success", data: { ok: true } });
    });

    const result = await api.post<{ ok: boolean }>(
      "/uploads/",
      { manifest: true },
      { headers: { "Idempotency-Key": "upload-1" }, csrfRetry: "idempotent" },
    );

    expect(result).toEqual({ ok: true });
    expect(fetch).toHaveBeenCalledTimes(3);
    const refreshOptions = vi.mocked(fetch).mock.calls[1][1];
    expect(refreshOptions).toMatchObject({
      method: "POST",
      credentials: "include",
      cache: "no-store",
    });
    expect(refreshOptions?.headers).not.toHaveProperty("X-CSRF-Token");
    const retryHeaders = vi.mocked(fetch).mock.calls[2][1]?.headers as Record<string, string>;
    expect(retryHeaders["X-CSRF-Token"]).toBe("new-token");
  });

  it("coalesces concurrent CSRF refreshes into one request", async () => {
    let cookie = "__Host-csrf_token=old-token";
    Object.defineProperty(document, "cookie", {
      configurable: true,
      get: () => cookie,
      set: () => {},
    });
    let refreshCalls = 0;
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      if (String(input).endsWith("/auth/csrf/refresh")) {
        refreshCalls += 1;
        await Promise.resolve();
        cookie = "__Host-csrf_token=new-token";
        return mockResponse(200, { code: 0, message: "success" });
      }
      const headers = init?.headers as Record<string, string>;
      if (headers["X-CSRF-Token"] === "old-token") {
        return mockResponse(403, { code: 4036, message: "Invalid CSRF token" });
      }
      return mockResponse(200, { code: 0, message: "success", data: null });
    });

    await Promise.all(
      Array.from({ length: 3 }, (_, index) =>
        api.post("/uploads/status", { upload_ids: [`id-${index}`] }, { csrfRetry: "idempotent" }),
      ),
    );

    expect(refreshCalls).toBe(1);
  });
});
