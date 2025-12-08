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
        {{ t('settings.mfa.backupCodesTitle') }}
      </h2>

      <div class="mb-4 rounded-lg bg-yellow-50 p-3 dark:bg-yellow-900/20">
        <p class="text-sm text-yellow-800 dark:text-yellow-200">
          {{ t('settings.mfa.backupCodesWarning') }}
        </p>
      </div>

      <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
        {{ t('settings.mfa.backupCodesDescription') }}
      </p>

      <!-- Backup Codes List -->
      <div class="mb-6 grid grid-cols-2 gap-2">
        <div
          v-for="(code, index) in codes"
          :key="index"
          class="rounded bg-gray-100 p-2 text-center font-mono text-sm dark:bg-gray-800"
        >
          {{ code }}
        </div>
      </div>

      <!-- Actions -->
      <div class="flex justify-end space-x-2">
        <button
          class="btn btn-secondary"
          @click="downloadCodes"
        >
          {{ t('settings.mfa.downloadCodes') }}
        </button>
        <button
          class="btn btn-primary"
          @click="$emit('close')"
        >
          {{ t('common.close') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  isOpen: boolean
  codes: string[]
}>()

defineEmits<{
  close: []
}>()

function downloadCodes() {
  const text = `WhenTo Backup Codes\n\nThese codes can be used to access your account if you lose access to your authenticator app.\nEach code can only be used once.\n\n${props.codes.join('\n')}\n\nKeep these codes in a safe place.`

  const blob = new Blob([text], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'whento-backup-codes.txt'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}
</script>
