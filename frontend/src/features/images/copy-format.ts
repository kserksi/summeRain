// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import {
  resolveCopyImageUrl,
  type LegacyCopyImageFormat,
} from './asset-url'

export type CopyLinkFormat = 'url' | 'markdown' | 'bbs' | 'html'
export type CopyImageFormat = LegacyCopyImageFormat

export interface CopyAssetMetadata {
  pipelineVersion?: number
  assetLink?: string
}

export interface CopyPrefs {
  link: CopyLinkFormat
  image: CopyImageFormat
}

export const DEFAULT_PREFS: CopyPrefs = { link: 'url', image: 'webp' }

const STORAGE_KEY = 'upload:copyPrefs'
const LINK_FORMATS: readonly CopyLinkFormat[] = ['url', 'markdown', 'bbs', 'html']
const IMAGE_FORMATS: readonly CopyImageFormat[] = ['original', 'webp', 'avif']

function isCopyLinkFormat(v: unknown): v is CopyLinkFormat {
  return typeof v === 'string' && (LINK_FORMATS as readonly string[]).includes(v)
}

function isCopyImageFormat(v: unknown): v is CopyImageFormat {
  return typeof v === 'string' && (IMAGE_FORMATS as readonly string[]).includes(v)
}

/** Strip the last extension from a filename. Dotfiles are kept as-is. */
export function baseName(fileName: string): string {
  const dot = fileName.lastIndexOf('.')
  if (dot <= 0) return fileName
  return fileName.slice(0, dot)
}

/** Build the public URL for an image given its link id and target image format. */
export function buildUrl(
  origin: string,
  link: string,
  format: CopyImageFormat,
  metadata?: CopyAssetMetadata,
): string {
  const path = resolveCopyImageUrl(
    {
      unique_link: link,
      pipeline_version: metadata?.pipelineVersion,
      asset_link: metadata?.assetLink,
    },
    format,
  )
  return new URL(path, origin).toString()
}

/** Format a single line according to the chosen link format. */
export function formatLine(
  url: string,
  alt: string,
  format: CopyLinkFormat,
): string {
  switch (format) {
    case 'markdown':
      return `![${alt}](${url})`
    case 'bbs':
      return `[img]${url}[/img]`
    case 'html':
      return `<img src="${url}" alt="${alt}">`
    case 'url':
    default:
      return url
  }
}

/** Build the full multi-line text to copy. */
export function buildCopyText(
  origin: string,
  items: readonly {
    uniqueLink: string
    fileName: string
    pipelineVersion?: number
    assetLink?: string
  }[],
  link: CopyLinkFormat,
  image: CopyImageFormat,
): string {
  return items
    .map((it) =>
      formatLine(
        buildUrl(origin, it.uniqueLink, image, {
          pipelineVersion: it.pipelineVersion,
          assetLink: it.assetLink,
        }),
        baseName(it.fileName),
        link,
      ),
    )
    .join('\n')
}

/** Read persisted prefs from localStorage; falls back to DEFAULT_PREFS. */
export function loadPrefs(): CopyPrefs {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return DEFAULT_PREFS
    const parsed = JSON.parse(raw) as Partial<CopyPrefs>
    if (isCopyLinkFormat(parsed.link) && isCopyImageFormat(parsed.image)) {
      return { link: parsed.link, image: parsed.image }
    }
    return DEFAULT_PREFS
  } catch {
    return DEFAULT_PREFS
  }
}

/** Persist prefs to localStorage. */
export function savePrefs(prefs: CopyPrefs): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs))
  } catch {
    // localStorage unavailable (private mode, quota); silently ignore
  }
}
