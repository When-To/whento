<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getLicenseStatus,
  activateLicense,
  deactivateLicense,
  type LicenseStatus,
} from '../api/license'
import { useRouter } from 'vue-router'

const { t } = useI18n()
const router = useRouter()

const license = ref<LicenseStatus | null>(null)
const loading = ref(true)
const activating = ref(false)
const deactivating = ref(false)
const error = ref<string | null>(null)
const success = ref<string | null>(null)

const licenseKey = ref('')
const showActivateForm = ref(false)
const isDragging = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)

const fetchLicense = async () => {
  try {
    loading.value = true
    error.value = null
    license.value = await getLicenseStatus()
  } catch (err: any) {
    // If 404, means no license activated (Community tier)
    if (err.response?.status === 404) {
      license.value = null
      showActivateForm.value = true
    } else {
      error.value = err.response?.data?.error || t('license.loadingFailed')
    }
    console.error('Failed to fetch license:', err)
  } finally {
    loading.value = false
  }
}

const handleActivate = async () => {
  if (!licenseKey.value.trim()) {
    error.value = t('errors.required')
    return
  }

  try {
    activating.value = true
    error.value = null
    success.value = null

    const response = await activateLicense(licenseKey.value)

    success.value = t('license.activationSuccess', { tier: response.tier })
    licenseKey.value = ''
    showActivateForm.value = false

    // Refresh license status
    await fetchLicense()
  } catch (err: any) {
    error.value = err.response?.data?.error || t('license.activationFailed')
    console.error('Activation error:', err)
  } finally {
    activating.value = false
  }
}

const handleDeactivate = async () => {
  if (!confirm(t('license.deactivateConfirm', { limit: 30 }))) {
    return
  }

  try {
    deactivating.value = true
    error.value = null
    success.value = null

    await deactivateLicense()

    success.value = t('license.deactivationSuccess')
    license.value = null
    showActivateForm.value = true
  } catch (err: any) {
    error.value = err.response?.data?.error || t('license.deactivationFailed')
    console.error('Deactivation error:', err)
  } finally {
    deactivating.value = false
  }
}

const tierColor = computed(() => {
  if (!license.value) return 'bg-gray-100 text-gray-800'

  switch (license.value.license.tier.toLowerCase()) {
    case 'enterprise':
      return 'bg-purple-100 text-purple-800'
    case 'pro':
      return 'bg-blue-100 text-blue-800'
    case 'community':
      return 'bg-gray-100 text-gray-800'
    default:
      return 'bg-gray-100 text-gray-800'
  }
})

const usagePercentage = computed(() => {
  if (!license.value || license.value.license.calendar_limit === 0) return 0
  return Math.round((license.value.usage / license.value.license.calendar_limit) * 100)
})

const usageColor = computed(() => {
  if (usagePercentage.value >= 90) return 'text-red-600'
  if (usagePercentage.value >= 75) return 'text-orange-600'
  return 'text-green-600'
})

const barColor = computed(() => {
  if (usagePercentage.value >= 90) return 'bg-red-500'
  if (usagePercentage.value >= 75) return 'bg-orange-500'
  return 'bg-green-500'
})

const copySupportKey = async () => {
  if (!license.value?.license.support_key) return

  try {
    await navigator.clipboard.writeText(license.value.license.support_key)
    success.value = t('license.supportKeyCopied')
    setTimeout(() => {
      success.value = null
    }, 3000)
  } catch (err) {
    error.value = t('license.copyFailed')
    console.error('Copy error:', err)
  }
}

const handleFileSelect = async (event: Event) => {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (file) {
    await readLicenseFile(file)
  }
}

const handleFileDrop = async (event: DragEvent) => {
  event.preventDefault()
  isDragging.value = false

  const file = event.dataTransfer?.files?.[0]
  if (file) {
    await readLicenseFile(file)
  }
}

const readLicenseFile = async (file: File) => {
  try {
    error.value = null

    // Check if it's a JSON file
    if (!file.name.endsWith('.json')) {
      error.value = t('license.invalidFileType')
      return
    }

    const text = await file.text()

    // Validate JSON
    try {
      JSON.parse(text)
      licenseKey.value = text
      success.value = t('license.fileLoaded')
      setTimeout(() => {
        success.value = null
      }, 3000)
    } catch (_e) {
      error.value = t('license.invalidJson')
    }
  } catch (err) {
    error.value = t('license.fileReadError')
    console.error('File read error:', err)
  }
}

const handleDragOver = (event: DragEvent) => {
  event.preventDefault()
  isDragging.value = true
}

const handleDragLeave = () => {
  isDragging.value = false
}

const triggerFileInput = () => {
  fileInputRef.value?.click()
}

onMounted(() => {
  fetchLicense()
})
</script>

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-4xl">
      <!-- Header -->
      <div class="mb-8">
        <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white mb-2">
          {{ t('license.title') }}
        </h1>
        <p class="text-gray-600 dark:text-gray-400">
          {{ t('license.subtitle') }}
        </p>
      </div>

      <!-- Success message -->
      <div
        v-if="success"
        class="mb-6 p-4 bg-green-50 border border-green-200 rounded-lg text-green-800"
      >
        <div class="flex items-start">
          <svg
            class="w-5 h-5 mr-2 mt-0.5"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
              clip-rule="evenodd"
            />
          </svg>
          <p>{{ success }}</p>
        </div>
      </div>

      <!-- Error message -->
      <div
        v-if="error"
        class="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg text-red-800"
      >
        <div class="flex items-start">
          <svg
            class="w-5 h-5 mr-2 mt-0.5"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
              clip-rule="evenodd"
            />
          </svg>
          <p>{{ error }}</p>
        </div>
      </div>

      <!-- Loading state -->
      <div
        v-if="loading"
        class="card flex items-center justify-center py-12"
      >
        <svg
          class="h-8 w-8 animate-spin text-primary-600"
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
      </div>

      <!-- Current License Display -->
      <div
        v-else-if="license && !showActivateForm"
        class="space-y-6"
      >
        <div class="card">
          <div class="flex items-center justify-between mb-6">
            <h2 class="font-display text-2xl font-semibold text-gray-900 dark:text-white">
              {{ t('license.currentLicense') }}
            </h2>
            <span
              :class="tierColor"
              class="px-3 py-1 rounded-full text-sm font-medium"
            >
              {{
                t('license.tier', {
                  tier: t('license.' + license.license.tier),
                })
              }}
            </span>
          </div>

          <div class="space-y-4">
            <!-- Issued To -->
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                {{ t('license.issuedTo') }}
              </label>
              <p class="text-gray-900 dark:text-white">
                {{ license.license.issued_to }}
              </p>
            </div>

            <!-- Support Key -->
            <div v-if="license.license.support_key">
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                {{ t('license.supportKey') }}
              </label>
              <div class="flex items-center space-x-2">
                <code
                  class="flex-1 px-3 py-2 bg-gray-100 dark:bg-gray-800 rounded border border-gray-300 dark:border-gray-600 text-sm font-mono text-gray-900 dark:text-white"
                >
                  {{ license.license.support_key }}
                </code>
                <button
                  class="btn btn-ghost px-3 py-2 text-sm"
                  :title="t('license.copy')"
                  @click="copySupportKey"
                >
                  <svg
                    class="w-4 h-4"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                    />
                  </svg>
                </button>
              </div>
              <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
                {{ t('license.supportKeyHelp') }}
              </p>
            </div>

            <!-- Calendar Usage -->
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                {{ t('license.serverCalendarUsage') }}
              </label>
              <div class="flex items-center justify-between text-sm mb-2">
                <span
                  :class="usageColor"
                  class="font-semibold"
                >
                  {{ license.usage }} /
                  {{
                    license.license.calendar_limit === 0
                      ? t('quota.unlimited')
                      : license.license.calendar_limit
                  }}
                </span>
                <span
                  v-if="license.license.calendar_limit > 0"
                  class="text-gray-600 dark:text-gray-400"
                >
                  {{ usagePercentage }}%
                </span>
              </div>
              <div
                v-if="license.license.calendar_limit > 0"
                class="w-full bg-gray-200 rounded-full h-2"
              >
                <div
                  :class="barColor"
                  class="h-2 rounded-full transition-all duration-300"
                  :style="{ width: `${usagePercentage}%` }"
                />
              </div>
            </div>

            <!-- License Type -->
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                {{ t('license.licenseType') }}
              </label>
              <p class="text-green-600 font-medium">
                {{ t('license.perpetualLicense') }}
              </p>
            </div>

            <!-- Support Status -->
            <div v-if="license.license.support_expires_at">
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                {{ t('license.supportStatus') }}
              </label>
              <div
                v-if="license.support_active"
                class="flex items-center text-green-600"
              >
                <svg
                  class="w-5 h-5 mr-2"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    fill-rule="evenodd"
                    d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                    clip-rule="evenodd"
                  />
                </svg>
                <span>{{
                  t('license.supportActiveUntil', {
                    date: new Date(license.license.support_expires_at).toLocaleDateString(),
                  })
                }}</span>
              </div>
              <div
                v-else
                class="flex items-center text-orange-600"
              >
                <svg
                  class="w-5 h-5 mr-2"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    fill-rule="evenodd"
                    d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
                    clip-rule="evenodd"
                  />
                </svg>
                <span>{{
                  t('license.supportExpiredOn', {
                    date: new Date(license.license.support_expires_at).toLocaleDateString(),
                  })
                }}</span>
              </div>
            </div>
          </div>

          <!-- Actions -->
          <div class="mt-6 pt-6 border-t border-gray-200 dark:border-gray-700 flex space-x-4">
            <button
              class="btn btn-primary"
              @click="showActivateForm = true"
            >
              {{ t('license.updateLicense') }}
            </button>
            <button
              v-if="license.license.tier.toLowerCase() !== 'community'"
              :disabled="deactivating"
              class="btn btn-secondary"
              @click="handleDeactivate"
            >
              <span v-if="deactivating">{{ t('license.deactivating') }}</span>
              <span v-else>{{ t('license.deactivateLicense') }}</span>
            </button>
          </div>
        </div>
      </div>

      <!-- Activate License Form -->
      <div
        v-else
        class="card"
      >
        <h2 class="font-display text-2xl font-semibold text-gray-900 dark:text-white mb-6">
          {{ t('license.activateLicense') }}
        </h2>

        <div class="mb-6 p-4 bg-blue-50 border border-blue-200 rounded-lg text-blue-800 text-sm">
          <p class="font-medium mb-2">
            ℹ️ {{ t('license.noActiveLicense') }}
          </p>
          <p v-html="t('license.communityTierMessage', { limit: 30 })" />
        </div>

        <form
          class="space-y-6"
          @submit.prevent="handleActivate"
        >
          <!-- File Upload Zone -->
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              {{ t('license.uploadLicenseFile') }}
            </label>
            <div
              :class="{
                'border-primary-500 bg-primary-50 dark:bg-primary-900/20': isDragging,
                'border-gray-300 dark:border-gray-600': !isDragging,
              }"
              class="relative border-2 border-dashed rounded-lg p-8 text-center cursor-pointer transition-all hover:border-primary-400 hover:bg-gray-50 dark:hover:bg-gray-800/50"
              @click="triggerFileInput"
              @dragover="handleDragOver"
              @dragleave="handleDragLeave"
              @drop="handleFileDrop"
            >
              <input
                ref="fileInputRef"
                type="file"
                accept=".json"
                class="hidden"
                @change="handleFileSelect"
              >
              <div class="flex flex-col items-center justify-center space-y-3">
                <svg
                  class="w-12 h-12 text-gray-400 dark:text-gray-500"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"
                  />
                </svg>
                <div class="space-y-1">
                  <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('license.dragDropFile') }}
                  </p>
                  <p class="text-xs text-gray-500 dark:text-gray-400">
                    {{ t('license.clickToSelect') }}
                  </p>
                </div>
                <p class="text-xs text-gray-400 dark:text-gray-500">
                  {{ t('license.jsonFileOnly') }}
                </p>
              </div>
            </div>
          </div>

          <!-- Or Manual Paste -->
          <div class="relative">
            <div
              class="absolute inset-0 flex items-center"
              aria-hidden="true"
            >
              <div class="w-full border-t border-gray-300 dark:border-gray-600" />
            </div>
            <div class="relative flex justify-center text-sm">
              <span class="px-2 bg-white dark:bg-gray-900 text-gray-500 dark:text-gray-400">
                {{ t('license.orPasteManually') }}
              </span>
            </div>
          </div>

          <div>
            <label
              for="licenseKey"
              class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
            >
              {{ t('license.licenseKeyLabel') }}
            </label>
            <textarea
              id="licenseKey"
              v-model="licenseKey"
              rows="8"
              class="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 font-mono text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-white"
              placeholder="{&quot;tier&quot;:&quot;standard&quot;,&quot;calendar_limit&quot;:100,&quot;issued_to&quot;:&quot;Your Company&quot;,...}"
              required
            />
            <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
              {{ t('license.licenseKeyPlaceholder') }}
            </p>
          </div>

          <div class="flex space-x-4">
            <button
              type="submit"
              :disabled="activating"
              class="btn btn-primary"
            >
              <span v-if="activating">{{ t('license.activating') }}</span>
              <span v-else>{{ t('license.activate') }}</span>
            </button>
            <button
              v-if="license"
              type="button"
              class="btn btn-ghost"
              @click="showActivateForm = false"
            >
              {{ t('common.cancel') }}
            </button>
          </div>
        </form>
      </div>

      <!-- Back to admin -->
      <div class="mt-8 text-center">
        <button
          class="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white transition-colors"
          @click="router.push('/admin')"
        >
          {{ t('license.backToAdmin') }}
        </button>
      </div>
    </div>
  </div>
</template>
