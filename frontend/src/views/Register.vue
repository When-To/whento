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
          <img
            src="/logo.png"
            alt="WhenTo"
            class="mx-auto mb-4 h-16 w-16"
          >
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ t('auth.register') }}
          </h1>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
            {{ t('auth.hasAccount') }}
            <router-link
              to="/login"
              class="font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
            >
              {{ t('auth.loginButton') }}
            </router-link>
          </p>
        </div>

        <!-- Error Message -->
        <div
          v-if="error"
          class="mb-6 rounded-lg border border-danger-200 bg-danger-50 p-4 text-sm text-danger-800 dark:border-danger-800 dark:bg-danger-900/20 dark:text-danger-400"
        >
          {{ error }}
        </div>

        <!-- Form -->
        <form
          class="space-y-6"
          @submit.prevent="handleSubmit"
        >
          <!-- Display Name -->
          <div>
            <label
              for="display_name"
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t('auth.displayName') }}
            </label>
            <input
              id="display_name"
              v-model="form.display_name"
              type="text"
              required
              autocomplete="name"
              class="input"
              :class="{ 'input-error': errors.display_name }"
              :placeholder="t('auth.displayName')"
            >
            <p
              v-if="errors.display_name"
              class="mt-1 text-sm text-danger-600 dark:text-danger-400"
            >
              {{ errors.display_name }}
            </p>
          </div>

          <!-- Email -->
          <div>
            <label
              for="email"
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t('auth.email') }}
            </label>
            <input
              id="email"
              v-model="form.email"
              type="email"
              required
              autocomplete="email"
              class="input"
              :class="{ 'input-error': errors.email }"
              :placeholder="t('auth.email')"
            >
            <p
              v-if="errors.email"
              class="mt-1 text-sm text-danger-600 dark:text-danger-400"
            >
              {{ errors.email }}
            </p>
          </div>

          <!-- Password -->
          <div>
            <label
              for="password"
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t('auth.password') }}
            </label>
            <input
              id="password"
              v-model="form.password"
              type="password"
              required
              autocomplete="new-password"
              class="input"
              :class="{ 'input-error': errors.password }"
              :placeholder="t('auth.password')"
            >
            <p
              v-if="errors.password"
              class="mt-1 text-sm text-danger-600 dark:text-danger-400"
            >
              {{ errors.password }}
            </p>
            <p
              v-else
              class="mt-1 text-sm text-gray-500 dark:text-gray-400"
            >
              {{ t('errors.passwordTooShort') }}
            </p>
          </div>

          <!-- Submit Button -->
          <button
            type="submit"
            :disabled="loading"
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
              {{ t('common.loading') }}
            </span>
            <span v-else>{{ t('auth.registerButton') }}</span>
          </button>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import type { RegisterRequest } from '@/types'
import { translateValidationError, translateErrorMessage } from '@/utils/errorTranslator'

const router = useRouter()
const { t, locale } = useI18n()
const authStore = useAuthStore()

const form = reactive<RegisterRequest>({
  display_name: '',
  email: '',
  password: '',
})

const errors = reactive({
  display_name: '',
  email: '',
  password: '',
})

const error = ref('')
const loading = ref(false)

function validateForm(): boolean {
  errors.display_name = ''
  errors.email = ''
  errors.password = ''
  let isValid = true

  if (!form.display_name || form.display_name.trim().length === 0) {
    errors.display_name = t('errors.required')
    isValid = false
  }

  if (!form.email) {
    errors.email = t('errors.required')
    isValid = false
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email)) {
    errors.email = t('errors.invalidEmail')
    isValid = false
  }

  if (!form.password) {
    errors.password = t('errors.required')
    isValid = false
  } else if (form.password.length < 8) {
    errors.password = t('errors.passwordTooShort')
    isValid = false
  }

  return isValid
}

async function handleSubmit() {
  error.value = ''
  errors.display_name = ''
  errors.email = ''
  errors.password = ''

  if (!validateForm()) {
    return
  }

  loading.value = true

  try {
    // Include current locale from UI
    const requestData: RegisterRequest = {
      ...form,
      locale: locale.value as 'fr' | 'en',
    }
    await authStore.register(requestData)
    router.push('/dashboard')
  } catch (err: any) {
    // Handle validation errors from backend
    if (err.code === 'VALIDATION_ERROR' && err.details) {
      err.details.forEach((detail: { field: string; message: string }) => {
        const { key, params } = translateValidationError(detail.field, detail.message)
        const translatedMessage = t(key, params || {})

        if (detail.field === 'display_name') {
          errors.display_name = translatedMessage
        } else if (detail.field === 'email') {
          errors.email = translatedMessage
        } else if (detail.field === 'password') {
          errors.password = translatedMessage
        }
      })
    } else {
      // Show generic error message (translate if possible)
      const errorKey = translateErrorMessage(err.message || '')
      error.value = errorKey === err.message ? err.message : t(errorKey)

      // If no error message, use fallback
      if (!error.value) {
        error.value = t('auth.registerError')
      }
    }
  } finally {
    loading.value = false
  }
}
</script>
