<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app">
      <!-- Header with back button -->
      <div class="mb-8">
        <button
          class="mb-4 inline-flex items-center gap-2 text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
          @click="router.back()"
        >
          <svg
            class="h-5 w-5"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M10 19l-7-7m0 0l7-7m-7 7h18"
            />
          </svg>
          {{ t('admin.backToUsers') }}
        </button>

        <h1 class="mb-2 font-display text-3xl font-bold text-gray-900 dark:text-white">
          {{
            userName
              ? t('admin.userCalendars', { name: userName })
              : t('admin.userCalendars', { name: '...' })
          }}
        </h1>
        <p class="text-gray-600 dark:text-gray-400">
          {{ calendars.length }} {{ t('admin.calendarsCount').toLowerCase() }}
        </p>
      </div>

      <!-- Loading state -->
      <div
        v-if="loading"
        class="card"
      >
        <div class="flex items-center justify-center py-12">
          <div
            class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"
          />
          <span class="ml-3 text-gray-600 dark:text-gray-400">{{ t('common.loading') }}</span>
        </div>
      </div>

      <!-- Empty state -->
      <div
        v-else-if="calendars.length === 0"
        class="card"
      >
        <div class="text-center py-12">
          <svg
            class="mx-auto h-12 w-12 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
            />
          </svg>
          <p class="mt-4 text-gray-500 dark:text-gray-400">
            {{ t('admin.noCalendars') }}
          </p>
        </div>
      </div>

      <!-- Calendars grid -->
      <div
        v-else
        class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3"
      >
        <div
          v-for="calendar in calendars"
          :key="calendar.id"
          class="card hover:border-primary-200 dark:hover:border-primary-800 transition-colors"
        >
          <!-- Calendar header -->
          <div class="mb-4">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ calendar.name }}
            </h3>
            <p
              v-if="calendar.description"
              class="mt-1 text-sm text-gray-500 dark:text-gray-400"
            >
              {{ calendar.description }}
            </p>
          </div>

          <!-- Calendar stats -->
          <div class="space-y-2 text-sm">
            <div class="flex items-center justify-between">
              <span class="text-gray-600 dark:text-gray-400">{{ t('calendar.participants') }}:</span>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ calendar.participants?.length || 0 }}
              </span>
            </div>

            <div class="flex items-center justify-between">
              <span class="text-gray-600 dark:text-gray-400">{{ t('calendar.threshold') }}:</span>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ calendar.threshold }}
              </span>
            </div>

            <div class="flex items-center justify-between">
              <span class="text-gray-600 dark:text-gray-400">{{ t('admin.createdAt') }}:</span>
              <span class="text-gray-900 dark:text-white">
                {{ formatDate(calendar.created_at) }}
              </span>
            </div>
          </div>

          <!-- Participants list -->
          <div
            v-if="calendar.participants && calendar.participants.length > 0"
            class="mt-4 pt-4 border-t border-gray-200 dark:border-gray-700"
          >
            <p class="text-xs font-medium text-gray-600 dark:text-gray-400 mb-2">
              {{ t('calendar.participants') }}:
            </p>
            <div class="flex flex-wrap gap-1">
              <span
                v-for="participant in calendar.participants.slice(0, 5)"
                :key="participant.id"
                class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-gray-700 dark:text-gray-300"
              >
                {{ participant.name }}
              </span>
              <span
                v-if="calendar.participants.length > 5"
                class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-gray-700 dark:text-gray-400"
              >
                +{{ calendar.participants.length - 5 }}
              </span>
            </div>
          </div>

          <!-- Actions -->
          <div class="mt-4 flex gap-2">
            <router-link
              :to="{ name: 'calendar-settings', params: { id: calendar.id } }"
              class="btn btn-secondary flex-1 text-center text-sm"
            >
              {{ t('common.edit') }}
            </router-link>
            <a
              :href="`/c/${calendar.public_token}`"
              target="_blank"
              class="btn btn-secondary text-sm"
              :title="t('calendar.publicLink')"
            >
              <svg
                class="h-4 w-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                />
              </svg>
            </a>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { adminApi } from '@/api/admin'
import type { CalendarWithParticipants } from '@/types'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const toastStore = useToastStore()

const loading = ref(true)
const calendars = ref<CalendarWithParticipants[]>([])
const userName = ref<string>('')

const userId = computed(() => route.params.userId as string)

onMounted(() => {
  // Get user name from query string
  userName.value = (route.query.userName as string) || ''
  loadCalendars()
})

async function loadCalendars() {
  if (!userId.value) {
    toastStore.error('Invalid user ID')
    loading.value = false
    router.push('/admin')
    return
  }

  loading.value = true

  try {
    // Load calendars
    calendars.value = await adminApi.getUserCalendars(userId.value)

    // Try to get user name from admin users list (if available)
    // Otherwise we could make an additional API call to get user details
    // For now, we'll just show the count
  } catch (err: any) {
    console.error('Failed to load calendars:', err)
    toastStore.error(err.message || t('errors.generic'))
  } finally {
    loading.value = false
  }
}

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString(authStore.user?.locale || 'fr', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
</script>
