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
            {{ t('auth.login') }}
          </h1>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
            {{ t('auth.noAccount') }}
            <router-link
              to="/register"
              class="font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
            >
              {{ t('auth.registerButton') }}
            </router-link>
          </p>
        </div>

        <!-- Passkey Login Button (if supported) - Direct login without email -->
        <div
          v-if="isWebAuthnSupported"
          class="mb-6"
        >
          <button
            type="button"
            :disabled="loading || passkeyLoading"
            class="w-full btn btn-primary flex items-center justify-center"
            @click="loginWithDiscoverablePasskey"
          >
            <svg
              v-if="passkeyLoading"
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
            <svg
              v-else
              class="mr-2 h-5 w-5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
              />
            </svg>
            {{ passkeyLoading ? t('common.loading') : t('auth.loginWithPasskeyDirect') }}
          </button>

          <!-- Separator -->
          <div class="relative mt-6">
            <div class="absolute inset-0 flex items-center">
              <div class="w-full border-t border-gray-300 dark:border-gray-600" />
            </div>
            <div class="relative flex justify-center text-xs">
              <span class="bg-white dark:bg-gray-800 px-4 py-1 rounded-full text-gray-500 dark:text-gray-400 font-medium uppercase tracking-wide">
                {{ t('auth.orLoginWithEmail') }}
              </span>
            </div>
          </div>
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

          <!-- Magic Link Button (conditionally shown) -->
          <div v-if="magicLinkAvailable && !magicLinkSuccess">
            <button
              type="button"
              :disabled="magicLinkLoading || !form.email"
              class="btn btn-secondary w-full"
              @click="handleMagicLinkRequest"
            >
              <span
                v-if="magicLinkLoading"
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
                {{ t('common.sending') }}
              </span>
              <span v-else>{{ t('auth.magicLink.sendLink') }}</span>
            </button>
          </div>

          <!-- Separator -->
          <div
            v-if="magicLinkAvailable && !magicLinkSuccess"
            class="relative"
          >
            <div class="absolute inset-0 flex items-center">
              <div class="w-full border-t border-gray-300 dark:border-gray-600" />
            </div>
            <div class="relative flex justify-center text-xs">
              <span class="bg-white dark:bg-gray-800 px-4 py-1 rounded-full text-gray-500 dark:text-gray-400 font-medium uppercase tracking-wide">
                {{ t('common.or') }}
              </span>
            </div>
          </div>

          <!-- Magic Link Success Message -->
          <div
            v-if="magicLinkSuccess"
            class="rounded-lg border border-success-200 bg-success-50 p-4 text-sm text-success-800 dark:border-success-800 dark:bg-success-900/20 dark:text-success-400"
          >
            {{ magicLinkMessage }}
          </div>

          <!-- Password -->
          <div>
            <div class="mb-2 flex items-center justify-between">
              <label
                for="password"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {{ t('auth.password') }}
              </label>
              <button
                type="button"
                class="text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                @click="showForgotPasswordModal = true"
              >
                {{ t('auth.forgotPassword.link') }}
              </button>
            </div>
            <input
              id="password"
              v-model="form.password"
              type="password"
              required
              autocomplete="current-password"
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
          </div>

          <!-- Submit Button -->
          <button
            type="submit"
            :disabled="loading || passkeyLoading"
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
            <span v-else>{{ t('auth.loginButton') }}</span>
          </button>
        </form>
      </div>
    </div>

    <!-- Forgot Password Modal -->
    <ForgotPasswordModal
      :is-open="showForgotPasswordModal"
      @close="showForgotPasswordModal = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import type { LoginRequest } from '@/types'
import { translateValidationError, translateErrorMessage } from '@/utils/errorTranslator'
import { passkeyApi } from '@/api/passkey'
import { authApi } from '@/api/auth'
import ForgotPasswordModal from '@/components/ForgotPasswordModal.vue'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const authStore = useAuthStore()

// Check if WebAuthn is supported in the browser
const isWebAuthnSupported = computed(() => {
  return typeof window !== 'undefined' && window.PublicKeyCredential !== undefined
})

const form = reactive<LoginRequest>({
  email: '',
  password: '',
})

const errors = reactive({
  email: '',
  password: '',
})

const error = ref('')
const loading = ref(false)
const passkeyLoading = ref(false)
const showForgotPasswordModal = ref(false)

// Magic link
const magicLinkAvailable = ref(false)
const magicLinkLoading = ref(false)
const magicLinkSuccess = ref(false)
const magicLinkMessage = ref('')

// Check magic link availability on mount
onMounted(async () => {
  try {
    const response = await authApi.checkMagicLinkAvailable()
    magicLinkAvailable.value = response.available
  } catch (_err) {
    // Silently fail - button won't show
  }
})

function validateForm(): boolean {
  errors.email = ''
  errors.password = ''
  let isValid = true

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
  errors.email = ''
  errors.password = ''

  if (!validateForm()) {
    return
  }

  loading.value = true

  try {
    const response = await authStore.login(form)

    // Check if 2FA is required
    if (response?.require_mfa && response?.temp_token) {
      localStorage.setItem('temp_token', response.temp_token)
      router.push('/verify-mfa')
      return
    }

    // Normal login - redirect to dashboard
    const redirect = (route.query.redirect as string) || '/dashboard'
    router.push(redirect)
  } catch (err: any) {
    // Handle validation errors from backend
    if (err.code === 'VALIDATION_ERROR' && err.details) {
      err.details.forEach((detail: { field: string; message: string }) => {
        const { key, params } = translateValidationError(detail.field, detail.message)
        const translatedMessage = t(key, params || {})

        if (detail.field === 'email') {
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
        error.value = t('auth.loginError')
      }
    }
  } finally {
    loading.value = false
  }
}

async function handleMagicLinkRequest() {
  // Validate email only
  errors.email = ''
  if (!form.email) {
    errors.email = t('errors.required')
    return
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email)) {
    errors.email = t('errors.invalidEmail')
    return
  }

  magicLinkLoading.value = true
  error.value = ''

  try {
    const response = await authApi.requestMagicLink(form.email)
    magicLinkSuccess.value = true
    magicLinkMessage.value = response.message
  } catch (err: any) {
    error.value = err.message || t('auth.magicLink.requestError')
  } finally {
    magicLinkLoading.value = false
  }
}

async function loginWithDiscoverablePasskey() {
  error.value = ''
  passkeyLoading.value = true

  try {
    // Begin passkey authentication (usernameless/passwordless)
    const { options, challengeId } = await passkeyApi.beginAuthentication()

    // Prompt user for passkey (biometric/PIN)
    const credential = (await navigator.credentials.get({
      publicKey: options,
    })) as PublicKeyCredential

    if (!credential) {
      error.value = t('auth.passkeyError')
      return
    }

    // Finish authentication with backend (requires challengeId)
    const response = await passkeyApi.finishAuthentication(credential, challengeId)

    // Store tokens in auth store
    if (response.access_token) {
      authStore.setTokens(response.access_token)
      authStore.user = response.user
    }

    // Check if 2FA is required
    if (response.require_mfa && response.temp_token) {
      localStorage.setItem('temp_token', response.temp_token)
      router.push('/verify-mfa')
      return
    }

    // Normal login - redirect to dashboard
    const redirect = (route.query.redirect as string) || '/dashboard'
    router.push(redirect)
  } catch (err: any) {
    console.error('Passkey login error:', err)

    // Handle specific error cases
    if (err.name === 'NotAllowedError') {
      error.value = t('auth.passkeyDenied')
    } else if (err.name === 'InvalidStateError') {
      error.value = t('auth.passkeyInvalidState')
    } else {
      // Generic error
      const errorKey = translateErrorMessage(err.message || '')
      error.value = errorKey === err.message ? err.message : t(errorKey)

      if (!error.value) {
        error.value = t('auth.passkeyError')
      }
    }
  } finally {
    passkeyLoading.value = false
  }
}
</script>
