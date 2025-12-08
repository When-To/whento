<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="flex min-h-[calc(100vh-4rem)] flex-col">
    <!-- Hero Section -->
    <section class="bg-gradient-to-br from-secondary-500 to-secondary-600 py-16 text-white">
      <div class="container-app">
        <div class="mx-auto max-w-3xl text-center">
          <h1 class="font-display text-4xl font-bold leading-tight md:text-5xl">
            {{ t('legal.terms.title') }}
          </h1>
          <p class="mt-4 text-secondary-100">
            {{ t('legal.terms.lastUpdated', { date: lastUpdated }) }}
          </p>
        </div>
      </div>
    </section>

    <!-- Content -->
    <section class="py-12">
      <div class="container-app">
        <div class="mx-auto max-w-3xl">
          <!-- Introduction -->
          <p class="mb-6 text-lg text-gray-600 dark:text-gray-400">
            {{ t('legal.terms.intro') }}
          </p>

          <!-- Acceptance -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.acceptance.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.acceptance.description') }}
          </p>

          <!-- Description -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.description.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.description.description') }}
          </p>

          <!-- Account -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.account.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.account.description') }}
          </p>
          <ul class="mb-4 list-disc space-y-2 pl-6">
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.account.accurate') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.account.secure') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.account.responsible') }}
            </li>
          </ul>

          <!-- Subscriptions -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.subscriptions.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.subscriptions.description') }}
          </p>
          <ul class="mb-4 list-disc space-y-2 pl-6">
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.subscriptions.billing') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.subscriptions.renewal') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.subscriptions.cancellation') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.subscriptions.refunds') }}
            </li>
          </ul>

          <!-- Acceptable Use -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.acceptableUse.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.acceptableUse.description') }}
          </p>
          <ul class="mb-4 list-disc space-y-2 pl-6">
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.acceptableUse.illegal') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.acceptableUse.harmful') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.acceptableUse.spam') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.acceptableUse.interference') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.terms.acceptableUse.impersonation') }}
            </li>
          </ul>

          <!-- Content -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.content.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.content.description') }}
          </p>

          <!-- Intellectual Property -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.intellectualProperty.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.intellectualProperty.description') }}
          </p>

          <!-- Disclaimer -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.disclaimer.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.disclaimer.description') }}
          </p>

          <!-- Limitation -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.limitation.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.limitation.description') }}
          </p>

          <!-- Termination -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.termination.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.termination.description') }}
          </p>

          <!-- Changes -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.changes.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.changes.description') }}
          </p>

          <!-- Governing Law -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.governingLaw.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.governingLaw.description') }}
          </p>

          <!-- Contact -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.terms.contact.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.terms.contact.description') }}
          </p>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const lastUpdated = '2025-12-04'
</script>
