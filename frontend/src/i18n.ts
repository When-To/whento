/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { createI18n } from 'vue-i18n';
import fr from './locales/fr.json';
import en from './locales/en.json';

/**
 * Supported locales - single source of truth
 *
 * To add a new language (frontend side):
 * 1. Import the locale file (e.g., import de from './locales/de.json')
 * 2. Add it to LOCALE_MESSAGES (e.g., { en, fr, de })
 * 3. Add its endonym to LOCALE_NAMES (e.g., de: 'Deutsch')
 * 4. Add its first day of the week to LOCALE_WEEK_START (e.g., de: 1)
 *
 * The new file must carry exactly the same key paths as `en.json`; `locales.test.ts`
 * asserts that parity across every entry of LOCALE_MESSAGES, so a partial translation
 * fails the suite rather than silently falling back to English at runtime.
 *
 * The backend renders e-mails from its own message catalogues, which this file does not
 * reach. A new language is only half-added until it also exists under:
 * - `internal/auth/handlers/templates/locales/`
 * - `internal/auth/service/templates/locales/`
 * - `internal/notify/**\/templates/locales/`
 */
const LOCALE_MESSAGES = { en, fr } as const;

export type SupportedLocale = keyof typeof LOCALE_MESSAGES;
export const SUPPORTED_LOCALES = Object.keys(LOCALE_MESSAGES) as SupportedLocale[];
export const DEFAULT_LOCALE: SupportedLocale = 'en';

/**
 * Language names, each written in its own language.
 *
 * Endonyms are deliberately *not* translation keys: a language picker shows "Français"
 * to an English reader, because someone who cannot read the current UI language still has
 * to be able to find their own. Translating them would produce "French" for an English
 * user — exactly the entry that reader cannot act on. Keeping them here rather than in the
 * template means a new language adds one line instead of an `<option>` per view.
 */
export const LOCALE_NAMES: Record<SupportedLocale, string> = {
  en: 'English',
  fr: 'Français',
};

/**
 * Checks if a string is a supported locale
 */
export function isSupportedLocale(locale: string): locale is SupportedLocale {
  return locale in LOCALE_MESSAGES;
}

/**
 * First day of the week per locale (0 = Sunday, 1 = Monday)
 * To add a new language: add its entry here (e.g., de: 1)
 */
const LOCALE_WEEK_START: Partial<Record<SupportedLocale, number>> = {
  fr: 1,
  en: 0,
};

/**
 * Returns the first day of the week for a given locale (0 = Sunday, 1 = Monday)
 * Defaults to Sunday (0) for unknown locales
 */
export function getWeekStartDay(locale: string): number {
  return LOCALE_WEEK_START[locale as SupportedLocale] ?? 0;
}

/**
 * Determines the initial locale based on (in order of priority):
 * 1. URL parameter ?lang=xx
 * 2. Browser language preference
 * Falls back to DEFAULT_LOCALE if no match
 */
function getInitialLocale(): SupportedLocale {
  // Check URL parameter first (for SEO hreflang support)
  const urlParams = new URLSearchParams(window.location.search);
  const langParam = urlParams.get('lang');
  if (langParam && isSupportedLocale(langParam)) {
    return langParam;
  }

  // Check browser language preference
  const browserLang = navigator.language.split('-')[0];
  if (isSupportedLocale(browserLang)) {
    return browserLang;
  }

  return DEFAULT_LOCALE;
}

export const i18n = createI18n({
  legacy: false,
  locale: getInitialLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: LOCALE_MESSAGES,
});
