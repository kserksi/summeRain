// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

export type ClientImageErrorCode =
  | "IMAGE_FILE_INVALID"
  | "IMAGE_FILE_SIZE_EXCEEDED"
  | "IMAGE_FORMAT_UNSUPPORTED"
  | "IMAGE_CONTENT_MISMATCH"
  | "IMAGE_ANIMATION_UNSUPPORTED"
  | "IMAGE_DIMENSION_EXCEEDED"
  | "IMAGE_RECIPE_UNSUPPORTED"
  | "IMAGE_PROCESSOR_UNAVAILABLE"
  | "IMAGE_DECODE_FAILED"
  | "IMAGE_WEBP_ENCODE_UNAVAILABLE"
  | "IMAGE_PROCESSING_TIMEOUT"
  | "IMAGE_PROCESSING_FAILED";

export type ClientImageErrorDetails = Record<string, string | number | boolean>;

interface ClientImageErrorOptions {
  cause?: unknown;
  details?: ClientImageErrorDetails;
  retryable?: boolean;
}

export class ClientImageError extends Error {
  readonly code: ClientImageErrorCode;
  readonly details: ClientImageErrorDetails;
  readonly retryable: boolean;

  constructor(code: ClientImageErrorCode, message: string, options: ClientImageErrorOptions = {}) {
    super(message, { cause: options.cause });
    this.name = "ClientImageError";
    this.code = code;
    this.details = options.details ?? {};
    this.retryable = options.retryable ?? false;
  }
}

export function isClientImageError(error: unknown): error is ClientImageError {
  return error instanceof ClientImageError;
}

