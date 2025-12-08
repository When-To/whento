<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="flex min-h-[calc(100vh-4rem)] items-center justify-center py-12">
    <div class="w-full max-w-md animate-slide-up">
      <!-- Card -->
      <div class="card">
        <!-- Header -->
        <div class="mb-8 text-center">
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ t('auth.resetPassword.title') }}
          </h1>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
            {{ t('auth.resetPassword.description') }}
          </p>
        </div>

        <!-- Form -->
        <form
          v-if="!success"
          class="space-y-6"
          @submit.prevent="handleSubmit"
        >
          <!-- New Password -->
          <div>
            <label
              for="new-password"
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t('auth.resetPassword.newPassword') }}
            </label>
            <input
              id="new-password"
              v-model="newPassword"
              type="password"
              required
              minlength="8"
              autocomplete="new-password"
              class="input"
              :class="{ 'input-error': error }"
              :disabled="loading"
            >
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('auth.resetPassword.passwordRequirement') }}
            </p>
          </div>

          <!-- Confirm Password -->
          <div>
            <label
              for="confirm-password"
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t('auth.resetPassword.confirmPassword') }}
            </label>
            <input
              id="confirm-password"
              v-model="confirmPassword"
              type="password"
              required
              minlength="8"
              autocomplete="new-password"
              class="input"
              :class="{ 'input-error': error }"
              :disabled="loading"
            >
          </div>

          <!-- Error Message -->
          <div
            v-if="error"
            class="rounded-lg border border-danger-200 bg-danger-50 p-4 text-sm text-danger-800 dark:border-danger-800 dark:bg-danger-900/20 dark:text-danger-400"
          >
            {{ error }}
          </div>

          <!-- Submit Button -->
          <button
            type="submit"
            :disabled="loading || !isPasswordValid"
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
              {{ t('common.processing') }}
            </span>
            <span v-else>{{ t('auth.resetPassword.submit') }}</span>
          </button>
        </form>

        <!-- Success State -->
        <div
          v-else
          class="py-6 text-center"
        >
          <div class="mx-auto mb-4 flex h-20 w-20 items-center justify-center rounded-full bg-success-100 dark:bg-success-900/20">
            <svg
              class="h-10 w-10 text-success-600 dark:text-success-400"
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
          <h2 class="mb-2 text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('auth.resetPassword.successTitle') }}
          </h2>
          <p class="mb-6 text-gray-600 dark:text-gray-400">
            {{ t('auth.resetPassword.successMessage') }}
          </p>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('auth.resetPassword.redirecting') }}
          </p>
        </div>

        <!-- Back to Login Link -->
        <div class="mt-6 text-center">
          <router-link
            to="/login"
            class="text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
          >
            {{ t('auth.resetPassword.backToLogin') }}
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const toastStore = useToastStore()

const token = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const loading = ref(false)
const error = ref('')
const success = ref(false)

const isPasswordValid = computed(() => {
  return (
    newPassword.value.length >= 8 &&
    confirmPassword.value.length >= 8 &&
    newPassword.value === confirmPassword.value
  )
})

onMounted(() => {
  token.value = route.params.token as string
  if (!token.value || token.value.length !== 64) {
    error.value = t('auth.resetPassword.invalidToken')
  }
})

const handleSubmit = async () => {
  if (!isPasswordValid.value) {
    error.value = t('auth.resetPassword.passwordMismatch')
    return
  }

  loading.value = true
  error.value = ''

  try {
    await authStore.resetPassword(token.value, newPassword.value)
    success.value = true
    toastStore.success(t('auth.resetPassword.successToast'))

    // Auto-redirect to dashboard after 2 seconds
    setTimeout(() => {
      router.push('/dashboard')
    }, 2000)
  } catch (err: any) {
    error.value = err.message || t('auth.resetPassword.error')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
@keyframes slideUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.animate-slide-up {
  animation: slideUp 0.4s ease-out;
}
</style>
