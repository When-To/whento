<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <!--
    Error boundary fallback. Rendered instead of the app — not inside it — because
    what threw may well be the navigation or the layout itself.
  -->
  <div
    v-if="failed"
    class="flex min-h-screen items-center justify-center bg-gray-50 p-4 dark:bg-gray-950"
  >
    <div
      class="w-full max-w-md rounded-lg border border-gray-200 bg-white p-8 text-center shadow-lg dark:border-gray-800 dark:bg-gray-900"
      role="alert"
    >
      <img src="/logo.png" :alt="t('common.logoAlt')" class="mx-auto mb-4 h-12 w-12" />
      <h1 class="mb-2 font-display text-xl font-bold text-gray-900 dark:text-white">
        {{ t('errors.boundary.title') }}
      </h1>
      <p class="mb-6 text-sm text-gray-600 dark:text-gray-400">
        {{ t('errors.boundary.message') }}
      </p>
      <div class="flex flex-col gap-2 sm:flex-row sm:justify-center">
        <button class="btn btn-primary" @click="reloadApp">
          {{ t('errors.boundary.reload') }}
        </button>
        <button class="btn btn-ghost" @click="goHome">
          {{ t('errors.boundary.home') }}
        </button>
      </div>
      <p class="mt-6 font-mono text-xs text-gray-400 dark:text-gray-600">
        {{ t('errors.boundary.reference', { reference }) }}
      </p>
    </div>
  </div>

  <!-- Loading Screen during auth initialization -->
  <div
    v-else-if="!authStore.initialized"
    class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-950"
  >
    <div class="text-center">
      <img src="/logo.png" :alt="t('common.logoAlt')" class="mb-4 h-16 w-16 mx-auto" />
      <svg class="h-8 w-8 animate-spin text-primary-600 mx-auto" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
        <path
          class="opacity-75"
          fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
        />
      </svg>
    </div>
  </div>

  <div v-else id="app" class="min-h-screen bg-gray-50 dark:bg-gray-950">
    <!--
      Skip link. First focusable thing in the document, off-screen until it takes focus.
      Without it a keyboard user pays for the whole <nav> — logo, up to three section
      links, theme, language and the user menu — on every single page before reaching
      the content they navigated to.
    -->
    <a href="#main-content" class="skip-link">{{ t('a11y.skipToContent') }}</a>

    <!-- Calendar Sidebar -->
    <CalendarSidebar />

    <!-- Navigation -->
    <nav
      class="sticky top-0 z-50 border-b border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-900"
    >
      <div class="container-app">
        <div class="flex h-16 items-center justify-between">
          <!-- Logo -->
          <router-link to="/" class="flex items-center space-x-2">
            <img src="/logo.png" :alt="t('common.logoAlt')" class="h-8 w-8" />
            <span class="font-display text-xl font-bold text-gray-900 dark:text-white">WhenTo</span>
          </router-link>

          <!-- Public Navigation Links (not authenticated) - Cloud Mode -->
          <div
            v-if="!isAuthenticated && isCloud"
            class="hidden md:flex md:items-center md:space-x-4"
          >
            <router-link
              to="/"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'home' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.home') }}
            </router-link>
            <router-link
              to="/why-whento"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'why-whento' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.whyWhento') }}
            </router-link>
          </div>

          <!-- Public Navigation Links (not authenticated) - Self-hosted Mode -->
          <div
            v-if="!isAuthenticated && isSelfHosted"
            class="hidden md:flex md:items-center md:space-x-4"
          >
            <router-link
              to="/"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'home' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.home') }}
            </router-link>
            <a
              :href="`${PUBLIC_APP_URL}/why-whento`"
              target="_blank"
              rel="noopener noreferrer"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
            >
              {{ t('nav.whyWhento') }}
            </a>
          </div>

          <!-- Authenticated Navigation Links -->
          <div v-if="isAuthenticated" class="hidden md:flex md:items-center md:space-x-4">
            <router-link
              to="/dashboard"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'dashboard' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.dashboard') }}
            </router-link>

            <router-link
              to="/settings"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'settings' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.settings') }}
            </router-link>

            <router-link
              v-if="isAdmin"
              to="/admin"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'admin' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.admin') }}
            </router-link>
          </div>

          <!-- User Menu (Desktop) -->
          <div class="hidden md:flex md:items-center md:space-x-4">
            <!-- Theme Toggle -->
            <button
              class="rounded-lg p-2 text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
              :aria-label="t('common.toggleTheme')"
              @click="toggleTheme"
            >
              <svg
                v-if="theme === 'light'"
                class="h-5 w-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"
                />
              </svg>
              <svg v-else class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"
                />
              </svg>
            </button>

            <!-- Language Toggle -->
            <!-- Named explicitly: "EN" alone is a two-letter accessible name that says
                 nothing about what pressing the button does. -->
            <button
              type="button"
              :aria-label="t('common.toggleLanguage')"
              class="rounded-lg px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
              @click="toggleLocale"
            >
              {{ locale.toUpperCase() }}
            </button>

            <!-- Auth Buttons -->
            <div v-if="!isAuthenticated" class="flex items-center space-x-2">
              <router-link to="/login" class="btn btn-ghost">
                {{ t('auth.login') }}
              </router-link>
              <router-link to="/register" class="btn btn-primary">
                {{ t('auth.register') }}
              </router-link>
            </div>

            <!-- User Menu -->
            <div v-else class="flex items-center space-x-2">
              <router-link
                to="/settings"
                class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                {{ user?.display_name }}
              </router-link>
              <button class="btn btn-ghost" @click="handleLogout">
                {{ t('auth.logout') }}
              </button>
            </div>
          </div>

          <!-- Mobile: Calendar History + Hamburger Menu -->
          <div class="flex md:hidden items-center space-x-2">
            <!-- Calendar History Button (mobile) -->
            <button
              v-if="hasCalendarHistory"
              class="relative rounded-lg p-2 text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
              :title="t('calendar.showCalendars')"
              @click="toggleCalendarHistory"
            >
              <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                />
              </svg>
              <span
                v-if="calendarHistoryCount > 0"
                class="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full bg-primary-600 text-xs font-bold text-white"
              >
                {{ calendarHistoryCount }}
              </span>
            </button>

            <!-- Hamburger Menu Button -->
            <button
              class="rounded-lg p-2 text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
              :aria-label="t('nav.toggleMenu')"
              @click="toggleMobileMenu"
            >
              <svg
                v-if="!mobileMenuOpen"
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 6h16M4 12h16M4 18h16"
                />
              </svg>
              <svg v-else class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            </button>
          </div>
        </div>

        <!-- Mobile Menu Dropdown (outside flex container) -->
        <div
          v-if="mobileMenuOpen"
          class="md:hidden border-t border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900 py-4"
        >
          <!-- Navigation Links (not authenticated) - Cloud Mode -->
          <div v-if="!isAuthenticated && isCloud" class="space-y-1 pb-3">
            <router-link
              to="/"
              class="block rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'home' ? 'bg-gray-100 dark:bg-gray-800' : ''"
              @click="closeMobileMenu"
            >
              {{ t('nav.home') }}
            </router-link>
            <router-link
              to="/why-whento"
              class="block rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'why-whento' ? 'bg-gray-100 dark:bg-gray-800' : ''"
              @click="closeMobileMenu"
            >
              {{ t('nav.whyWhento') }}
            </router-link>
          </div>

          <!-- Navigation Links (not authenticated) - Self-hosted Mode -->
          <div v-if="!isAuthenticated && isSelfHosted" class="space-y-1 pb-3">
            <router-link
              to="/"
              class="block rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'home' ? 'bg-gray-100 dark:bg-gray-800' : ''"
              @click="closeMobileMenu"
            >
              {{ t('nav.home') }}
            </router-link>
            <a
              :href="`${PUBLIC_APP_URL}/why-whento`"
              target="_blank"
              rel="noopener noreferrer"
              class="block rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              @click="closeMobileMenu"
            >
              {{ t('nav.whyWhento') }}
            </a>
          </div>

          <!-- Authenticated Navigation Links -->
          <div v-if="isAuthenticated" class="space-y-1 pb-3">
            <router-link
              to="/dashboard"
              class="block rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'dashboard' ? 'bg-gray-100 dark:bg-gray-800' : ''"
              @click="closeMobileMenu"
            >
              {{ t('nav.dashboard') }}
            </router-link>
            <router-link
              to="/settings"
              class="block rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'settings' ? 'bg-gray-100 dark:bg-gray-800' : ''"
              @click="closeMobileMenu"
            >
              {{ t('nav.settings') }}
            </router-link>
            <router-link
              v-if="isAdmin"
              to="/admin"
              class="block rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'admin' ? 'bg-gray-100 dark:bg-gray-800' : ''"
              @click="closeMobileMenu"
            >
              {{ t('nav.admin') }}
            </router-link>
          </div>

          <!-- Settings & Actions -->
          <div class="border-t border-gray-200 dark:border-gray-700 pt-3 space-y-1">
            <!-- Theme & Language Row -->
            <div class="flex items-center justify-between px-3 py-2">
              <span class="text-sm text-gray-600 dark:text-gray-400">{{
                t('settings.theme')
              }}</span>
              <button
                class="rounded-lg p-2 text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
                :aria-label="t('common.toggleTheme')"
                @click="toggleTheme"
              >
                <svg
                  v-if="theme === 'light'"
                  class="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"
                  />
                </svg>
                <svg v-else class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"
                  />
                </svg>
              </button>
            </div>

            <div class="flex items-center justify-between px-3 py-2">
              <span class="text-sm text-gray-600 dark:text-gray-400">{{
                t('settings.language')
              }}</span>
              <button
                type="button"
                :aria-label="t('common.toggleLanguage')"
                class="rounded-lg px-3 py-1 text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
                @click="toggleLocale"
              >
                {{ locale.toUpperCase() }}
              </button>
            </div>
          </div>

          <!-- Auth Actions -->
          <div class="border-t border-gray-200 dark:border-gray-700 pt-3 mt-3">
            <div v-if="!isAuthenticated" class="space-y-2 px-3">
              <router-link
                to="/login"
                class="block w-full btn btn-ghost text-center"
                @click="closeMobileMenu"
              >
                {{ t('auth.login') }}
              </router-link>
              <router-link
                to="/register"
                class="block w-full btn btn-primary text-center"
                @click="closeMobileMenu"
              >
                {{ t('auth.register') }}
              </router-link>
            </div>

            <div v-else class="space-y-1">
              <div class="px-3 py-2 text-sm font-medium text-gray-900 dark:text-white">
                {{ user?.display_name }}
              </div>
              <button
                class="block w-full text-left px-3 py-2 text-sm text-danger-600 hover:bg-gray-100 dark:text-danger-400 dark:hover:bg-gray-800 rounded-lg"
                @click="
                  handleLogout();
                  closeMobileMenu();
                "
              >
                {{ t('auth.logout') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </nav>

    <!-- Main Content -->
    <!-- `tabindex="-1"` so the skip link can actually move focus here, not just scroll. -->
    <main id="main-content" tabindex="-1">
      <router-view v-slot="{ Component }">
        <transition name="page-fade" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </main>

    <!-- Footer -->
    <Footer />

    <!-- Toast Notifications -->
    <ToastContainer />
  </div>

  <!--
    Outside the two branches above on purpose: a confirmation opened from anywhere
    must survive an auth-state flip, and it is the single host for useConfirm().
  -->
  <ConfirmDialog />
</template>

<script setup lang="ts">
import { computed, onErrorCaptured, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { SUPPORTED_LOCALES } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { useCalendarHistoryStore } from '@/stores/calendarHistory';
import { useAppError } from '@/composables/useAppError';
import { useBuildType } from '@/composables/useBuildType';
import { applyTheme, resolveTheme, THEME_STORAGE_KEY, type Theme } from '@/utils/theme';
import { PUBLIC_APP_URL } from '@/config/constants';
import Footer from '@/components/Footer.vue';
import CalendarSidebar from '@/components/CalendarSidebar.vue';
import ConfirmDialog from '@/components/ConfirmDialog.vue';
import ToastContainer from '@/components/ToastContainer.vue';

const route = useRoute();
const router = useRouter();
const { t, locale } = useI18n();
const authStore = useAuthStore();
const historyStore = useCalendarHistoryStore();
const { isCloud, isSelfHosted } = useBuildType();
const { failed, reference, reportFatalError, clearFatalError } = useAppError();

// Resolved at setup, not on mount: the pre-paint snippet in index.html has already
// put the class on <html>, and reading the same source here keeps the toggle's icon
// in step with what is actually on screen from the very first frame.
const theme = ref<Theme>(resolveTheme());
const mobileMenuOpen = ref(false);

/**
 * The error boundary.
 *
 * Returning `false` stops the error propagating to `app.config.errorHandler`, so a
 * render failure is reported exactly once. Everything below this component — which
 * is to say every view — is covered; what escapes (the root, watchers with no
 * rendering parent) is caught by the global handler in main.ts, which routes into
 * the same state and so shows the same screen.
 */
onErrorCaptured((error, _instance, info) => {
  reportFatalError(error, `boundary:${info}`);
  return false;
});

// Navigating away is a recovery: the broken view is gone, so drop the fallback.
watch(
  () => route.fullPath,
  () => clearFatalError()
);

function reloadApp() {
  window.location.reload();
}

function goHome() {
  clearFatalError();
  // A hard navigation if the router itself is what failed.
  router.push('/').catch(() => window.location.assign('/'));
}

const isAuthenticated = computed(() => authStore.isAuthenticated);
const isAdmin = computed(() => authStore.isAdmin);
const user = computed(() => authStore.user);
const hasCalendarHistory = computed(() => historyStore.calendars.length > 0);
const calendarHistoryCount = computed(() => historyStore.calendars.length);

function toggleTheme() {
  theme.value = theme.value === 'light' ? 'dark' : 'light';
  localStorage.setItem(THEME_STORAGE_KEY, theme.value);
  applyTheme(theme.value);
}

function toggleLocale() {
  const idx = SUPPORTED_LOCALES.indexOf(locale.value as (typeof SUPPORTED_LOCALES)[number]);
  locale.value = SUPPORTED_LOCALES[(idx + 1) % SUPPORTED_LOCALES.length];
  localStorage.setItem('locale', locale.value);
}

function toggleMobileMenu() {
  mobileMenuOpen.value = !mobileMenuOpen.value;
}

function closeMobileMenu() {
  mobileMenuOpen.value = false;
}

function toggleCalendarHistory() {
  historyStore.toggle();
}

async function handleLogout() {
  await authStore.logout();
  router.push('/login');
}

onMounted(() => {
  // The class is already on <html> from the pre-paint snippet; this only covers the
  // case where that snippet could not run (storage disabled, CSP).
  applyTheme(theme.value);

  // Initialize locale
  const savedLocale = localStorage.getItem('locale');
  if (savedLocale) {
    locale.value = savedLocale;
  }

  // Initialize calendar history
  historyStore.init();

  // Auth initialization is handled in main.ts
});
</script>
