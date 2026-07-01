// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach } from 'vitest'

import {
  baseName,
  buildUrl,
  formatLine,
  buildCopyText,
  loadPrefs,
  savePrefs,
  DEFAULT_PREFS,
} from './copy-format'

describe('baseName', () => {
  it('strips last extension', () => {
    expect(baseName('photo.jpg')).toBe('photo')
    expect(baseName('a.b.c.png')).toBe('a.b.c')
  })
  it('returns as-is when no extension', () => {
    expect(baseName('README')).toBe('README')
  })
  it('handles dotfiles', () => {
    expect(baseName('.hidden')).toBe('.hidden')
  })
})

describe('buildUrl', () => {
  const origin = 'https://img.example.com'
  it('original: no extension, no query', () => {
    expect(buildUrl(origin, 'abc123', 'original')).toBe(
      'https://img.example.com/i/abc123',
    )
  })
  it('webp: appends .webp?q=85', () => {
    expect(buildUrl(origin, 'abc123', 'webp')).toBe(
      'https://img.example.com/i/abc123.webp?q=85',
    )
  })
  it('avif: appends .avif?q=85', () => {
    expect(buildUrl(origin, 'abc123', 'avif')).toBe(
      'https://img.example.com/i/abc123.avif?q=85',
    )
  })
})

describe('formatLine', () => {
  const url = 'https://img.example.com/i/abc.webp?q=85'
  it('url: passthrough', () => {
    expect(formatLine(url, 'photo', 'url')).toBe(url)
  })
  it('markdown: ![alt](url)', () => {
    expect(formatLine(url, 'photo', 'markdown')).toBe(
      `![photo](${url})`,
    )
  })
  it('bbs: [img]url[/img]', () => {
    expect(formatLine(url, 'photo', 'bbs')).toBe(`[img]${url}[/img]`)
  })
  it('html: <img src=url alt=alt>', () => {
    expect(formatLine(url, 'photo', 'html')).toBe(
      `<img src="${url}" alt="photo">`,
    )
  })
})

describe('buildCopyText', () => {
  const origin = 'https://img.example.com'
  const items = [
    { uniqueLink: 'aaa', fileName: 'cat.jpg' },
    { uniqueLink: 'bbb', fileName: 'dog.png' },
  ]
  it('combines url + webp by default', () => {
    expect(buildCopyText(origin, items, 'url', 'webp')).toBe(
      'https://img.example.com/i/aaa.webp?q=85\nhttps://img.example.com/i/bbb.webp?q=85',
    )
  })
  it('markdown + avif uses baseName as alt', () => {
    expect(buildCopyText(origin, items, 'markdown', 'avif')).toBe(
      '![cat](https://img.example.com/i/aaa.avif?q=85)\n![dog](https://img.example.com/i/bbb.avif?q=85)',
    )
  })
  it('empty list returns empty string', () => {
    expect(buildCopyText(origin, [], 'url', 'webp')).toBe('')
  })
})

describe('prefs persistence', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('DEFAULT_PREFS is url + webp', () => {
    expect(DEFAULT_PREFS).toEqual({ link: 'url', image: 'webp' })
  })

  it('loadPrefs returns DEFAULT_PREFS when nothing stored', () => {
    expect(loadPrefs()).toEqual(DEFAULT_PREFS)
  })

  it('savePrefs then loadPrefs roundtrip', () => {
    savePrefs({ link: 'markdown', image: 'avif' })
    expect(loadPrefs()).toEqual({ link: 'markdown', image: 'avif' })
  })

  it('loadPrefs falls back on malformed JSON', () => {
    localStorage.setItem('upload:copyPrefs', '{not json')
    expect(loadPrefs()).toEqual(DEFAULT_PREFS)
  })

  it('loadPrefs falls back on invalid enum values', () => {
    localStorage.setItem(
      'upload:copyPrefs',
      JSON.stringify({ link: 'bogus', image: 'webp' }),
    )
    expect(loadPrefs()).toEqual(DEFAULT_PREFS)
  })
})
