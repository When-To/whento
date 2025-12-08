<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <div class="flex min-h-[calc(100vh-4rem)] items-center justify-center py-12">
    <div class="w-full max-w-md animate-slide-up">
      <div class="card">
        <!-- Header -->
        <div class="mb-6 text-center">
          <div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900">
            <svg
              class="h-8 w-8 text-primary-600 dark:text-primary-400"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
              />
            </svg>
          </div>
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ t('auth.verify2FA') }}
          </h1>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
            {{ useBackupCode ? t('auth.enterBackupCode') : t('auth.enter2FACode') }}
          </p>
        </div>

        <!-- Error Message -->
        <div
          v-if="error"
          class="mb-6 rounded-lg border border-danger-200 bg-danger-50 p-4 text-sm text-danger-800 dark:border-danger-800 dark:bg-danger-900/20 dark:text-danger-400"
        >
          {{ error }}
        </div>

        <!-- Code Input -->
        <form
          class="space-y-6"
          @submit.prevent="handleVerify"
        >
          <div>
            <input
              v-model="code"
              type="text"
              :inputmode="useBackupCode ? 'text' : 'numeric'"
              :pattern="useBackupCode ? '[A-Z0-9]*' : '[0-9]*'"
              :maxlength="useBackupCode ? 8 : 6"
              :placeholder="useBackupCode ? 'ABCD1234' : '000000'"
              class="input text-center text-2xl tracking-widest uppercase"
              :class="{ 'input-error': error }"
              autofocus
              autocomplete="one-time-code"
            >
            <p class="mt-2 text-center text-xs text-gray-500 dark:text-gray-400">
              {{ useBackupCode ? t('auth.backupCodeFormat') : t('auth.totpCodeFormat') }}
            </p>
          </div>

          <button
            type="submit"
            :disabled="loading || !isCodeValid"
            class="btn btn-primary w-full"
          >
            <span
              v-if="loading"
              class="flex items-center justify-center"
            >
              <svg
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
              {{ t('common.verifying') }}
            </span>
            <span v-else>{{ t('common.verify') }}</span>
          </button>
        </form>

        <!-- Toggle Backup Code / TOTP -->
        <div class="mt-6 text-center">
          <button
            type="button"
            class="text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
            @click="toggleCodeType"
          >
            {{ useBackupCode ? t('auth.use2FACode') : t('auth.useBackupCode') }}
          </button>
        </div>

        <!-- Back to Login -->
        <div class="mt-4 text-center">
          <button
            type="button"
            class="text-sm text-gray-600 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
            @click="backToLogin"
          >
            ← {{ t('auth.backToLogin') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { mfaApi } from '@/api/mfa'

const router = useRouter()
const { t } = useI18n()
const authStore = useAuthStore()

const code = ref('')
const error = ref('')
const loading = ref(false)
const useBackupCode = ref(false)

// Validate code format (6 digits or 8 alphanumeric)
const isCodeValid = computed(() => {
  if (useBackupCode.value) {
    return code.value.length === 8 && /^[A-Z0-9]{8}$/.test(code.value.toUpperCase())
  } else {
    return code.value.length === 6 && /^\d{6}$/.test(code.value)
  }
})

onMounted(() => {
  // Check if temp token exists
  const tempToken = localStorage.getItem('temp_token')
  if (!tempToken) {
    // No temp token, redirect to login
    router.push('/login')
  }
})

function toggleCodeType() {
  useBackupCode.value = !useBackupCode.value
  code.value = ''
  error.value = ''
}

function backToLogin() {
  localStorage.removeItem('temp_token')
  router.push('/login')
}

async function handleVerify() {
  const tempToken = localStorage.getItem('temp_token')
  if (!tempToken) {
    router.push('/login')
    return
  }

  loading.value = true
  error.value = ''

  try {
    // Normalize backup code to uppercase
    const normalizedCode = useBackupCode.value ? code.value.toUpperCase() : code.value

    // Verify MFA code with backend
    const response = await mfaApi.verify(tempToken, normalizedCode)

    // Clear temp token
    localStorage.removeItem('temp_token')

    // Store real JWT tokens
    authStore.setTokens(response.access_token)
    authStore.user = response.user

    // Redirect to dashboard
    router.push('/dashboard')
  } catch (err: any) {
    console.error('MFA verification error:', err)

    // Show error message
    if (useBackupCode.value) {
      error.value = t('auth.invalidBackupCode')
    } else {
      error.value = t('auth.invalid2FACode')
    }

    // Clear code input
    code.value = ''
  } finally {
    loading.value = false
  }
}
</script>
