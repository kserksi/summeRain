// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";

import { sniffInput } from "./sniff";

describe("sniffInput", () => {
  it("accepts a static WebP header", async () => {
    const bytes = new Uint8Array(30);
    bytes.set(new TextEncoder().encode("RIFF"), 0);
    bytes.set(new TextEncoder().encode("WEBP"), 8);
    bytes.set(new TextEncoder().encode("VP8X"), 12);
    bytes[16] = 10;
    const file = new File([bytes], "photo.webp", { type: "image/webp" });
    await expect(sniffInput(file)).resolves.toEqual({
      mimeType: "image/webp",
      animated: false,
      width: 1,
      height: 1,
    });
  });

  it("detects animated WebP and rejects mismatched extensions", async () => {
    const bytes = new Uint8Array(30);
    bytes.set(new TextEncoder().encode("RIFF"), 0);
    bytes.set(new TextEncoder().encode("WEBP"), 8);
    bytes.set(new TextEncoder().encode("VP8X"), 12);
    bytes[16] = 10;
    bytes[20] = 0x02;
    await expect(
      sniffInput(new File([bytes], "animated.webp", { type: "image/webp" })),
    ).resolves.toMatchObject({
      animated: true,
    });
    await expect(sniffInput(new File([bytes], "fake.jpg", { type: "image/jpeg" }))).rejects.toThrow(
      "does not match",
    );
  });

  it("parses PNG chunks and ignores animation-like metadata text", async () => {
    const signature = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);
    const ihdr = pngChunk("IHDR", new Uint8Array([0, 0, 0, 2, 0, 0, 0, 3, 8, 6, 0, 0, 0]));
    const text = pngChunk("tEXt", new TextEncoder().encode("the word acTL is only metadata"));
    const still = new File(
      [signature, ihdr, text, pngChunk("IEND", new Uint8Array())].map(blobPart),
      "still.png",
      {
        type: "image/png",
      },
    );
    await expect(sniffInput(still)).resolves.toMatchObject({
      width: 2,
      height: 3,
      animated: false,
    });

    const motion = new File(
      [
        signature,
        ihdr,
        pngChunk("acTL", new Uint8Array(8)),
        pngChunk("IEND", new Uint8Array()),
      ].map(blobPart),
      "motion.png",
      { type: "image/png" },
    );
    await expect(sniffInput(motion)).resolves.toMatchObject({ animated: true });
  });
});

function pngChunk(type: string, payload: Uint8Array): Uint8Array {
  const chunk = new Uint8Array(12 + payload.length);
  new DataView(chunk.buffer).setUint32(0, payload.length);
  chunk.set(new TextEncoder().encode(type), 4);
  chunk.set(payload, 8);
  return chunk;
}

function blobPart(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}
