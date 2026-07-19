// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { ClientImageError } from "./errors";

const MAX_SOURCE_BYTES = 15 * 1024 * 1024;
const MAX_SOURCE_PIXELS = 50_000_000;

const MIME_BY_EXTENSION: Record<string, string> = {
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".png": "image/png",
  ".bmp": "image/bmp",
  ".webp": "image/webp",
  ".avif": "image/avif",
};

const ALLOWED_MIME = new Set([
  ...Object.values(MIME_BY_EXTENSION),
  "image/x-ms-bmp",
  "image/x-bmp",
]);

export interface SniffedInput {
  mimeType: string;
  animated: boolean;
  width: number;
  height: number;
}

interface InspectedImage {
  mimeType: string;
  animated: boolean;
  width: number;
  height: number;
}

export async function sniffInput(file: File): Promise<SniffedInput> {
  if (file.size <= 0) {
    throw new ClientImageError("IMAGE_FILE_INVALID", "The image file is empty");
  }
  if (file.size > MAX_SOURCE_BYTES) {
    throw new ClientImageError("IMAGE_FILE_SIZE_EXCEEDED", "Image exceeds the 15 MB limit", {
      details: { maxMB: 15 },
    });
  }
  const dot = file.name.lastIndexOf(".");
  const extension = dot >= 0 ? file.name.slice(dot).toLowerCase() : "";
  const extensionMime = MIME_BY_EXTENSION[extension];
  if (!extensionMime) {
    throw new ClientImageError("IMAGE_FORMAT_UNSUPPORTED", "Unsupported image format");
  }

  // The source is capped at 15 MB, so a single bounded read lets us parse real
  // container boundaries and reject animation before allocating decoded pixels.
  const bytes = new Uint8Array(await file.arrayBuffer());
  const inspected = inspectImage(bytes);
  if (!inspected || inspected.mimeType !== extensionMime) {
    throw new ClientImageError(
      "IMAGE_CONTENT_MISMATCH",
      "Image content does not match its extension",
    );
  }
  const declaredType = file.type.toLowerCase();
  if (
    declaredType &&
    ALLOWED_MIME.has(declaredType) &&
    normalizeMime(declaredType) !== inspected.mimeType
  ) {
    throw new ClientImageError(
      "IMAGE_CONTENT_MISMATCH",
      "Image MIME type does not match its content",
    );
  }
  if (inspected.width <= 0 || inspected.height <= 0) {
    throw new ClientImageError("IMAGE_FILE_INVALID", "Image dimensions are invalid");
  }
  if (inspected.width > Math.floor(MAX_SOURCE_PIXELS / inspected.height)) {
    throw new ClientImageError("IMAGE_DIMENSION_EXCEEDED", "Image exceeds the 50 MP limit", {
      details: { maxMP: 50 },
    });
  }
  return inspected;
}

function inspectImage(bytes: Uint8Array): InspectedImage | null {
  if (bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) {
    const dimensions = inspectJPEG(bytes);
    return dimensions && { mimeType: "image/jpeg", animated: false, ...dimensions };
  }
  if (
    bytes.length >= 24 &&
    bytes[0] === 0x89 &&
    ascii(bytes, 1, 3) === "PNG" &&
    bytes[4] === 0x0d &&
    bytes[5] === 0x0a &&
    bytes[6] === 0x1a &&
    bytes[7] === 0x0a
  ) {
    const png = inspectPNG(bytes);
    return png && { mimeType: "image/png", ...png };
  }
  if (bytes.length >= 26 && ascii(bytes, 0, 2) === "BM") {
    const dimensions = inspectBMP(bytes);
    return dimensions && { mimeType: "image/bmp", animated: false, ...dimensions };
  }
  if (bytes.length >= 20 && ascii(bytes, 0, 4) === "RIFF" && ascii(bytes, 8, 4) === "WEBP") {
    const webp = inspectWebP(bytes);
    return webp && { mimeType: "image/webp", ...webp };
  }
  if (bytes.length >= 16 && ascii(bytes, 4, 4) === "ftyp") {
    const avif = inspectAVIF(bytes);
    return avif && { mimeType: "image/avif", ...avif };
  }
  return null;
}

function inspectJPEG(bytes: Uint8Array): { width: number; height: number } | null {
  const sofMarkers = new Set([
    0xc0, 0xc1, 0xc2, 0xc3, 0xc5, 0xc6, 0xc7, 0xc9, 0xca, 0xcb, 0xcd, 0xce, 0xcf,
  ]);
  let offset = 2;
  while (offset + 3 < bytes.length) {
    while (offset < bytes.length && bytes[offset] !== 0xff) offset += 1;
    while (offset < bytes.length && bytes[offset] === 0xff) offset += 1;
    if (offset >= bytes.length) break;
    const marker = bytes[offset++];
    if (marker === 0xd9 || marker === 0xda) break;
    if (marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) continue;
    if (offset + 2 > bytes.length) return null;
    const length = readU16BE(bytes, offset);
    if (length < 2 || offset + length > bytes.length) return null;
    if (sofMarkers.has(marker) && length >= 7) {
      return { height: readU16BE(bytes, offset + 3), width: readU16BE(bytes, offset + 5) };
    }
    offset += length;
  }
  return null;
}

function inspectPNG(
  bytes: Uint8Array,
): { width: number; height: number; animated: boolean } | null {
  let offset = 8;
  let width = 0;
  let height = 0;
  let animated = false;
  while (offset + 12 <= bytes.length) {
    const length = readU32BE(bytes, offset);
    const type = ascii(bytes, offset + 4, 4);
    const next = offset + 12 + length;
    if (next > bytes.length) return null;
    if (type === "IHDR" && length === 13) {
      width = readU32BE(bytes, offset + 8);
      height = readU32BE(bytes, offset + 12);
    } else if (type === "acTL") {
      animated = true;
    }
    offset = next;
    if (type === "IEND") break;
  }
  return width > 0 && height > 0 ? { width, height, animated } : null;
}

function inspectBMP(bytes: Uint8Array): { width: number; height: number } | null {
  const dibSize = readU32LE(bytes, 14);
  if (dibSize === 12 && bytes.length >= 22) {
    return { width: readU16LE(bytes, 18), height: readU16LE(bytes, 20) };
  }
  if (dibSize < 40 || bytes.length < 26) return null;
  const width = Math.abs(readI32LE(bytes, 18));
  const height = Math.abs(readI32LE(bytes, 22));
  return width > 0 && height > 0 ? { width, height } : null;
}

function inspectWebP(
  bytes: Uint8Array,
): { width: number; height: number; animated: boolean } | null {
  let offset = 12;
  let width = 0;
  let height = 0;
  let animated = false;
  while (offset + 8 <= bytes.length) {
    const type = ascii(bytes, offset, 4);
    const length = readU32LE(bytes, offset + 4);
    const payload = offset + 8;
    if (payload + length > bytes.length) return null;
    if (type === "VP8X" && length >= 10) {
      animated ||= (bytes[payload] & 0x02) !== 0;
      width = 1 + readU24LE(bytes, payload + 4);
      height = 1 + readU24LE(bytes, payload + 7);
    } else if (
      type === "VP8 " &&
      length >= 10 &&
      ascii(bytes, payload + 3, 3) === "\u009d\u0001\u002a"
    ) {
      width ||= readU16LE(bytes, payload + 6) & 0x3fff;
      height ||= readU16LE(bytes, payload + 8) & 0x3fff;
    } else if (type === "VP8L" && length >= 5 && bytes[payload] === 0x2f) {
      width ||= 1 + bytes[payload + 1] + ((bytes[payload + 2] & 0x3f) << 8);
      height ||=
        1 +
        (bytes[payload + 2] >> 6) +
        (bytes[payload + 3] << 2) +
        ((bytes[payload + 4] & 0x0f) << 10);
    } else if (type === "ANIM" || type === "ANMF") {
      animated = true;
    }
    offset = payload + length + (length & 1);
  }
  return width > 0 && height > 0 ? { width, height, animated } : null;
}

function inspectAVIF(
  bytes: Uint8Array,
): { width: number; height: number; animated: boolean } | null {
  const ftypSize = readU32BE(bytes, 0);
  if (ftypSize < 16 || ftypSize > bytes.length || ascii(bytes, 4, 4) !== "ftyp") return null;
  const brands: string[] = [];
  for (let offset = 8; offset + 4 <= ftypSize; offset += 4) brands.push(ascii(bytes, offset, 4));
  if (!brands.includes("avif") && !brands.includes("avis")) return null;

  const dimensions: Array<{ width: number; height: number }> = [];
  walkISOBoxes(bytes, 0, bytes.length, 0, dimensions);
  if (dimensions.length === 0) return null;
  const largest = dimensions.reduce((best, item) =>
    item.width * item.height > best.width * best.height ? item : best,
  );
  return {
    ...largest,
    animated: brands.includes("avis") || brands.includes("msf1"),
  };
}

const ISO_CONTAINERS = new Set([
  "moov",
  "trak",
  "mdia",
  "minf",
  "stbl",
  "dinf",
  "edts",
  "meta",
  "iprp",
  "ipco",
  "iref",
]);

function walkISOBoxes(
  bytes: Uint8Array,
  start: number,
  end: number,
  depth: number,
  dimensions: Array<{ width: number; height: number }>,
): void {
  if (depth > 12) return;
  let offset = start;
  while (offset + 8 <= end) {
    let size = readU32BE(bytes, offset);
    const type = ascii(bytes, offset + 4, 4);
    let header = 8;
    if (size === 1) {
      if (offset + 16 > end) return;
      const high = readU32BE(bytes, offset + 8);
      const low = readU32BE(bytes, offset + 12);
      if (high !== 0) return;
      size = low;
      header = 16;
    } else if (size === 0) {
      size = end - offset;
    }
    if (size < header || offset + size > end) return;
    const payload = offset + header;
    if (type === "ispe" && size >= header + 12) {
      const width = readU32BE(bytes, payload + 4);
      const height = readU32BE(bytes, payload + 8);
      if (width > 0 && height > 0) dimensions.push({ width, height });
    } else if (ISO_CONTAINERS.has(type)) {
      walkISOBoxes(
        bytes,
        payload + (type === "meta" ? 4 : 0),
        offset + size,
        depth + 1,
        dimensions,
      );
    }
    offset += size;
  }
}

function normalizeMime(mime: string): string {
  return mime === "image/x-ms-bmp" || mime === "image/x-bmp" ? "image/bmp" : mime;
}

function readU16BE(bytes: Uint8Array, offset: number): number {
  return (bytes[offset] << 8) | bytes[offset + 1];
}

function readU16LE(bytes: Uint8Array, offset: number): number {
  return bytes[offset] | (bytes[offset + 1] << 8);
}

function readU24LE(bytes: Uint8Array, offset: number): number {
  return bytes[offset] | (bytes[offset + 1] << 8) | (bytes[offset + 2] << 16);
}

function readU32BE(bytes: Uint8Array, offset: number): number {
  return (
    (bytes[offset] * 0x1000000 +
      (bytes[offset + 1] << 16) +
      (bytes[offset + 2] << 8) +
      bytes[offset + 3]) >>>
    0
  );
}

function readU32LE(bytes: Uint8Array, offset: number): number {
  return (
    (bytes[offset] +
      (bytes[offset + 1] << 8) +
      (bytes[offset + 2] << 16) +
      bytes[offset + 3] * 0x1000000) >>>
    0
  );
}

function readI32LE(bytes: Uint8Array, offset: number): number {
  return new DataView(bytes.buffer, bytes.byteOffset + offset, 4).getInt32(0, true);
}

function ascii(bytes: Uint8Array, offset: number, length: number): string {
  let result = "";
  const end = Math.min(bytes.length, offset + length);
  for (let index = offset; index < end; index += 1) result += String.fromCharCode(bytes[index]);
  return result;
}
