// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

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
}

export async function preflightClientImage(
  file: File,
  recipe: V2RecipeResponse,
  signal?: AbortSignal,
): Promise<ClientProcessingPlan> {
  throwIfAborted(signal);
  assertSupportedRecipe(recipe);
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
    };
  }
  if (nativeCapability.safe) {
    return { input, processor: "native-pica", nativeFallbackSafe: true };
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
    recipe.recipe_version !== "2.0.0" ||
    !Number.isSafeInteger(recipe.max_pixels) ||
    recipe.max_pixels <= 0
  ) {
    throw new ClientImageError(
      "IMAGE_RECIPE_UNSUPPORTED",
      "This client does not support the server image recipe",
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

