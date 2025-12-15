<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="weekly-calendar-grid">
    <!-- Week navigation -->
    <div class="mb-4 grid grid-cols-3 items-center gap-2">
      <div class="flex justify-start">
        <button
          v-if="showNavigation"
          class="btn btn-ghost btn-sm md:btn-sm min-h-[44px] md:min-h-0 px-3"
          :title="t('calendar.previousWeek', 'Previous week')"
          @click="previousWeek"
        >
          <svg class="h-6 w-6 md:h-5 md:w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 19l-7-7 7-7"
            />
          </svg>
        </button>
      </div>

      <h3 class="text-base md:text-lg font-semibold text-gray-900 dark:text-white text-center px-2">
        {{ weekRangeText }}
      </h3>

      <div class="flex justify-end">
        <button
          v-if="showNavigation"
          class="btn btn-ghost btn-sm md:btn-sm min-h-[44px] md:min-h-0 px-3"
          :title="t('calendar.nextWeek', 'Next week')"
          @click="nextWeek"
        >
          <svg class="h-6 w-6 md:h-5 md:w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M9 5l7 7-7 7"
            />
          </svg>
        </button>
      </div>
    </div>

    <!-- Grid container with sticky header -->
    <div
      class="overflow-auto rounded-lg border border-gray-200 dark:border-gray-700"
      @touchmove="handleContainerTouchMove"
    >
      <div
        class="min-w-[600px] md:min-w-[800px]"
        @mouseup="handlePointerUp"
        @touchend="handlePointerUp"
        @touchcancel="handlePointerLeave"
      >
        <!-- Header row with days -->
        <div
          class="sticky top-0 z-10 grid grid-cols-[60px_repeat(7,1fr)] md:grid-cols-[80px_repeat(7,1fr)] bg-gray-50 dark:bg-gray-800"
          @touchmove="handleHeaderGridTouchMove"
        >
          <!-- Empty corner cell -->
          <div class="border-b border-r border-gray-200 p-1 md:p-2 dark:border-gray-700" />

          <!-- Day headers -->
          <div
            v-for="day in weekDays"
            :key="day.dateString"
            :class="[
              'border-b border-r border-gray-200 p-1 md:p-2 text-center dark:border-gray-700 last:border-r-0 select-none',
              // Cursor styles
              isDateEnabled(day.date) ? 'cursor-pointer' : 'cursor-not-allowed',
              // Full-day availability indicator
              hasFullDayAvailability(day.dateString) &&
                !isHeaderSelected(day.dateString) &&
                'bg-primary-500 dark:bg-primary-600 text-white',
              // Hover state when no full-day availability
              isDateEnabled(day.date) &&
                !hasFullDayAvailability(day.dateString) &&
                !isHeaderSelected(day.dateString) &&
                'hover:bg-primary-50 dark:hover:bg-primary-900/20',
              // Holiday borders
              day.isHoliday &&
                !isHeaderSelected(day.dateString) &&
                'border-t-2 border-l-2 border-r-2 border-t-orange-400! border-l-orange-400! border-r-orange-400! dark:border-t-orange-500! dark:border-l-orange-500! dark:border-r-orange-500!',
              !day.isHoliday &&
                props.allowHolidayEves &&
                day.isHolidayEve &&
                !isHeaderSelected(day.dateString) &&
                'border-t-2 border-l-2 border-r-2 border-t-purple-400! border-l-purple-400! border-r-purple-400! dark:border-t-purple-500! dark:border-l-purple-500 dark:border-r-purple-500!',
              // Header drag selection - add mode (yellow)
              isHeaderDragging &&
                headerDragMode === 'add' &&
                isHeaderSelected(day.dateString) &&
                isDateEnabled(day.date) &&
                'ring-2 ring-inset ring-yellow-500 bg-yellow-100 dark:bg-yellow-900/30',
              // Header drag selection - remove mode (red)
              isHeaderDragging &&
                headerDragMode === 'remove' &&
                isHeaderSelected(day.dateString) &&
                isDateEnabled(day.date) &&
                'ring-2 ring-inset ring-red-500 bg-red-100 dark:bg-red-900/30',
            ]"
            :title="day.holidayName || undefined"
            :data-header-date="day.dateString"
            @mousedown="handleHeaderPointerDown(day.dateString, day.date, $event)"
            @mouseenter="handleHeaderPointerMove(day.dateString, day.date)"
            @mouseup="handleHeaderPointerUp"
            @touchstart="handleHeaderPointerDown(day.dateString, day.date, $event)"
          >
            <div
              :class="[
                'text-xs font-medium',
                hasFullDayAvailability(day.dateString) && !isHeaderSelected(day.dateString)
                  ? 'text-white'
                  : 'text-gray-900 dark:text-white',
              ]"
            >
              {{ day.dayName }}
            </div>
            <div
              :class="[
                'text-xs',
                hasFullDayAvailability(day.dateString) && !isHeaderSelected(day.dateString)
                  ? 'text-white/80'
                  : 'text-gray-600 dark:text-gray-400',
              ]"
            >
              {{ day.dateFormatted }}
            </div>
          </div>
        </div>

        <!-- Time slots grid -->
        <div class="relative" @touchmove="handleGridTouchMove">
          <!-- Time labels and slots -->
          <div
            v-for="timeSlot in timeSlots"
            :key="timeSlot.time"
            class="grid grid-cols-[60px_repeat(7,1fr)] md:grid-cols-[80px_repeat(7,1fr)]"
          >
            <!-- Time label -->
            <div
              class="flex items-center justify-end border-b border-r border-gray-200 pr-1 md:pr-2 text-xs text-gray-600 dark:border-gray-700 dark:text-gray-400"
              :class="{
                'font-semibold border-t-2 border-t-gray-300 dark:border-t-gray-600':
                  timeSlot.isHourStart,
                'border-t border-t-gray-100 dark:border-t-gray-800': !timeSlot.isHourStart,
              }"
              :style="{ height: getCellHeight() }"
            >
              {{ timeSlot.isHourStart ? timeSlot.time : '' }}
            </div>

            <!-- Day cells for this time slot -->
            <div
              v-for="day in weekDays"
              :key="`${day.dateString}-${timeSlot.time}`"
              class="relative overflow-hidden border-b border-r border-gray-200 p-0 dark:border-gray-700 last:border-r-0 transition-colors"
              :class="getCellClasses(day, timeSlot)"
              :style="getCellStyle(day.dateString, timeSlot.time, timeSlot.isHourStart)"
              :title="day.holidayName || undefined"
              :data-date="day.dateString"
              :data-time="timeSlot.time"
              @mousedown="handlePointerDown(day.dateString, timeSlot.time, day.date, $event)"
              @mouseenter="handlePointerMove(day.dateString, timeSlot.time, day.date)"
              @mouseup="handlePointerUp"
              @touchstart="handlePointerDown(day.dateString, timeSlot.time, day.date, $event)"
            >
              <!-- Fill overlays (for all fills) -->
              <template v-if="!isSlotSelected(day.dateString, timeSlot.time)">
                <div
                  v-for="(fill, fillIndex) in getCellFills(day.dateString, timeSlot.time)"
                  :key="`fill-${fillIndex}`"
                  :style="getFillStyle(fill)"
                  class="z-10"
                />
              </template>

              <!-- Threshold indicator (green border) - full cell -->
              <div
                v-if="
                  cellStyles[`${day.dateString}:${timeSlot.time}`]?.threshold &&
                  !hasPartialThreshold(day.dateString, timeSlot.time) &&
                  !isDragging &&
                  !isSlotSelected(day.dateString, timeSlot.time)
                "
                class="absolute inset-0 pointer-events-none z-20"
                :style="getThresholdBorderStyle(day.dateString, timeSlot.time)"
              />

              <!-- Threshold indicator (green border) - partial cell -->
              <div
                v-if="
                  hasPartialThreshold(day.dateString, timeSlot.time) &&
                  !isDragging &&
                  !isSlotSelected(day.dateString, timeSlot.time)
                "
                :style="getThresholdIndicatorStyle(day.dateString, timeSlot.time)"
                class="z-20"
              />

              <!-- Participant count labels (one per segment starting in this cell) -->
              <template v-if="!isSlotSelected(day.dateString, timeSlot.time)">
                <div
                  v-for="(fill, fillIndex) in getCellFills(day.dateString, timeSlot.time).filter(
                    f => f.isFirst
                  )"
                  :key="`label-${fillIndex}`"
                  :style="getLabelPositionStyleForFill(fill)"
                  class="z-30"
                  @mouseenter="
                    handleParticipantCountHoverStart(day.dateString, timeSlot.time, $event)
                  "
                  @mouseleave="handleParticipantCountHoverEnd"
                >
                  <span
                    class="text-[10px] font-semibold text-white hover:underline cursor-pointer pointer-events-auto"
                    @click.stop="handleParticipantCountClick(day.dateString, timeSlot.time, $event)"
                    @mousedown.stop
                  >
                    <span class="lg:hidden"
                      >{{ fill.count }} {{ t('calendar.participantShort', 'part.') }}</span
                    >
                    <span class="hidden lg:inline"
                      >{{ fill.count }} {{ t('calendar.participantCount', 'participant(s)') }}</span
                    >
                  </span>
                </div>
              </template>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Legend -->
    <div
      v-if="showLegend"
      class="mt-4 flex items-center gap-4 flex-wrap text-sm text-gray-600 dark:text-gray-400"
    >
      <div class="flex items-center gap-2">
        <div class="h-4 w-4 rounded bg-primary-500" />
        <span>{{ t('availability.available', 'Available') }}</span>
      </div>
      <div class="flex items-center gap-2">
        <div
          class="h-4 w-4 rounded border-[3px] border-green-600 dark:border-green-500 bg-white dark:bg-gray-800"
        />
        <span>{{ t('calendar.thresholdMet', 'Event (threshold met)') }}</span>
      </div>
      <div class="flex items-center gap-2">
        <div class="h-4 w-4 rounded bg-gray-200 dark:bg-gray-700" />
        <span>{{ t('calendar.dateNotAllowed', 'Date not allowed') }}</span>
      </div>
      <div class="flex items-center gap-2">
        <div
          class="h-4 w-4 rounded border border-gray-300 ring-1 ring-orange-400 dark:border-gray-600 dark:ring-orange-500"
        />
        <span>{{ t('calendar.publicHoliday', 'Public holiday') }}</span>
      </div>
      <div v-if="props.allowHolidayEves" class="flex items-center gap-2">
        <div
          class="h-4 w-4 rounded border border-gray-300 ring-1 ring-purple-400 dark:border-gray-600 dark:ring-purple-500"
        />
        <span>{{ t('calendar.holidayEve', 'Holiday eve') }}</span>
      </div>

      <!-- Time range and slot duration controls -->
      <div class="mb-4 grid grid-cols-1 md:grid-cols-3 gap-3">
        <div class="flex items-center gap-2">
          <label
            for="startHour"
            class="text-sm text-gray-700 dark:text-gray-300 shrink-0 w-20 md:w-auto"
          >
            {{ t('calendar.startHour', 'Start hour') }}
          </label>
          <TimeSelect
            id="startHour"
            v-model="startHourTime"
            class="input text-sm flex-1 md:w-24 min-h-[44px] md:min-h-0"
            :max="endHourTime"
            :round-interval="slotDuration as 15 | 30 | 60 | undefined"
          />
        </div>

        <div class="flex items-center gap-2">
          <label
            for="endHour"
            class="text-sm text-gray-700 dark:text-gray-300 shrink-0 w-20 md:w-auto"
          >
            {{ t('calendar.endHour', 'End hour') }}
          </label>
          <TimeSelect
            id="endHour"
            v-model="endHourTime"
            class="input text-sm flex-1 md:w-24 min-h-[44px] md:min-h-0"
            :min="startHourTime"
            :round-interval="slotDuration as 15 | 30 | 60 | undefined"
          />
        </div>

        <div class="flex items-center gap-2">
          <label
            for="slotDuration"
            class="text-sm text-gray-700 dark:text-gray-300 shrink-0 w-20 md:w-auto"
          >
            {{ t('calendar.slotDuration', 'Slot duration') }}
          </label>
          <select
            id="slotDuration"
            v-model.number="slotDuration"
            class="input text-sm flex-1 md:w-28 min-h-[44px] md:min-h-0"
          >
            <option :value="15">15 min</option>
            <option :value="30">30 min</option>
            <option :value="60">60 min</option>
          </select>
        </div>
      </div>
    </div>

    <!-- Participant Details Tooltip -->
    <div
      v-if="selectedSlotKey"
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
            {{ formatSelectedDateTime }}
          </h3>
          <button
            class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            @click="closeParticipantPopup"
          >
            <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>

        <div v-if="loadingDetails" class="text-center py-8">
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
              {{ slotParticipants.length }}
              {{
                slotParticipants.length > 1
                  ? t('calendar.participants', 'Participants')
                  : t('calendar.participantCount', 'participant(s)')
              }}
            </p>
          </div>

          <div class="space-y-2">
            <div
              v-for="participant in slotParticipants"
              :key="participant.participant_name"
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
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
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

        <div v-else class="text-center py-8">
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('availability.noAvailabilities', 'No availabilities') }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useToastStore } from '@/stores/toast'
import { availabilitiesApi } from '@/api/availabilities'
import type { Availability, DateAvailabilitySummary } from '@/types'
import TimeSelect from '@/components/TimeSelect.vue'
import { useDateValidation, clearHolidaysCache } from '@/composables/useDateValidation'

const { t, locale } = useI18n()
const toastStore = useToastStore()

// IMPORTANT: Clear holidays cache on each component creation
// to ensure computed properties use the correct data
clearHolidaysCache()

const { checkIsHoliday, checkIsHolidayEve, getHolidayName } = useDateValidation()

export interface AvailabilityOperation {
  type: 'create' | 'delete' | 'update'
  date: string
  startTime: string
  endTime: string
  oldStartTime?: string
  oldEndTime?: string
}

interface Props {
  // Week configuration
  initialYear: number
  initialMonth: number
  initialWeek: number // Week number (1-5)
  weekStartDate?: Date // Direct week start date (overrides initialYear/Month/Week if provided)
  showNavigation?: boolean
  showTimeControls?: boolean
  showLegend?: boolean

  // Calendar data
  availabilities: Availability[]
  dateSummaries?: DateAvailabilitySummary[]
  participantCounts?: Record<string, number>
  threshold?: number
  allowedWeekdays?: number[]
  timezone?: string
  startDate?: string
  endDate?: string
  holidaysPolicy?: string
  allowHolidayEves?: boolean
  weekdayTimes?: Record<string, { min_time?: string; max_time?: string }>
  holidayMinTime?: string
  holidayMaxTime?: string
  holidayEveMinTime?: string
  holidayEveMaxTime?: string

  // Tokens
  calendarToken: string
  currentParticipantId: string // UUID for API calls
  currentParticipantName: string // Name for visual comparison with range data

  // Display settings
  initialStartHour?: number
  initialEndHour?: number
  initialSlotDuration?: number
}

const props = withDefaults(defineProps<Props>(), {
  showNavigation: true,
  showTimeControls: true,
  showLegend: true,
  allowedWeekdays: () => [0, 1, 2, 3, 4, 5, 6],
  initialStartHour: 8,
  initialEndHour: 20,
  initialSlotDuration: 15,
})

interface Emits {
  (e: 'week-change', weekStartDate: Date): void
  (e: 'availability-create', date: string, startTime: string, endTime: string): void
  (e: 'availability-delete', date: string, startTime: string, endTime: string): void
  (
    e: 'availability-update',
    date: string,
    oldStartTime: string,
    oldEndTime: string,
    newStartTime: string,
    newEndTime: string
  ): void
  (e: 'batch-operations', operations: AvailabilityOperation[]): void
  (
    e: 'settings-change',
    settings: { startHour?: number; endHour?: number; slotDuration?: number }
  ): void
  (e: 'availability-updated'): void
}

const emit = defineEmits<Emits>()

// Popup state for participant details
const selectedSlotKey = ref<string | null>(null) // format: "YYYY-MM-DD|HH:MM"
const participantDetails = ref<DateAvailabilitySummary | null>(null)
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

// Helper to convert decimal hours to HH:MM format
function decimalHourToTimeString(decimalHour: number): string {
  const hours = Math.floor(decimalHour)
  const minutes = Math.round((decimalHour - hours) * 60)
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`
}

// Time range and slot duration filters - store times as "HH:MM" strings
const startHourTime = ref(decimalHourToTimeString(props.initialStartHour))
const endHourTime = ref(
  props.initialEndHour === 24 ? '00:00' : decimalHourToTimeString(props.initialEndHour)
)
const slotDuration = ref(props.initialSlotDuration)

// Watch for changes and emit to parent (convert back to decimal for parent)
watch(startHourTime, newValue => {
  if (!newValue || !newValue.includes(':')) return
  const [hourStr, minuteStr] = newValue.split(':')
  const hour = parseInt(hourStr, 10)
  const minute = parseInt(minuteStr, 10)
  if (isNaN(hour) || isNaN(minute)) return
  const decimalValue = hour + minute / 60
  emit('settings-change', { startHour: decimalValue })
})

watch(endHourTime, newValue => {
  if (!newValue || !newValue.includes(':')) return
  const [hourStr, minuteStr] = newValue.split(':')
  const hour = parseInt(hourStr, 10)
  const minute = parseInt(minuteStr, 10)
  if (isNaN(hour) || isNaN(minute)) return
  // If time is 00:00, treat as 24:00 (end of day)
  const decimalValue = hour === 0 && minute === 0 ? 24 : hour + minute / 60
  emit('settings-change', { endHour: decimalValue })
})

watch(slotDuration, newValue => {
  emit('settings-change', { slotDuration: newValue })
})

// Watch props to sync local state when parent updates (for multiple grid instances)
watch(
  () => props.initialStartHour,
  newValue => {
    const newTimeString = decimalHourToTimeString(newValue)
    if (startHourTime.value !== newTimeString) {
      startHourTime.value = newTimeString
    }
  }
)

watch(
  () => props.initialEndHour,
  newValue => {
    const newTimeString = newValue === 24 ? '00:00' : decimalHourToTimeString(newValue)
    if (endHourTime.value !== newTimeString) {
      endHourTime.value = newTimeString
    }
  }
)

watch(
  () => props.initialSlotDuration,
  newValue => {
    if (slotDuration.value !== newValue) {
      slotDuration.value = newValue
    }
  }
)

// Calculate initial week start date
function getWeekStartDate(year: number, month: number, week: number): Date {
  const firstDayOfMonth = new Date(year, month, 1)
  const firstDayOfWeek = locale.value === 'fr' ? 1 : 0 // Monday for fr, Sunday for en

  const dayOfWeek = firstDayOfMonth.getDay()
  const diff = (dayOfWeek - firstDayOfWeek + 7) % 7
  const weekStart = new Date(firstDayOfMonth)
  weekStart.setDate(firstDayOfMonth.getDate() - diff + (week - 1) * 7)

  return weekStart
}

// Current week tracking - use weekStartDate prop if provided, otherwise calculate
const currentWeekStartDate = ref<Date>(
  props.weekStartDate
    ? new Date(props.weekStartDate)
    : getWeekStartDate(props.initialYear, props.initialMonth, props.initialWeek)
)

// Pointer drag state (mouse + touch)
const isDragging = ref(false)
const dragStartDate = ref<string | null>(null)
const dragStartTime = ref<string | null>(null)
const dragEndDate = ref<string | null>(null)
const dragEndTime = ref<string | null>(null)
const dragMode = ref<'add' | 'remove'>('add')
const lastMoveTime = ref(0)
const THROTTLE_DELAY = 16 // ~60fps

// Touch gesture detection (to distinguish drag from scroll)
const touchStartTime = ref<number | null>(null)
const touchHoldTimer = ref<number | null>(null)
const touchIsHolding = ref(false) // True if user held for 100ms without moving
const TOUCH_DELAY_THRESHOLD = 100 // ms - minimum delay before drag is confirmed
const isDragConfirmed = ref(false)
const isHeaderDragConfirmed = ref(false)

// Header drag state (for day header click-and-drag)
const isHeaderDragging = ref(false)
const headerDragStartDate = ref<string | null>(null)
const headerDragEndDate = ref<string | null>(null)
const headerDragMode = ref<'add' | 'remove'>('add')

// Generate time slots with custom duration and filtered by start/end hours
const timeSlots = computed(() => {
  const slots = []
  const duration = slotDuration.value

  // Parse start time
  const [startHourStr, startMinuteStr] = startHourTime.value.split(':')
  const startHourInt = parseInt(startHourStr, 10)
  const startMinute = parseInt(startMinuteStr, 10)

  // Parse end time
  const [endHourStr, endMinuteStr] = endHourTime.value.split(':')
  let endHourInt = parseInt(endHourStr, 10)
  const endMinute = parseInt(endMinuteStr, 10)

  // Handle 00:00 as 24:00 (end of day)
  if (endHourInt === 0 && endMinute === 0) {
    endHourInt = 24
  }

  // Generate time slots from start to end
  for (let hour = startHourInt; hour <= endHourInt; hour++) {
    // Determine minute range for this hour
    const minMinute = hour === startHourInt ? startMinute : 0
    const maxMinute = hour === endHourInt ? endMinute : 60

    for (let minute = minMinute; minute < maxMinute; minute += duration) {
      const time = `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
      slots.push({
        time,
        hour,
        minute,
        isHourStart: minute === 0,
      })
    }
  }

  return slots
})

// Generate 7 days for the week
const weekDays = computed(() => {
  const days = []
  const start = new Date(currentWeekStartDate.value)
  const timezone = props.timezone || 'Europe/Paris'

  for (let i = 0; i < 7; i++) {
    const date = new Date(start)
    date.setDate(start.getDate() + i)

    const dateString = formatDateForAPI(date)
    const dayName = formatDayName(date)
    const dateFormatted = formatDateShort(date)

    days.push({
      date,
      dateString,
      dayName,
      dateFormatted,
      isHoliday: checkIsHoliday(date, timezone),
      isHolidayEve: checkIsHolidayEve(date, timezone),
      holidayName: getHolidayName(date, timezone) ?? undefined,
    })
  }

  return days
})

// Precompute unified segments for ALL participants
// Segments are split by total participant count, then colored based on current participant presence
const allParticipantSegments = computed(() => {
  const result: Record<
    string,
    Array<{ startMin: number; endMin: number; count: number; hasCurrentParticipant: boolean }>
  > = {}

  if (!props.dateSummaries) return result

  for (const summary of props.dateSummaries) {
    const events: { time: number; type: 'start' | 'end'; participantName: string }[] = []
    for (const participant of summary.participants) {
      events.push({
        time: participant.start_time ? timeToMinutes(participant.start_time) : 0,
        type: 'start',
        participantName: participant.participant_name,
      })
      events.push({
        time: participant.end_time ? timeToMinutes(participant.end_time) : 24 * 60,
        type: 'end',
        participantName: participant.participant_name,
      })
    }

    events.sort((a, b) => {
      if (a.time !== b.time) return a.time - b.time
      return a.type === 'start' ? -1 : 1
    })

    const segments: Array<{
      startMin: number
      endMin: number
      count: number
      hasCurrentParticipant: boolean
    }> = []
    const activeParticipants = new Set<string>()
    let segmentStart: number | null = null

    for (const event of events) {
      // Save current segment if there are active participants
      if (segmentStart !== null && activeParticipants.size > 0 && event.time > segmentStart) {
        segments.push({
          startMin: segmentStart,
          endMin: event.time,
          count: activeParticipants.size,
          hasCurrentParticipant: activeParticipants.has(props.currentParticipantName),
        })
      }

      // Update active participants
      if (event.type === 'start') {
        activeParticipants.add(event.participantName)
      } else {
        activeParticipants.delete(event.participantName)
      }

      // Set new segment start
      if (activeParticipants.size > 0) {
        segmentStart = event.time
      } else {
        segmentStart = null
      }
    }

    result[summary.date] = segments
  }

  return result
})

// Precompute threshold segments for the week
const thresholdSegments = computed(() => {
  const result: Record<string, Array<{ startMin: number; endMin: number }>> = {}

  if (!props.dateSummaries) return result

  const threshold = props.threshold ?? 1

  for (const summary of props.dateSummaries) {
    const intervals: { start: number; end: number }[] = []

    for (const participant of summary.participants) {
      const startMin = participant.start_time ? timeToMinutes(participant.start_time) : 0
      const endMin = participant.end_time ? timeToMinutes(participant.end_time) : 24 * 60
      intervals.push({ start: startMin, end: endMin })
    }

    if (intervals.length < threshold) continue

    // Sweep line to find ranges where threshold is met
    const events: { time: number; type: 'start' | 'end' }[] = []
    for (const interval of intervals) {
      events.push({ time: interval.start, type: 'start' })
      events.push({ time: interval.end, type: 'end' })
    }

    events.sort((a, b) => {
      if (a.time !== b.time) return a.time - b.time
      return a.type === 'start' ? -1 : 1
    })

    let count = 0
    let thresholdStart: number | null = null
    const ranges: { start: number; end: number }[] = []

    for (const event of events) {
      if (event.type === 'start') {
        count++
        if (count >= threshold && thresholdStart === null) {
          thresholdStart = event.time
        }
      } else {
        if (count >= threshold && thresholdStart !== null && count === threshold) {
          ranges.push({ start: thresholdStart, end: event.time })
          thresholdStart = null
        }
        count--
      }
    }

    if (thresholdStart !== null) {
      ranges.push({ start: thresholdStart, end: 24 * 60 })
    }

    if (ranges.length > 0) {
      result[summary.date] = mergeIntervals(ranges).map(r => ({ startMin: r.start, endMin: r.end }))
    }
  }

  return result
})

// Fill segment within a cell
interface CellFill {
  type: 'availability' | 'participantCount'
  count: number
  topPercent: number // 0-100, where fill starts
  bottomPercent: number // 0-100, where fill ends
  isFirst: boolean // First cell of this segment
  isLast: boolean // Last cell of this segment
}

// Cell style type for the new per-cell approach
interface CellStyle {
  // Multiple fills possible in one cell (different segments)
  fills: CellFill[]
  // Threshold (green border)
  threshold?: boolean
  thresholdIsFirst?: boolean
  thresholdIsLast?: boolean
  thresholdTopPercent?: number
  thresholdBottomPercent?: number
}

// Precompute ALL cell styles once (avoids 1000s of function calls in template)
// Each cell knows its own fill and border styles
const cellStyles = computed(() => {
  const result: Record<string, CellStyle> = {}

  const slots = timeSlots.value
  const days = weekDays.value
  const duration = slotDuration.value

  // For each day, process all segments and mark cells
  for (const day of days) {
    const dateStr = day.dateString
    const allSegs = allParticipantSegments.value[dateStr] || []
    const threshSegs = thresholdSegments.value[dateStr] || []

    // Clip segments to visible range
    const firstSlotMin = slots.length > 0 ? timeToMinutes(slots[0].time) : 0
    const lastSlotMin = slots.length > 0 ? timeToMinutes(slots[slots.length - 1].time) : 24 * 60
    const maxVisibleMin = lastSlotMin + duration

    // Process availability segments - each segment adds a fill to the cell
    for (const seg of allSegs) {
      // Skip segments outside visible range
      if (seg.endMin <= firstSlotMin || seg.startMin >= maxVisibleMin) continue

      // Clip segment to visible range
      const segStartMin = Math.max(seg.startMin, firstSlotMin)
      const segEndMin = Math.min(seg.endMin, maxVisibleMin)

      // Find all slots this segment covers
      for (let i = 0; i < slots.length; i++) {
        const slot = slots[i]
        const slotStartMin = timeToMinutes(slot.time)
        const slotEndMin = slotStartMin + duration

        // Check if this slot is covered by the segment
        if (slotEndMin <= segStartMin || slotStartMin >= segEndMin) continue

        const key = `${dateStr}:${slot.time}`

        // Initialize cell data if needed
        if (!result[key]) result[key] = { fills: [] }

        // Calculate position in segment
        const isFirstCell = segStartMin >= slotStartMin && segStartMin < slotEndMin
        const isLastCell = segEndMin > slotStartMin && segEndMin <= slotEndMin

        // Calculate fill percentages within this cell
        const topPercent = isFirstCell ? ((segStartMin - slotStartMin) / duration) * 100 : 0
        const bottomPercent = isLastCell ? ((segEndMin - slotStartMin) / duration) * 100 : 100

        // Add this fill to the cell
        result[key].fills.push({
          type: seg.hasCurrentParticipant ? 'availability' : 'participantCount',
          count: seg.count,
          topPercent,
          bottomPercent,
          isFirst: isFirstCell,
          isLast: isLastCell,
        })
      }
    }

    // Process threshold segments
    for (const seg of threshSegs) {
      // Skip segments outside visible range
      if (seg.endMin <= firstSlotMin || seg.startMin >= maxVisibleMin) continue

      // Clip segment to visible range
      const segStartMin = Math.max(seg.startMin, firstSlotMin)
      const segEndMin = Math.min(seg.endMin, maxVisibleMin)

      // Find all slots this segment covers
      for (let i = 0; i < slots.length; i++) {
        const slot = slots[i]
        const slotStartMin = timeToMinutes(slot.time)
        const slotEndMin = slotStartMin + duration

        // Check if this slot is covered by the segment
        if (slotEndMin <= segStartMin || slotStartMin >= segEndMin) continue

        const key = `${dateStr}:${slot.time}`

        // Initialize cell data if needed
        if (!result[key]) result[key] = { fills: [] }
        const cellData = result[key]

        cellData.threshold = true

        // Calculate position in threshold segment
        const isFirstCell = segStartMin >= slotStartMin && segStartMin < slotEndMin
        const isLastCell = segEndMin > slotStartMin && segEndMin <= slotEndMin

        if (isFirstCell) {
          cellData.thresholdIsFirst = true
          cellData.thresholdTopPercent = ((segStartMin - slotStartMin) / duration) * 100
        } else {
          cellData.thresholdTopPercent = 0
        }

        if (isLastCell) {
          cellData.thresholdIsLast = true
          cellData.thresholdBottomPercent = ((segEndMin - slotStartMin) / duration) * 100
        } else {
          cellData.thresholdBottomPercent = 100
        }
      }
    }
  }

  return result
})

// Helper to get threshold border styles
function getThresholdBorderStyle(dateString: string, time: string): Record<string, string> {
  const style = cellStyles.value[`${dateString}:${time}`]
  if (!style || !style.threshold) return {}

  const isDark = document.documentElement.classList.contains('dark')
  const greenColor = isDark ? 'rgb(34, 197, 94)' : 'rgb(22, 163, 74)' // green-500/600

  const result: Record<string, string> = {
    borderLeftColor: greenColor,
    borderRightColor: greenColor,
    borderLeftWidth: '3px',
    borderRightWidth: '3px',
  }

  // Add top border if first cell of threshold
  const isFullTop = style.thresholdTopPercent === 0 || style.thresholdTopPercent === undefined
  if (style.thresholdIsFirst && isFullTop) {
    result.borderTopColor = greenColor
    result.borderTopWidth = '3px'
  }

  // Add bottom border if last cell of threshold
  const isFullBottom =
    style.thresholdBottomPercent === 100 || style.thresholdBottomPercent === undefined
  if (style.thresholdIsLast && isFullBottom) {
    result.borderBottomColor = greenColor
    result.borderBottomWidth = '3px'
  }

  return result
}

// Helper to get all fills for a cell
function getCellFills(dateString: string, time: string): CellFill[] {
  const style = cellStyles.value[`${dateString}:${time}`]
  return style?.fills || []
}

// Helper to check if cell is fully covered by fills (no gaps at top/bottom)
function isFullCellFill(dateString: string, time: string): boolean {
  const fills = getCellFills(dateString, time)
  if (fills.length === 0) return false
  // Check if fills cover from 0% to 100%
  const minTop = Math.min(...fills.map(f => f.topPercent))
  const maxBottom = Math.max(...fills.map(f => f.bottomPercent))
  return minTop === 0 && maxBottom === 100
}

// Helper to get label position style for a specific fill (centered within its boundaries)
function getLabelPositionStyleForFill(fill: CellFill): Record<string, string> {
  const topPercent = fill.topPercent
  const bottomPercent = fill.bottomPercent

  // Position the label container to match the segment boundaries
  return {
    position: 'absolute',
    left: '0',
    right: '0',
    top: `${topPercent}%`,
    height: `${bottomPercent - topPercent}%`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    pointerEvents: 'none',
  }
}

// Helper to get fill style for a specific fill segment
function getFillStyle(fill: CellFill): Record<string, string> {
  const isDark = document.documentElement.classList.contains('dark')

  // Use CSS variables for theme colors
  const bgColor =
    fill.type === 'availability'
      ? isDark
        ? 'var(--color-primary-600)'
        : 'var(--color-primary-500)'
      : isDark
        ? 'rgba(59, 130, 246, 0.5)' // blue-500/50
        : 'rgb(147, 197, 253)' // blue-300

  return {
    position: 'absolute',
    left: '0',
    right: '0',
    top: `${fill.topPercent}%`,
    height: `${fill.bottomPercent - fill.topPercent}%`,
    backgroundColor: bgColor,
    pointerEvents: 'none',
  }
}

// Helper to check if cell needs threshold lines
function hasPartialThreshold(dateString: string, time: string): boolean {
  const style = cellStyles.value[`${dateString}:${time}`]
  if (!style || !style.threshold) return false
  const topPercent = style.thresholdTopPercent ?? 0
  const bottomPercent = style.thresholdBottomPercent ?? 100
  return topPercent > 0 || bottomPercent < 100
}

// Helper to get threshold indicator style for partial cells
function getThresholdIndicatorStyle(dateString: string, time: string): Record<string, string> {
  const style = cellStyles.value[`${dateString}:${time}`]
  if (!style || !style.threshold) return {}

  const topPercent = style.thresholdTopPercent ?? 0
  const bottomPercent = style.thresholdBottomPercent ?? 100

  const isDark = document.documentElement.classList.contains('dark')
  const greenColor = isDark ? 'rgb(34, 197, 94)' : 'rgb(22, 163, 74)'

  return {
    position: 'absolute',
    left: '0',
    right: '0',
    top: `${topPercent}%`,
    height: `${bottomPercent - topPercent}%`,
    borderLeft: `3px solid ${greenColor}`,
    borderRight: `3px solid ${greenColor}`,
    borderTop: style.thresholdIsFirst ? `3px solid ${greenColor}` : 'none',
    borderBottom: style.thresholdIsLast ? `3px solid ${greenColor}` : 'none',
    pointerEvents: 'none',
    boxSizing: 'border-box',
  }
}

// Helper to get cell CSS classes (for non-fill related styling)
function getCellClasses(
  day: { dateString: string; date: Date; isHoliday: boolean; isHolidayEve: boolean },
  timeSlot: { time: string; isHourStart: boolean }
): Record<string, boolean> {
  const fills = getCellFills(day.dateString, timeSlot.time)
  const hasFills = fills.length > 0 && !isSlotSelected(day.dateString, timeSlot.time)
  const isFullCell = hasFills && isFullCellFill(day.dateString, timeSlot.time)

  return {
    // Disabled state
    'bg-gray-50 dark:bg-gray-800 cursor-not-allowed':
      !isDateEnabled(day.date) || !isTimeSlotAllowed(day.date, timeSlot.time),

    // Hover state (only when no fill and not dragging)
    'cursor-pointer hover:bg-primary-50 dark:hover:bg-primary-900/20':
      isDateEnabled(day.date) &&
      isTimeSlotAllowed(day.date, timeSlot.time) &&
      !isDragging.value &&
      !isFullCell,

    // Cursor pointer for filled cells
    'cursor-pointer': !!(isFullCell && !isDragging.value),

    // Hour border (top) - only show when cell doesn't have a full fill
    'border-t-2 border-t-gray-300 dark:border-t-gray-600': timeSlot.isHourStart && !isFullCell,
    'border-t border-t-gray-100 dark:border-t-gray-800': !timeSlot.isHourStart && !isFullCell,

    // Holiday borders (only left and right sides) - only when no fill
    'border-l-2 border-r-2 !border-l-orange-400 !border-r-orange-400 dark:!border-l-orange-500 dark:!border-r-orange-500':
      day.isHoliday &&
      !isDragging.value &&
      !isSlotSelected(day.dateString, timeSlot.time) &&
      !isFullCell,
    'border-l-2 border-r-2 !border-l-purple-400 !border-r-purple-400 dark:!border-l-purple-500 dark:!border-r-purple-500':
      !day.isHoliday &&
      props.allowHolidayEves &&
      day.isHolidayEve &&
      !isDragging.value &&
      !isSlotSelected(day.dateString, timeSlot.time) &&
      !isFullCell,

    // Rectangle selection style - add mode (yellow)
    'ring-2 ring-inset ring-yellow-500 bg-yellow-100 dark:bg-yellow-900/30':
      isDragging.value &&
      dragMode.value === 'add' &&
      isSlotSelected(day.dateString, timeSlot.time) &&
      isDateEnabled(day.date) &&
      isTimeSlotAllowed(day.date, timeSlot.time),

    // Rectangle selection style - remove mode (red)
    'ring-2 ring-inset ring-red-500 bg-red-100 dark:bg-red-900/30':
      isDragging.value &&
      dragMode.value === 'remove' &&
      isSlotSelected(day.dateString, timeSlot.time) &&
      isDateEnabled(day.date) &&
      isTimeSlotAllowed(day.date, timeSlot.time) &&
      hasAvailability(day.dateString, timeSlot.time),
  }
}

// Helper to get cell inline styles (for fill colors and border hiding)
function getCellStyle(
  dateString: string,
  time: string,
  _isHourStart: boolean
): Record<string, string> {
  const baseStyle: Record<string, string> = { height: getCellHeight() }
  const fills = getCellFills(dateString, time)

  if (fills.length === 0 || isSlotSelected(dateString, time)) {
    return baseStyle
  }

  // For cells with multiple fills or partial fills, use overlay divs
  // Only apply background styling for single full fills
  if (fills.length !== 1) {
    return baseStyle
  }

  const fill = fills[0]
  const isFullTop = fill.topPercent === 0
  const isFullBottom = fill.bottomPercent === 100

  // For partial fills, we use an overlay div instead
  if (!isFullTop || !isFullBottom) {
    return baseStyle
  }

  // Get colors for border blending (theme colors from style.css)
  const isDark = document.documentElement.classList.contains('dark')
  const isAvailability = fill.type === 'availability'

  const bgColor = isAvailability
    ? isDark
      ? 'rgb(2, 132, 199)' // --color-primary-600: #0284c7
      : 'rgb(14, 165, 233)' // --color-primary-500: #0ea5e9
    : isDark
      ? 'rgb(59, 130, 246)' // blue-500
      : 'rgb(147, 197, 253)' // blue-300

  // Hide top border if not first cell of segment (blend with cell above)
  if (!fill.isFirst) {
    baseStyle.borderTopColor = bgColor
  }

  // Hide bottom border if not last cell of segment (blend with cell below)
  if (!fill.isLast) {
    baseStyle.borderBottomColor = bgColor
  }

  return baseStyle
}

// Week range text for display
const weekRangeText = computed(() => {
  const start = weekDays.value[0]?.date
  const end = weekDays.value[6]?.date

  if (!start || !end) return ''

  return `${formatDateShort(start)} - ${formatDateShort(end)}`
})

// Check if a date is in the past
function isPastDate(date: Date): boolean {
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const checkDate = new Date(date)
  checkDate.setHours(0, 0, 0, 0)
  return checkDate < today
}

// Check if a time slot is allowed for a given date
function isTimeSlotAllowed(date: Date, time: string): boolean {
  const dayOfWeek = date.getDay()
  const timezone = props.timezone || 'Europe/Paris'
  const isHoliday = checkIsHoliday(date, timezone)
  const isHolidayEve = checkIsHolidayEve(date, timezone)

  // Check if the day itself is allowed by weekday restrictions
  const isDayAllowed = props.allowedWeekdays && props.allowedWeekdays.includes(dayOfWeek)

  // Determine if this day should be enabled based on holiday policy
  let dayIsEnabled = false
  let minTime: string | undefined
  let maxTime: string | undefined

  // Case 1: Day is a holiday
  if (isHoliday) {
    if (props.holidaysPolicy === 'allow') {
      // Holiday is allowed - use holiday time restrictions if any
      minTime = props.holidayMinTime
      maxTime = props.holidayMaxTime
      dayIsEnabled = true
    } else if (props.holidaysPolicy === 'block') {
      // Holiday is blocked
      return false
    } else {
      // holidays_policy === 'ignore' - treat as regular day
      if (!isDayAllowed) return false
      // Use weekday restrictions
      if (props.weekdayTimes && props.weekdayTimes[dayOfWeek]) {
        minTime = props.weekdayTimes[dayOfWeek].min_time
        maxTime = props.weekdayTimes[dayOfWeek].max_time
      }
      dayIsEnabled = true
    }

    // If day is allowed AND has weekday restrictions, merge with holiday restrictions
    if (dayIsEnabled && isDayAllowed && props.weekdayTimes && props.weekdayTimes[dayOfWeek]) {
      const weekdayMin = props.weekdayTimes[dayOfWeek].min_time
      const weekdayMax = props.weekdayTimes[dayOfWeek].max_time

      // Take the widest range (earliest start, latest end)
      if (minTime && weekdayMin) {
        minTime = minTime < weekdayMin ? minTime : weekdayMin
      } else {
        minTime = minTime || weekdayMin
      }

      if (maxTime && weekdayMax) {
        maxTime = maxTime > weekdayMax ? maxTime : weekdayMax
      } else {
        maxTime = maxTime || weekdayMax
      }
    }
  }
  // Case 2: Day is a holiday eve
  else if (isHolidayEve) {
    if (props.allowHolidayEves) {
      // Holiday eve is allowed - use holiday eve time restrictions if any
      minTime = props.holidayEveMinTime
      maxTime = props.holidayEveMaxTime
      dayIsEnabled = true
    } else {
      // Holiday eve not explicitly allowed - treat as regular day
      if (!isDayAllowed) return false
      // Use weekday restrictions
      if (props.weekdayTimes && props.weekdayTimes[dayOfWeek]) {
        minTime = props.weekdayTimes[dayOfWeek].min_time
        maxTime = props.weekdayTimes[dayOfWeek].max_time
      }
      dayIsEnabled = true
    }

    // If day is allowed AND has weekday restrictions, merge with holiday eve restrictions
    if (dayIsEnabled && isDayAllowed && props.weekdayTimes && props.weekdayTimes[dayOfWeek]) {
      const weekdayMin = props.weekdayTimes[dayOfWeek].min_time
      const weekdayMax = props.weekdayTimes[dayOfWeek].max_time

      // Take the widest range (earliest start, latest end)
      if (minTime && weekdayMin) {
        minTime = minTime < weekdayMin ? minTime : weekdayMin
      } else {
        minTime = minTime || weekdayMin
      }

      if (maxTime && weekdayMax) {
        maxTime = maxTime > weekdayMax ? maxTime : weekdayMax
      } else {
        maxTime = maxTime || weekdayMax
      }
    }
  }
  // Case 3: Regular day (not holiday or holiday eve)
  else {
    if (!isDayAllowed) return false

    // Use weekday restrictions
    if (props.weekdayTimes && props.weekdayTimes[dayOfWeek]) {
      minTime = props.weekdayTimes[dayOfWeek].min_time
      maxTime = props.weekdayTimes[dayOfWeek].max_time
    }
    dayIsEnabled = true
  }

  // If day is not enabled at all, slot is not allowed
  if (!dayIsEnabled) return false

  // If no time restrictions, all times are allowed
  if (!minTime && !maxTime) return true

  // Check if the time slot is within the allowed time range
  const slotStartMin = timeToMinutes(time)
  const slotEndMin = slotStartMin + slotDuration.value

  // Parse min and max times
  const minTimeMin = minTime ? timeToMinutes(minTime) : 0
  const maxTimeMin = maxTime ? timeToMinutes(maxTime) : 24 * 60

  // Slot is allowed if it overlaps with the allowed time range
  // Overlap check: slot start < allowed end AND slot end > allowed start
  return slotStartMin < maxTimeMin && slotEndMin > minTimeMin
}

// Check if a date is enabled (day-level check, without time consideration)
function isDateEnabled(date: Date): boolean {
  const dateString = formatDateForAPI(date)

  // Disable past dates
  if (isPastDate(date)) return false

  // Check if date is within allowed range
  if (props.startDate && dateString < props.startDate) return false
  if (props.endDate && dateString > props.endDate) return false

  const dayOfWeek = date.getDay()
  const timezone = props.timezone || 'Europe/Paris'
  const isHoliday = checkIsHoliday(date, timezone)
  const isHolidayEve = checkIsHolidayEve(date, timezone)

  // Check if the day itself is allowed
  const isDayAllowed = props.allowedWeekdays && props.allowedWeekdays.includes(dayOfWeek)

  // Holiday handling
  if (isHoliday) {
    if (props.holidaysPolicy === 'allow') return true
    if (props.holidaysPolicy === 'block') return false
    // holidays_policy === 'ignore' - treat as regular day
    return isDayAllowed
  }

  // Holiday eve handling
  if (isHolidayEve && props.allowHolidayEves) {
    return true
  }

  // Regular day - check if weekday is allowed
  return isDayAllowed
}

// Check if there's an availability at this time slot
function hasAvailability(dateString: string, time: string): boolean {
  return props.availabilities.some(av => {
    if (av.date !== dateString) return false

    // If no times specified, it's all day
    if (!av.start_time && !av.end_time) return true

    const startTime = av.start_time || '00:00'
    const endTime = av.end_time || '23:59'

    return time >= startTime && time < endTime
  })
}

// Check if a date has a full-day availability (no times or 00:00-23:59)
function hasFullDayAvailability(dateString: string): boolean {
  return props.availabilities.some(av => {
    if (av.date !== dateString) return false

    // Full day if no times specified
    if (!av.start_time && !av.end_time) return true

    // Full day if 00:00-23:59
    return av.start_time === '00:00' && av.end_time === '23:59'
  })
}

// Get cell height - constant regardless of slot duration
function getCellHeight(): string {
  const height = slotDuration.value !== 60 ? 20 : 30
  return `${height}px`
}

// Get total participant count (including current participant) at this time slot
function getTotalParticipantCount(dateString: string, time: string): number {
  if (!props.dateSummaries) return 0

  // Find the summary for this date
  const summary = props.dateSummaries.find(s => s.date === dateString)
  if (!summary) return 0

  const slotStartMin = timeToMinutes(time)
  const slotEndMin = slotStartMin + slotDuration.value

  // Count all participants who have availability overlapping this slot
  let count = 0
  for (const participant of summary.participants) {
    // If no times specified, participant is available all day
    if (!participant.start_time && !participant.end_time) {
      count++
      continue
    }

    const startTime = participant.start_time || '00:00'
    const endTime = participant.end_time || '23:59'
    const availStartMin = timeToMinutes(startTime)
    const availEndMin = timeToMinutes(endTime)

    // Check for overlap with this slot
    if (slotStartMin < availEndMin && slotEndMin > availStartMin) {
      count++
    }
  }

  return count
}

// Calculate selected cells in the rectangle during drag
const selectedSlots = computed((): Set<string> => {
  if (
    !isDragging.value ||
    !dragStartDate.value ||
    !dragStartTime.value ||
    !dragEndDate.value ||
    !dragEndTime.value
  ) {
    return new Set()
  }

  // Get day indices
  const startDayIndex = weekDays.value.findIndex(d => d.dateString === dragStartDate.value)
  const endDayIndex = weekDays.value.findIndex(d => d.dateString === dragEndDate.value)

  if (startDayIndex < 0 || endDayIndex < 0) return new Set()

  // Get time slot indices
  const startTimeIndex = timeSlots.value.findIndex(t => t.time === dragStartTime.value)
  const endTimeIndex = timeSlots.value.findIndex(t => t.time === dragEndTime.value)

  if (startTimeIndex < 0 || endTimeIndex < 0) return new Set()

  // Calculate rectangle boundaries
  const minDayIndex = Math.min(startDayIndex, endDayIndex)
  const maxDayIndex = Math.max(startDayIndex, endDayIndex)
  const minTimeIndex = Math.min(startTimeIndex, endTimeIndex)
  const maxTimeIndex = Math.max(startTimeIndex, endTimeIndex)

  // Collect all slots in the rectangle
  const slots = new Set<string>()
  for (let dayIdx = minDayIndex; dayIdx <= maxDayIndex; dayIdx++) {
    const day = weekDays.value[dayIdx]
    if (!day) continue

    for (let timeIdx = minTimeIndex; timeIdx <= maxTimeIndex; timeIdx++) {
      const timeSlot = timeSlots.value[timeIdx]
      if (!timeSlot) continue

      const key = `${day.dateString}:${timeSlot.time}`
      slots.add(key)
    }
  }

  return slots
})

// Check if a slot is currently selected in the drag rectangle
function isSlotSelected(dateString: string, time: string): boolean {
  return selectedSlots.value.has(`${dateString}:${time}`)
}

// Check if a day header is currently selected in the header drag
function isHeaderSelected(dateString: string): boolean {
  if (!isHeaderDragging.value || !headerDragStartDate.value || !headerDragEndDate.value) {
    return false
  }

  // Get day indices
  const startDayIndex = weekDays.value.findIndex(d => d.dateString === headerDragStartDate.value)
  const endDayIndex = weekDays.value.findIndex(d => d.dateString === headerDragEndDate.value)
  const currentDayIndex = weekDays.value.findIndex(d => d.dateString === dateString)

  if (startDayIndex < 0 || endDayIndex < 0 || currentDayIndex < 0) return false

  const minDayIndex = Math.min(startDayIndex, endDayIndex)
  const maxDayIndex = Math.max(startDayIndex, endDayIndex)

  return currentDayIndex >= minDayIndex && currentDayIndex <= maxDayIndex
}

// Helper: Convert time string to minutes for proper comparison
function timeToMinutes(time: string): number {
  const [hours, minutes] = time.split(':').map(Number)
  return hours * 60 + minutes
}

// Note: Old get*Style() functions removed - replaced with cellStyles computed property for performance

// Helper function to merge overlapping intervals
function mergeIntervals(
  intervals: { start: number; end: number }[]
): { start: number; end: number }[] {
  if (intervals.length === 0) return []

  // Sort by start time
  const sorted = [...intervals].sort((a, b) => a.start - b.start)
  const merged: { start: number; end: number }[] = [sorted[0]]

  for (let i = 1; i < sorted.length; i++) {
    const current = sorted[i]
    const lastMerged = merged[merged.length - 1]

    // If current interval overlaps or touches the last merged interval
    if (current.start <= lastMerged.end) {
      // Extend the last merged interval
      lastMerged.end = Math.max(lastMerged.end, current.end)
    } else {
      // No overlap, add as new interval
      merged.push(current)
    }
  }

  return merged
}

// Helper to create or extend availability intelligently
function createOrExtendAvailability(
  date: string,
  newStartTime: string,
  newEndTime: string
): AvailabilityOperation | null {
  // Find existing availability for this date
  const existingAvailability = props.availabilities.find(av => av.date === date)

  if (!existingAvailability) {
    // No existing availability - create new
    return {
      type: 'create',
      date,
      startTime: newStartTime,
      endTime: newEndTime,
    }
  }

  const existingStart = existingAvailability.start_time || '00:00'
  const existingEnd = existingAvailability.end_time || '23:59'

  // Convert to minutes for proper comparison
  const newStartMinutes = timeToMinutes(newStartTime)
  const newEndMinutes = timeToMinutes(newEndTime)
  const existingStartMinutes = timeToMinutes(existingStart)
  const existingEndMinutes = timeToMinutes(existingEnd)

  // Check if new selection is adjacent or overlapping
  // Adjacent: new selection starts where existing ends, or vice versa
  // Overlapping: any overlap between ranges
  const isAdjacentOrOverlapping =
    newStartMinutes <= existingEndMinutes && newEndMinutes >= existingStartMinutes

  if (isAdjacentOrOverlapping) {
    // Extend the existing availability to cover both ranges
    const mergedStartMinutes = Math.min(newStartMinutes, existingStartMinutes)
    const mergedEndMinutes = Math.max(newEndMinutes, existingEndMinutes)

    // Cap end time at 23:59 (1439 minutes) to avoid 00:00 ambiguity
    const cappedEndMinutes = Math.min(mergedEndMinutes, 23 * 60 + 59)

    // Convert back to time strings
    const mergedStart = `${String(Math.floor(mergedStartMinutes / 60)).padStart(2, '0')}:${String(mergedStartMinutes % 60).padStart(2, '0')}`
    const mergedEnd = `${String(Math.floor(cappedEndMinutes / 60)).padStart(2, '0')}:${String(cappedEndMinutes % 60).padStart(2, '0')}`

    return {
      type: 'update',
      date,
      oldStartTime: existingStart,
      oldEndTime: existingEnd,
      startTime: mergedStart,
      endTime: mergedEnd,
    }
  }

  // Not adjacent - cannot create another availability for the same day
  return null
}

// Pointer handlers for drag selection (mouse + touch)
function handlePointerDown(
  dateString: string,
  time: string,
  date: Date,
  event: MouseEvent | TouchEvent
) {
  // Don't allow interaction on disabled dates or time slots
  if (!isDateEnabled(date) || !isTimeSlotAllowed(date, time)) {
    return
  }

  // For mouse events, start dragging immediately
  if (event.type === 'mousedown') {
    isDragging.value = true
    dragStartDate.value = dateString
    dragStartTime.value = time
    dragEndDate.value = dateString
    dragEndTime.value = time
    dragMode.value = hasAvailability(dateString, time) ? 'remove' : 'add'
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

    dragStartDate.value = dateString
    dragStartTime.value = time
    dragEndDate.value = dateString
    dragEndTime.value = time
    dragMode.value = hasAvailability(dateString, time) ? 'remove' : 'add'
    // Don't prevent default yet - allow scroll to work
  }
}

function handlePointerMove(dateString: string, time: string, date: Date) {
  if (!isDragging.value) return

  // Throttle for performance
  const now = Date.now()
  if (now - lastMoveTime.value < THROTTLE_DELAY) return
  lastMoveTime.value = now

  // Don't allow interaction on disabled dates or time slots
  if (!isDateEnabled(date) || !isTimeSlotAllowed(date, time)) {
    return
  }

  dragEndDate.value = dateString
  dragEndTime.value = time
}

// Container-level touch move handler to block scroll when drag is active
function handleContainerTouchMove(event: TouchEvent) {
  // Block scrolling on the container if any drag is confirmed
  if (isDragConfirmed.value || isHeaderDragConfirmed.value) {
    event.preventDefault()
  }
}

// Grid-level touch move handler for time slots (touch drag selection)
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
      dragStartDate.value = null
      dragStartTime.value = null
      dragEndDate.value = null
      dragEndTime.value = null
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
    const element = document.elementFromPoint(touch.clientX, touch.clientY)
    if (!element) return

    // Find the time slot cell
    const cell = element.closest('[data-date][data-time]') as HTMLElement
    if (!cell) return

    const dateString = cell.getAttribute('data-date')
    const time = cell.getAttribute('data-time')
    if (!dateString || !time) return

    // Find the corresponding day and validate
    const day = weekDays.value.find(d => d.dateString === dateString)
    if (!day || !isDateEnabled(day.date) || !isTimeSlotAllowed(day.date, time)) return

    dragEndDate.value = dateString
    dragEndTime.value = time
  }
}

function handlePointerUp() {
  if (!isDragging.value) return

  // Collect all operations to perform in batch
  const operations: AvailabilityOperation[] = []

  // Calculate the selected range
  if (dragStartDate.value && dragStartTime.value && dragEndDate.value && dragEndTime.value) {
    // Check if it's a simple click (same date and time)
    const isSimpleClick =
      dragStartDate.value === dragEndDate.value && dragStartTime.value === dragEndTime.value

    if (isSimpleClick) {
      // Simple click: toggle availability at this specific time
      if (hasAvailability(dragStartDate.value, dragStartTime.value)) {
        // Find the availability that contains this time
        const availability = props.availabilities.find(av => {
          if (av.date !== dragStartDate.value) return false

          const startTime = av.start_time || '00:00'
          const endTime = av.end_time || '23:59'
          const clickedTime = dragStartTime.value!

          return clickedTime >= startTime && clickedTime < endTime
        })

        if (availability) {
          const avStartTime = availability.start_time || '00:00'
          const avEndTime = availability.end_time || '23:59'
          const clickedTime = dragStartTime.value!
          const clickedSlotEnd = addMinutes(clickedTime, slotDuration.value)

          // Check if this is a single-slot availability (clicking would remove it entirely)
          const isSingleSlot = addMinutes(avStartTime, slotDuration.value) >= avEndTime

          // Check if clicked on first slot
          const isFirstSlot = clickedTime === avStartTime

          // Check if clicked on last slot (the slot that ends at or after avEndTime)
          const isLastSlot = clickedSlotEnd >= avEndTime

          if (isSingleSlot || (!isFirstSlot && !isLastSlot)) {
            // Single slot or middle slot: delete the entire availability
            operations.push({
              type: 'delete',
              date: availability.date,
              startTime: avStartTime,
              endTime: avEndTime,
            })
          } else if (isFirstSlot) {
            // First slot: shrink from start (move start time forward)
            const newStartTime = addMinutes(avStartTime, slotDuration.value)
            operations.push({
              type: 'update',
              date: availability.date,
              oldStartTime: avStartTime,
              oldEndTime: avEndTime,
              startTime: newStartTime,
              endTime: avEndTime,
            })
          } else if (isLastSlot) {
            // Last slot: shrink from end (move end time backward)
            // Calculate new end time by going back one slot from the end
            const newEndTime = clickedTime
            operations.push({
              type: 'update',
              date: availability.date,
              oldStartTime: avStartTime,
              oldEndTime: avEndTime,
              startTime: avStartTime,
              endTime: newEndTime,
            })
          }
        }
      } else {
        // Create or extend availability for one slot duration
        const endTime = addMinutes(dragStartTime.value, slotDuration.value)
        const operation = createOrExtendAvailability(
          dragStartDate.value,
          dragStartTime.value,
          endTime
        )
        if (operation) {
          operations.push(operation)
        }
        // Note: Don't show error here - let parent handle it via batch operations
      }
    } else {
      // Drag selection: create or delete availability for the selected range

      // For rectangular selection, we need to normalize the selection bounds
      // regardless of drag direction (top-left to bottom-right, or vice versa)

      // Get the day indices for start and end dates
      const startDayIndex = weekDays.value.findIndex(d => d.dateString === dragStartDate.value)
      const endDayIndex = weekDays.value.findIndex(d => d.dateString === dragEndDate.value)

      // Normalize day range (handle left-to-right or right-to-left drag)
      const minDayIndex = Math.min(startDayIndex, endDayIndex)
      const maxDayIndex = Math.max(startDayIndex, endDayIndex)

      // Normalize time range (handle top-to-bottom or bottom-to-top drag)
      const times = [dragStartTime.value, dragEndTime.value].sort()
      let startTime = times[0]
      let endTime = times[1]

      // Add slot duration to include the end slot
      endTime = addMinutes(endTime, slotDuration.value)

      if (dragMode.value === 'add') {
        // Add mode: create or extend availabilities
        if (minDayIndex === maxDayIndex) {
          const day = weekDays.value[minDayIndex]
          if (day && isDateEnabled(day.date)) {
            const operation = createOrExtendAvailability(day.dateString, startTime, endTime)
            if (operation) {
              operations.push(operation)
            }
            // Note: Don't show error here - let parent handle it via batch operations
          }
        } else {
          // Multi-day rectangular selection: same time range for all selected days
          for (let i = minDayIndex; i <= maxDayIndex; i++) {
            const day = weekDays.value[i]
            if (day && isDateEnabled(day.date)) {
              const operation = createOrExtendAvailability(day.dateString, startTime, endTime)
              if (operation) {
                operations.push(operation)
              }
              // Note: Conflicts are silently ignored - only valid operations are sent
            }
          }
          // Note: Don't show error here - let parent handle it via batch operations
        }
      } else {
        // Remove mode: intelligently cut/modify availabilities
        // First, check if any availability would be split in the middle (forbidden)
        let hasSplitError = false

        for (let i = minDayIndex; i <= maxDayIndex; i++) {
          const day = weekDays.value[i]
          if (!day || !isDateEnabled(day.date)) continue

          // Find all availabilities that overlap with the selected time range for this day
          const overlappingAvailabilities = props.availabilities.filter(av => {
            if (av.date !== day.dateString) return false

            const avStart = av.start_time || '00:00'
            const avEnd = av.end_time || '23:59'

            // Check if availability overlaps with selected range
            return avStart < endTime && avEnd > startTime
          })

          // Check for middle splits (Case 4)
          for (const availability of overlappingAvailabilities) {
            const avStart = availability.start_time || '00:00'
            const avEnd = availability.end_time || '23:59'

            // Case 4: Selection cuts the middle - This is forbidden!
            if (startTime > avStart && endTime < avEnd) {
              hasSplitError = true
              break
            }
          }

          if (hasSplitError) break
        }

        // If there's a split error, show error message and abort
        if (hasSplitError) {
          toastStore.error(
            t(
              'availability.cannotSplitError',
              'Cannot split availability in two. Only one availability per day per participant is allowed.'
            )
          )
          // Reset drag state before aborting
          isDragging.value = false
          dragStartDate.value = null
          dragStartTime.value = null
          dragEndDate.value = null
          dragEndTime.value = null
          dragMode.value = 'add'
          // Don't process the deletion
          return
        }

        // No split error, proceed with deletion/modification
        for (let i = minDayIndex; i <= maxDayIndex; i++) {
          const day = weekDays.value[i]
          if (!day || !isDateEnabled(day.date)) continue

          // Find all availabilities that overlap with the selected time range for this day
          const overlappingAvailabilities = props.availabilities.filter(av => {
            if (av.date !== day.dateString) return false

            const avStart = av.start_time || '00:00'
            const avEnd = av.end_time || '23:59'

            // Check if availability overlaps with selected range
            return avStart < endTime && avEnd > startTime
          })

          // Process each overlapping availability
          for (const availability of overlappingAvailabilities) {
            const avStart = availability.start_time || '00:00'
            const avEnd = availability.end_time || '23:59'

            // Case 1: Selection completely covers the availability
            // Delete the entire availability
            if (startTime <= avStart && endTime >= avEnd) {
              operations.push({
                type: 'delete',
                date: availability.date,
                startTime: avStart,
                endTime: avEnd,
              })
            }
            // Case 2: Selection cuts the beginning
            // Update to keep only the end part
            else if (startTime <= avStart && endTime < avEnd) {
              operations.push({
                type: 'update',
                date: availability.date,
                oldStartTime: avStart,
                oldEndTime: avEnd,
                startTime: endTime,
                endTime: avEnd,
              })
            }
            // Case 3: Selection cuts the end
            // Update to keep only the beginning part
            else if (startTime > avStart && endTime >= avEnd) {
              operations.push({
                type: 'update',
                date: availability.date,
                oldStartTime: avStart,
                oldEndTime: avEnd,
                startTime: avStart,
                endTime: startTime,
              })
            }
          }
        }
      }
    }
  }

  // Emit all operations in batch
  if (operations.length > 0) {
    emit('batch-operations', operations)
  } else if (dragStartDate.value && dragStartTime.value) {
    // No valid operations - all selections were invalid (non-adjacent availabilities)
    toastStore.error(t('errors.availabilityConflict'))
  }

  // Reset drag state
  isDragging.value = false
  dragStartDate.value = null
  dragStartTime.value = null
  dragEndDate.value = null
  dragEndTime.value = null
  dragMode.value = 'add'
  touchStartTime.value = null
  touchIsHolding.value = false
  isDragConfirmed.value = false

  // Clean up timer
  if (touchHoldTimer.value !== null) {
    window.clearTimeout(touchHoldTimer.value)
    touchHoldTimer.value = null
  }
}

// Cancel drag if pointer leaves the grid
function handlePointerLeave() {
  if (isDragging.value) {
    isDragging.value = false
    dragStartDate.value = null
    dragStartTime.value = null
    dragEndDate.value = null
    dragEndTime.value = null
    dragMode.value = 'add'
    touchStartTime.value = null
    touchIsHolding.value = false
    isDragConfirmed.value = false
  }

  if (isHeaderDragging.value) {
    isHeaderDragging.value = false
    headerDragStartDate.value = null
    headerDragEndDate.value = null
    headerDragMode.value = 'add'
    touchStartTime.value = null
    touchIsHolding.value = false
    isHeaderDragConfirmed.value = false
  }

  // Clean up timer
  if (touchHoldTimer.value !== null) {
    window.clearTimeout(touchHoldTimer.value)
    touchHoldTimer.value = null
  }
}

// Header pointer handlers for day header click-and-drag (mouse + touch)
function handleHeaderPointerDown(dateString: string, date: Date, event: MouseEvent | TouchEvent) {
  // Don't allow interaction on disabled dates
  if (!isDateEnabled(date)) {
    return
  }

  // For mouse events, start dragging immediately
  if (event.type === 'mousedown') {
    isHeaderDragging.value = true
    headerDragStartDate.value = dateString
    headerDragEndDate.value = dateString
    headerDragMode.value = hasFullDayAvailability(dateString) ? 'remove' : 'add'
    isHeaderDragConfirmed.value = true
    event.preventDefault()
  }
  // For touch events, start a timer to detect if user holds without moving
  else if (event.type === 'touchstart' && 'touches' in event) {
    touchStartTime.value = Date.now()
    touchIsHolding.value = false
    isHeaderDragConfirmed.value = false

    // Start timer - if it expires without touchmove, user is holding
    touchHoldTimer.value = window.setTimeout(() => {
      touchIsHolding.value = true
      // Activate drag mode immediately when timer expires
      isHeaderDragging.value = true
      isHeaderDragConfirmed.value = true
    }, TOUCH_DELAY_THRESHOLD)

    headerDragStartDate.value = dateString
    headerDragEndDate.value = dateString
    headerDragMode.value = hasFullDayAvailability(dateString) ? 'remove' : 'add'
    // Don't prevent default yet - allow scroll to work
  }
}

function handleHeaderPointerMove(dateString: string, date: Date) {
  if (!isHeaderDragging.value) return

  // Throttle for performance
  const now = Date.now()
  if (now - lastMoveTime.value < THROTTLE_DELAY) return
  lastMoveTime.value = now

  // Don't allow interaction on disabled dates
  if (!isDateEnabled(date)) {
    return
  }

  headerDragEndDate.value = dateString
}

// Grid-level touch move handler for headers (touch drag selection)
function handleHeaderGridTouchMove(event: TouchEvent) {
  // If we haven't started dragging yet, check if this is a drag or scroll
  if (!isHeaderDragConfirmed.value && touchStartTime.value !== null) {
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
      headerDragStartDate.value = null
      headerDragEndDate.value = null
      return
    }
    // If user held for 100ms WITHOUT moving, then moved → this is intentional drag
    else {
      isHeaderDragging.value = true
      isHeaderDragConfirmed.value = true
      // Now prevent default to block scrolling during drag
      event.preventDefault()
    }
  }

  if (!isHeaderDragging.value || !isHeaderDragConfirmed.value) return

  // IMPORTANT: Prevent scrolling IMMEDIATELY on ALL touchmove events once drag is confirmed
  // This must happen BEFORE throttle check to avoid scroll jank
  event.preventDefault()

  // Throttle for performance (only for updating drag position)
  const now = Date.now()
  if (now - lastMoveTime.value < THROTTLE_DELAY) return
  lastMoveTime.value = now

  if (event.touches.length > 0) {
    const touch = event.touches[0]
    const element = document.elementFromPoint(touch.clientX, touch.clientY)
    if (!element) return

    // Find the header cell
    const cell = element.closest('[data-header-date]') as HTMLElement
    if (!cell) return

    const dateString = cell.getAttribute('data-header-date')
    if (!dateString) return

    // Find the corresponding day and validate
    const day = weekDays.value.find(d => d.dateString === dateString)
    if (!day || !isDateEnabled(day.date)) return

    headerDragEndDate.value = dateString
  }
}

function handleHeaderPointerUp() {
  if (!isHeaderDragging.value) return

  // Collect all operations to perform in batch
  const operations: AvailabilityOperation[] = []

  if (headerDragStartDate.value && headerDragEndDate.value) {
    // Get day indices for start and end dates
    const startDayIndex = weekDays.value.findIndex(d => d.dateString === headerDragStartDate.value)
    const endDayIndex = weekDays.value.findIndex(d => d.dateString === headerDragEndDate.value)

    if (startDayIndex >= 0 && endDayIndex >= 0) {
      // Normalize range (handle left-to-right or right-to-left drag)
      const minDayIndex = Math.min(startDayIndex, endDayIndex)
      const maxDayIndex = Math.max(startDayIndex, endDayIndex)

      if (headerDragMode.value === 'add') {
        // Add mode: create full-day availabilities for all selected dates
        for (let i = minDayIndex; i <= maxDayIndex; i++) {
          const day = weekDays.value[i]
          if (day && isDateEnabled(day.date)) {
            // Check if there's already an availability for this date
            const existingAvailability = props.availabilities.find(av => av.date === day.dateString)

            if (!existingAvailability) {
              // No existing availability - create new full-day
              operations.push({
                type: 'create',
                date: day.dateString,
                startTime: '', // Empty means full day
                endTime: '', // Empty means full day
              })
            } else if (!hasFullDayAvailability(day.dateString)) {
              // Existing availability but not full day - extend to full day
              operations.push({
                type: 'update',
                date: day.dateString,
                oldStartTime: existingAvailability.start_time || '00:00',
                oldEndTime: existingAvailability.end_time || '23:59',
                startTime: '', // Empty means full day
                endTime: '', // Empty means full day
              })
            }
            // If already full-day, skip
          }
        }
      } else {
        // Remove mode: delete availabilities for all selected dates
        for (let i = minDayIndex; i <= maxDayIndex; i++) {
          const day = weekDays.value[i]
          if (day && isDateEnabled(day.date)) {
            // Find existing availability for this date
            const existingAvailability = props.availabilities.find(av => av.date === day.dateString)

            if (existingAvailability) {
              operations.push({
                type: 'delete',
                date: day.dateString,
                startTime: existingAvailability.start_time || '00:00',
                endTime: existingAvailability.end_time || '23:59',
              })
            }
          }
        }
      }
    }
  }

  // Emit all operations in batch
  if (operations.length > 0) {
    emit('batch-operations', operations)
  }

  // Reset header drag state
  isHeaderDragging.value = false
  headerDragStartDate.value = null
  headerDragEndDate.value = null
  headerDragMode.value = 'add'
  touchStartTime.value = null
  touchIsHolding.value = false
  isHeaderDragConfirmed.value = false

  // Clean up timer
  if (touchHoldTimer.value !== null) {
    window.clearTimeout(touchHoldTimer.value)
    touchHoldTimer.value = null
  }
}

// Helper to add minutes to a time string (capped at 23:59)
function addMinutes(time: string, minutes: number): string {
  const [hours, mins] = time.split(':').map(Number)
  const totalMinutes = hours * 60 + mins + minutes
  // Cap at 23:59 (1439 minutes) to avoid wrapping to 00:00
  const cappedMinutes = Math.min(totalMinutes, 23 * 60 + 59)
  const newHours = Math.floor(cappedMinutes / 60)
  const newMins = cappedMinutes % 60
  return `${String(newHours).padStart(2, '0')}:${String(newMins).padStart(2, '0')}`
}

// Navigation
function previousWeek() {
  const newStart = new Date(currentWeekStartDate.value)
  newStart.setDate(newStart.getDate() - 7)
  currentWeekStartDate.value = newStart
  emit('week-change', newStart)
}

function nextWeek() {
  const newStart = new Date(currentWeekStartDate.value)
  newStart.setDate(newStart.getDate() + 7)
  currentWeekStartDate.value = newStart
  emit('week-change', newStart)
}

// Formatting helpers
function formatDateForAPI(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function formatDayName(date: Date): string {
  const localeCode = locale.value === 'fr' ? 'fr-FR' : 'en-US'
  return new Intl.DateTimeFormat(localeCode, { weekday: 'short' }).format(date)
}

function formatDateShort(date: Date): string {
  const localeCode = locale.value === 'fr' ? 'fr-FR' : 'en-US'
  return new Intl.DateTimeFormat(localeCode, {
    day: 'numeric',
    month: 'short',
  }).format(date)
}

// Watch for prop changes
watch(
  [
    () => props.initialYear,
    () => props.initialMonth,
    () => props.initialWeek,
    () => props.weekStartDate,
  ],
  () => {
    // If weekStartDate prop is provided, use it; otherwise calculate from year/month/week
    currentWeekStartDate.value = props.weekStartDate
      ? new Date(props.weekStartDate)
      : getWeekStartDate(props.initialYear, props.initialMonth, props.initialWeek)
  }
)

// Watch for prop changes ONLY if this grid does NOT have time controls.
// Grids with time controls are the "master" and emit changes to parent.
// Grids without time controls are "followers" and should sync with parent props.
watch(
  [() => props.initialStartHour, () => props.initialEndHour, () => props.initialSlotDuration],
  ([newStartHour, newEndHour, newSlotDuration]) => {
    // Only sync if this grid doesn't have time controls (it's a "follower")
    if (!props.showTimeControls) {
      startHourTime.value = decimalHourToTimeString(newStartHour)
      endHourTime.value = newEndHour === 24 ? '00:00' : decimalHourToTimeString(newEndHour)
      slotDuration.value = newSlotDuration
    }
  }
)

// Format time range for display
function isFullDayTime(startTime?: string, endTime?: string): boolean {
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

// Computed property for formatted selected date/time
const formatSelectedDateTime = computed(() => {
  if (!selectedSlotKey.value) return ''
  // Format is "YYYY-MM-DD|HH:MM"
  const [dateString, time] = selectedSlotKey.value.split('|')
  const date = new Date(dateString)
  const localeCode = locale.value === 'fr' ? 'fr-FR' : 'en-US'
  const formattedDate = date.toLocaleDateString(localeCode, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
  })
  return `${formattedDate} ${time}`
})

// Computed property to filter participants who are available at the selected time slot
const slotParticipants = computed(() => {
  if (!participantDetails.value || !selectedSlotKey.value) return []

  // Format is "YYYY-MM-DD|HH:MM"
  const [, time] = selectedSlotKey.value.split('|')
  const slotStartMin = timeToMinutes(time)
  const slotEndMin = slotStartMin + slotDuration.value

  return participantDetails.value.participants.filter(participant => {
    // If no times specified, participant is available all day
    if (!participant.start_time && !participant.end_time) {
      return true
    }

    const startTime = participant.start_time || '00:00'
    const endTime = participant.end_time || '23:59'
    const availStartMin = timeToMinutes(startTime)
    const availEndMin = timeToMinutes(endTime)

    // Check for overlap with this slot
    return slotStartMin < availEndMin && slotEndMin > availStartMin
  })
})

// Hover handlers for participant count
async function handleParticipantCountHoverStart(
  dateString: string,
  time: string,
  event: MouseEvent
) {
  const count = getTotalParticipantCount(dateString, time)
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
    loadParticipantDetails(dateString, time)
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

async function loadParticipantDetails(dateString: string, time: string) {
  selectedSlotKey.value = `${dateString}|${time}`
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

function handleParticipantCountClick(dateString: string, time: string, event: MouseEvent) {
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
  loadParticipantDetails(dateString, time)

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
  if (!props.calendarToken || !props.currentParticipantId || !selectedSlotKey.value) {
    return
  }

  // Format is "YYYY-MM-DD|HH:MM"
  const [dateString, time] = selectedSlotKey.value.split('|')
  savingNote.value = true

  try {
    // Update the availability with the new note and times
    await availabilitiesApi.update(props.calendarToken, props.currentParticipantId, dateString, {
      note: editedNote.value || undefined,
      start_time: editedStartTime.value || undefined,
      end_time: editedEndTime.value || undefined,
    })

    // Reload participant details to show updated note
    await loadParticipantDetails(dateString, time)

    // Close edit mode
    editingNote.value = false
    editedNote.value = ''
    editedStartTime.value = ''
    editedEndTime.value = ''

    // Notify parent to reload availabilities
    emit('availability-updated')
  } catch (err) {
    console.error('Failed to update availability:', err)
    toastStore.error(t('errors.updateFailed', 'Failed to update availability'))
  } finally {
    savingNote.value = false
  }
}

function closeParticipantPopup() {
  selectedSlotKey.value = null
  participantDetails.value = null
  editingNote.value = false
  editedNote.value = ''
  editedStartTime.value = ''
  editedEndTime.value = ''
}

// Add global pointer up listeners to handle drag end outside grid
if (typeof window !== 'undefined') {
  window.addEventListener('mouseup', handlePointerUp)
  window.addEventListener('touchend', handlePointerUp)
}
</script>

<style scoped>
.weekly-calendar-grid {
  user-select: none;
}

/* Prevent text selection during drag */
.weekly-calendar-grid .grid.grid-cols-\[80px_repeat\(7\,1fr\)\] {
  -webkit-user-select: none;
  user-select: none;
}

/* Mobile optimizations */
@media (max-width: 768px) {
  /* Reduce transition duration on mobile for better performance */
  .weekly-calendar-grid .transition-colors {
    transition-duration: 0.1s;
  }

  /* Use hardware acceleration for transforms */
  .weekly-calendar-grid .relative {
    transform: translateZ(0);
    -webkit-transform: translateZ(0);
  }

  /* Improve touch target size on mobile */
  .weekly-calendar-grid .border-b.border-r {
    min-height: 3rem;
  }
}

/* Disable hover effects on touch devices */
@media (hover: none) and (pointer: coarse) {
  .weekly-calendar-grid .hover\:bg-primary-50:hover,
  .weekly-calendar-grid .dark\:hover\:bg-primary-900\/20:hover {
    background-color: inherit;
  }
}
</style>
