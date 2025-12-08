<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="flex min-h-[calc(100vh-4rem)] flex-col">
    <!-- Hero Section -->
    <section class="bg-gradient-to-br from-primary-500 to-primary-600 py-16 text-white">
      <div class="container-app">
        <div class="mx-auto max-w-3xl text-center">
          <h1 class="font-display text-4xl font-bold leading-tight md:text-5xl">
            {{ t('legal.privacy.title') }}
          </h1>
          <p class="mt-4 text-primary-100">
            {{ t('legal.privacy.lastUpdated', { date: lastUpdated }) }}
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
            {{ t('legal.privacy.intro') }}
          </p>

          <!-- Data Collection -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.dataCollection.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.dataCollection.description') }}
          </p>
          <ul class="mb-4 list-disc space-y-2 pl-6">
            <li class="text-gray-700 dark:text-gray-300">
              <strong>{{ t('legal.privacy.dataCollection.accountData.title') }}</strong>
              {{ t('legal.privacy.dataCollection.accountData.description') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              <strong>{{ t('legal.privacy.dataCollection.calendarData.title') }}</strong>
              {{ t('legal.privacy.dataCollection.calendarData.description') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              <strong>{{ t('legal.privacy.dataCollection.usageData.title') }}</strong>
              {{ t('legal.privacy.dataCollection.usageData.description') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              <strong>{{ t('legal.privacy.dataCollection.paymentData.title') }}</strong>
              {{ t('legal.privacy.dataCollection.paymentData.description') }}
            </li>
          </ul>

          <!-- Data Usage -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.dataUsage.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.dataUsage.description') }}
          </p>
          <ul class="mb-4 list-disc space-y-2 pl-6">
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.dataUsage.purposes.service') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.dataUsage.purposes.communication') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.dataUsage.purposes.improvement') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.dataUsage.purposes.security') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.dataUsage.purposes.legal') }}
            </li>
          </ul>

          <!-- Data Storage -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.dataStorage.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.dataStorage.description') }}
          </p>

          <!-- Third Party -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.thirdParty.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.thirdParty.description') }}
          </p>
          <ul class="mb-4 list-disc space-y-2 pl-6">
            <li class="text-gray-700 dark:text-gray-300">
              <strong>Stripe</strong> - {{ t('legal.privacy.thirdParty.stripe') }}
            </li>
          </ul>

          <!-- Cookies -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.cookies.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.cookies.description') }}
          </p>
          <ul class="mb-4 list-disc space-y-2 pl-6">
            <li class="text-gray-700 dark:text-gray-300">
              <strong>{{ t('legal.privacy.cookies.essential.title') }}</strong>
              {{ t('legal.privacy.cookies.essential.description') }}
            </li>
          </ul>

          <!-- User Rights -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.userRights.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.userRights.description') }}
          </p>
          <ul class="mb-4 list-disc space-y-2 pl-6">
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.userRights.access') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.userRights.rectification') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.userRights.erasure') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.userRights.portability') }}
            </li>
            <li class="text-gray-700 dark:text-gray-300">
              {{ t('legal.privacy.userRights.objection') }}
            </li>
          </ul>

          <!-- Data Retention -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.dataRetention.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.dataRetention.description') }}
          </p>

          <!-- Children -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.children.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.children.description') }}
          </p>

          <!-- Changes -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.changes.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.changes.description') }}
          </p>

          <!-- Contact -->
          <h2 class="mt-8 mb-4 font-display text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('legal.privacy.contact.title') }}
          </h2>
          <p class="mb-4 text-gray-700 dark:text-gray-300">
            {{ t('legal.privacy.contact.description') }}
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
