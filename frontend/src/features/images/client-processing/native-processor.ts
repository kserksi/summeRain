// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import pica, { type PicaSource } from "pica";

import { assertNativeCanvasCapability } from "./native-capability";
import type { ProcessedImage, ProcessedPart, ProcessingProgress, V2VariantKind } from "./types";

const concurrency = Math.max(1, Math.min(2, Math.floor((navigator.hardwareConcurrency || 2) / 2)));
const resizer = pica({ tile: 512, concurrency });

export async function processWithNative(
  file: File,
  mimeType: string,
  onProgress: ProcessingProgress,
  signal?: AbortSignal,
): Promise<ProcessedImage> {
  throwIfAborted(signal);
  const decoded = await decodeSource(file);
  try {
    const pixels = decoded.width * decoded.height;
    if (pixels <= 0 || pixels > 50_000_000) throw new Error("Image exceeds the 50 MP limit");
    assertNativeCanvasCapability(decoded.width, decoded.height);
    throwIfAborted(signal);

    const parts: ProcessedPart[] = [];
    parts.push(await renderMaster(decoded.source, decoded.width, decoded.height));
    throwIfAborted(signal);
    onProgress(30);
    parts.push(
      await renderCover(
        decoded.source,
        decoded.width,
        decoded.height,
        "gallery",
        400,
        400,
        60,
        signal,
      ),
    );
    throwIfAborted(signal);
    onProgress(50);
    parts.push(
      await renderCover(
        decoded.source,
        decoded.width,
        decoded.height,
        "admin",
        120,
        160,
        60,
        signal,
      ),
    );
    throwIfAborted(signal);
    onProgress(65);
    parts.push(await renderPublishSource(decoded.source, decoded.width, decoded.height, signal));
    throwIfAborted(signal);
    onProgress(80);

    return {
      source: {
        mime_type: mimeType,
        width: decoded.width,
        height: decoded.height,
        animated: false,
      },
      processor_version: "native-pica-10.0.2",
      recipe_version: "2.0.0",
      parts,
    };
  } finally {
    decoded.close();
  }
}

async function renderMaster(
  source: PicaSource,
  width: number,
  height: number,
): Promise<ProcessedPart> {
  const canvas = createCanvas(width, height);
  try {
    const context = canvas.getContext("2d") as
      | CanvasRenderingContext2D
      | OffscreenCanvasRenderingContext2D
      | null;
    if (!context) throw new Error("Canvas 2D is unavailable");
    context.drawImage(source, 0, 0);
    const blob = await resizer.toBlob(canvas, "image/webp", 0.8);
    return makePart("master", blob, width, height, 80);
  } finally {
    releaseCanvas(canvas);
  }
}

async function renderCover(
  source: PicaSource,
  sourceImageWidth: number,
  sourceImageHeight: number,
  kind: "gallery" | "admin",
  width: number,
  height: number,
  quality: 60,
  signal?: AbortSignal,
): Promise<ProcessedPart> {
  const sourceRatio = sourceImageWidth / sourceImageHeight;
  const targetRatio = width / height;
  let sourceWidth = sourceImageWidth;
  let sourceHeight = sourceImageHeight;
  if (sourceRatio > targetRatio)
    sourceWidth = Math.max(1, Math.round(sourceImageHeight * targetRatio));
  else sourceHeight = Math.max(1, Math.round(sourceImageWidth / targetRatio));
  const sourceX = Math.max(0, Math.floor((sourceImageWidth - sourceWidth) / 2));
  const sourceY = Math.max(0, Math.floor((sourceImageHeight - sourceHeight) / 2));
  const canvas = createCanvas(width, height);
  let cropped: ImageBitmap | undefined;
  try {
    if (typeof createImageBitmap === "function") {
      cropped = await createImageBitmap(source, sourceX, sourceY, sourceWidth, sourceHeight);
      throwIfAborted(signal);
      await resizeWithSignal(cropped, canvas, signal);
    } else {
      const context = canvas.getContext("2d") as
        | CanvasRenderingContext2D
        | OffscreenCanvasRenderingContext2D
        | null;
      if (!context) throw new Error("Canvas 2D is unavailable");
      context.drawImage(source, sourceX, sourceY, sourceWidth, sourceHeight, 0, 0, width, height);
    }
    const blob = await resizer.toBlob(canvas, "image/webp", quality / 100);
    return makePart(kind, blob, width, height, quality);
  } finally {
    cropped?.close();
    releaseCanvas(canvas);
  }
}

async function renderPublishSource(
  source: PicaSource,
  sourceWidth: number,
  sourceHeight: number,
  signal?: AbortSignal,
): Promise<ProcessedPart> {
  const scale = Math.min(1, 2048 / Math.max(sourceWidth, sourceHeight));
  const width = Math.max(1, Math.round(sourceWidth * scale));
  const height = Math.max(1, Math.round(sourceHeight * scale));
  const canvas = createCanvas(width, height);
  try {
    await resizeWithSignal(source, canvas, signal);
    const blob = await resizer.toBlob(canvas, "image/webp", 0.8);
    return makePart("publish_source", blob, width, height, 80);
  } finally {
    releaseCanvas(canvas);
  }
}

async function resizeWithSignal(
  source: PicaSource,
  canvas: HTMLCanvasElement | OffscreenCanvas,
  signal?: AbortSignal,
): Promise<void> {
  throwIfAborted(signal);
  if (!signal) {
    await resizer.resize(source, canvas, { filter: "mks2013" });
    return;
  }

  let rejectCancellation: (reason?: unknown) => void = () => {};
  const cancelToken = new Promise<never>((_resolve, reject) => {
    rejectCancellation = reject;
  });
  const abort = () => rejectCancellation(signal.reason ?? abortError());
  signal.addEventListener("abort", abort, { once: true });
  try {
    await resizer.resize(source, canvas, { filter: "mks2013", cancelToken });
  } finally {
    signal.removeEventListener("abort", abort);
  }
}

async function makePart(
  kind: V2VariantKind,
  blob: Blob,
  width: number,
  height: number,
  quality: 60 | 80,
): Promise<ProcessedPart> {
  await assertWebP(blob);
  return {
    kind,
    blob,
    size: blob.size,
    sha256: await sha256(blob),
    mime_type: "image/webp",
    width,
    height,
    quality,
  };
}

async function decodeSource(file: File): Promise<{
  source: ImageBitmap | HTMLImageElement;
  width: number;
  height: number;
  close: () => void;
}> {
  if (typeof createImageBitmap === "function") {
    const bitmap = await createImageBitmap(file, { imageOrientation: "from-image" });
    return {
      source: bitmap,
      width: bitmap.width,
      height: bitmap.height,
      close: () => bitmap.close(),
    };
  }

  const url = URL.createObjectURL(file);
  const image = new Image();
  try {
    await new Promise<void>((resolve, reject) => {
      image.onload = () => resolve();
      image.onerror = () => reject(new Error("This browser cannot decode the selected image"));
      image.src = url;
    });
    return {
      source: image,
      width: image.naturalWidth,
      height: image.naturalHeight,
      close: () => URL.revokeObjectURL(url),
    };
  } catch (error) {
    URL.revokeObjectURL(url);
    throw error;
  }
}

function createCanvas(width: number, height: number): HTMLCanvasElement | OffscreenCanvas {
  if (typeof OffscreenCanvas !== "undefined") return new OffscreenCanvas(width, height);
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  return canvas;
}

function releaseCanvas(canvas: HTMLCanvasElement | OffscreenCanvas): void {
  canvas.width = 1;
  canvas.height = 1;
}

async function sha256(blob: Blob): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", await blob.arrayBuffer());
  return Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, "0")).join(
    "",
  );
}

async function assertWebP(blob: Blob): Promise<void> {
  const header = new Uint8Array(await blob.slice(0, 12).arrayBuffer());
  const riff = String.fromCharCode(...header.slice(0, 4));
  const webp = String.fromCharCode(...header.slice(8, 12));
  if (blob.type !== "image/webp" || riff !== "RIFF" || webp !== "WEBP") {
    throw new Error("This browser cannot encode WebP images");
  }
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) {
    throw signal.reason ?? abortError();
  }
}

function abortError(): DOMException {
  return new DOMException("Image processing aborted", "AbortError");
}
