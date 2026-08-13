<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div
    v-if="isOpen"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50"
  >
    <div
      ref="panel"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      class="card max-w-md max-h-[90vh] overflow-y-auto"
    >
      <h2 :id="titleId" class="mb-4 text-xl font-bold text-gray-900 dark:text-white">
        {{ t('settings.mfa.setupTitle') }}
      </h2>

      <!-- Step 1: Scan QR Code -->
      <div class="mb-6">
        <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
          {{ t('settings.mfa.scanQRCode') }}
        </p>

        <!-- QR Code -->
        <div class="mb-4 flex justify-center">
          <img :src="qrCodeURL" :alt="t('settings.mfa.qrCodeAlt')" class="h-48 w-48 rounded" />
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
        <label for="mfa-verification-code" class="block">
          <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('settings.mfa.enterCode') }}
          </span>
          <input
            id="mfa-verification-code"
            ref="codeInput"
            v-model="verificationCode"
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
            maxlength="6"
            autocomplete="one-time-code"
            class="input text-center text-2xl tracking-widest"
            :placeholder="t('formats.totpCode')"
            @input="verificationCode = verificationCode.replace(/\D/g, '')"
          />
        </label>
      </div>

      <!-- Backup Codes Info -->
      <div class="mb-6 rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
        <p class="text-sm text-blue-800 dark:text-blue-200">
          {{ t('settings.mfa.backupCodesWarning') }}
        </p>
      </div>

      <!-- Actions -->
      <div class="flex justify-end space-x-2">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          :disabled="verificationCode.length !== 6 || verifying"
          class="btn btn-primary"
          @click="verify"
        >
          <span v-if="verifying" class="flex items-center">
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
            {{ t('common.verifying') }}
          </span>
          <span v-else>{{ t('common.verify') }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * Enrolling a second factor is a step you cannot abandon halfway without locking
 * yourself out of the flow, so the dialog needs the full treatment: `role="dialog"`,
 * a focus trap, Escape to cancel, and focus returned to the "Enable" button that
 * opened it.
 *
 * Focus lands on the verification field, which is the only thing the user has to do
 * here once they have scanned the code.
 *
 * Backdrop clicks no longer dismiss, matching ConfirmDialog — and here that is also
 * a safeguard: a stray click outside used to abandon a half-finished enrolment.
 */
import { ref, useId } from 'vue';
import { useI18n } from 'vue-i18n';
import { useFocusTrap } from '@/composables/useFocusTrap';

const { t } = useI18n();

const props = defineProps<{
  isOpen: boolean;
  secret: string;
  qrCodeURL: string;
  backupCodes: string[];
}>();

const emit = defineEmits<{
  verify: [code: string];
  close: [];
}>();

const verificationCode = ref('');
const verifying = ref(false);

const titleId = useId();
const panel = ref<HTMLElement | null>(null);
const codeInput = ref<HTMLInputElement | null>(null);

useFocusTrap(() => props.isOpen, {
  container: panel,
  onEscape: () => emit('close'),
  initialFocus: () => codeInput.value,
});

async function verify() {
  verifying.value = true;
  try {
    emit('verify', verificationCode.value);
  } finally {
    verifying.value = false;
  }
}
</script>
