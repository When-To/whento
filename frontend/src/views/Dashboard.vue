<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app">
      <!-- Header -->
      <div class="mb-8 flex items-center justify-between">
        <div>
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ t('calendar.myCalendars') }}
          </h1>
          <p class="mt-1 text-gray-600 dark:text-gray-400">
            {{ t('common.welcome', { name: user?.display_name }) }}
          </p>
        </div>
        <router-link
          to="/calendars/new"
          class="btn btn-primary"
        >
          <svg
            class="mr-2 h-5 w-5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 4v16m8-8H4"
            />
          </svg>
          {{ t('calendar.newCalendar') }}
        </router-link>
      </div>

      <!-- Upgrade Banner -->
      <UpgradeBanner />

      <!-- Quota Usage -->
      <div class="mb-6">
        <QuotaUsage />
      </div>

      <!-- Loading State -->
      <div
        v-if="loading"
        class="flex items-center justify-center py-12"
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

      <!-- Error State -->
      <div
        v-else-if="fetchError"
        class="flex flex-col items-center justify-center py-12"
      >
        <div
          class="mb-6 flex h-24 w-24 items-center justify-center rounded-full bg-danger-100 dark:bg-danger-900"
        >
          <svg
            class="h-12 w-12 text-danger-600 dark:text-danger-400"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
        </div>
        <h2 class="mb-2 font-display text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('errors.generic') }}
        </h2>
        <p class="mb-6 text-gray-600 dark:text-gray-400">
          {{ fetchError }}
        </p>
        <button
          class="btn btn-primary"
          @click="loadCalendars()"
        >
          {{ t('common.retry', 'Retry') }}
        </button>
      </div>

      <!-- Empty State -->
      <div
        v-else-if="calendars.length === 0"
        class="flex flex-col items-center justify-center py-12"
      >
        <div
          class="mb-6 flex h-24 w-24 items-center justify-center rounded-full bg-gray-100 dark:bg-gray-800"
        >
          <svg
            class="h-12 w-12 text-gray-400"
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
        </div>
        <h2 class="mb-2 font-display text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('calendar.noCalendars') }}
        </h2>
        <p class="mb-6 text-gray-600 dark:text-gray-400">
          {{ t('calendar.createFirstCalendar') }}
        </p>
        <router-link
          to="/calendars/new"
          class="btn btn-primary"
        >
          <svg
            class="mr-2 h-5 w-5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 4v16m8-8H4"
            />
          </svg>
          {{ t('calendar.newCalendar') }}
        </router-link>
      </div>

      <!-- Calendar Grid -->
      <div
        v-else
        class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3"
      >
        <div
          v-for="calendar in calendars"
          :key="calendar.id"
          class="card card-hover group cursor-pointer"
          @click="router.push(`/c/${calendar.public_token}`)"
        >
          <!-- Calendar Header -->
          <div class="mb-4 flex items-start justify-between">
            <div class="flex-1">
              <h3
                class="mb-1 font-display text-xl font-semibold text-gray-900 group-hover:text-primary-600 dark:text-white dark:group-hover:text-primary-400"
              >
                {{ calendar.name }}
              </h3>
              <p
                v-if="calendar.description"
                class="text-sm text-gray-600 dark:text-gray-400"
              >
                {{ calendar.description }}
              </p>
            </div>
            <div
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-400"
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
                  d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                />
              </svg>
            </div>
          </div>

          <!-- Calendar Stats -->
          <div class="mb-4 flex items-center space-x-4 text-sm">
            <div class="flex items-center text-gray-600 dark:text-gray-400">
              <svg
                class="mr-1 h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
                />
              </svg>
              <span>{{ calendar.participants?.length || 0 }} {{ t('calendar.participantCount') }}</span>
            </div>
          </div>

          <!-- Quick Actions -->
          <div class="flex items-center space-x-2">
            <button
              class="btn btn-ghost btn-sm flex-1"
              :title="t('calendar.copyLink')"
              @click.stop="copyPublicLink(calendar.public_token)"
            >
              <svg
                class="mr-1 h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                />
              </svg>
              {{ t('common.copy', 'Copy') }}
            </button>
            <button
              class="btn btn-ghost btn-sm"
              :title="t('common.settings', 'Settings')"
              @click.stop="router.push(`/calendars/${calendar.id}/settings`)"
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
                  d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
                />
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useCalendarStore } from '@/stores/calendar'
import { useToastStore } from '@/stores/toast'
import QuotaUsage from '@/components/QuotaUsage.vue'
import UpgradeBanner from '@/components/UpgradeBanner.vue'

const router = useRouter()
const { t } = useI18n()
const authStore = useAuthStore()
const calendarStore = useCalendarStore()
const toastStore = useToastStore()

const user = computed(() => authStore.user)
const calendars = computed(() => {
  const cals = calendarStore.calendars
  return Array.isArray(cals) ? cals.filter(c => c != null) : []
})
const loading = computed(() => calendarStore.loading)
const fetchError = ref<string | null>(null)

function copyPublicLink(token: string) {
  const url = `${window.location.origin}/c/${token}`
  navigator.clipboard.writeText(url)
  toastStore.success(t('common.linkCopied'))
}

async function loadCalendars() {
  fetchError.value = null
  try {
    await calendarStore.fetchCalendars()
  } catch (error: any) {
    fetchError.value = error.message || 'Failed to load calendars'
    console.error('Error loading calendars:', error)
  }
}

onMounted(() => {
  loadCalendars()
})
</script>
