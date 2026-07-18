// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import type { ProcessedImage, ProcessedPart } from "./client-processing/types";
import { createProcessedPreviewURL } from "./processed-preview";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("createProcessedPreviewURL", () => {
  it("creates the queue preview from the bounded gallery output", () => {
    const master = part("master", new Blob(["large-master"]));
    const gallery = part("gallery", new Blob(["small-gallery"]));
    const createObjectURL = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:gallery");

    expect(createProcessedPreviewURL(processed([master, gallery]))).toBe("blob:gallery");
    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(createObjectURL.mock.calls[0][0]).toBe(gallery.blob);
    expect(createObjectURL.mock.calls[0][0]).not.toBe(master.blob);
  });

  it("does not create an unbounded fallback preview when gallery is missing", () => {
    const createObjectURL = vi.spyOn(URL, "createObjectURL");

    expect(createProcessedPreviewURL(processed([part("master", new Blob(["master"]))]))).toBe(
      undefined,
    );
    expect(createObjectURL).not.toHaveBeenCalled();
  });
});

function processed(parts: ProcessedPart[]): ProcessedImage {
  return {
    source: { mime_type: "image/jpeg", width: 8_000, height: 6_250, animated: false },
    processor_version: "test",
    recipe_version: "2.0.0",
    parts,
  };
}

function part(kind: ProcessedPart["kind"], blob: Blob): ProcessedPart {
  const square = kind === "gallery" ? 400 : 8_000;
  return {
    kind,
    blob,
    size: blob.size,
    sha256: "a".repeat(64),
    mime_type: "image/webp",
    width: square,
    height: square,
    quality: kind === "gallery" ? 60 : 80,
  };
}
