<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="flex min-h-[calc(100vh-4rem)] items-center justify-center">
    <div class="text-center">
      <h1 class="font-display text-9xl font-bold text-primary-600">
        404
      </h1>
      <h2 class="mt-4 font-display text-2xl font-semibold text-gray-900 dark:text-white">
        {{ t('errors.notFound', 'Page not found') }}
      </h2>
      <p class="mt-2 text-gray-600 dark:text-gray-400">
        {{ t('common.notFoundMessage', "Sorry, we couldn't find the page you're looking for.") }}
      </p>
      <router-link
        to="/"
        class="btn btn-primary mt-6"
      >
        {{ t('nav.home', 'Go back home') }}
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>
