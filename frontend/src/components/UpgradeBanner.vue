<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { getQuotaStatus, type QuotaStatus } from '../api/quota'
import { useAuthStore } from '../stores/auth'

const { t } = useI18n()
const authStore = useAuthStore()
const quota = ref<QuotaStatus | null>(null)
const dismissed = ref(false)

const fetchQuota = async () => {
  try {
    quota.value = await getQuotaStatus()
  } catch (err) {
    console.error('Failed to fetch quota:', err)
  }
}

const limit = computed(() => {
  if (!quota.value) return 0
  if (quota.value.limitation_type === 'per_user') {
    return quota.value.user_limit ?? 0
  } else if (quota.value.limitation_type === 'per_server') {
    return quota.value.server_limit ?? 0
  }
  return 0
})

const usage = computed(() => {
  if (!quota.value) return 0
  if (quota.value.limitation_type === 'per_user') {
    return quota.value.user_usage ?? 0
  } else if (quota.value.limitation_type === 'per_server') {
    return quota.value.server_usage ?? 0
  }
  return 0
})

const isUnlimited = computed(() => limit.value === 0)

const percentage = computed(() => {
  if (isUnlimited.value) return 0
  return Math.round((usage.value / limit.value) * 100)
})

const isCloudMode = computed(() => {
  return quota.value?.limitation_type === 'per_user'
})

const isAdmin = computed(() => {
  return authStore.user?.role === 'admin'
})

const canUpgrade = computed(() => {
  // In cloud mode, anyone can upgrade their own subscription
  // In self-hosted mode, only admins can activate licenses
  return isCloudMode.value || isAdmin.value
})

const shouldShow = computed(() => {
  return !dismissed.value && !isUnlimited.value && percentage.value >= 75
})

const bannerType = computed(() => {
  if (percentage.value >= 100) return 'critical'
  if (percentage.value >= 90) return 'warning'
  return 'info'
})

const bannerClass = computed(() => {
  switch (bannerType.value) {
    case 'critical':
      return 'bg-red-50 border-red-300 text-red-800'
    case 'warning':
      return 'bg-orange-50 border-orange-300 text-orange-800'
    case 'info':
    default:
      return 'bg-blue-50 border-blue-300 text-blue-800'
  }
})

const bannerIcon = computed(() => {
  switch (bannerType.value) {
    case 'critical':
      return '🚨'
    case 'warning':
      return '⚠️'
    case 'info':
    default:
      return '💡'
  }
})

const bannerMessage = computed(() => {
  const params = { usage: usage.value, limit: limit.value, percentage: percentage.value }

  if (percentage.value >= 100) {
    if (isCloudMode.value) {
      return t('dashboard.bannerReachedUser', params)
    } else {
      return t('dashboard.bannerReachedServer', params)
    }
  } else if (percentage.value >= 90) {
    if (isCloudMode.value) {
      return t('dashboard.bannerWarningUser', params)
    } else {
      return t('dashboard.bannerWarningServer', params)
    }
  } else {
    if (isCloudMode.value) {
      return t('dashboard.bannerInfoUser', params)
    } else {
      return t('dashboard.bannerInfoServer', params)
    }
  }
})

const dismiss = () => {
  dismissed.value = true
  // Store dismissal in localStorage for session
  localStorage.setItem('upgradeBannerDismissed', 'true')
}

onMounted(() => {
  fetchQuota()
  // Check if banner was previously dismissed this session
  if (localStorage.getItem('upgradeBannerDismissed') === 'true') {
    dismissed.value = true
  }
})
</script>

<template>
  <div
    v-if="shouldShow"
    :class="bannerClass"
    class="border rounded-lg p-4 mb-4"
  >
    <div class="flex items-start justify-between">
      <div class="flex items-start space-x-3 flex-1">
        <span class="text-2xl">{{ bannerIcon }}</span>
        <div class="flex-1">
          <p class="font-medium">
            {{ bannerMessage }}
          </p>
          <div class="mt-2">
            <a
              v-if="canUpgrade && quota?.upgrade_url"
              :href="quota.upgrade_url"
              class="inline-flex items-center px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors text-sm font-medium"
            >
              {{ t('quota.upgradeNow') }} →
            </a>
            <p
              v-else
              class="text-sm"
            >
              {{ t('dashboard.contactAdmin') }}
            </p>
          </div>
        </div>
      </div>
      <button
        class="ml-4 text-gray-400 hover:text-gray-600 transition-colors"
        :aria-label="t('common.close')"
        @click="dismiss"
      >
        <svg
          class="w-5 h-5"
          fill="currentColor"
          viewBox="0 0 20 20"
        >
          <path
            fill-rule="evenodd"
            d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
            clip-rule="evenodd"
          />
        </svg>
      </button>
    </div>
  </div>
</template>
