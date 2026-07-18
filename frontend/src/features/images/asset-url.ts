// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

export type ImageAssetRole = 'publish' | 'gallery' | 'admin' | 'master'
export type LegacyCopyImageFormat = 'original' | 'webp' | 'avif'

export interface ImageAssetReference {
  unique_link: string
  pipeline_version?: number
  asset_link?: string
}

export function hasV2Assets(
  image: ImageAssetReference,
): image is ImageAssetReference & { pipeline_version: number; asset_link: string } {
  return (image.pipeline_version ?? 0) >= 2 && Boolean(image.asset_link?.trim())
}

function pathSegment(value: string): string {
  return encodeURIComponent(value)
}

function resolveV1Asset(image: ImageAssetReference, role: ImageAssetRole): string {
  const link = pathSegment(image.unique_link)
  switch (role) {
    case 'gallery':
      return `/i/${link}.webp?w=400`
    case 'admin':
      return `/i/${link}.webp?w=80&h=60`
    case 'master':
      return `/i/${link}`
    case 'publish':
    default:
      return `/i/${link}.webp?q=85`
  }
}

export function appendAccessToken(url: string, token?: string | null): string {
  if (!token) return url

  const hashIndex = url.indexOf('#')
  const hash = hashIndex >= 0 ? url.slice(hashIndex) : ''
  const withoutHash = hashIndex >= 0 ? url.slice(0, hashIndex) : url
  const queryIndex = withoutHash.indexOf('?')
  const path = queryIndex >= 0 ? withoutHash.slice(0, queryIndex) : withoutHash
  const params = new URLSearchParams(queryIndex >= 0 ? withoutHash.slice(queryIndex + 1) : '')
  params.set('token', token)
  return `${path}?${params.toString()}${hash}`
}

export function resolveImageAssetUrl(
  image: ImageAssetReference,
  role: ImageAssetRole,
  token?: string | null,
): string {
  if (!hasV2Assets(image)) {
    return appendAccessToken(resolveV1Asset(image, role), token)
  }

  const asset = pathSegment(image.asset_link)
  const path = role === 'publish' ? `/i/${asset}.webp` : `/i/${asset}/${role}.webp`
  return appendAccessToken(path, token)
}

export function resolveAbsoluteImageAssetUrl(
  origin: string,
  image: ImageAssetReference,
  role: ImageAssetRole,
  token?: string | null,
): string {
  return new URL(resolveImageAssetUrl(image, role, token), origin).toString()
}

export function resolveDetailPreviewUrl(
  image: ImageAssetReference,
  token?: string | null,
): string {
  return resolveImageAssetUrl(image, 'publish', token)
}

export function resolveCopyImageUrl(
  image: ImageAssetReference,
  format: LegacyCopyImageFormat,
): string {
  if (hasV2Assets(image)) return resolveImageAssetUrl(image, 'publish')
  if (format === 'original') return resolveImageAssetUrl(image, 'master')
  if (format === 'webp') return resolveImageAssetUrl(image, 'publish')
  return `/i/${pathSegment(image.unique_link)}.avif?q=85`
}
