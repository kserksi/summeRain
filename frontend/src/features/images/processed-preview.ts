// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import type { ProcessedImage } from "./client-processing/types";

export function createProcessedPreviewURL(processed: ProcessedImage): string | undefined {
  const gallery = processed.parts.find((part) => part.kind === "gallery");
  return gallery ? URL.createObjectURL(gallery.blob) : undefined;
}
