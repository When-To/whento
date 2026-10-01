<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="space-y-6">
    <!-- Password Change Section -->
    <div class="card">
      <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
        {{ t('settings.changePassword') }}
      </h2>
      <form class="space-y-4" @submit.prevent="changePassword">
        <div>
          <label for="security-current-password" class="block">
            <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('auth.currentPassword') }}
            </span>
            <input
              id="security-current-password"
              v-model="passwordForm.currentPassword"
              type="password"
              class="input"
              autocomplete="current-password"
            />
          </label>
        </div>
        <div>
          <label for="security-new-password" class="block">
            <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('auth.newPassword') }}
            </span>
            <input
              id="security-new-password"
              v-model="passwordForm.newPassword"
              type="password"
              class="input"
              :class="{ 'input-error': passwordError }"
              autocomplete="new-password"
              @blur="passwordTouched = true"
            />
          </label>
          <p v-if="passwordError" class="mt-1 text-xs text-danger-600 dark:text-danger-400">
            {{ passwordError }}
          </p>
        </div>
        <div>
          <label for="security-confirm-password" class="block">
            <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('auth.confirmPassword') }}
            </span>
            <input
              id="security-confirm-password"
              v-model="passwordForm.confirmPassword"
              type="password"
              class="input"
              autocomplete="new-password"
            />
          </label>
          <p
            v-if="
              passwordForm.confirmPassword.length > 0 &&
              passwordForm.newPassword !== passwordForm.confirmPassword
            "
            class="mt-1 text-xs text-danger-600 dark:text-danger-400"
          >
            {{ t('auth.resetPassword.passwordMismatch') }}
          </p>
        </div>
        <p v-if="changeError" class="text-sm text-danger-600 dark:text-danger-400">
          {{ changeError }}
        </p>
        <button
          type="submit"
          :disabled="!canChangePassword || changingPassword"
          class="btn btn-primary"
        >
          {{ changingPassword ? t('common.saving') : t('settings.updatePassword') }}
        </button>
      </form>
    </div>

    <!-- Passkeys Section -->
    <div class="card">
      <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
        {{ t('settings.passkeys.title') }}
      </h2>
      <PasskeyManager />
    </div>

    <!-- 2FA Section -->
    <div class="card">
      <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
        {{ t('settings.mfa.title') }}
      </h2>
      <MFASetup />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useToastStore } from '@/stores/toast';
import { apiClient } from '@/api/client';
import { validatePassword } from '@/utils/password';
import PasskeyManager from './PasskeyManager.vue';
import MFASetup from './MFASetup.vue';

const { t } = useI18n();
const toast = useToastStore();

const passwordForm = reactive({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
});

const changingPassword = ref(false);
const changeError = ref('');

// The same rule the server enforces (validatePassword mirrors the backend's
// strongpassword + maxbytes), so the form never submits a password the backend
// immediately refuses. A translated key is displayed under the field.
//
// The message is only rendered once the field has been touched: an empty (still
// pristine) form is not an error state, and showing "too short" on first paint
// made the tab look broken before anyone typed. The submit gate uses the
// untampered result, so a pristine form still stays disabled.
const passwordValidationError = computed(() => {
  const err = validatePassword(passwordForm.newPassword);
  return err ? t(err) : '';
});
const passwordTouched = ref(false);
const passwordError = computed(() => (passwordTouched.value ? passwordValidationError.value : ''));

const canChangePassword = computed(() => {
  return (
    passwordForm.currentPassword.length > 0 &&
    passwordValidationError.value === '' &&
    passwordForm.confirmPassword.length > 0 &&
    passwordForm.newPassword === passwordForm.confirmPassword
  );
});

async function changePassword() {
  if (!canChangePassword.value) return;

  changingPassword.value = true;
  changeError.value = '';

  try {
    await apiClient.patch('/auth/me/password', {
      current_password: passwordForm.currentPassword,
      new_password: passwordForm.newPassword,
    });

    // Reset form
    passwordForm.currentPassword = '';
    passwordForm.newPassword = '';
    passwordForm.confirmPassword = '';
    passwordTouched.value = false;

    toast.success(t('settings.passwordChanged'));
  } catch (error: any) {
    if (error.message?.includes('current password') || error.message?.includes('incorrect')) {
      toast.error(t('auth.invalidPassword'));
    } else {
      changeError.value = t('settings.passwordChangeFailed');
    }
  } finally {
    changingPassword.value = false;
  }
}
</script>
