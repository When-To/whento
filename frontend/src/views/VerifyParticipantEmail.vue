<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 px-4 py-12 dark:bg-gray-900 sm:px-6 lg:px-8">
    <div class="w-full max-w-md space-y-8">
      <div class="text-center">
        <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
          {{ t('notifications.emailVerification') }}
        </h1>
      </div>

      <div class="card">
        <!-- Loading -->
        <div
          v-if="loading"
          class="flex flex-col items-center justify-center py-12"
        >
          <svg
            class="h-12 w-12 animate-spin text-primary-600"
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
          <p class="mt-4 text-sm text-gray-600 dark:text-gray-400">
            {{ t('notifications.verifying') }}
          </p>
        </div>

        <!-- Success -->
        <div
          v-else-if="success"
          class="text-center"
        >
          <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-green-100 dark:bg-green-900">
            <svg
              class="h-10 w-10 text-green-600 dark:text-green-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M5 13l4 4L19 7"
              />
            </svg>
          </div>
          <h2 class="mt-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('notifications.emailVerified') }}
          </h2>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
            {{ t('notifications.emailVerifiedMessage') }}
          </p>
          <p class="mt-4 text-sm text-gray-600 dark:text-gray-400">
            {{ t('notifications.closeWindow') }}
          </p>
        </div>

        <!-- Error -->
        <div
          v-else-if="error"
          class="text-center"
        >
          <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-red-100 dark:bg-red-900">
            <svg
              class="h-10 w-10 text-red-600 dark:text-red-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </div>
          <h2 class="mt-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('notifications.verificationFailed') }}
          </h2>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
            {{ errorMessage }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { verifyParticipantEmail } from '@/api/notify'

const route = useRoute()
const { t } = useI18n()

const loading = ref(true)
const success = ref(false)
const error = ref(false)
const errorMessage = ref('')

onMounted(async () => {
  const token = route.params.token as string

  if (!token) {
    error.value = true
    errorMessage.value = t('notifications.invalidToken')
    loading.value = false
    return
  }

  try {
    await verifyParticipantEmail(token)
    success.value = true
  } catch (err: any) {
    error.value = true
    if (err.response?.status === 400 || err.response?.status === 404) {
      errorMessage.value = t('notifications.tokenExpired')
    } else {
      errorMessage.value = t('notifications.verificationError')
    }
  } finally {
    loading.value = false
  }
})
</script>
