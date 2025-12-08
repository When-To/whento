<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <div class="space-y-6">
    <!-- Password Change Section -->
    <div class="card">
      <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
        {{ t('settings.changePassword') }}
      </h2>
      <form
        class="space-y-4"
        @submit.prevent="changePassword"
      >
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('auth.currentPassword') }}
          </label>
          <input
            v-model="passwordForm.currentPassword"
            type="password"
            class="input"
            autocomplete="current-password"
          >
        </div>
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('auth.newPassword') }}
          </label>
          <input
            v-model="passwordForm.newPassword"
            type="password"
            class="input"
            autocomplete="new-password"
          >
        </div>
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('auth.confirmPassword') }}
          </label>
          <input
            v-model="passwordForm.confirmPassword"
            type="password"
            class="input"
          >
        </div>
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
import { ref, reactive, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useToastStore } from '@/stores/toast'
import { apiClient } from '@/api/client'
import PasskeyManager from './PasskeyManager.vue'
import MFASetup from './MFASetup.vue'

const { t } = useI18n()
const toast = useToastStore()

const passwordForm = reactive({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const changingPassword = ref(false)

const canChangePassword = computed(() => {
  return (
    passwordForm.currentPassword.length > 0 &&
    passwordForm.newPassword.length >= 12 &&
    passwordForm.newPassword === passwordForm.confirmPassword
  )
})

async function changePassword() {
  if (!canChangePassword.value) return

  changingPassword.value = true

  try {
    await apiClient.patch('/auth/me/password', {
      current_password: passwordForm.currentPassword,
      new_password: passwordForm.newPassword,
    })

    // Reset form
    passwordForm.currentPassword = ''
    passwordForm.newPassword = ''
    passwordForm.confirmPassword = ''

    toast.success(t('settings.passwordChanged'))
  } catch (error: any) {
    if (error.message?.includes('current password') || error.message?.includes('incorrect')) {
      toast.error(t('auth.invalidPassword'))
    } else {
      toast.error(t('settings.passwordChangeFailed'))
    }
  } finally {
    changingPassword.value = false
  }
}
</script>
