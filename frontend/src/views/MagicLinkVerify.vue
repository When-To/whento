<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <div class="flex min-h-[calc(100vh-4rem)] items-center justify-center py-12">
    <div class="w-full max-w-md animate-slide-up">
      <div class="card text-center">
        <!-- Loading State -->
        <div v-if="loading">
          <div
            class="mx-auto mb-4 h-16 w-16 animate-spin rounded-full border-4 border-primary-200 border-t-primary-600 dark:border-primary-800 dark:border-t-primary-400"
          />
          <p class="text-gray-600 dark:text-gray-400">
            {{ t('auth.magicLink.verifying') }}
          </p>
        </div>

        <!-- Error State -->
        <div
          v-else-if="error"
          class="text-center"
        >
          <div
            class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-danger-100 dark:bg-danger-900/20"
          >
            <svg
              class="h-8 w-8 text-danger-600"
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
          <h2 class="mb-2 text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('auth.magicLink.invalidTitle') }}
          </h2>
          <p class="mb-6 text-gray-600 dark:text-gray-400">
            {{ error }}
          </p>
          <router-link
            to="/login"
            class="btn btn-primary"
          >
            {{ t('auth.backToLogin') }}
          </router-link>
        </div>

        <!-- Success State (should auto-redirect) -->
        <div v-else>
          <div
            class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-success-100 dark:bg-success-900/20"
          >
            <svg
              class="h-8 w-8 text-success-600"
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
          <p class="text-gray-600 dark:text-gray-400">
            {{ t('auth.magicLink.success') }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { authApi } from '@/api/auth'
import { apiClient } from '@/api/client'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const authStore = useAuthStore()

const loading = ref(true)
const error = ref('')

onMounted(async () => {
  const token = route.params.token as string

  if (!token) {
    error.value = t('auth.magicLink.missingToken')
    loading.value = false
    return
  }

  try {
    // Verify magic link
    const response = await authApi.verifyMagicLink(token)

    // Set auth tokens in store
    authStore.user = response.user
    apiClient.setToken(response.access_token)

    // Redirect to dashboard
    await router.push('/dashboard')
  } catch (err: any) {
    loading.value = false

    // Translate error messages
    if (err.message?.includes('expired')) {
      error.value = t('auth.magicLink.expired')
    } else if (err.message?.includes('invalid')) {
      error.value = t('auth.magicLink.invalid')
    } else {
      error.value = t('auth.magicLink.verifyError')
    }
  }
})
</script>
