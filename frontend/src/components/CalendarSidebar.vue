<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <Teleport to="body">
    <div
      v-if="isOpen && calendars.length > 0"
      class="fixed left-0 top-0 z-60 h-full w-64 transform bg-white shadow-lg transition-transform dark:bg-gray-900"
      :class="{ 'translate-x-0': isOpen, '-translate-x-full': !isOpen }"
    >
      <!-- Header -->
      <div
        class="flex items-center justify-between border-b border-gray-200 p-4 dark:border-gray-700"
      >
        <h2 class="font-display text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('calendar.visitedCalendars', 'Mes calendriers') }}
        </h2>
        <button
          class="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
          :title="t('common.close', 'Close')"
          @click="close"
        >
          <svg
            class="h-5 w-5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M6 18L18 6M6 6l12 12"
            />
          </svg>
        </button>
      </div>

      <!-- Calendar List -->
      <div
        class="overflow-y-auto p-4"
        style="max-height: calc(100vh - 140px)"
      >
        <div class="space-y-2">
          <router-link
            v-for="calendar in calendars"
            :key="calendar.token"
            :to="getCalendarLink(calendar)"
            class="block rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 transition-all hover:border-primary-500 hover:bg-primary-50 dark:border-gray-700 dark:bg-gray-800 dark:hover:border-primary-500 dark:hover:bg-primary-900/20"
            :class="{
              'border-primary-500 bg-primary-50 dark:border-primary-500 dark:bg-primary-900/30':
                currentToken === calendar.token,
            }"
            @click="close"
          >
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                  {{ calendar.name }}
                </p>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ formatLastVisited(calendar.lastVisited) }}
                </p>
              </div>
              <button
                class="shrink-0 text-gray-400 hover:text-danger-600 dark:hover:text-danger-400"
                :title="t('common.remove', 'Remove')"
                @click.prevent.stop="handleRemove(calendar.token)"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              </button>
            </div>
          </router-link>
        </div>
      </div>

      <!-- Footer -->
      <div
        class="absolute bottom-0 left-0 right-0 border-t border-gray-200 p-4 dark:border-gray-700"
      >
        <button
          class="w-full text-center text-xs text-gray-600 hover:text-danger-600 dark:text-gray-400 dark:hover:text-danger-400"
          @click="handleClearAll"
        >
          {{ t('calendar.clearHistory', "Effacer l'historique") }}
        </button>
      </div>
    </div>

    <!-- Overlay -->
    <div
      v-if="isOpen && calendars.length > 0"
      class="fixed inset-0 z-50 bg-black/25"
      @click="close"
    />

    <!-- Toggle Button (always visible on public pages) -->
    <button
      v-if="shouldShowButton"
      class="fixed left-4 top-20 z-50 flex h-12 w-12 items-center justify-center rounded-full bg-primary-600 text-white shadow-lg transition-all hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600"
      :title="t('calendar.showCalendars', 'Show calendars')"
      @click="toggle"
    >
      <svg
        class="h-6 w-6"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
        />
      </svg>
      <span
        v-if="calendars.length > 0"
        class="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full bg-danger-600 text-xs font-bold text-white"
      >
        {{ calendars.length }}
      </span>
    </button>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCalendarHistoryStore, type CalendarHistoryItem } from '@/stores/calendarHistory'

const { t } = useI18n()
const route = useRoute()
const historyStore = useCalendarHistoryStore()

const currentToken = computed(() => {
  return route.params.token as string
})

const calendars = computed(() => historyStore.calendars)
const isOpen = computed(() => historyStore.isOpen)

const shouldShowButton = computed(() => {
  return calendars.value.length > 0
})

function getCalendarLink(calendar: CalendarHistoryItem): string {
  if (calendar.participantId) {
    return `/c/${calendar.token}/p/${calendar.participantId}`
  }
  return `/c/${calendar.token}`
}

function toggle() {
  historyStore.toggle()
}

function close() {
  historyStore.close()
}

function handleRemove(token: string) {
  if (confirm(t('calendar.confirmRemoveHistory', 'Remove this calendar from history?'))) {
    historyStore.removeCalendar(token)
  }
}

function handleClearAll() {
  if (confirm(t('calendar.confirmClearHistory', 'Clear all history?'))) {
    historyStore.clearHistory()
    close()
  }
}

function formatLastVisited(timestamp: number): string {
  const now = Date.now()
  const diff = now - timestamp

  const seconds = Math.floor(diff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  if (days > 0) {
    return t('calendar.daysAgo', { days })
  } else if (hours > 0) {
    return t('calendar.hoursAgo', { hours })
  } else if (minutes > 0) {
    return t('calendar.minutesAgo', { minutes })
  } else {
    return t('calendar.justNow')
  }
}

// Initialize calendar history when component is mounted
onMounted(() => {
  historyStore.init()
})
</script>
