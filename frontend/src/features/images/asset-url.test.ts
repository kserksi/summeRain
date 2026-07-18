// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import {
  appendAccessToken,
  hasV2Assets,
  resolveAbsoluteImageAssetUrl,
  resolveCopyImageUrl,
  resolveDetailPreviewUrl,
  resolveImageAssetUrl,
} from './asset-url'

describe('image asset URL resolver', () => {
  const v1 = { unique_link: 'legacy-link' }
  const v2 = {
    unique_link: 'legacy-link',
    pipeline_version: 2,
    asset_link: 'asset-link',
  }

  it('preserves the existing V1 role URLs', () => {
    expect(resolveImageAssetUrl(v1, 'publish')).toBe('/i/legacy-link.webp?q=85')
    expect(resolveImageAssetUrl(v1, 'gallery')).toBe('/i/legacy-link.webp?w=400')
    expect(resolveImageAssetUrl(v1, 'admin')).toBe('/i/legacy-link.webp?w=80&h=60')
    expect(resolveImageAssetUrl(v1, 'master')).toBe('/i/legacy-link')
  })

  it('uses fixed V2 role URLs', () => {
    expect(hasV2Assets(v2)).toBe(true)
    expect(resolveImageAssetUrl(v2, 'publish')).toBe('/i/asset-link.webp')
    expect(resolveImageAssetUrl(v2, 'gallery')).toBe('/i/asset-link/gallery.webp')
    expect(resolveImageAssetUrl(v2, 'admin')).toBe('/i/asset-link/admin.webp')
    expect(resolveImageAssetUrl(v2, 'master')).toBe('/i/asset-link/master.webp')
  })

  it('uses the bounded publish asset for the V2 detail preview', () => {
    expect(resolveDetailPreviewUrl(v2)).toBe('/i/asset-link.webp')
  })

  it('falls back to V1 until both V2 fields are present', () => {
    expect(
      resolveImageAssetUrl({ unique_link: 'legacy-link', pipeline_version: 2 }, 'gallery'),
    ).toBe('/i/legacy-link.webp?w=400')
  })

  it('appends or replaces an encoded token without losing query or hash', () => {
    expect(appendAccessToken('/i/a.webp?q=85#preview', 'a+b&c')).toBe(
      '/i/a.webp?q=85&token=a%2Bb%26c#preview',
    )
    expect(appendAccessToken('/i/a.webp?token=old', 'new token')).toBe('/i/a.webp?token=new+token')
  })

  it('builds absolute URLs and pins V2 copy links to publish', () => {
    expect(resolveAbsoluteImageAssetUrl('https://img.example.com', v2, 'gallery')).toBe(
      'https://img.example.com/i/asset-link/gallery.webp',
    )
    expect(resolveCopyImageUrl(v2, 'avif')).toBe('/i/asset-link.webp')
  })
})
