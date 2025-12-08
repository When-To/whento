<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-screen bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-4xl">
      <!-- Loading State -->
      <div
        v-if="loading"
        class="flex items-center justify-center py-12"
      >
        <div class="text-center">
          <svg
            class="mx-auto h-12 w-12 animate-spin text-primary-600"
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
          <p class="mt-4 text-sm text-gray-600 dark:text-gray-400">
            {{ t('common.loading') }}
          </p>
        </div>
      </div>

      <!-- Calendar Content -->
      <template v-else-if="calendar">
        <!-- Header -->
        <div class="mb-8">
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ calendar.name }}
          </h1>
          <p
            v-if="calendar.description"
            class="mt-2 text-gray-600 dark:text-gray-400"
          >
            {{ calendar.description }}
          </p>
          <div class="mt-4 flex items-center gap-4 text-sm text-gray-500 dark:text-gray-400">
            <span class="flex items-center gap-1">
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
                  d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
                />
              </svg>
              {{ calendar.participants?.length || 0 }}
              {{ t('calendar.participantCount', 'participant(s)') }}
            </span>
            <span class="flex items-center gap-1">
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
                  d="M13 10V3L4 14h7v7l9-11h-7z"
                />
              </svg>
              {{ t('calendar.threshold') }}: {{ calendar.threshold }}
            </span>
          </div>
        </div>

        <!-- Participant Selection -->
        <div class="card">
          <!-- No participants -->
          <div
            v-if="!calendar.participants || calendar.participants.length === 0"
            class="text-center"
          >
            <div
              class="rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 p-8 dark:border-gray-700 dark:bg-gray-800"
            >
              <svg
                class="mx-auto h-16 w-16 text-gray-400"
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
              <h3 class="mt-4 text-lg font-medium text-gray-900 dark:text-white">
                {{ t('calendar.noParticipants', 'No participants') }}
              </h3>
              <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
                {{ t('calendar.noParticipantsDescription', 'This calendar has no participants yet. No availability can be entered at this time.') }}
              </p>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('calendar.contactOwnerToAddParticipants', 'Contact the calendar owner to add participants.') }}
              </p>
            </div>
          </div>

          <!-- Participant selection (when participants exist) -->
          <template v-else>
            <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('participant.whoAreYou') }}
            </h2>

            <!-- Locked participants message -->
            <div
              v-if="calendar.lock_participants"
              class="mb-6 rounded-lg bg-yellow-50 p-4 dark:bg-yellow-900/20"
            >
              <div class="flex">
                <svg
                  class="h-5 w-5 text-yellow-600 dark:text-yellow-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
                  />
                </svg>
                <p class="ml-3 text-sm text-yellow-700 dark:text-yellow-300">
                  {{
                    t(
                      'calendar.participantLockedMessage',
                      'This calendar requires a direct participant link. Contact the calendar owner to get your personal link.'
                    )
                  }}
                </p>
              </div>
            </div>

            <p
              v-else
              class="mb-6 text-sm text-gray-600 dark:text-gray-400"
            >
              {{ t('participant.selectYourName') }}
            </p>

            <!-- Participants List -->
            <div class="space-y-2">
              <!-- Locked: show as non-clickable -->
              <template v-if="calendar.lock_participants">
                <div
                  v-for="participant in calendar.participants"
                  :key="participant.id"
                  class="flex items-center gap-3 rounded-lg border border-gray-200 bg-gray-100 px-4 py-3 opacity-60 cursor-not-allowed dark:border-gray-700 dark:bg-gray-800"
                >
                  <div
                    class="flex h-10 w-10 items-center justify-center rounded-full bg-gray-200 dark:bg-gray-700"
                  >
                    <svg
                      class="h-5 w-5 text-gray-500 dark:text-gray-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
                      />
                    </svg>
                  </div>
                  <span class="flex-1 text-gray-600 dark:text-gray-400">{{ participant.name }}</span>
                  <svg
                    class="h-5 w-5 text-gray-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
                    />
                  </svg>
                </div>
              </template>

              <!-- Unlocked: show as clickable links -->
              <template v-else>
                <router-link
                  v-for="participant in calendar.participants"
                  :key="participant.id"
                  :to="`/c/${token}/p/${participant.id}`"
                  class="flex items-center gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 transition-all hover:border-primary-500 hover:bg-primary-50 dark:border-gray-700 dark:bg-gray-800 dark:hover:border-primary-500 dark:hover:bg-primary-900/20"
                >
                  <div
                    class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
                  >
                    <svg
                      class="h-5 w-5 text-primary-600 dark:text-primary-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
                      />
                    </svg>
                  </div>
                  <span class="flex-1 text-gray-900 dark:text-white">{{ participant.name }}</span>
                  <svg
                    class="h-5 w-5 text-gray-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M9 5l7 7-7 7"
                    />
                  </svg>
                </router-link>
              </template>
            </div>
          </template>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCalendarStore } from '@/stores/calendar'
import { useCalendarHistoryStore } from '@/stores/calendarHistory'
import { useToastStore } from '@/stores/toast'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const calendarStore = useCalendarStore()
const historyStore = useCalendarHistoryStore()
const toastStore = useToastStore()

const token = route.params.token as string
const loading = ref(false)

const calendar = computed(() => calendarStore.currentCalendar)

async function loadCalendar() {
  loading.value = true

  try {
    await calendarStore.fetchPublicCalendar(token)

    // Add calendar to history
    if (calendar.value) {
      historyStore.addCalendar(token, calendar.value.name)
    }

    // Check if there's a saved participant for this calendar
    const savedParticipantId = historyStore.getParticipantId(token)
    if (savedParticipantId && calendar.value && calendar.value.participants) {
      // Verify the participant still exists
      const participantExists = calendar.value.participants.some(p => p.id === savedParticipantId)
      if (participantExists) {
        // Redirect to the saved participant
        router.replace(`/c/${token}/p/${savedParticipantId}`)
        return
      } else {
        // Remove invalid saved participant
        historyStore.updateParticipantId(token, undefined)
      }
    }
  } catch (err: any) {
    toastStore.error(err.message || t('calendar.fetchError', 'Failed to load calendar'))
    // Remove invalid calendar from history and redirect to home
    historyStore.removeCalendar(token)
    router.push('/')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadCalendar()
})
</script>
