<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { getQuotaStatus, type QuotaStatus } from '../api/quota'

const { t } = useI18n()
const quota = ref<QuotaStatus | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)

const fetchQuota = async () => {
  try {
    loading.value = true
    error.value = null
    quota.value = await getQuotaStatus()
  } catch (err: any) {
    error.value = err.response?.data?.error || t('quota.errorLoading')
    console.error('Failed to fetch quota:', err)
  } finally {
    loading.value = false
  }
}

// Compute display values based on limitation_type
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
  } else if (
    quota.value.limitation_type === 'per_server' ||
    quota.value.limitation_type === 'none'
  ) {
    return quota.value.server_usage ?? 0
  }
  return 0
})

const isUnlimited = computed(() => limit.value === 0)

const percentage = computed(() => {
  if (isUnlimited.value) return 0
  return Math.round((usage.value / limit.value) * 100)
})

const usageColor = computed(() => {
  if (isUnlimited.value) return 'text-green-600'
  if (percentage.value >= 90) return 'text-red-600'
  if (percentage.value >= 75) return 'text-orange-600'
  return 'text-green-600'
})

const barColor = computed(() => {
  if (isUnlimited.value) return 'bg-green-500'
  if (percentage.value >= 90) return 'bg-red-500'
  if (percentage.value >= 75) return 'bg-orange-500'
  return 'bg-green-500'
})

const quotaLabel = computed(() => {
  if (!quota.value) return ''
  if (quota.value.limitation_type === 'per_user') {
    return t('quota.yourCalendars')
  } else if (quota.value.limitation_type === 'per_server') {
    return t('quota.serverCalendars')
  }
  return t('quota.calendars')
})

onMounted(() => {
  fetchQuota()
})
</script>

<template>
  <div class="p-4 bg-white border border-gray-200 rounded-lg dark:bg-gray-900 dark:border-gray-800">
    <!-- Loading state -->
    <div
      v-if="loading"
      class="text-gray-500 text-sm dark:text-gray-400"
    >
      {{ t('common.loading') }}
    </div>

    <!-- Error state -->
    <div
      v-else-if="error"
      class="text-red-600 text-sm dark:text-red-400"
    >
      {{ error }}
    </div>

    <!-- Quota display -->
    <div
      v-else-if="quota"
      class="space-y-2"
    >
      <!-- Header -->
      <div class="flex items-center justify-between text-sm">
        <span class="text-gray-700 font-medium dark:text-gray-300">{{ quotaLabel }}</span>
        <span
          :class="usageColor"
          class="font-semibold"
        >
          <template v-if="isUnlimited"> {{ usage }} / {{ t('quota.unlimited') }} </template>
          <template v-else> {{ usage }} / {{ limit }} </template>
        </span>
      </div>

      <!-- Progress bar (only if not unlimited) -->
      <div
        v-if="!isUnlimited"
        class="w-full bg-gray-200 rounded-full h-2 dark:bg-gray-700"
      >
        <div
          :class="barColor"
          class="h-2 rounded-full transition-all duration-300"
          :style="{ width: `${percentage}%` }"
        />
      </div>

      <!-- Warning message -->
      <div
        v-if="!isUnlimited && percentage >= 90"
        class="text-xs text-red-600 mt-1 dark:text-red-400"
      >
        ⚠️ {{ t('quota.approaching') }}
      </div>
      <div
        v-else-if="!isUnlimited && percentage >= 75"
        class="text-xs text-orange-600 mt-1 dark:text-orange-400"
      >
        ⚠️ {{ t('quota.using', { percentage }) }}
      </div>
    </div>
  </div>
</template>
