<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <!-- Loading Screen during auth initialization -->
  <div
    v-if="!authStore.initialized"
    class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-950"
  >
    <div class="text-center">
      <img
        src="/logo.png"
        alt="WhenTo"
        class="mb-4 h-16 w-16 mx-auto"
      >
      <svg
        class="h-8 w-8 animate-spin text-primary-600 mx-auto"
        fill="none"
        viewBox="0 0 24 24"
      >
        <circle
          class="opacity-25"
          cx="12"
          cy="12"
          r="10"
          stroke="currentColor"
          stroke-width="4"
        />
        <path
          class="opacity-75"
          fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
        />
      </svg>
    </div>
  </div>

  <div
    v-else
    id="app"
    class="min-h-screen bg-gray-50 dark:bg-gray-950"
  >
    <!-- Calendar Sidebar -->
    <CalendarSidebar />

    <!-- Navigation -->
    <nav
      class="sticky top-0 z-50 border-b border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-900"
    >
      <div class="container-app">
        <div class="flex h-16 items-center justify-between">
          <!-- Logo -->
          <router-link
            to="/"
            class="flex items-center space-x-2"
          >
            <img
              src="/logo.png"
              alt="WhenTo"
              class="h-8 w-8"
            >
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
            <router-link
              to="/pricing"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'pricing' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.pricing') }}
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
            <a
              :href="`${PUBLIC_APP_URL}/pricing`"
              target="_blank"
              rel="noopener noreferrer"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
            >
              {{ t('nav.pricing') }}
            </a>
          </div>

          <!-- Authenticated Navigation Links -->
          <div
            v-if="isAuthenticated"
            class="hidden md:flex md:items-center md:space-x-4"
          >
            <router-link
              to="/dashboard"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'dashboard' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.dashboard') }}
            </router-link>

            <!-- Cloud only: Billing/Subscription link -->
            <router-link
              v-if="isCloud"
              to="/billing"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'billing' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.billing') }}
            </router-link>

            <router-link
              to="/settings"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'settings' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.settings') }}
            </router-link>

            <!-- Self-hosted only: License link (admin only) -->
            <router-link
              v-if="isSelfHosted && isAdmin"
              to="/admin/license"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              :class="route.name === 'admin-license' ? 'bg-gray-100 dark:bg-gray-800' : ''"
            >
              {{ t('nav.license') }}
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

          <!-- User Menu -->
          <div class="flex items-center space-x-4">
            <!-- Cloud only: Shopping Cart (only for non-authenticated users) -->
            <router-link
              v-if="isCloud && !isAuthenticated"
              to="/cart"
              class="relative rounded-lg p-2 text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
              aria-label="Shopping cart"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M3 3h2l.4 2M7 13h10l4-8H5.4M7 13L5.4 5M7 13l-2.293 2.293c-.63.63-.184 1.707.707 1.707H17m0 0a2 2 0 100 4 2 2 0 000-4zm-8 2a2 2 0 11-4 0 2 2 0 014 0z"
                />
              </svg>
              <span
                v-if="cartItemCount > 0"
                class="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full bg-primary-600 text-xs font-bold text-white"
              >
                {{ cartItemCount }}
              </span>
            </router-link>

            <!-- Theme Toggle -->
            <button
              class="rounded-lg p-2 text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
              aria-label="Toggle theme"
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
              <svg
                v-else
                class="h-5 w-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"
                />
              </svg>
            </button>

            <!-- Language Toggle -->
            <button
              class="rounded-lg px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
              @click="toggleLocale"
            >
              {{ locale.toUpperCase() }}
            </button>

            <!-- Auth Buttons -->
            <div
              v-if="!isAuthenticated"
              class="flex items-center space-x-2"
            >
              <router-link
                to="/login"
                class="btn btn-ghost"
              >
                {{ t('auth.login') }}
              </router-link>
              <router-link
                to="/register"
                class="btn btn-primary"
              >
                {{
                  t('auth.register')
                }}
              </router-link>
            </div>

            <!-- User Menu -->
            <div
              v-else
              class="flex items-center space-x-2"
            >
              <router-link
                to="/settings"
                class="rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                {{ user?.display_name }}
              </router-link>
              <button
                class="btn btn-ghost"
                @click="handleLogout"
              >
                {{ t('auth.logout') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </nav>

    <!-- Main Content -->
    <main>
      <router-view v-slot="{ Component }">
        <transition
          name="page-fade"
          mode="out-in"
        >
          <component :is="Component" />
        </transition>
      </router-view>
    </main>

    <!-- Footer -->
    <Footer />

    <!-- Toast Notifications -->
    <ToastContainer />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useCalendarHistoryStore } from '@/stores/calendarHistory'
import { useCartStore } from '@/stores/cart'
import { useBuildType } from '@/composables/useBuildType'
import { PUBLIC_APP_URL } from '@/config/constants'
import Footer from '@/components/Footer.vue'
import CalendarSidebar from '@/components/CalendarSidebar.vue'
import ToastContainer from '@/components/ToastContainer.vue'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const authStore = useAuthStore()
const historyStore = useCalendarHistoryStore()
const cartStore = useCartStore()
const { isCloud, isSelfHosted } = useBuildType()

const theme = ref<'light' | 'dark'>('light')

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const user = computed(() => authStore.user)
const cartItemCount = computed(() => cartStore.itemCount)

function toggleTheme() {
  theme.value = theme.value === 'light' ? 'dark' : 'light'
  localStorage.setItem('theme', theme.value)
  updateTheme()
}

function updateTheme() {
  if (theme.value === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}

function toggleLocale() {
  locale.value = locale.value === 'fr' ? 'en' : 'fr'
  localStorage.setItem('locale', locale.value)
}

async function handleLogout() {
  await authStore.logout()
  router.push('/login')
}

onMounted(() => {
  // Initialize theme
  const savedTheme = localStorage.getItem('theme') as 'light' | 'dark' | null
  if (savedTheme) {
    theme.value = savedTheme
  } else if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
    theme.value = 'dark'
  }
  updateTheme()

  // Initialize locale
  const savedLocale = localStorage.getItem('locale')
  if (savedLocale) {
    locale.value = savedLocale
  }

  // Initialize calendar history
  historyStore.init()

  // Initialize cart (Cloud only, for non-authenticated users - guest checkout)
  if (isCloud.value && !authStore.isAuthenticated) {
    cartStore.initialize().catch(err => {
      console.error('Failed to initialize cart:', err)
    })
  }

  // Auth initialization is handled in main.ts
})
</script>
