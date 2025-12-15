<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="calendar-grid">
    <!-- Month Navigation -->
    <div class="mb-4 flex items-center justify-between gap-2">
      <button
        v-if="props.showNavigation !== false"
        class="btn btn-ghost p-2 md:p-2 min-h-[44px] md:min-h-0 min-w-[44px] md:min-w-0"
        :title="t('calendar.previousMonth', 'Previous month')"
        @click="previousMonth"
      >
        <svg
          class="h-6 w-6 md:h-5 md:w-5"
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
      </button>
      <div
        v-else
        class="w-11 md:w-10"
      />

      <h3 class="font-display text-base md:text-lg font-semibold text-gray-900 dark:text-white text-center flex-1 px-2">
        {{ currentMonthLabel }}
      </h3>

      <button
        v-if="props.showNavigation !== false"
        class="btn btn-ghost p-2 md:p-2 min-h-[44px] md:min-h-0 min-w-[44px] md:min-w-0"
        :title="t('calendar.nextMonth', 'Next month')"
        @click="nextMonth"
      >
        <svg
          class="h-6 w-6 md:h-5 md:w-5"
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
      </button>
      <div
        v-else
        class="w-11 md:w-10"
      />
    </div>

    <!-- Weekday Headers -->
    <div class="mb-2 grid grid-cols-7 gap-1">
      <div
        v-for="day in weekDays"
        :key="day"
        class="text-center text-xs font-medium text-gray-600 dark:text-gray-400"
      >
        {{ day }}
      </div>
    </div>

    <!-- Days Grid -->
    <div
      class="grid grid-cols-7 gap-1"
      @mouseleave="handlePointerLeave"
      @mouseup="handlePointerUp"
      @touchend="handlePointerUp"
      @touchmove="handleGridTouchMove"
      @touchcancel="handlePointerLeave"
    >
      <div
        v-for="(day, index) in calendarDays"
        :key="`${day.date}-${day.isCurrentMonth}`"
        v-memo="[
          day.hasAvailability,
          day.hasRecurrence,
          day.meetsThreshold,
          day.isToday,
          day.isPast,
          day.isAllowed,
          day.isHoliday,
          day.isHolidayEve,
          isCellSelected(index),
          isDragging,
          dragMode,
          getParticipantCount(day.dateString),
        ]"
        :class="[
          'relative min-h-20 rounded-lg border p-2 transition-all',
          !day.isCurrentMonth
            ? 'border-transparent'
            : day.isCurrentMonth && !day.isPast && day.isAllowed
              ? 'cursor-pointer border-gray-200 bg-white hover:border-primary-300 hover:shadow-sm dark:border-gray-700 dark:bg-gray-800 dark:hover:border-primary-600'
              : day.isCurrentMonth && (day.isPast || !day.isAllowed)
                ? 'cursor-not-allowed border-gray-200 bg-gray-100 dark:border-gray-700 dark:bg-gray-900 opacity-50'
                : 'border-transparent',
          day.isCurrentMonth && day.isToday && 'ring-2 ring-primary-500 ring-offset-1',
          day.isCurrentMonth &&
            !day.isPast &&
            day.isAllowed &&
            day.meetsThreshold &&
            'bg-green-50 border-green-200 dark:bg-green-900/20 dark:border-green-700',
          day.isCurrentMonth &&
            !day.isPast &&
            day.isAllowed &&
            !day.meetsThreshold &&
            day.hasAvailability &&
            'bg-primary-50 border-primary-200 dark:bg-primary-900/20 dark:border-primary-700',
          day.isCurrentMonth &&
            !day.isPast &&
            day.isAllowed &&
            !day.meetsThreshold &&
            day.hasRecurrence &&
            !day.hasAvailability &&
            'bg-blue-50 border-blue-200 dark:bg-blue-900/20 dark:border-blue-700',
          day.isCurrentMonth && day.isHoliday && 'ring-1 ring-orange-400 dark:ring-orange-500',
          day.isCurrentMonth &&
            allowHolidayEves &&
            day.isHolidayEve &&
            'ring-1 ring-purple-400 dark:ring-purple-500',
          // Rectangle selection style - add mode (yellow)
          isDragging &&
            dragMode === 'add' &&
            isCellSelected(index) &&
            canSelectCell(day) &&
            'ring-2 ring-yellow-500 bg-yellow-100 dark:bg-yellow-900/30',
          // Rectangle selection style - remove mode (red)
          isDragging &&
            dragMode === 'remove' &&
            isCellSelected(index) &&
            canSelectCell(day) &&
            cellHasAvailability(day) &&
            'ring-2 ring-red-500 bg-red-100 dark:bg-red-900/30',
        ]"
        :title="day.holidayName || undefined"
        @mousedown="handlePointerDown(index, day, $event)"
        @mousemove="handlePointerMove(index)"
        @mouseup="handlePointerUp"
        @touchstart="handlePointerDown(index, day, $event)"
      >
        <!-- Only show content for current month days -->
        <template v-if="day.isCurrentMonth">
          <!-- Day Number and Participant Count -->
          <div class="mb-1 flex items-baseline gap-1">
            <span
              :class="[
                'text-base font-semibold',
                day.isCurrentMonth
                  ? day.isToday
                    ? 'text-primary-600 dark:text-primary-400'
                    : 'text-gray-900 dark:text-white'
                  : 'text-gray-400 dark:text-gray-600',
              ]"
            >
              {{ day.date }}
            </span>
            <span
              v-if="getParticipantCount(day.dateString) >= 1"
              data-no-drag
              :class="[
                'text-xs font-normal cursor-pointer hover:underline',
                day.isCurrentMonth
                  ? 'text-gray-600 dark:text-gray-400'
                  : 'text-gray-400 dark:text-gray-600',
              ]"
              @click.stop="handleParticipantCountClick(day.dateString, $event)"
              @mousedown.stop
              @mouseenter="handleParticipantCountHoverStart(day.dateString, $event)"
              @mouseleave="handleParticipantCountHoverEnd"
            >
              <span class="lg:hidden">{{ getParticipantCount(day.dateString) }} {{ t('calendar.participantShort', 'part.') }}</span>
              <span class="hidden lg:inline">{{ getParticipantCount(day.dateString) }} {{ t('calendar.participantCount', 'participant(s)') }}</span>
            </span>
          </div>

          <!-- Availability Indicator -->
          <div
            v-if="day.hasAvailability"
            class="mt-1 space-y-0.5"
          >
            <div
              v-for="(avail, idx) in day.availabilities"
              :key="idx"
              :class="[
                'rounded px-1.5 py-0.5 text-xs text-white',
                day.meetsThreshold
                  ? 'bg-green-600 dark:bg-green-500'
                  : 'bg-primary-600 dark:bg-primary-500',
              ]"
            >
              {{ formatTimeRange(avail.start_time, avail.end_time) }}
            </div>
          </div>

          <!-- Recurrence Indicator (only if no explicit availability) -->
          <div
            v-else-if="day.hasRecurrence"
            class="mt-1 space-y-0.5"
          >
            <div
              :class="[
                'flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-white',
                day.meetsThreshold ? 'bg-green-500' : 'bg-blue-500',
              ]"
            >
              <span class="flex-1">
                {{ formatTimeRange(day.recurrenceStartTime, day.recurrenceEndTime) }}
              </span>
              <button
                v-if="day.recurrenceId && !day.isPast"
                data-no-drag
                :class="[
                  'rounded p-0.5 transition-colors',
                  day.meetsThreshold ? 'hover:bg-green-600' : 'hover:bg-blue-600',
                ]"
                :title="t('availability.addException', 'Add exception')"
                @click.stop="handleAddException(day.recurrenceId!, day.dateString)"
                @mousedown.stop
              >
                <svg
                  class="h-3 w-3"
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
        </template>
      </div>
    </div>

    <!-- Legend -->
    <div class="mt-4 flex flex-wrap gap-4 text-xs">
      <div class="flex items-center gap-1">
        <div class="h-3 w-3 rounded border-2 border-primary-500" />
        <span class="text-gray-600 dark:text-gray-400">{{ t('calendar.today', 'Today') }}</span>
      </div>
      <div class="flex items-center gap-1">
        <div
          class="h-3 w-3 rounded bg-green-100 border border-green-200 dark:bg-green-900/20 dark:border-green-700"
        />
        <span class="text-gray-600 dark:text-gray-400">{{
          t('availability.thresholdMet', 'Threshold met')
        }}</span>
      </div>
      <div class="flex items-center gap-1">
        <div
          class="h-3 w-3 rounded bg-primary-100 border border-primary-200 dark:bg-primary-900/20 dark:border-primary-700"
        />
        <span class="text-gray-600 dark:text-gray-400">{{
          t('availability.available', 'Available')
        }}</span>
      </div>
      <div class="flex items-center gap-1">
        <div
          class="h-3 w-3 rounded bg-blue-100 border border-blue-200 dark:bg-blue-900/20 dark:border-blue-700"
        />
        <span class="text-gray-600 dark:text-gray-400">{{
          t('availability.recurring', 'Recurring')
        }}</span>
      </div>
      <div class="flex items-center gap-1">
        <div
          class="h-3 w-3 rounded border border-gray-300 ring-1 ring-orange-400 dark:border-gray-600 dark:ring-orange-500"
        />
        <span class="text-gray-600 dark:text-gray-400">{{
          t('calendar.publicHoliday', 'Public holiday')
        }}</span>
      </div>
      <div
        v-if="allowHolidayEves"
        class="flex items-center gap-1"
      >
        <div
          class="h-3 w-3 rounded border border-gray-300 ring-1 ring-purple-400 dark:border-gray-600 dark:ring-purple-500"
        />
        <span class="text-gray-600 dark:text-gray-400">{{
          t('calendar.holidayEve', 'Holiday eve')
        }}</span>
      </div>
    </div>

    <!-- Participant Details Tooltip -->
    <div
      v-if="selectedDate"
      ref="tooltipRef"
      class="absolute z-50 bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-md p-6 border border-gray-200 dark:border-gray-700 pointer-events-auto"
      :style="{
        left: `${popupPosition.x}px`,
        top: `${popupPosition.y}px`,
      }"
      @mouseenter="handlePopupHoverStart"
      @mouseleave="handlePopupHoverEnd"
    >
      <div>
        <div class="mb-4 flex items-center justify-between">
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('participant.participantsForDate', 'Participants for') }}
            {{ formatSelectedDate }}
          </h3>
          <button
            class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            @click="closeParticipantPopup"
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

        <div
          v-if="loadingDetails"
          class="text-center py-8"
        >
          <div
            class="inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-primary-600 border-r-transparent"
          />
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
            {{ t('common.loading', 'Loading...') }}
          </p>
        </div>

        <div v-else-if="participantDetails">
          <div class="mb-4">
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ participantDetails.total_count }}
              {{
                participantDetails.total_count > 1
                  ? t('calendar.participants', 'Participants')
                  : t('calendar.participantCount', 'participant(s)')
              }}
            </p>
          </div>

          <div class="space-y-2">
            <div
              v-for="participant in participantDetails.participants"
              :key="participant.participant_id || participant.participant_name"
              :class="[
                'rounded-lg border p-3',
                participant.participant_name === currentParticipantName
                  ? 'border-primary-300 bg-primary-50/50 dark:border-primary-700 dark:bg-primary-900/10'
                  : 'border-gray-200 dark:border-gray-700',
              ]"
            >
              <div class="flex items-start justify-between">
                <div class="flex-1">
                  <div class="flex items-center gap-2">
                    <div class="font-medium text-gray-900 dark:text-white">
                      {{ participant.participant_name }}
                    </div>
                    <span
                      v-if="participant.participant_name === currentParticipantName"
                      class="text-xs px-2 py-0.5 rounded-full bg-primary-100 text-primary-700 dark:bg-primary-900 dark:text-primary-300"
                    >
                      {{ t('common.you', 'You') }}
                    </span>
                  </div>
                  <div class="mt-1 text-sm text-gray-600 dark:text-gray-400">
                    {{ formatTimeRange(participant.start_time, participant.end_time) }}
                  </div>

                  <!-- Availability edit form -->
                  <div
                    v-if="participant.participant_name === currentParticipantName && editingNote"
                    class="mt-2"
                  >
                    <!-- Time Range -->
                    <div class="grid grid-cols-2 gap-2 mb-2">
                      <div>
                        <label
                          class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                        >
                          {{ t('availability.startTime', 'Start time') }}
                        </label>
                        <TimeSelect
                          v-model="editedStartTime"
                          class="w-full text-sm"
                          :max="editedEndTime || undefined"
                        />
                      </div>
                      <div>
                        <label
                          class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                        >
                          {{ t('availability.endTime', 'End time') }}
                        </label>
                        <TimeSelect
                          v-model="editedEndTime"
                          class="w-full text-sm"
                          :min="editedStartTime || undefined"
                        />
                      </div>
                    </div>

                    <!-- Note -->
                    <div class="mb-2">
                      <label
                        class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                      >
                        {{ t('availability.note', 'Note') }}
                      </label>
                      <textarea
                        v-model="editedNote"
                        rows="2"
                        class="input w-full text-sm"
                        :placeholder="t('availability.note', 'Note')"
                      />
                    </div>

                    <!-- Action buttons -->
                    <div class="flex gap-2">
                      <button
                        :disabled="savingNote"
                        class="btn btn-primary btn-sm"
                        @click="saveNote"
                      >
                        <svg
                          v-if="savingNote"
                          class="mr-1 h-3 w-3 animate-spin"
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
                        {{ t('common.save', 'Save') }}
                      </button>
                      <button
                        :disabled="savingNote"
                        class="btn btn-ghost btn-sm"
                        @click="cancelEdit"
                      >
                        {{ t('common.cancel', 'Cancel') }}
                      </button>
                    </div>
                  </div>
                  <div
                    v-else-if="participant.note"
                    class="mt-1 text-sm text-gray-500 dark:text-gray-400 italic"
                  >
                    {{ participant.note }}
                  </div>
                  <div
                    v-else-if="participant.participant_name === currentParticipantName"
                    class="mt-1 text-sm text-gray-400 dark:text-gray-500 italic"
                  >
                    {{ t('availability.noNote', 'No note') }}
                  </div>
                </div>

                <!-- Edit button for current participant -->
                <button
                  v-if="participant.participant_name === currentParticipantName && !editingNote"
                  class="ml-2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                  :title="t('common.edit', 'Edit')"
                  @click="
                    startEdit(participant.note || '', participant.start_time, participant.end_time)
                  "
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
                      d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                    />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        </div>

        <div
          v-else
          class="text-center py-8"
        >
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('availability.noAvailabilities', 'No availabilities') }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { availabilitiesApi } from '@/api/availabilities'
import type { Availability, RecurrenceWithExceptions } from '@/types'
import { useDateValidation, clearHolidaysCache } from '@/composables/useDateValidation'
import TimeSelect from '@/components/TimeSelect.vue'

interface Props {
  availabilities?: Availability[]
  recurrences?: RecurrenceWithExceptions[]
  participantCounts?: Record<string, number>
  threshold?: number
  calendarToken?: string
  allowedWeekdays?: number[]
  timezone?: string
  holidaysPolicy?: 'ignore' | 'allow' | 'block'
  allowHolidayEves?: boolean
  currentParticipantId?: string // ID of the connected participant (for API calls)
  currentParticipantName?: string // Name of the connected participant (for visual comparison)
  initialYear?: number // Initial year to display
  initialMonth?: number // Initial month to display (0-11)
  showNavigation?: boolean // Show month navigation buttons (default true)
  startDate?: string // Calendar start date (YYYY-MM-DD format)
  endDate?: string // Calendar end date (YYYY-MM-DD format)
}

interface Emits {
  (e: 'day-click', date: string): void
  (e: 'days-select', dates: string[]): void
  (e: 'days-deselect', dates: string[]): void
  (e: 'add-exception', recurrenceId: string, date: string): void
  (e: 'month-change', year: number, month: number): void
  (e: 'availability-updated'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t, locale } = useI18n()

// IMPORTANT: Clear holidays cache on each component creation
// to ensure computed properties use the correct data
clearHolidaysCache()

const { isDateAllowed, checkIsHoliday, checkIsHolidayEve, getHolidayName } = useDateValidation()

// Initialize currentDate with props or default to current date
const initDate =
  props.initialYear !== undefined && props.initialMonth !== undefined
    ? new Date(props.initialYear, props.initialMonth, 1)
    : new Date()
const currentDate = ref(initDate)
const selectedDate = ref<string | null>(null)

// Watch for prop changes to keep currentDate in sync
watch(
  () => [props.initialYear, props.initialMonth],
  ([year, month]) => {
    if (year !== undefined && month !== undefined) {
      currentDate.value = new Date(year, month, 1)
    }
  }
)
const participantDetails = ref<any>(null)
const loadingDetails = ref(false)
const hoverTimeout = ref<number | null>(null)
const closeTimeout = ref<number | null>(null)
const popupPosition = ref({ x: 0, y: 0 })
const tooltipRef = ref<HTMLElement | null>(null)

// States for note editing
const editingNote = ref(false)
const editedNote = ref('')
const editedStartTime = ref('')
const editedEndTime = ref('')
const savingNote = ref(false)

// States for rectangle selection (drag-select)
const isDragging = ref(false)
const dragStartIndex = ref<number | null>(null)
const dragCurrentIndex = ref<number | null>(null)
const dragMode = ref<'add' | 'remove'>('add') // Drag mode: add or remove

const weekDays = computed(() => {
  const localeCode = locale.value === 'fr' ? 'fr-FR' : 'en-US'
  // For fr-FR, week starts on Monday (day 1)
  // For en-US, week starts on Sunday (day 0)
  const firstDayOfWeek = localeCode === 'fr-FR' ? 1 : 0

  const baseDate = new Date(2025, 0, 6) // A Monday
  const startDate = new Date(baseDate)
  startDate.setDate(baseDate.getDate() - baseDate.getDay() + firstDayOfWeek)

  return Array.from({ length: 7 }, (_, i) => {
    const date = new Date(startDate)
    date.setDate(startDate.getDate() + i)
    return date.toLocaleDateString(localeCode, { weekday: 'short' })
  })
})

const currentMonthLabel = computed(() => {
  const localeCode = locale.value === 'fr' ? 'fr-FR' : 'en-US'
  return currentDate.value.toLocaleDateString(localeCode, {
    month: 'long',
    year: 'numeric',
  })
})

const formatSelectedDate = computed(() => {
  if (!selectedDate.value) return ''
  const date = new Date(selectedDate.value)
  const localeCode = locale.value === 'fr' ? 'fr-FR' : 'en-US'
  return date.toLocaleDateString(localeCode, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  })
})

interface CalendarDay {
  date: number
  dateString: string
  isCurrentMonth: boolean
  isToday: boolean
  isPast: boolean
  isAllowed: boolean
  isHoliday: boolean
  isHolidayEve: boolean
  holidayName?: string
  hasAvailability: boolean
  hasRecurrence: boolean
  meetsThreshold: boolean
  availabilities: Availability[]
  dayOfWeek: number
  recurrenceId?: string
  recurrenceStartTime?: string
  recurrenceEndTime?: string
}

// Helper function to check if a date is allowed for availability
const checkDateAllowed = (dateObj: Date): boolean => {
  const timezone = props.timezone || 'Europe/Paris'
  const allowedWeekdays = props.allowedWeekdays || [0, 1, 2, 3, 4, 5, 6]
  const holidaysPolicy = props.holidaysPolicy || 'ignore'
  const allowHolidayEves = props.allowHolidayEves || false

  // Check if date is within calendar's date range
  if (props.startDate) {
    const startDate = new Date(props.startDate)
    startDate.setHours(0, 0, 0, 0)
    if (dateObj < startDate) {
      return false
    }
  }

  if (props.endDate) {
    const endDate = new Date(props.endDate)
    endDate.setHours(0, 0, 0, 0)
    if (dateObj > endDate) {
      return false
    }
  }

  return isDateAllowed(dateObj, timezone, allowedWeekdays, holidaysPolicy, allowHolidayEves)
}

const calendarDays = computed((): CalendarDay[] => {
  const year = currentDate.value.getFullYear()
  const month = currentDate.value.getMonth()

  // First day of the month
  const firstDay = new Date(year, month, 1)
  // Adjust for Monday as first day of week (fr-FR)
  // getDay() returns 0 for Sunday, 1 for Monday, etc.
  // For Monday-first week, we need to adjust: (getDay() - 1 + 7) % 7
  const firstDayOfWeek = (firstDay.getDay() - 1 + 7) % 7

  // Last day of the month
  const lastDay = new Date(year, month + 1, 0)
  const daysInMonth = lastDay.getDate()

  // Previous month
  const prevMonthLastDay = new Date(year, month, 0)
  const daysInPrevMonth = prevMonthLastDay.getDate()

  const days: CalendarDay[] = []
  const today = new Date()
  today.setHours(0, 0, 0, 0)

  // Previous month days
  for (let i = firstDayOfWeek - 1; i >= 0; i--) {
    const date = daysInPrevMonth - i
    const dateObj = new Date(year, month - 1, date)
    dateObj.setHours(0, 0, 0, 0)
    const dateString = formatDateString(dateObj)
    const isPast = dateObj < today
    const timezone = props.timezone || 'Europe/Paris'

    days.push({
      date,
      dateString,
      isCurrentMonth: false,
      isToday: false,
      isPast,
      isAllowed: checkDateAllowed(dateObj),
      isHoliday: checkIsHoliday(dateObj, timezone),
      isHolidayEve: checkIsHolidayEve(dateObj, timezone),
      hasAvailability: false,
      hasRecurrence: false,
      meetsThreshold: false,
      availabilities: [],
      dayOfWeek: dateObj.getDay(),
    })
  }

  // Current month days
  for (let date = 1; date <= daysInMonth; date++) {
    const dateObj = new Date(year, month, date)
    dateObj.setHours(0, 0, 0, 0)
    const dateString = formatDateString(dateObj)
    const dayOfWeek = dateObj.getDay()
    const isPast = dateObj < today

    // Check if this date has an availability
    const dateAvailabilities = (props.availabilities || []).filter(a => a.date === dateString)

    // Check if this date matches any recurrence (and is not an exception)
    let recurrenceId: string | undefined
    let recurrenceStartTime: string | undefined
    let recurrenceEndTime: string | undefined
    const hasRecurrence = (props.recurrences || []).some(rec => {
      if (rec.day_of_week !== dayOfWeek) return false

      // Compare dates as strings to avoid timezone issues
      if (dateString < rec.start_date) return false
      if (rec.end_date && dateString > rec.end_date) return false

      // Check if this date is in the exceptions
      const isException = rec.exceptions?.some(ex => ex.excluded_date === dateString)

      if (!isException) {
        recurrenceId = rec.id
        recurrenceStartTime = rec.start_time
        recurrenceEndTime = rec.end_time
        return true
      }
      return false
    })

    // Check if this day meets the threshold
    const participantCount = props.participantCounts?.[dateString] || 0
    const meetsThreshold = participantCount >= (props.threshold || 1)
    const timezone = props.timezone || 'Europe/Paris'

    days.push({
      date,
      dateString,
      isCurrentMonth: true,
      isToday: dateObj.getTime() === today.getTime(),
      isPast,
      isAllowed: checkDateAllowed(dateObj),
      isHoliday: checkIsHoliday(dateObj, timezone),
      isHolidayEve: checkIsHolidayEve(dateObj, timezone),
      holidayName: getHolidayName(dateObj, timezone) ?? undefined,
      hasAvailability: dateAvailabilities.length > 0,
      hasRecurrence,
      meetsThreshold,
      availabilities: dateAvailabilities,
      dayOfWeek,
      recurrenceId,
      recurrenceStartTime,
      recurrenceEndTime,
    })
  }

  // Next month days to fill the grid
  // Only add enough days to complete the last row (multiple of 7)
  const currentDaysCount = days.length
  const remainingDays = currentDaysCount % 7 === 0 ? 0 : 7 - (currentDaysCount % 7)
  for (let date = 1; date <= remainingDays; date++) {
    const dateObj = new Date(year, month + 1, date)
    dateObj.setHours(0, 0, 0, 0)
    const dateString = formatDateString(dateObj)
    const isPast = dateObj < today
    const timezone = props.timezone || 'Europe/Paris'

    days.push({
      date,
      dateString,
      isCurrentMonth: false,
      isToday: false,
      isPast,
      isAllowed: checkDateAllowed(dateObj),
      isHoliday: checkIsHoliday(dateObj, timezone),
      isHolidayEve: checkIsHolidayEve(dateObj, timezone),
      hasAvailability: false,
      hasRecurrence: false,
      meetsThreshold: false,
      availabilities: [],
      dayOfWeek: dateObj.getDay(),
    })
  }

  return days
})

function formatDateString(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function previousMonth() {
  const newDate = new Date(currentDate.value.getFullYear(), currentDate.value.getMonth() - 1, 1)

  // If we have controlled props, only emit the event and let parent update us
  if (props.initialYear !== undefined && props.initialMonth !== undefined) {
    emit('month-change', newDate.getFullYear(), newDate.getMonth())
  } else {
    // Otherwise, update local state and emit
    currentDate.value = newDate
    emit('month-change', newDate.getFullYear(), newDate.getMonth())
  }
}

function nextMonth() {
  const newDate = new Date(currentDate.value.getFullYear(), currentDate.value.getMonth() + 1, 1)

  // If we have controlled props, only emit the event and let parent update us
  if (props.initialYear !== undefined && props.initialMonth !== undefined) {
    emit('month-change', newDate.getFullYear(), newDate.getMonth())
  } else {
    // Otherwise, update local state and emit
    currentDate.value = newDate
    emit('month-change', newDate.getFullYear(), newDate.getMonth())
  }
}

function handleAddException(recurrenceId: string, dateString: string) {
  emit('add-exception', recurrenceId, dateString)
}

function getParticipantCount(dateString: string): number {
  return props.participantCounts?.[dateString] || 0
}

function isFullDayTime(startTime?: string, endTime?: string): boolean {
  // Consider as full day if:
  // - Both times are null/undefined
  // - Times are "00:00" and "23:59"
  if (!startTime && !endTime) return true

  const start = startTime ?? '00:00'
  const end = endTime ?? '23:59'

  return start === '00:00' && end === '23:59'
}

function formatTimeRange(startTime?: string, endTime?: string): string {
  if (isFullDayTime(startTime, endTime)) {
    return t('availability.allDay', 'All day')
  }
  return `${startTime ?? '00:00'}-${endTime ?? '23:59'}`
}

async function handleParticipantCountHoverStart(dateString: string, event: MouseEvent) {
  const count = getParticipantCount(dateString)
  if (count === 0 || !props.calendarToken) return

  // Initial position near cursor (use page coordinates for absolute positioning)
  const offset = 10
  popupPosition.value = {
    x: event.pageX + offset,
    y: event.pageY + offset,
  }

  // Clear any existing close timeout
  if (closeTimeout.value !== null) {
    window.clearTimeout(closeTimeout.value)
    closeTimeout.value = null
  }

  // Clear any existing hover timeout
  if (hoverTimeout.value !== null) {
    window.clearTimeout(hoverTimeout.value)
  }

  // Set a timeout to show the popup after 300ms
  hoverTimeout.value = window.setTimeout(() => {
    loadParticipantDetails(dateString)
  }, 300)
}

function handleParticipantCountHoverEnd() {
  // Clear the timeout if mouse leaves before it triggers
  if (hoverTimeout.value !== null) {
    window.clearTimeout(hoverTimeout.value)
    hoverTimeout.value = null
  }

  // Schedule popup close after a delay
  schedulePopupClose()
}

function handlePopupHoverStart() {
  // Cancel any scheduled close when mouse enters popup
  if (closeTimeout.value !== null) {
    window.clearTimeout(closeTimeout.value)
    closeTimeout.value = null
  }
}

function handlePopupHoverEnd() {
  // Don't close if we're in edit mode (TimeSelect dropdown is teleported to body)
  if (editingNote.value) return

  // Schedule popup close when mouse leaves popup
  schedulePopupClose()
}

function schedulePopupClose() {
  if (closeTimeout.value !== null) {
    window.clearTimeout(closeTimeout.value)
  }

  closeTimeout.value = window.setTimeout(() => {
    closeParticipantPopup()
  }, 200)
}

async function loadParticipantDetails(dateString: string) {
  selectedDate.value = dateString
  loadingDetails.value = true

  try {
    const details = await availabilitiesApi.getDateSummary(props.calendarToken!, dateString)
    participantDetails.value = details
  } catch (err) {
    console.error('Failed to load participant details:', err)
    participantDetails.value = null
  } finally {
    loadingDetails.value = false

    // Adjust tooltip position after content is loaded and rendered
    await nextTick()
    adjustTooltipPosition()
  }
}

function adjustTooltipPosition() {
  if (!tooltipRef.value) return

  const tooltip = tooltipRef.value
  const rect = tooltip.getBoundingClientRect()
  const offset = 10

  let { x, y } = popupPosition.value

  // Get scroll position and viewport dimensions
  const scrollX = window.scrollX || window.pageXOffset
  const scrollY = window.scrollY || window.pageYOffset
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight

  // Check right boundary (use page coordinates)
  if (x + rect.width > scrollX + viewportWidth) {
    x = scrollX + viewportWidth - rect.width - offset
  }

  // Check bottom boundary (use page coordinates)
  if (y + rect.height > scrollY + viewportHeight) {
    y = scrollY + viewportHeight - rect.height - offset
  }

  // Ensure tooltip doesn't go off left edge
  if (x < scrollX + offset) {
    x = scrollX + offset
  }

  // Ensure tooltip doesn't go off top edge
  if (y < scrollY + offset) {
    y = scrollY + offset
  }

  // Update position if it changed
  if (x !== popupPosition.value.x || y !== popupPosition.value.y) {
    popupPosition.value = { x, y }
  }
}

function handleParticipantCountClick(dateString: string, event: MouseEvent) {
  // Clear any pending timeouts
  if (hoverTimeout.value !== null) {
    window.clearTimeout(hoverTimeout.value)
    hoverTimeout.value = null
  }
  if (closeTimeout.value !== null) {
    window.clearTimeout(closeTimeout.value)
    closeTimeout.value = null
  }

  // Load participant details and keep popup open
  loadParticipantDetails(dateString)

  // Calculate position (use page coordinates for absolute positioning)
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  const scrollX = window.scrollX || window.pageXOffset
  const scrollY = window.scrollY || window.pageYOffset
  popupPosition.value = {
    x: rect.right + scrollX + 10,
    y: rect.top + scrollY,
  }
}

function startEdit(currentNote: string, startTime?: string, endTime?: string) {
  editedNote.value = currentNote
  editedStartTime.value = startTime || ''
  editedEndTime.value = endTime || ''
  editingNote.value = true
}

function cancelEdit() {
  editingNote.value = false
  editedNote.value = ''
  editedStartTime.value = ''
  editedEndTime.value = ''
}

async function saveNote() {
  if (!props.calendarToken || !props.currentParticipantId || !selectedDate.value) {
    return
  }

  savingNote.value = true

  try {
    // Update the availability with the new note and times
    await availabilitiesApi.update(
      props.calendarToken,
      props.currentParticipantId,
      selectedDate.value,
      {
        note: editedNote.value || undefined,
        start_time: editedStartTime.value || undefined,
        end_time: editedEndTime.value || undefined,
      }
    )

    // Reload participant details to show updated note
    await loadParticipantDetails(selectedDate.value)

    // Close edit mode
    editingNote.value = false
    editedNote.value = ''
    editedStartTime.value = ''
    editedEndTime.value = ''

    // Notify parent to reload availabilities
    emit('availability-updated')
  } catch (err) {
    console.error('Failed to update availability:', err)
  } finally {
    savingNote.value = false
  }
}

function closeParticipantPopup() {
  selectedDate.value = null
  participantDetails.value = null
  editingNote.value = false
  editedNote.value = ''
  editedStartTime.value = ''
  editedEndTime.value = ''
}

// Functions for rectangle selection
const GRID_COLUMNS = 7
const lastMoveTime = ref(0)
const THROTTLE_DELAY = 16 // ~60fps

// Touch gesture detection (to distinguish drag from scroll)
const touchStartTime = ref<number | null>(null)
const touchHoldTimer = ref<number | null>(null)
const touchIsHolding = ref(false) // True if user held for 100ms without moving
const TOUCH_DELAY_THRESHOLD = 100 // ms - minimum delay before drag is confirmed
const isDragConfirmed = ref(false)

// Calculate selected cells in the rectangle
const selectedCellIndices = computed((): Set<number> => {
  if (dragStartIndex.value === null || dragCurrentIndex.value === null) {
    return new Set()
  }

  const startIdx = dragStartIndex.value
  const endIdx = dragCurrentIndex.value

  // Convert indices to coordinates (row, col)
  const startRow = Math.floor(startIdx / GRID_COLUMNS)
  const startCol = startIdx % GRID_COLUMNS
  const endRow = Math.floor(endIdx / GRID_COLUMNS)
  const endCol = endIdx % GRID_COLUMNS

  // Find rectangle boundaries
  const minRow = Math.min(startRow, endRow)
  const maxRow = Math.max(startRow, endRow)
  const minCol = Math.min(startCol, endCol)
  const maxCol = Math.max(startCol, endCol)

  // Collect all indices in the rectangle
  const indices = new Set<number>()
  for (let row = minRow; row <= maxRow; row++) {
    for (let col = minCol; col <= maxCol; col++) {
      indices.add(row * GRID_COLUMNS + col)
    }
  }

  return indices
})

// Check if a cell is selected
function isCellSelected(index: number): boolean {
  return selectedCellIndices.value.has(index)
}

// Check if a cell can be selected
function canSelectCell(day: CalendarDay): boolean {
  return day.isCurrentMonth && !day.isPast && day.isAllowed
}

// Check if a cell has availability (explicit or recurrence)
function cellHasAvailability(day: CalendarDay): boolean {
  return day.hasAvailability || day.hasRecurrence
}

// Get cell index from touch/mouse coordinates
function getCellIndexFromPoint(x: number, y: number): number | null {
  const element = document.elementFromPoint(x, y)
  if (!element) return null

  // Find the calendar cell element - try multiple selectors
  let cell = element.closest('.calendar-grid .grid.grid-cols-7 > div')

  // If we're inside a cell's child element, find the parent cell
  if (!cell && element.closest('.calendar-grid')) {
    const parent = element.parentElement
    if (parent && parent.classList.contains('grid-cols-7')) {
      // We might be a child of the grid, find which cell
      const allCells = parent.children
      for (let i = 0; i < allCells.length; i++) {
        if (allCells[i].contains(element)) {
          cell = allCells[i]
          break
        }
      }
    } else if (parent) {
      // Try going up one more level
      cell = parent.closest('.calendar-grid .grid.grid-cols-7 > div')
    }
  }

  if (!cell) return null

  // Find the index by looking at all cells
  const allCells = cell.parentElement?.children
  if (!allCells) return null

  for (let i = 0; i < allCells.length; i++) {
    if (allCells[i] === cell) return i
  }

  return null
}

// Unified pointer down handler (mouse + touch)
function handlePointerDown(index: number, day: CalendarDay, event: MouseEvent | TouchEvent) {
  // Ignore if it's a click on participant counter or other interactive element
  const target = event.target as HTMLElement
  if (target.closest('[data-no-drag]')) {
    return
  }

  if (!canSelectCell(day)) return

  // For mouse events, start dragging immediately
  if (event.type === 'mousedown') {
    isDragging.value = true
    dragStartIndex.value = index
    dragCurrentIndex.value = index
    dragMode.value = cellHasAvailability(day) ? 'remove' : 'add'
    isDragConfirmed.value = true
    event.preventDefault()
  }
  // For touch events, start a timer to detect if user holds without moving
  else if (event.type === 'touchstart' && 'touches' in event) {
    touchStartTime.value = Date.now()
    touchIsHolding.value = false
    isDragConfirmed.value = false

    // Start timer - if it expires without touchmove, user is holding
    touchHoldTimer.value = window.setTimeout(() => {
      touchIsHolding.value = true
      // Activate drag mode immediately when timer expires
      isDragging.value = true
      isDragConfirmed.value = true
    }, TOUCH_DELAY_THRESHOLD)

    dragStartIndex.value = index
    dragCurrentIndex.value = index
    dragMode.value = cellHasAvailability(day) ? 'remove' : 'add'
    // Don't prevent default yet - allow scroll to work
  }
}

// Unified pointer move handler (mouse only) with throttling
function handlePointerMove(index: number) {
  if (!isDragging.value) return

  // Throttle for performance
  const now = Date.now()
  if (now - lastMoveTime.value < THROTTLE_DELAY) return
  lastMoveTime.value = now

  // For mouse events, we can use the index directly
  dragCurrentIndex.value = index
}

// Grid-level touch move handler (for touch drag selection)
function handleGridTouchMove(event: TouchEvent) {
  // If we haven't started dragging yet, check if this is a drag or scroll
  if (!isDragConfirmed.value && touchStartTime.value !== null) {
    // If user moved BEFORE holding for 100ms → this is a scroll
    if (!touchIsHolding.value) {
      // Cancel the hold timer
      if (touchHoldTimer.value !== null) {
        window.clearTimeout(touchHoldTimer.value)
        touchHoldTimer.value = null
      }
      // Reset touch tracking - allow scroll to proceed
      touchStartTime.value = null
      touchIsHolding.value = false
      dragStartIndex.value = null
      dragCurrentIndex.value = null
      return
    }
    // If user held for 100ms WITHOUT moving, then moved → this is intentional drag
    else {
      isDragging.value = true
      isDragConfirmed.value = true
      // Now prevent default to block scrolling during drag
      event.preventDefault()
    }
  }

  if (!isDragging.value || !isDragConfirmed.value) return

  // IMPORTANT: Prevent scrolling IMMEDIATELY on ALL touchmove events once drag is confirmed
  // This must happen BEFORE throttle check to avoid scroll jank
  event.preventDefault()

  // Throttle for performance (only for updating drag position)
  const now = Date.now()
  if (now - lastMoveTime.value < THROTTLE_DELAY) return
  lastMoveTime.value = now

  if (event.touches.length > 0) {
    const touch = event.touches[0]
    const cellIndex = getCellIndexFromPoint(touch.clientX, touch.clientY)
    if (cellIndex !== null) {
      dragCurrentIndex.value = cellIndex
    }
  }
}

// Unified pointer up handler
function handlePointerUp() {
  // Clean up timer if still running
  if (touchHoldTimer.value !== null) {
    window.clearTimeout(touchHoldTimer.value)
    touchHoldTimer.value = null
  }

  // Only process if drag was confirmed (for touch) or if it was a mouse drag
  if (!isDragging.value && !isDragConfirmed.value) {
    // Reset touch tracking
    touchStartTime.value = null
    touchIsHolding.value = false
    dragStartIndex.value = null
    dragCurrentIndex.value = null
    isDragConfirmed.value = false
    return
  }

  // If touch wasn't confirmed as drag (just a tap), treat as single click
  if (!isDragConfirmed.value && dragStartIndex.value !== null) {
    const day = calendarDays.value[dragStartIndex.value]
    if (day && canSelectCell(day)) {
      emit('day-click', day.dateString)
    }
    // Reset state
    isDragging.value = false
    dragStartIndex.value = null
    dragCurrentIndex.value = null
    dragMode.value = 'add'
    touchStartTime.value = null
    touchIsHolding.value = false
    isDragConfirmed.value = false
    return
  }

  const currentMode = dragMode.value

  // Collect valid selected dates
  const selectedDates: string[] = []

  selectedCellIndices.value.forEach(index => {
    const day = calendarDays.value[index]
    if (day && canSelectCell(day)) {
      // In remove mode, only keep cells with availability
      // In add mode, keep all valid cells
      if (currentMode === 'remove') {
        if (cellHasAvailability(day)) {
          selectedDates.push(day.dateString)
        }
      } else {
        selectedDates.push(day.dateString)
      }
    }
  })

  // Emit event if dates were selected
  if (selectedDates.length > 0) {
    // Sort dates chronologically
    selectedDates.sort()

    // If single date, emit day-click for compatibility (toggle)
    if (selectedDates.length === 1) {
      emit('day-click', selectedDates[0])
    } else {
      // Emit appropriate event based on mode
      if (currentMode === 'remove') {
        emit('days-deselect', selectedDates)
      } else {
        emit('days-select', selectedDates)
      }
    }
  }

  // Reset drag state
  isDragging.value = false
  dragStartIndex.value = null
  dragCurrentIndex.value = null
  dragMode.value = 'add'
  touchStartTime.value = null
  touchIsHolding.value = false
  isDragConfirmed.value = false
}

// Cancel drag if pointer leaves the grid
function handlePointerLeave() {
  // Clean up timer if still running
  if (touchHoldTimer.value !== null) {
    window.clearTimeout(touchHoldTimer.value)
    touchHoldTimer.value = null
  }

  if (isDragging.value) {
    isDragging.value = false
    dragStartIndex.value = null
    dragCurrentIndex.value = null
    dragMode.value = 'add'
    touchStartTime.value = null
    touchIsHolding.value = false
    isDragConfirmed.value = false
  }
}
</script>

<style scoped>
.calendar-grid {
  user-select: none;
}

/* Prevent text selection during drag */
.calendar-grid .grid.grid-cols-7 {
  -webkit-user-select: none;
  user-select: none;
}

/* Mobile optimizations */
@media (max-width: 768px) {
  /* Reduce transition duration on mobile for better performance */
  .calendar-grid .transition-all {
    transition-duration: 0.1s;
  }

  /* Use hardware acceleration for transforms */
  .calendar-grid .grid > div {
    transform: translateZ(0);
    -webkit-transform: translateZ(0);
  }

  /* Improve touch target size on mobile */
  .calendar-grid .grid > div {
    min-height: 5rem;
  }
}

/* Disable hover effects on touch devices */
@media (hover: none) and (pointer: coarse) {
  .calendar-grid .hover\:border-primary-300:hover,
  .calendar-grid .hover\:shadow-sm:hover,
  .calendar-grid .dark\:hover\:border-primary-600:hover {
    border-color: inherit;
    box-shadow: none;
  }
}
</style>
