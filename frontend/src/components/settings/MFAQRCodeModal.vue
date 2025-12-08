<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <div
    v-if="isOpen"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50"
    @click.self="$emit('close')"
  >
    <div class="card max-w-md max-h-[90vh] overflow-y-auto">
      <h2 class="mb-4 text-xl font-bold text-gray-900 dark:text-white">
        {{ t('settings.mfa.setupTitle') }}
      </h2>

      <!-- Step 1: Scan QR Code -->
      <div class="mb-6">
        <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
          {{ t('settings.mfa.scanQRCode') }}
        </p>

        <!-- QR Code -->
        <div class="mb-4 flex justify-center">
          <img
            :src="qrCodeURL"
            alt="QR Code"
            class="h-48 w-48 rounded"
          >
        </div>

        <!-- Manual Entry Secret -->
        <div class="mb-4">
          <p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('settings.mfa.manualEntry') }}:
          </p>
          <code class="block rounded bg-gray-100 p-2 text-center text-sm dark:bg-gray-800">
            {{ secret }}
          </code>
        </div>
      </div>

      <!-- Step 2: Verify Code -->
      <div class="mb-6">
        <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('settings.mfa.enterCode') }}
        </label>
        <input
          v-model="verificationCode"
          type="text"
          inputmode="numeric"
          pattern="[0-9]*"
          maxlength="6"
          class="input text-center text-2xl tracking-widest"
          placeholder="000000"
          @input="verificationCode = verificationCode.replace(/\D/g, '')"
        >
      </div>

      <!-- Backup Codes Info -->
      <div class="mb-6 rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
        <p class="text-sm text-blue-800 dark:text-blue-200">
          {{ t('settings.mfa.backupCodesWarning') }}
        </p>
      </div>

      <!-- Actions -->
      <div class="flex justify-end space-x-2">
        <button
          class="btn btn-secondary"
          @click="$emit('close')"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          :disabled="verificationCode.length !== 6 || verifying"
          class="btn btn-primary"
          @click="verify"
        >
          <span
            v-if="verifying"
            class="flex items-center"
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
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{
  isOpen: boolean
  secret: string
  qrCodeURL: string
  backupCodes: string[]
}>()

const emit = defineEmits<{
  verify: [code: string]
  close: []
}>()

const verificationCode = ref('')
const verifying = ref(false)

async function verify() {
  verifying.value = true
  try {
    emit('verify', verificationCode.value)
  } finally {
    verifying.value = false
  }
}
</script>
