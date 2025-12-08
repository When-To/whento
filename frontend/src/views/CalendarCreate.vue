<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-6xl">
      <!-- Header -->
      <div class="mb-8">
        <router-link
          to="/dashboard"
          class="mb-4 inline-flex items-center text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
        >
          <svg
            class="mr-2 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 19l-7-7 7-7"
            />
          </svg>
          {{ t('common.back') }}
        </router-link>
        <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
          {{ t('calendar.newCalendar') }}
        </h1>
      </div>

      <!-- Form -->
      <form
        class="space-y-6"
        @submit.prevent="handleSubmit"
      >
        <!-- Calendar Info Card -->
        <div class="card">
          <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('calendar.calendarInfo') }}
          </h2>

          <div class="space-y-4">
            <!-- Name -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('calendar.calendarName') }}
                <span class="text-danger-600">*</span>
              </label>
              <input
                v-model="form.name"
                type="text"
                class="input"
                :class="{ 'border-danger-500': errors.name }"
                :placeholder="t('calendar.calendarNamePlaceholder')"
                required
              >
              <p
                v-if="errors.name"
                class="mt-1 text-sm text-danger-600"
              >
                {{ errors.name }}
              </p>
            </div>

            <!-- Description -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('calendar.description') }}
              </label>
              <textarea
                v-model="form.description"
                rows="3"
                class="input"
                :placeholder="t('calendar.descriptionPlaceholder')"
              />
            </div>

            <!-- Timezone -->
            <div>
              <TimezoneSelector
                v-model="form.timezone"
                :label="t('calendar.timezone')"
                :help="t('calendar.timezoneHelp')"
              />
            </div>
          </div>
        </div>

        <!-- Participants -->
        <CollapsibleSection
          :title="t('calendar.participants')"
          :default-open="true"
        >
          <!-- Participants List -->
          <div
            v-if="participants.length > 0"
            class="mb-4 space-y-2"
          >
            <div
              v-for="(_participant, index) in participants"
              :key="index"
              class="flex items-center gap-2"
            >
              <input
                v-model="participants[index]"
                type="text"
                class="input flex-1"
                :placeholder="t('calendar.participantName')"
                required
              >
              <button
                v-if="participants.length > 1"
                type="button"
                class="btn btn-ghost text-danger-600 hover:bg-danger-50 dark:hover:bg-danger-900/20"
                :title="t('common.delete')"
                @click="removeParticipant(index)"
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
                    d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                  />
                </svg>
              </button>
            </div>
          </div>

          <!-- Empty State -->
          <div
            v-else
            class="mb-4 rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 p-6 text-center dark:border-gray-700 dark:bg-gray-800"
          >
            <svg
              class="mx-auto h-12 w-12 text-gray-400"
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
            <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
              {{ t('calendar.noParticipants') }}
            </p>
          </div>

          <!-- Add Participant -->
          <div class="flex gap-2 mb-4">
            <input
              v-model="newParticipantName"
              type="text"
              class="input flex-1"
              :placeholder="t('calendar.participantNamePlaceholder')"
              @keyup.enter.prevent="addParticipant"
            >
            <button
              type="button"
              class="btn btn-secondary"
              @click="addParticipant"
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
              {{ t('calendar.addParticipant') }}
            </button>
          </div>

          <p
            v-if="errors.participants"
            class="mb-4 text-sm text-danger-600"
          >
            {{ errors.participants }}
          </p>

          <!-- Lock Participants Toggle -->
          <div
            class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
          >
            <div class="flex items-start">
              <input
                id="lock-participants-create"
                v-model="form.lock_participants"
                type="checkbox"
                class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
              >
              <label
                for="lock-participants-create"
                class="ml-2 text-sm text-gray-700 dark:text-gray-300"
              >
                <span class="font-medium">{{ t('calendar.lockParticipants') }}</span>
                <p class="text-gray-500 dark:text-gray-400">
                  {{ t('calendar.lockParticipantsHelp') }}
                </p>
              </label>
            </div>
          </div>
        </CollapsibleSection>

        <!-- Participant threshold and minimum duration -->
        <CollapsibleSection
          :title="
            locale === 'fr'
              ? 'Seuil de participants et durée minimale'
              : 'Participant threshold and minimum duration'
          "
          :default-open="true"
        >
          <!-- Threshold -->
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('calendar.threshold') }}
              <span class="text-danger-600">*</span>
            </label>
            <input
              v-model.number="form.threshold"
              type="number"
              min="1"
              :max="participants.length || undefined"
              class="input"
              :class="{ 'border-danger-500': errors.threshold }"
              required
            >
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('calendar.thresholdHelp') }}
              <span v-if="participants.length > 0">
                ({{ t('common.max') }}: {{ participants.length }})</span>
            </p>
            <p
              v-if="errors.threshold"
              class="mt-1 text-sm text-danger-600"
            >
              {{ errors.threshold }}
            </p>
          </div>

          <!-- Minimum Duration -->
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('calendar.minDurationHours') }}
            </label>
            <input
              v-model.number="form.min_duration_hours"
              type="number"
              min="0"
              class="input"
            >
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('calendar.minDurationHelp') }}
            </p>
          </div>
        </CollapsibleSection>

        <!-- Allow/block days/hours -->
        <CollapsibleSection
          :title="
            locale === 'fr' ? 'Autoriser/bloquer des jours/heures' : 'Allow/block days/hours'
          "
          :default-open="false"
        >
          <!-- Date Range -->
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('calendar.calendarDateRange') }}
            </label>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <input
                  v-model="form.start_date"
                  type="date"
                  class="input"
                  :placeholder="t('calendar.calendarStartDatePlaceholder')"
                >
              </div>
              <div>
                <input
                  v-model="form.end_date"
                  type="date"
                  class="input"
                  :placeholder="t('calendar.calendarEndDatePlaceholder')"
                >
              </div>
            </div>
            <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
              {{ t('calendar.calendarDateRangeHelp') }}
            </p>
          </div>

          <!-- Allowed Weekdays -->
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('calendar.allowedWeekdays') }}
            </label>

            <!-- Grid layout: 8 columns (1 for labels + 7 for days) -->
            <div
              class="grid grid-cols-[auto_repeat(7,minmax(0,1fr))] gap-2 items-center overflow-x-auto"
            >
              <!-- Row 1: Label "Jour" + Day buttons -->
              <label
                class="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap pr-2"
              >
                {{ locale === 'fr' ? 'Jour' : 'Day' }}
              </label>
              <button
                v-for="day in weekdays"
                :key="day.value"
                type="button"
                class="rounded-lg border-2 px-2 py-2 text-sm font-medium transition-colors"
                :class="{
                  'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300':
                    form.allowed_weekdays.includes(day.value),
                  'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700':
                    !form.allowed_weekdays.includes(day.value),
                }"
                @click.prevent="toggleWeekday(day.value)"
              >
                {{ day.short }}
              </button>

              <!-- Row 2: Label + Start times -->
              <label
                class="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap pr-2"
              >
                {{ t('availability.startTime') }}
              </label>
              <TimeSelect
                v-for="day in weekdays"
                :key="`min-${day.value}`"
                v-model="form.weekday_times[day.value].min_time"
                :disabled="!form.allowed_weekdays.includes(day.value)"
                class="text-sm min-w-0"
                :class="{
                  'opacity-50 cursor-not-allowed': !form.allowed_weekdays.includes(day.value),
                }"
                placeholder="--:--"
              />

              <!-- Row 3: Label + End times -->
              <label
                class="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap pr-2"
              >
                {{ t('availability.endTime') }}
              </label>
              <TimeSelect
                v-for="day in weekdays"
                :key="`max-${day.value}`"
                v-model="form.weekday_times[day.value].max_time"
                :disabled="!form.allowed_weekdays.includes(day.value)"
                class="text-sm min-w-0"
                :class="{
                  'opacity-50 cursor-not-allowed': !form.allowed_weekdays.includes(day.value),
                }"
                placeholder="--:--"
              />
            </div>

            <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
              {{ t('calendar.allowedWeekdaysHelp') }}
            </p>
          </div>

          <!-- Holidays Policy -->
          <div>
            <div class="flex items-center justify-between">
              <div class="flex items-start flex-1">
                <div class="flex flex-col gap-1">
                  <label
                    for="holidays-policy"
                    class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t('calendar.holidaysPolicy') }}
                  </label>
                  <select
                    id="holidays-policy"
                    v-model="form.holidays_policy"
                    class="input w-48"
                  >
                    <option value="ignore">
                      {{ t('calendar.holidaysPolicyIgnore') }}
                    </option>
                    <option value="allow">
                      {{ t('calendar.holidaysPolicyAllow') }}
                    </option>
                    <option value="block">
                      {{ t('calendar.holidaysPolicyBlock') }}
                    </option>
                  </select>
                  <p class="text-xs text-gray-500 dark:text-gray-400">
                    {{ t('calendar.holidaysPolicyHelp') }}
                  </p>
                </div>
              </div>
              <div class="flex items-center gap-2 ml-4">
                <TimeSelect
                  v-model="form.holiday_min_time"
                  :disabled="form.holidays_policy !== 'allow'"
                  class="w-32 text-sm"
                  :class="{
                    'opacity-50 cursor-not-allowed': form.holidays_policy !== 'allow',
                  }"
                  placeholder="Min"
                />
                <span class="text-gray-500 dark:text-gray-400">-</span>
                <TimeSelect
                  v-model="form.holiday_max_time"
                  :disabled="form.holidays_policy !== 'allow'"
                  class="w-32 text-sm"
                  :class="{
                    'opacity-50 cursor-not-allowed': form.holidays_policy !== 'allow',
                  }"
                  placeholder="Max"
                />
              </div>
            </div>
          </div>

          <!-- Allow Holiday Eves -->
          <div>
            <div class="flex items-center justify-between">
              <div class="flex items-start flex-1">
                <input
                  id="allow-holiday-eves"
                  v-model="form.allow_holiday_eves"
                  type="checkbox"
                  :disabled="allWeekdaysSelected"
                  class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed dark:border-gray-600 dark:bg-gray-700"
                >
                <label
                  for="allow-holiday-eves"
                  class="ml-2 text-sm text-gray-700 dark:text-gray-300"
                  :class="{
                    'opacity-50 cursor-not-allowed': allWeekdaysSelected,
                  }"
                >
                  <span class="font-medium">{{ t('calendar.allowHolidayEves') }}</span>
                  <p class="text-gray-500 dark:text-gray-400">
                    {{ t('calendar.allowHolidayEvesHelp') }}
                  </p>
                </label>
              </div>
              <div class="flex items-center gap-2 ml-4">
                <TimeSelect
                  v-model="form.holiday_eve_min_time"
                  :disabled="!form.allow_holiday_eves || allWeekdaysSelected"
                  class="w-32 text-sm"
                  :class="{
                    'opacity-50 cursor-not-allowed':
                      !form.allow_holiday_eves || allWeekdaysSelected,
                  }"
                  placeholder="Min"
                />
                <span class="text-gray-500 dark:text-gray-400">-</span>
                <TimeSelect
                  v-model="form.holiday_eve_max_time"
                  :disabled="!form.allow_holiday_eves || allWeekdaysSelected"
                  class="w-32 text-sm"
                  :class="{
                    'opacity-50 cursor-not-allowed':
                      !form.allow_holiday_eves || allWeekdaysSelected,
                  }"
                  placeholder="Max"
                />
              </div>
            </div>
          </div>
        </CollapsibleSection>

        <!-- Warning Message - No Participants -->
        <div
          v-if="participants.length === 0"
          class="rounded-lg bg-orange-50 p-4 dark:bg-orange-900/20"
        >
          <div class="flex">
            <svg
              class="h-5 w-5 text-orange-600 dark:text-orange-400"
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
            <p class="ml-3 text-sm text-orange-600 dark:text-orange-400">
              {{ t('calendar.noParticipantsWarningCreate') }}
            </p>
          </div>
        </div>

        <!-- Error Message -->
        <div
          v-if="errorMessage"
          class="rounded-lg bg-danger-50 p-4 dark:bg-danger-900/20"
        >
          <div class="flex">
            <svg
              class="h-5 w-5 text-danger-600 dark:text-danger-400"
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
            <p class="ml-3 text-sm text-danger-600 dark:text-danger-400">
              {{ errorMessage }}
            </p>
          </div>
        </div>

        <!-- Actions -->
        <div class="flex items-center justify-end gap-4">
          <router-link
            to="/dashboard"
            class="btn btn-ghost"
          >
            {{ t('common.cancel') }}
          </router-link>
          <button
            type="submit"
            :disabled="loading || participants.length === 0"
            class="btn btn-primary"
          >
            <svg
              v-if="loading"
              class="mr-2 h-5 w-5 animate-spin"
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
            {{ loading ? t('common.creating') : t('common.create') }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCalendarStore } from '@/stores/calendar'
import { useAuthStore } from '@/stores/auth'
import TimezoneSelector from '@/components/TimezoneSelector.vue'
import TimeSelect from '@/components/TimeSelect.vue'
import CollapsibleSection from '@/components/CollapsibleSection.vue'

const router = useRouter()
const { t, locale } = useI18n()
const calendarStore = useCalendarStore()
const authStore = useAuthStore()

const form = reactive({
  name: '',
  description: '',
  threshold: 1,
  allowed_weekdays: [0, 1, 2, 3, 4, 5, 6] as number[],
  min_duration_hours: 0,
  timezone: 'Europe/Paris',
  holidays_policy: 'ignore' as 'ignore' | 'allow' | 'block',
  allow_holiday_eves: false,
  lock_participants: false,
  weekday_times: {
    0: { min_time: '', max_time: '' },
    1: { min_time: '', max_time: '' },
    2: { min_time: '', max_time: '' },
    3: { min_time: '', max_time: '' },
    4: { min_time: '', max_time: '' },
    5: { min_time: '', max_time: '' },
    6: { min_time: '', max_time: '' },
  } as Record<number, { min_time: string; max_time: string }>,
  holiday_min_time: '',
  holiday_max_time: '',
  holiday_eve_min_time: '',
  holiday_eve_max_time: '',
  start_date: '',
  end_date: '',
})

const participants = ref<string[]>([])

// Weekdays (0=Sunday, 6=Saturday)
// Order depends on locale: Monday-first (fr) or Sunday-first (en)
const weekdays = computed(() => {
  const days = [
    { value: 0, short: t('weekdays.short.sunday') },
    { value: 1, short: t('weekdays.short.monday') },
    { value: 2, short: t('weekdays.short.tuesday') },
    { value: 3, short: t('weekdays.short.wednesday') },
    { value: 4, short: t('weekdays.short.thursday') },
    { value: 5, short: t('weekdays.short.friday') },
    { value: 6, short: t('weekdays.short.saturday') },
  ]

  // For locales that start the week on Monday (fr, most of Europe)
  if (locale.value === 'fr') {
    // Move Sunday to the end
    return [...days.slice(1), days[0]]
  }

  // For locales that start on Sunday (en, US)
  return days
})

// Check if all weekdays are selected
const allWeekdaysSelected = computed(() => {
  return form.allowed_weekdays.length === 7
})

// Automatically add the connected user as a default participant
// and initialize timezone with the user's timezone
onMounted(() => {
  if (authStore.user?.display_name) {
    participants.value.push(authStore.user.display_name)
  }
  if (authStore.user?.timezone) {
    form.timezone = authStore.user.timezone
  }
})
const newParticipantName = ref('')
const loading = ref(false)
const errorMessage = ref('')
const errors = reactive({
  name: '',
  threshold: '',
  participants: '',
})

function toggleWeekday(day: number) {
  const index = form.allowed_weekdays.indexOf(day)
  if (index > -1) {
    // Remove if already selected (but keep at least one day)
    if (form.allowed_weekdays.length > 1) {
      form.allowed_weekdays.splice(index, 1)
    }
  } else {
    // Add if not selected
    form.allowed_weekdays.push(day)
    // Sort to keep in order
    form.allowed_weekdays.sort((a, b) => a - b)
  }
}

function addParticipant() {
  if (newParticipantName.value.trim()) {
    // Check for duplicates
    if (participants.value.includes(newParticipantName.value.trim())) {
      errors.participants = t('calendar.duplicateParticipant')
      return
    }

    participants.value.push(newParticipantName.value.trim())
    newParticipantName.value = ''
    errors.participants = ''
  }
}

function removeParticipant(index: number) {
  participants.value.splice(index, 1)
}

// Normalise "00:00" to empty string (00:00 is not meaningful as a time restriction)
function normalizeTime(time: string): string {
  return time === '00:00' ? '' : time
}

// Prepare weekday_times for API: normalize 00:00 to empty
function prepareWeekdayTimes(
  weekdayTimes: Record<number, { min_time: string; max_time: string }>
): Record<number, { min_time?: string; max_time?: string }> {
  const result: Record<number, { min_time?: string; max_time?: string }> = {}
  for (const [day, times] of Object.entries(weekdayTimes)) {
    const minTime = normalizeTime(times.min_time)
    const maxTime = normalizeTime(times.max_time)
    result[Number(day)] = {
      ...(minTime ? { min_time: minTime } : {}),
      ...(maxTime ? { max_time: maxTime } : {}),
    }
  }
  return result
}

function validateForm(): boolean {
  // Reset errors
  errors.name = ''
  errors.threshold = ''
  errors.participants = ''

  let isValid = true

  if (!form.name.trim()) {
    errors.name = t('errors.required')
    isValid = false
  }

  if (participants.value.length === 0) {
    errors.participants = t('calendar.participantsRequired')
    isValid = false
  }

  if (!form.threshold || form.threshold < 1) {
    errors.threshold = t('calendar.thresholdMinError')
    isValid = false
  }

  if (participants.value.length > 0 && form.threshold > participants.value.length) {
    errors.threshold = t('calendar.thresholdMaxError')
    isValid = false
  }

  return isValid
}

async function handleSubmit() {
  errorMessage.value = ''

  if (!validateForm()) {
    return
  }

  loading.value = true

  try {
    // Create calendar with participants in a single atomic request
    // Normalize 00:00 times to empty (not meaningful as restrictions)
    const normalizedHolidayMinTime = normalizeTime(form.holiday_min_time)
    const normalizedHolidayMaxTime = normalizeTime(form.holiday_max_time)
    const normalizedHolidayEveMinTime = normalizeTime(form.holiday_eve_min_time)
    const normalizedHolidayEveMaxTime = normalizeTime(form.holiday_eve_max_time)

    const calendar = await calendarStore.createCalendar({
      name: form.name.trim(),
      description: form.description.trim() || undefined,
      threshold: form.threshold,
      allowed_weekdays: form.allowed_weekdays,
      min_duration_hours: form.min_duration_hours,
      timezone: form.timezone,
      holidays_policy: form.holidays_policy,
      allow_holiday_eves: form.allow_holiday_eves,
      lock_participants: form.lock_participants,
      weekday_times: prepareWeekdayTimes(form.weekday_times),
      // Send empty string (not undefined) for consistency with update
      holiday_min_time: normalizedHolidayMinTime,
      holiday_max_time: normalizedHolidayMaxTime,
      holiday_eve_min_time: normalizedHolidayEveMinTime,
      holiday_eve_max_time: normalizedHolidayEveMaxTime,
      start_date: form.start_date || undefined,
      end_date: form.end_date || undefined,
      participants: participants.value.filter(name => name.trim() !== ''),
    } as any)

    // Redirect to calendar management page
    router.push(`/calendars/${calendar.id}/settings`)
  } catch (error: any) {
    console.error('Error creating calendar:', error)
    errorMessage.value = error.message || t('calendar.createError')
  } finally {
    loading.value = false
  }
}
</script>
