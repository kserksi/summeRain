// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import zhCN from './locales/zh-CN.json'
import jaJP from './locales/ja-JP.json'

const SUPPORTED = ['zh-CN', 'ja-JP'] as const
const FALLBACK = 'zh-CN'

function detectLang(): string {
  const stored = localStorage.getItem('lang')
  if (stored && (SUPPORTED as readonly string[]).includes(stored)) return stored

  const browser = navigator.language
  if ((SUPPORTED as readonly string[]).includes(browser)) return browser

  const prefix = browser.split('-')[0]
  const match = SUPPORTED.find((l) => l.startsWith(prefix))
  if (match) return match

  return FALLBACK
}

i18n.use(initReactI18next).init({
  resources: {
    'zh-CN': { translation: zhCN },
    'ja-JP': { translation: jaJP },
  },
  lng: detectLang(),
  fallbackLng: FALLBACK,
  interpolation: { escapeValue: false },
})

export function changeLanguage(lng: string) {
  localStorage.setItem('lang', lng)
  return i18n.changeLanguage(lng)
}

export default i18n
