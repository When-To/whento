<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="flex min-h-[calc(100vh-4rem)] items-center justify-center py-12">
    <div class="w-full max-w-md animate-slide-up">
      <div class="card text-center">
        <h1 class="sr-only">{{ t('a11y.magicLinkTitle') }}</h1>

        <!-- Loading State (verification in flight after the user confirms) -->
        <div v-if="verifying">
          <div
            class="mx-auto mb-4 h-16 w-16 animate-spin rounded-full border-4 border-primary-200 border-t-primary-600 dark:border-primary-800 dark:border-t-primary-400"
          />
          <p class="text-gray-600 dark:text-gray-400">
            {{ t('auth.magicLink.verifying') }}
          </p>
        </div>

        <!-- Error State -->
        <div v-else-if="error" class="text-center">
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
          <router-link to="/login" class="btn btn-primary">
            {{ t('auth.backToLogin') }}
          </router-link>
        </div>

        <!-- Confirmation State: the link alone must never sign the visitor in -->
        <div v-else>
          <div
            class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/20"
          >
            <svg
              class="h-8 w-8 text-primary-600"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 6z"
              />
            </svg>
          </div>
          <h2 class="mb-2 text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('auth.magicLink.confirmTitle') }}
          </h2>
          <p class="mb-4 text-gray-600 dark:text-gray-400">
            {{ t('auth.magicLink.confirmDescription') }}
          </p>

          <!-- An already-authenticated visitor must be told this switches accounts
               before they trigger it. -->
          <div
            v-if="authStore.isAuthenticated"
            class="mb-4 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-900/20 dark:text-amber-200"
          >
            {{ t('auth.magicLink.accountSwitchWarning') }}
          </div>

          <button type="button" class="btn btn-primary w-full" @click="confirmVerification">
            {{ t('auth.magicLink.confirmButton') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useAuthStore } from '@/stores/auth';
import { authApi } from '@/api/auth';

const router = useRouter();
const route = useRoute();
const { t } = useI18n();
const authStore = useAuthStore();

const verifying = ref(false);
const error = ref('');
// The 64-hex token is held in memory only: it is removed from the URL below so it
// stops being visible in the address bar and in history, and it is never written
// to localStorage.
const token = ref('');

onMounted(() => {
  const rawToken = route.params.token as string;
  // Update both browser state and Router's internal current location. A direct
  // history.replaceState leaves Router holding the secret URL, which its next
  // push can copy back into history.state.back. Keep the proof only in memory.
  const cleanPath = '/auth/magic-link/verify/';
  router.options.history.replace(cleanPath, { ...history.state, current: cleanPath });

  if (!rawToken || rawToken.length !== 64) {
    error.value = t('auth.magicLink.missingToken');
    return;
  }

  token.value = rawToken;
});

async function confirmVerification() {
  if (!token.value) {
    error.value = t('auth.magicLink.missingToken');
    return;
  }

  verifying.value = true;
  error.value = '';

  try {
    const response = await authApi.verifyMagicLink(token.value);

    // An MFA-protected account gets a pending challenge, not a session: route to
    // the same second-factor flow a password login would use. The temp token
    // lives in the store so the redirect survives.
    if (response?.require_mfa && response?.temp_token) {
      authStore.setTempToken(response.temp_token);
      router.push('/verify-mfa');
      return;
    }

    if (!response?.access_token) {
      error.value = t('auth.magicLink.verifyError');
      verifying.value = false;
      return;
    }

    authStore.user = response.user;
    // Route through the store action so the account generation advances (a verified
    // link is a new session; the previous account's pending work must not commit).
    authStore.setTokens(response.access_token, response.expires_in, response.session_id);
    router.push('/dashboard');
  } catch (err: any) {
    verifying.value = false;
    if (err.message?.includes('expired')) {
      error.value = t('auth.magicLink.expired');
    } else if (err.message?.includes('invalid')) {
      error.value = t('auth.magicLink.invalid');
    } else {
      error.value = t('auth.magicLink.verifyError');
    }
  }
}
</script>
