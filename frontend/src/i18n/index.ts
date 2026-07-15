// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import enUS from './locales/en-US.json'
import zhCN from './locales/zh-CN.json'
import jaJP from './locales/ja-JP.json'

i18n.use(initReactI18next).init({
  resources: {
    'en-US': { translation: enUS },
    'zh-CN': { translation: zhCN },
    'ja-JP': { translation: jaJP },
  },
  lng: 'en-US',
  fallbackLng: 'en-US',
  interpolation: { escapeValue: false },
})

const syncDocumentLanguage = (language: string) => {
  document.documentElement.lang = language
}

i18n.on('languageChanged', syncDocumentLanguage)
syncDocumentLanguage(i18n.resolvedLanguage ?? 'en-US')

export default i18n
