<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900 py-12 px-4 sm:px-6 lg:px-8">
    <div class="max-w-md w-full space-y-8">
      <div>
        <h2 class="mt-6 text-center text-3xl font-extrabold text-gray-900 dark:text-white">
          {{ $t('auth.emailVerification') }}
        </h2>
      </div>

      <!-- Loading state -->
      <div
        v-if="loading"
        class="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6 text-center"
      >
        <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 dark:border-indigo-400 mx-auto" />
        <p class="mt-4 text-gray-600 dark:text-gray-400">
          {{ $t('auth.verifyingEmail') }}
        </p>
      </div>

      <!-- Success state -->
      <div
        v-else-if="success"
        class="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6"
      >
        <div class="flex items-center justify-center w-12 h-12 mx-auto bg-green-100 dark:bg-green-900/20 rounded-full">
          <svg
            class="w-6 h-6 text-green-600 dark:text-green-400"
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
        <h3 class="mt-4 text-lg font-medium text-center text-gray-900 dark:text-white">
          {{ $t('auth.emailVerifiedSuccess') }}
        </h3>
        <p class="mt-2 text-sm text-center text-gray-600 dark:text-gray-400">
          {{ $t('auth.emailVerifiedDescription') }}
        </p>
        <div class="mt-6">
          <router-link
            to="/dashboard"
            class="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 dark:focus:ring-offset-gray-800"
          >
            {{ $t('auth.goToDashboard') }}
          </router-link>
        </div>
      </div>

      <!-- Error state -->
      <div
        v-else-if="error"
        class="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6"
      >
        <div class="flex items-center justify-center w-12 h-12 mx-auto bg-red-100 dark:bg-red-900/20 rounded-full">
          <svg
            class="w-6 h-6 text-red-600 dark:text-red-400"
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
        <h3 class="mt-4 text-lg font-medium text-center text-gray-900 dark:text-white">
          {{ $t('auth.emailVerificationFailed') }}
        </h3>
        <p class="mt-2 text-sm text-center text-gray-600 dark:text-gray-400">
          {{ errorMessage }}
        </p>
        <div class="mt-6">
          <router-link
            to="/login"
            class="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 dark:focus:ring-offset-gray-800"
          >
            {{ $t('auth.backToLogin') }}
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import axios from 'axios'

const route = useRoute()
const { t } = useI18n()
const authStore = useAuthStore()

const loading = ref(true)
const success = ref(false)
const error = ref(false)
const errorMessage = ref('')

const verifyEmail = async (token: string) => {
  try {
    loading.value = true
    error.value = false

    const response = await axios.get(`/api/v1/auth/verify-email/${token}`)

    if (response.data.success) {
      success.value = true

      // Refresh user data if authenticated to update email_verified status
      if (authStore.isAuthenticated) {
        try {
          await authStore.fetchUser()
        } catch (err) {
          // Ignore error - user data will be refreshed on next page load
          console.error('Failed to refresh user data:', err)
        }
      }
    } else {
      throw new Error(response.data.error?.message || 'Verification failed')
    }
  } catch (err: any) {
    error.value = true

    if (err.response?.data?.error?.message) {
      errorMessage.value = err.response.data.error.message
    } else if (err.response?.status === 400) {
      errorMessage.value = t('auth.invalidOrExpiredToken')
    } else {
      errorMessage.value = t('auth.verificationError')
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  const token = route.params.token as string

  if (!token) {
    error.value = true
    errorMessage.value = t('auth.missingVerificationToken')
    loading.value = false
    return
  }

  verifyEmail(token)
})
</script>
