/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Development-only preview harness for the calendar components.
 *
 * The participant calendar is behind a database and a public token, which makes the
 * redesign impossible to look at without a full stack. This entry renders the views
 * against fabricated data instead, so layouts, the density ramp and both themes can be
 * inspected — and screenshotted by Playwright — with nothing running behind them.
 *
 * Served by `vite dev` at /dev/preview.html. It is not part of the production build:
 * Vite's default rollup input is index.html only.
 */

import { createApp } from 'vue';
import { createI18n } from 'vue-i18n';
import en from '../src/locales/en.json';
import fr from '../src/locales/fr.json';
import '../src/style.css';
// Loaded explicitly, as `main.ts` does: `styles/calendar.css` is imported from JS
// rather than `@import`ed from `style.css`, because Vite does not invalidate a CSS
// `@import` on change. Every entry that renders calendar components needs both.
import '../src/styles/calendar.css';
import CalendarPreview from './CalendarPreview.vue';

const i18n = createI18n({
  legacy: false,
  locale: new URLSearchParams(window.location.search).get('lang') || 'en',
  fallbackLocale: 'en',
  messages: { en, fr },
});

createApp(CalendarPreview).use(i18n).mount('#app');
