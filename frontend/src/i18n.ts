/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { createI18n } from 'vue-i18n'
import fr from './locales/fr.json'
import en from './locales/en.json'

/**
 * Supported locales - single source of truth
 * To add a new language:
 * 1. Import the locale file (e.g., import de from './locales/de.json')
 * 2. Add it to LOCALE_MESSAGES (e.g., { en, fr, de })
 */
const LOCALE_MESSAGES = { en, fr } as const

export type SupportedLocale = keyof typeof LOCALE_MESSAGES
export const SUPPORTED_LOCALES = Object.keys(LOCALE_MESSAGES) as SupportedLocale[]
export const DEFAULT_LOCALE: SupportedLocale = 'en'

/**
 * Checks if a string is a supported locale
 */
export function isSupportedLocale(locale: string): locale is SupportedLocale {
  return locale in LOCALE_MESSAGES
}

/**
 * Determines the initial locale based on (in order of priority):
 * 1. URL parameter ?lang=xx
 * 2. Browser language preference
 * Falls back to DEFAULT_LOCALE if no match
 */
function getInitialLocale(): SupportedLocale {
  // Check URL parameter first (for SEO hreflang support)
  const urlParams = new URLSearchParams(window.location.search)
  const langParam = urlParams.get('lang')
  if (langParam && isSupportedLocale(langParam)) {
    return langParam
  }

  // Check browser language preference
  const browserLang = navigator.language.split('-')[0]
  if (isSupportedLocale(browserLang)) {
    return browserLang
  }

  return DEFAULT_LOCALE
}

export const i18n = createI18n({
  legacy: false,
  locale: getInitialLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: LOCALE_MESSAGES,
})
