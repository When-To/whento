<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-6xl">
      <h1 class="mb-8 font-display text-3xl font-bold text-gray-900 dark:text-white">
        {{ t('nav.settings') }}
      </h1>

      <!-- Email Verification Warning -->
      <div
        v-if="!user.email_verified"
        class="mb-6 rounded-lg border-l-4 border-yellow-500 bg-yellow-50 p-4 dark:bg-yellow-900/20"
      >
        <div class="flex items-start">
          <svg
            class="mt-0.5 h-5 w-5 text-yellow-600 dark:text-yellow-400"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
          <div class="ml-3 flex-1">
            <h3 class="text-sm font-medium text-yellow-800 dark:text-yellow-200">
              {{ t('settings.emailNotVerified') }}
            </h3>
            <p class="mt-1 text-sm text-yellow-700 dark:text-yellow-300">
              {{ t('settings.emailNotVerifiedDescription') }}
            </p>
            <div class="mt-3">
              <button
                :disabled="resending"
                class="inline-flex items-center rounded-md bg-yellow-600 px-3 py-2 text-sm font-medium text-white hover:bg-yellow-700 focus:outline-none focus:ring-2 focus:ring-yellow-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed dark:bg-yellow-500 dark:hover:bg-yellow-600"
                @click="resendVerificationEmail"
              >
                <svg
                  v-if="resending"
                  class="mr-2 h-4 w-4 animate-spin"
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
                {{ resending ? t('auth.resending') : t('auth.resendVerificationEmail') }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab Navigation -->
      <div class="mb-6 border-b border-gray-200 dark:border-gray-700">
        <nav class="-mb-px flex space-x-8">
          <button
            :class="[
              'py-4 px-1 border-b-2 font-medium text-sm transition-colors',
              activeTab === 'profile'
                ? 'border-primary-600 text-primary-600 dark:border-primary-400 dark:text-primary-400'
                : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
            ]"
            @click="activeTab = 'profile'"
          >
            {{ t('settings.tabs.profile') }}
          </button>
          <button
            :class="[
              'py-4 px-1 border-b-2 font-medium text-sm transition-colors',
              activeTab === 'security'
                ? 'border-primary-600 text-primary-600 dark:border-primary-400 dark:text-primary-400'
                : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
            ]"
            @click="activeTab = 'security'"
          >
            {{ t('settings.tabs.security') }}
          </button>
        </nav>
      </div>

      <!-- Tab Content -->
      <ProfileTab v-if="activeTab === 'profile'" />
      <SecurityTab v-else-if="activeTab === 'security'" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { apiClient } from '@/api/client'
import ProfileTab from '@/components/settings/ProfileTab.vue'
import SecurityTab from '@/components/settings/SecurityTab.vue'

const { t } = useI18n()
const authStore = useAuthStore()
const toast = useToastStore()

const activeTab = ref('profile')
const resending = ref(false)

const user = computed(
  () =>
    authStore.user || {
      display_name: '',
      email: '',
      timezone: 'Europe/Paris',
      locale: 'fr',
      email_verified: false,
    }
)

async function resendVerificationEmail() {
  resending.value = true

  try {
    await apiClient.post('/auth/send-verification')
    toast.success(t('auth.verificationEmailResent'))
  } catch (error: any) {
    toast.error(error.message || t('auth.failedToResendEmail'))
  } finally {
    resending.value = false
  }
}
</script>
