<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="flex min-h-[calc(100vh-4rem)] flex-col">
    <!-- Hero Section -->
    <section
      class="relative overflow-hidden bg-gradient-to-br from-primary-500 via-primary-600 to-secondary-600 py-20 text-white lg:py-32"
    >
      <div class="container-app relative z-10">
        <div class="mx-auto max-w-6xl text-center">
          <h1
            class="animate-fade-in font-display text-5xl font-bold leading-tight md:text-6xl lg:text-7xl"
          >
            {{ t('home.heroTitle') }}<br>
            <span class="text-primary-200">{{ t('home.heroSubtitle') }}</span>
          </h1>
          <p class="mt-6 animate-slide-up text-lg text-primary-100 md:text-xl">
            {{ t('common.tagline') }}
          </p>
          <div class="mt-10 flex flex-col justify-center gap-4 sm:flex-row">
            <router-link
              to="/register"
              class="btn bg-white text-primary-600 hover:bg-gray-100 focus:ring-white"
            >
              {{ t('auth.register') }}
            </router-link>
            <router-link
              to="/login"
              class="btn border-2 border-white bg-transparent text-white hover:bg-white/10"
            >
              {{ t('auth.login') }}
            </router-link>
          </div>
        </div>
      </div>

      <!-- Decorative elements -->
      <div class="absolute inset-0 overflow-hidden opacity-10">
        <div class="absolute -left-20 -top-20 h-96 w-96 rounded-full bg-white blur-3xl" />
        <div class="absolute -bottom-20 -right-20 h-96 w-96 rounded-full bg-white blur-3xl" />
      </div>
    </section>

    <!-- Features Section -->
    <section class="py-20">
      <div class="container-app">
        <h2
          class="mb-12 text-center font-display text-3xl font-bold text-gray-900 dark:text-white md:text-4xl"
        >
          {{ t('common.features') }}
        </h2>
        <div class="grid gap-8 md:grid-cols-2 lg:grid-cols-3">
          <!-- Feature 1 -->
          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-400"
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
                  d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-xl font-semibold">
              {{ t('home.feature1Title') }}
            </h3>
            <p class="text-gray-600 dark:text-gray-400">
              {{ t('home.feature1Description') }}
            </p>
          </div>

          <!-- Feature 2 -->
          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-success-100 text-success-600 dark:bg-success-900 dark:text-success-400"
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
                  d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-xl font-semibold">
              {{ t('home.feature2Title') }}
            </h3>
            <p class="text-gray-600 dark:text-gray-400">
              {{ t('home.feature2Description') }}
            </p>
          </div>

          <!-- Feature 3 -->
          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-secondary-100 text-secondary-600 dark:bg-secondary-900 dark:text-secondary-400"
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
                  d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-xl font-semibold">
              {{ t('home.feature3Title') }}
            </h3>
            <p class="text-gray-600 dark:text-gray-400">
              {{ t('home.feature3Description') }}
            </p>
          </div>

          <!-- Feature 4 -->
          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-danger-100 text-danger-600 dark:bg-danger-900 dark:text-danger-400"
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
                  d="M3 15a4 4 0 004 4h9a5 5 0 10-.1-9.999 5.002 5.002 0 10-9.78 2.096A4.001 4.001 0 003 15z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-xl font-semibold">
              {{ t('home.feature4Title') }}
            </h3>
            <p class="text-gray-600 dark:text-gray-400">
              {{ t('home.feature4Description') }}
            </p>
          </div>

          <!-- Feature 5 -->
          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-400"
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
                  d="M12 8v13m0-13V6a2 2 0 112 2h-2zm0 0V5.5A2.5 2.5 0 109.5 8H12zm-7 4h14M5 12a2 2 0 110-4h14a2 2 0 110 4M5 12v7a2 2 0 002 2h10a2 2 0 002-2v-7"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-xl font-semibold">
              {{ t('home.feature5Title') }}
            </h3>
            <p class="text-gray-600 dark:text-gray-400">
              {{ t('home.feature5Description') }}
            </p>
          </div>

          <!-- Feature 6 -->
          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-success-100 text-success-600 dark:bg-success-900 dark:text-success-400"
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
                  d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-xl font-semibold">
              {{ t('home.feature6Title') }}
            </h3>
            <p class="text-gray-600 dark:text-gray-400">
              {{ t('home.feature6Description') }}
            </p>
          </div>
        </div>
      </div>
    </section>

    <!-- Not Convinced Section -->
    <section class="bg-gray-100 py-16 dark:bg-gray-900">
      <div class="container-app">
        <div class="mx-auto max-w-2xl text-center">
          <div
            class="card border-2 border-dashed border-gray-300 bg-transparent dark:border-gray-700"
          >
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-secondary-100 text-secondary-600 dark:bg-secondary-900 dark:text-secondary-400"
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
                  d="M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('home.notConvincedTitle') }}
            </h3>
            <p class="mb-4 text-gray-600 dark:text-gray-400">
              {{ t('home.notConvincedDescription') }}
            </p>
            <!-- Cloud mode: internal link -->
            <router-link
              v-if="isCloud"
              to="/why-whento"
              class="inline-flex items-center font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
            >
              {{ t('home.notConvincedLink') }}
              <svg
                class="ml-1 h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 5l7 7-7 7"
                />
              </svg>
            </router-link>
            <!-- Self-hosted mode: external link -->
            <a
              v-else
              :href="`${PUBLIC_APP_URL}/why-whento`"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
            >
              {{ t('home.notConvincedLink') }}
              <svg
                class="ml-1 h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 5l7 7-7 7"
                />
              </svg>
            </a>
          </div>
        </div>
      </div>
    </section>

    <!-- CTA Section -->
    <section
      class="py-20"
      :class="isCloud ? '' : 'bg-gray-100 dark:bg-gray-900'"
    >
      <div class="container-app text-center">
        <h2 class="mb-4 font-display text-3xl font-bold text-gray-900 dark:text-white md:text-4xl">
          {{ t('home.ctaTitle') }}
        </h2>
        <p class="mb-8 text-lg text-gray-600 dark:text-gray-400">
          {{ t('home.ctaDescription') }}
        </p>
        <router-link
          to="/register"
          class="btn btn-primary btn-lg"
        >
          {{ t('home.ctaButton') }}
        </router-link>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useBuildType } from '@/composables/useBuildType'
import { PUBLIC_APP_URL } from '@/config/constants'

const { t } = useI18n()
const { isCloud } = useBuildType()
</script>
