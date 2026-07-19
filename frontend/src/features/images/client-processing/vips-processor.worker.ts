// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

/// <reference lib="webworker" />

import Vips from "wasm-vips";
import vipsHeifURL from "wasm-vips/vips-heif.wasm?url";
import vipsWasmURL from "wasm-vips/vips.wasm?url";

import type { ProcessedPart, WorkerRequest, WorkerResponse } from "./types";

const assetURLs: Record<string, string> = {
  "vips.wasm": vipsWasmURL,
  "vips-heif.wasm": vipsHeifURL,
};

let vipsPromise: ReturnType<typeof Vips> | undefined;
type VipsInstance = Awaited<ReturnType<typeof Vips>>;
type VipsImage = InstanceType<VipsInstance["Image"]>;

self.onmessage = async (event: MessageEvent<WorkerRequest>) => {
  const request = event.data;
  const { id } = request;
  let initialized = false;
  try {
    const vips = await getVips();
    initialized = true;
    if (request.type === "probe") {
      probeWebPEncoder(vips);
      post({ type: "ready", id });
      return;
    }

    const { file, mimeType } = request;
    progress(id, 5);
    const source = vips.Image.newFromBuffer(new Uint8Array(await file.arrayBuffer()), "", {
      fail_on: "error",
    });
    const image = source.autorot();
    source.delete();
    try {
      if (image.width * image.height > 50_000_000) throw new Error("Image exceeds the 50 MP limit");
      const parts: ProcessedPart[] = [];
      parts.push(await encodePart("master", image, 80));
      progress(id, 35);

      const gallery = image.thumbnailImage(400, { height: 400, crop: "centre", size: "both" });
      try {
        parts.push(await encodePart("gallery", gallery, 60));
      } finally {
        gallery.delete();
      }
      progress(id, 55);

      const admin = image.thumbnailImage(120, { height: 160, crop: "centre", size: "both" });
      try {
        parts.push(await encodePart("admin", admin, 60));
      } finally {
        admin.delete();
      }
      progress(id, 70);

      const publish = image.thumbnailImage(2048, { height: 2048, size: "down" });
      try {
        parts.push(await encodePart("publish_source", publish, 80));
      } finally {
        publish.delete();
      }
      progress(id, 85);

      post({
        type: "result",
        id,
        result: {
          source: {
            mime_type: mimeType,
            width: image.width,
            height: image.height,
            animated: false,
          },
          recipe_version: "2.0.0",
          parts,
        },
      });
    } finally {
      image.delete();
    }
  } catch (error) {
    post({
      type: "error",
      id,
      message: error instanceof Error ? error.message : "WASM image processing failed",
      recoverable: !initialized,
    });
  }
};

function probeWebPEncoder(vips: VipsInstance): void {
  const image = vips.Image.newFromMemory(
    new Uint8Array([0, 0, 0]),
    1,
    1,
    3,
    vips.BandFormat.uchar,
  );
  try {
    const encoded = Uint8Array.from(image.webpsaveBuffer({ Q: 80 }));
    if (
      encoded.length < 12 ||
      String.fromCharCode(...encoded.slice(0, 4)) !== "RIFF" ||
      String.fromCharCode(...encoded.slice(8, 12)) !== "WEBP"
    ) {
      throw new Error("WASM WebP encoder is unavailable");
    }
  } finally {
    image.delete();
  }
}

async function getVips(): Promise<VipsInstance> {
  if (!vipsPromise) {
    vipsPromise = Vips({
      dynamicLibraries: ["vips-heif.wasm"],
      locateFile: (name) => assetURLs[name] || name,
      printErr: () => undefined,
    }).then((vips) => {
      vips.concurrency(1);
      // Each request owns a short-lived worker. Disable operation retention and
      // bound tracked cache memory on top of the live 50 MP pipeline and blobs.
      vips.Cache.max(0);
      vips.Cache.maxMem(64 * 1024 * 1024);
      return vips;
    });
  }
  return vipsPromise;
}

async function encodePart(
  kind: ProcessedPart["kind"],
  image: VipsImage,
  quality: 60 | 80,
): Promise<ProcessedPart> {
  const bytes = image.webpsaveBuffer({ Q: quality, effort: 4, smart_subsample: true });
  const encoded = Uint8Array.from(bytes);
  const hash = await sha256(encoded);
  const blob = new Blob([encoded.buffer], { type: "image/webp" });
  return {
    kind,
    blob,
    size: blob.size,
    sha256: hash,
    mime_type: "image/webp",
    width: image.width,
    height: image.height,
    quality,
  };
}

async function sha256(bytes: Uint8Array<ArrayBuffer>): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, "0")).join(
    "",
  );
}

function progress(id: string, percent: number): void {
  post({ type: "progress", id, percent });
}

function post(message: WorkerResponse): void {
  self.postMessage(message);
}
