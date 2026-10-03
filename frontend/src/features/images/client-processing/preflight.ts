// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { resolveUploadLimits } from "../upload-limits";
import type { V2RecipeResponse } from "../v2-upload";
import { ClientImageError } from "./errors";
import { getNativeCanvasCapability } from "./native-capability";
import { sniffInput, type SniffedInput } from "./sniff";
import type { ClientProcessorKind } from "./types";
import { probeWasmVips } from "./wasm-capability";

const MAX_WEBP_DIMENSION = 16_383;

export interface ClientProcessingPlan {
  readonly input: SniffedInput;
  readonly processor: ClientProcessorKind;
  readonly nativeFallbackSafe: boolean;
  // Resolved from the server recipe, the device capability, and the caps.
  readonly recipeVersion: string;
  readonly nativeConcurrency: number;
}

export async function preflightClientImage(
  file: File,
  recipe: V2RecipeResponse,
  signal?: AbortSignal,
): Promise<ClientProcessingPlan> {
  throwIfAborted(signal);
  assertSupportedRecipe(recipe);
  const limits = resolveUploadLimits(recipe);
  if (file.size > limits.maxSourceBytes) {
    throw new ClientImageError(
      "IMAGE_FILE_SIZE_EXCEEDED",
      "Image exceeds the server source-size limit",
      { details: { maxMB: Math.floor(limits.maxSourceBytes / (1024 * 1024)) } },
    );
  }
  const input = await waitWithSignal(sniffInput(file), signal);
  throwIfAborted(signal);

  if (input.animated) {
    throw new ClientImageError(
      "IMAGE_ANIMATION_UNSUPPORTED",
      "Animated images are not supported in V2",
    );
  }
  if (
    input.width > MAX_WEBP_DIMENSION ||
    input.height > MAX_WEBP_DIMENSION ||
    input.width > Math.floor(recipe.max_pixels / input.height)
  ) {
    throw new ClientImageError(
      "IMAGE_DIMENSION_EXCEEDED",
      "Image dimensions exceed the negotiated recipe limit",
      {
        details: {
          width: input.width,
          height: input.height,
          maxMP: Number((recipe.max_pixels / 1_000_000).toFixed(1)),
          maxDimension: MAX_WEBP_DIMENSION,
        },
      },
    );
  }

  const nativeCapability = getNativeCanvasCapability(input.width, input.height);
  if (await probeWasmVips(signal)) {
    return {
      input,
      processor: "wasm-vips",
      nativeFallbackSafe: nativeCapability.safe,
      recipeVersion: recipe.recipe_version,
      nativeConcurrency: limits.nativeConcurrency,
    };
  }
  if (nativeCapability.safe) {
    return {
      input,
      processor: "native-pica",
      nativeFallbackSafe: true,
      recipeVersion: recipe.recipe_version,
      nativeConcurrency: limits.nativeConcurrency,
    };
  }

  throw new ClientImageError(
    "IMAGE_PROCESSOR_UNAVAILABLE",
    "This browser cannot safely process the image without wasm-vips",
    {
      details: {
        width: input.width,
        height: input.height,
        maxMP: Number((nativeCapability.maxPixels / 1_000_000).toFixed(1)),
      },
    },
  );
}

function assertSupportedRecipe(recipe: V2RecipeResponse): void {
  if (
    recipe.v2_enabled === false ||
    recipe.pipeline_version !== 2 ||
    !Number.isSafeInteger(recipe.max_pixels) ||
    recipe.max_pixels <= 0
  ) {
    throw new ClientImageError(
      "IMAGE_RECIPE_UNSUPPORTED",
      "This client does not support the server image recipe",
      {
        details: {
          recipe_version: recipe.recipe_version,
          pipeline_version: recipe.pipeline_version,
        },
      },
    );
  }
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw signal.reason ?? abortError();
}

function abortError(): DOMException {
  return new DOMException("Image processing aborted", "AbortError");
}

function waitWithSignal<T>(promise: Promise<T>, signal?: AbortSignal): Promise<T> {
  if (!signal) return promise;
  throwIfAborted(signal);
  return new Promise<T>((resolve, reject) => {
    const abort = () => reject(signal.reason ?? abortError());
    signal.addEventListener("abort", abort, { once: true });
    promise.then(
      (value) => {
        signal.removeEventListener("abort", abort);
        resolve(value);
      },
      (error: unknown) => {
        signal.removeEventListener("abort", abort);
        reject(error);
      },
    );
  });
}

