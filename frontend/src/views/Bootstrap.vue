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
          <img src="/logo.png" :alt="t('common.logoAlt')" class="mx-auto mb-4 h-16 w-16" />
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ t('auth.bootstrap.title') }}
          </h1>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
            {{ t('auth.bootstrap.description') }}
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
        <form class="space-y-6" @submit.prevent="handleSubmit">
          <!-- Boot key -->
          <div>
            <label for="boot_key" class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('auth.bootstrap.bootKeyLabel') }}
              </span>
              <input
                id="boot_key"
                v-model="form.boot_key"
                type="password"
                required
                autocomplete="off"
                class="input font-mono"
                :class="{ 'input-error': errors.boot_key }"
                :placeholder="t('auth.bootstrap.bootKeyLabel')"
              />
            </label>
            <p v-if="errors.boot_key" class="mt-1 text-sm text-danger-600 dark:text-danger-400">
              {{ errors.boot_key }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('auth.bootstrap.bootKeyHelp') }}
            </p>
          </div>

          <div class="relative">
            <div class="absolute inset-0 flex items-center">
              <div class="w-full border-t border-gray-300 dark:border-gray-600" />
            </div>
            <div class="relative flex justify-center text-xs">
              <span
                class="bg-white dark:bg-gray-800 px-4 py-1 rounded-full text-gray-500 dark:text-gray-400 font-medium uppercase tracking-wide"
              >
                {{ t('auth.bootstrap.needUser') }}
              </span>
            </div>
          </div>

          <!-- Display Name -->
          <div>
            <label for="display_name" class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('auth.displayName') }}
              </span>
              <input
                id="display_name"
                v-model="form.display_name"
                type="text"
                required
                autocomplete="name"
                class="input"
                :class="{ 'input-error': errors.display_name }"
                :placeholder="t('auth.displayName')"
              />
            </label>
            <p v-if="errors.display_name" class="mt-1 text-sm text-danger-600 dark:text-danger-400">
              {{ errors.display_name }}
            </p>
          </div>

          <!-- Email -->
          <div>
            <label for="email" class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('auth.email') }}
              </span>
              <input
                id="email"
                v-model="form.email"
                type="email"
                required
                autocomplete="email"
                class="input"
                :class="{ 'input-error': errors.email }"
                :placeholder="t('auth.email')"
              />
            </label>
            <p v-if="errors.email" class="mt-1 text-sm text-danger-600 dark:text-danger-400">
              {{ errors.email }}
            </p>
          </div>

          <!-- Password -->
          <div>
            <label for="password" class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('auth.password') }}
              </span>
              <input
                id="password"
                v-model="form.password"
                type="password"
                required
                autocomplete="new-password"
                class="input"
                :class="{ 'input-error': errors.password }"
                :placeholder="t('auth.password')"
              />
            </label>
            <p v-if="errors.password" class="mt-1 text-sm text-danger-600 dark:text-danger-400">
              {{ errors.password }}
            </p>
            <p v-else class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('errors.passwordTooShort') }}
            </p>
          </div>

          <!-- Submit Button -->
          <button type="submit" :disabled="loading" class="btn btn-primary w-full">
            <span v-if="loading" class="flex items-center justify-center">
              <svg class="mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
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
            <span v-else>{{ t('auth.bootstrap.createAccount') }}</span>
          </button>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useAuthStore } from '@/stores/auth';
import type { BootstrapRequest, ApiError } from '@/types';
import { translateValidationError, translateErrorMessage } from '@/utils/errorTranslator';
import { validatePassword } from '@/utils/password';

const router = useRouter();
const { t, locale } = useI18n();
const authStore = useAuthStore();

const form = reactive<BootstrapRequest>({
  boot_key: '',
  display_name: '',
  email: '',
  password: '',
});

const errors = reactive({
  boot_key: '',
  display_name: '',
  email: '',
  password: '',
});

const error = ref('');
const loading = ref(false);

// When the instance is no longer unconfigured — a bootstrap or a racing first
// registration that this tab has actually *seen* complete — this page has
// nothing left to do. The router guard owns navigation on capability state: it
// only redirects away from /bootstrap once a real /auth/status answer says the
// instance is configured (bootstrapStatusKnown && !bootstrapRequired). Mirroring
// that exact condition here stops the component from independently bouncing the
// operator on a *failed* status read, where the defaults (not known, therefore
// "not required") would otherwise hide the only setup page.
const alreadyConfigured = computed(
  () => authStore.bootstrapStatusKnown && !authStore.bootstrapRequired
);
if (alreadyConfigured.value) {
  router.replace('/login');
}

function validateForm(): boolean {
  errors.boot_key = '';
  errors.display_name = '';
  errors.email = '';
  errors.password = '';
  let isValid = true;

  if (!form.boot_key) {
    errors.boot_key = t('errors.required');
    isValid = false;
  }

  if (!form.display_name || form.display_name.trim().length === 0) {
    errors.display_name = t('errors.required');
    isValid = false;
  }

  if (!form.email) {
    errors.email = t('errors.required');
    isValid = false;
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email)) {
    errors.email = t('errors.invalidEmail');
    isValid = false;
  }

  if (!form.password) {
    errors.password = t('errors.required');
    isValid = false;
  } else {
    const passwordError = validatePassword(form.password);
    if (passwordError) {
      errors.password = t(passwordError);
      isValid = false;
    }
  }

  return isValid;
}

async function handleSubmit() {
  error.value = '';
  errors.boot_key = '';
  errors.display_name = '';
  errors.email = '';
  errors.password = '';

  if (!validateForm()) {
    return;
  }

  loading.value = true;

  try {
    const requestData: BootstrapRequest = {
      ...form,
      locale: locale.value as 'fr' | 'en',
    };
    await authStore.bootstrap(requestData);
    router.push('/dashboard');
  } catch (err) {
    const apiError = err as ApiError;

    // Validation errors map to their fields.
    if (apiError.code === 'VALIDATION_ERROR' && apiError.details) {
      apiError.details.forEach(detail => {
        const { key, params } = translateValidationError(detail.field ?? '', detail.message ?? '');
        const translatedMessage = t(key, params || {});
        const field = detail.field ?? '';

        if (field === 'boot_key') {
          errors.boot_key = translatedMessage;
        } else if (field === 'display_name') {
          errors.display_name = translatedMessage;
        } else if (field === 'email') {
          errors.email = translatedMessage;
        } else if (field === 'password') {
          errors.password = translatedMessage;
        }
      });
      return;
    }

    // A 401 here is a wrong boot key, not "you are not signed in".
    if (apiError.code === 'UNAUTHORIZED') {
      error.value = t('auth.bootstrap.invalidKey');
      return;
    }
    if (apiError.code === 'CONFLICT') {
      error.value = t('auth.bootstrap.alreadyConfigured');
      authStore.bootstrapRequired = false; // reflected by the guard next nav
      router.replace('/login');
      return;
    }
    error.value = t(translateErrorMessage(apiError, { fallback: 'auth.bootstrap.invalidKey' }));
  } finally {
    loading.value = false;
  }
}
</script>
