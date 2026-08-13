/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { createApp } from 'vue';
import { createPinia } from 'pinia';
import App from './App.vue';
import router from './router';
import { i18n } from './i18n';
import { useAuthStore } from './stores/auth';
import { useToastStore } from './stores/toast';
import { reportFatalError } from './composables/useAppError';
import './style.css';
import './styles/calendar.css';

const app = createApp(App);
const pinia = createPinia();

app.use(pinia);
app.use(router);
app.use(i18n);

/**
 * Last-resort handler for anything the `onErrorCaptured` boundary in App.vue does
 * not see: errors thrown from watchers and lifecycle hooks that run outside a
 * rendering parent, from the app root itself, and from custom directives.
 *
 * It shows the same fallback screen, so no failure path ends in a white page.
 */
app.config.errorHandler = (error, _instance, info) => {
  reportFatalError(error, `vue:${info}`);
};

/**
 * Rejected promises nobody awaited — a store action whose caller forgot a `catch`,
 * a fire-and-forget refresh. These are usually recoverable, so they get a toast
 * rather than the full fallback screen: blanking a working page because a background
 * fetch failed would be a worse outcome than the failure itself.
 *
 * `preventDefault` is deliberately not called, so the browser still logs the
 * original stack for developers.
 */
window.addEventListener('unhandledrejection', event => {
  const toastStore = useToastStore();
  toastStore.error(i18n.global.t('errors.unexpected'));
  // eslint-disable-next-line no-console
  console.error('[whento] unhandled promise rejection', event.reason);
});

// Kick off session restore without blocking the first paint. Anything that needs a
// settled auth state awaits `authStore.whenReady()` — see the router guard.
const authStore = useAuthStore();
authStore.initializeAuth();

app.mount('#app');
