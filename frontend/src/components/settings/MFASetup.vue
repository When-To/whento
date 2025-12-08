<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <div>
    <!-- MFA Disabled State -->
    <div v-if="!mfaStatus.enabled">
      <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
        {{ t('settings.mfa.description') }}
      </p>
      <button
        :disabled="settingUp"
        class="btn btn-primary"
        @click="beginSetup"
      >
        {{ settingUp ? t('common.loading') : t('settings.mfa.enable') }}
      </button>
    </div>

    <!-- MFA Enabled State -->
    <div v-else>
      <div class="mb-4 flex items-center">
        <svg
          class="mr-2 h-5 w-5 text-success-600 dark:text-success-400"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
          />
        </svg>
        <span class="font-medium text-success-600 dark:text-success-400">
          {{ t('settings.mfa.enabled') }}
        </span>
      </div>

      <div class="flex flex-wrap gap-2">
        <button
          :disabled="regenerating"
          class="btn btn-secondary"
          @click="regenerateBackupCodes"
        >
          {{ regenerating ? t('common.loading') : t('settings.mfa.regenerateBackupCodes') }}
        </button>
        <button
          :disabled="disabling"
          class="btn btn-danger"
          @click="disable2FA"
        >
          {{ disabling ? t('common.loading') : t('settings.mfa.disable') }}
        </button>
      </div>
    </div>

    <!-- Setup Modal -->
    <MFAQRCodeModal
      v-if="showQRModal"
      :is-open="showQRModal"
      :secret="setupData.secret"
      :qr-code-u-r-l="setupData.qr_code_url"
      :backup-codes="setupData.backup_codes"
      @verify="verifySetup"
      @close="closeSetupModal"
    />

    <!-- Backup Codes Modal -->
    <BackupCodesModal
      :is-open="showBackupCodesModal"
      :codes="backupCodes"
      @close="showBackupCodesModal = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { mfaApi } from '@/api/mfa'
import { useToastStore } from '@/stores/toast'
import MFAQRCodeModal from './MFAQRCodeModal.vue'
import BackupCodesModal from './BackupCodesModal.vue'

const { t } = useI18n()
const toast = useToastStore()

const mfaStatus = ref({ enabled: false })
const setupData = ref({
  secret: '',
  qr_code_url: '',
  backup_codes: [] as string[]
})
const showQRModal = ref(false)
const showBackupCodesModal = ref(false)
const backupCodes = ref<string[]>([])
const settingUp = ref(false)
const disabling = ref(false)
const regenerating = ref(false)

onMounted(() => {
  loadStatus()
})

async function loadStatus() {
  try {
    mfaStatus.value = await mfaApi.getStatus()
  } catch (error) {
    console.error('Failed to load MFA status:', error)
  }
}

async function beginSetup() {
  settingUp.value = true
  try {
    setupData.value = await mfaApi.beginSetup()
    showQRModal.value = true
  } catch (error) {
    console.error('Failed to begin MFA setup:', error)
    toast.error(t('settings.mfa.setupError'))
  } finally {
    settingUp.value = false
  }
}

async function verifySetup(code: string) {
  try {
    await mfaApi.finishSetup(code)
    mfaStatus.value.enabled = true
    showQRModal.value = false

    // Show backup codes
    backupCodes.value = setupData.value.backup_codes
    showBackupCodesModal.value = true

    toast.success(t('settings.mfa.enableSuccess'))
  } catch (error) {
    console.error('Failed to verify MFA code:', error)
    toast.error(t('settings.mfa.invalidCode'))
  }
}

function closeSetupModal() {
  showQRModal.value = false
  setupData.value = {
    secret: '',
    qr_code_url: '',
    backup_codes: []
  }
}

async function disable2FA() {
  // Confirm the action
  if (!confirm(t('settings.mfa.confirmDisable'))) {
    return
  }

  disabling.value = true
  try {
    await mfaApi.disable()
    mfaStatus.value.enabled = false
    toast.success(t('settings.mfa.disableSuccess'))
  } catch (error: any) {
    console.error('Failed to disable 2FA:', error)
    toast.error(t('settings.mfa.disableError'))
  } finally {
    disabling.value = false
  }
}

async function regenerateBackupCodes() {
  if (!confirm(t('settings.mfa.backupCodesWarning'))) {
    return
  }

  regenerating.value = true
  try {
    backupCodes.value = await mfaApi.regenerateBackupCodes()
    showBackupCodesModal.value = true
    toast.success(t('settings.mfa.regenerateSuccess'))
  } catch (error) {
    console.error('Failed to regenerate backup codes:', error)
    toast.error(t('settings.mfa.regenerateError'))
  } finally {
    regenerating.value = false
  }
}
</script>
