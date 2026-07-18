// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it } from "vitest";

import { getNativeCanvasCapability } from "./native-capability";

const originalDeviceMemory = Object.getOwnPropertyDescriptor(navigator, "deviceMemory");

afterEach(() => {
  if (originalDeviceMemory) {
    Object.defineProperty(navigator, "deviceMemory", originalDeviceMemory);
  } else {
    Reflect.deleteProperty(navigator, "deviceMemory");
  }
});

describe("getNativeCanvasCapability", () => {
  it("allows a typical 12 MP phone image on the conservative default tier", () => {
    setDeviceMemory(4);
    expect(getNativeCanvasCapability(4_032, 3_024)).toMatchObject({
      safe: true,
      maxPixels: 13_000_000,
    });
  });

  it("lowers the pixel budget on a low-memory device", () => {
    setDeviceMemory(2);
    expect(getNativeCanvasCapability(4_032, 3_024)).toMatchObject({
      safe: false,
      maxPixels: 8_000_000,
    });
  });

  it("rejects an unsafe dimension even when the total pixel count is small", () => {
    setDeviceMemory(8);
    expect(getNativeCanvasCapability(9_000, 1_000)).toMatchObject({
      safe: false,
      maxPixels: 16_000_000,
    });
  });
});

function setDeviceMemory(value: number): void {
  Object.defineProperty(navigator, "deviceMemory", {
    configurable: true,
    value,
  });
}
