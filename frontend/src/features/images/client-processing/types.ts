// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

export type V2VariantKind = "master" | "gallery" | "admin" | "publish_source";

export interface ProcessedPart {
  kind: V2VariantKind;
  blob: Blob;
  size: number;
  sha256: string;
  mime_type: "image/webp";
  width: number;
  height: number;
  quality: 60 | 80;
}

export interface ProcessedImage {
  source: {
    mime_type: string;
    width: number;
    height: number;
    animated: false;
  };
  processor_version: string;
  recipe_version: "2.0.0";
  parts: ProcessedPart[];
}

export type ProcessingProgress = (percent: number) => void;

export interface WorkerSuccessMessage {
  type: "result";
  id: string;
  result: Omit<ProcessedImage, "processor_version">;
}

export interface WorkerErrorMessage {
  type: "error";
  id: string;
  message: string;
  recoverable: boolean;
}

export interface WorkerProgressMessage {
  type: "progress";
  id: string;
  percent: number;
}

export type WorkerResponse = WorkerSuccessMessage | WorkerErrorMessage | WorkerProgressMessage;
