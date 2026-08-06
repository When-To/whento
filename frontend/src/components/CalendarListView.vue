<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="calendar-list-view">
    <!-- Navigation header -->
    <div class="mb-4 flex items-center justify-between gap-2">
      <button
        class="btn btn-ghost p-2 md:p-2 min-h-11 md:min-h-0 min-w-11 md:min-w-0"
        :title="
          props.displayMode === 'week'
            ? t('calendar.previousWeek', 'Previous week')
            : t('calendar.previousMonth', 'Previous month')
        "
        @click="navigatePrevious"
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

      <div class="flex flex-col items-center gap-1 flex-1 px-2">
        <h3
          class="font-display text-base md:text-lg font-semibold text-gray-900 dark:text-white text-center"
        >
          {{ rangeLabel }}
        </h3>
        <select
          value="list"
          class="text-xs md:text-sm px-2 py-1 rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 focus:outline-none focus:ring-2 focus:ring-primary-500"
          @change="handleViewStyleSelect"
        >
          <option value="classic">{{ t('calendar.viewClassic') }}</option>
          <option v-if="props.displayMode === 'month'" value="compact">
            {{ t('calendar.viewCompact') }}
          </option>
          <option value="list">{{ t('calendar.listView') }}</option>
        </select>
      </div>

      <button
        class="btn btn-ghost p-2 md:p-2 min-h-[44px] md:min-h-0 min-w-[44px] md:min-w-0"
        :title="
          props.displayMode === 'week'
            ? t('calendar.nextWeek', 'Next week')
            : t('calendar.nextMonth', 'Next month')
        "
        @click="navigateNext"
      >
        <svg class="h-6 w-6 md:h-5 md:w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
        </svg>
      </button>
    </div>

    <!-- Empty state -->
    <div
      v-if="actionableDays.length === 0"
      class="py-12 text-center text-gray-500 dark:text-gray-400"
    >
      <p class="text-sm">
        {{ t('calendar.noActionableDays', 'No available days in this period') }}
      </p>
    </div>

    <!-- Day cards list -->
    <div v-else class="space-y-2">
      <button
        v-for="day in actionableDays"
        :key="day.dateString"
        type="button"
        class="w-full text-left rounded-lg border p-3 md:p-4 transition-all cursor-pointer"
        :class="[
          day.meetsThreshold
            ? 'border-green-200 bg-green-50 hover:border-green-300 hover:shadow-sm dark:border-green-700 dark:bg-green-900/20 dark:hover:border-green-600'
            : day.hasAvailability
              ? 'border-primary-200 bg-primary-50 hover:border-primary-300 hover:shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:hover:border-primary-600'
              : day.hasRecurrence
                ? 'border-blue-200 bg-blue-50 hover:border-blue-300 hover:shadow-sm dark:border-blue-700 dark:bg-blue-900/20 dark:hover:border-blue-600'
                : 'border-gray-200 bg-white hover:border-primary-300 hover:shadow-sm dark:border-gray-700 dark:bg-gray-800 dark:hover:border-primary-600',
          day.isToday && 'ring-2 ring-primary-500 ring-offset-1',
          day.isHighlighted && 'ring-2 ring-purple-500 bg-purple-100 dark:bg-purple-900/40',
        ]"
        @pointerdown="onRowPointerDown(day, $event)"
        @pointermove="onRowPointerMove($event)"
        @pointerup="onRowPointerUp()"
        @pointercancel="onRowPointerCancel()"
        @click="onRowClick(day, $event)"
      >
        <div class="flex items-center justify-between gap-3">
          <!-- Left: date info -->
          <div class="flex flex-col min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="font-semibold text-sm md:text-base text-gray-900 dark:text-white">
                {{ day.dayName }}
              </span>
              <span class="text-sm text-gray-600 dark:text-gray-400">
                {{ day.dateFormatted }}
              </span>
              <span
                v-if="day.isToday"
                class="inline-flex items-center rounded-full bg-primary-100 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/40 dark:text-primary-300"
              >
                {{ t('calendar.today', 'Today') }}
              </span>
            </div>
            <!-- Holiday / holiday eve badges -->
            <div v-if="day.isHoliday || day.isHolidayEve" class="mt-1 flex items-center gap-1">
              <span
                v-if="day.isHoliday"
                class="inline-flex items-center rounded-full bg-orange-100 px-2 py-0.5 text-xs font-medium text-orange-700 dark:bg-orange-900/40 dark:text-orange-300"
              >
                {{ day.holidayName || t('calendar.publicHoliday', 'Public holiday') }}
              </span>
              <span
                v-else-if="day.isHolidayEve"
                class="inline-flex items-center rounded-full bg-purple-100 px-2 py-0.5 text-xs font-medium text-purple-700 dark:bg-purple-900/40 dark:text-purple-300"
              >
                {{ t('calendar.holidayEve', 'Holiday eve') }}
              </span>
            </div>
            <!-- Time info for availabilities -->
            <div v-if="day.timeLabel" class="mt-1">
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ day.timeLabel }}</span>
            </div>
          </div>

          <!-- Right: availability status + participant count -->
          <div class="flex items-center gap-2 shrink-0">
            <!-- Availability indicator -->
            <span
              v-if="day.hasAvailability"
              class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-bold text-white"
              :class="
                day.meetsThreshold
                  ? 'bg-green-600 dark:bg-green-500'
                  : 'bg-primary-600 dark:bg-primary-500'
              "
            >
              {{ t('availability.available', 'Available') }}
            </span>
            <!-- Recurrence indicator -->
            <span
              v-else-if="day.hasRecurrence"
              class="inline-flex items-center rounded-full bg-blue-600 px-2 py-0.5 text-xs font-bold text-white dark:bg-blue-500"
            >
              {{ t('availability.recurring', 'Recurring') }}
            </span>
            <!-- Participant count -->
            <span
              v-if="day.participantCount > 0"
              class="inline-flex items-center gap-1 text-xs font-medium"
              :class="
                day.meetsThreshold
                  ? 'text-green-700 dark:text-green-400'
                  : 'text-gray-600 dark:text-gray-400'
              "
            >
              <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z"
                />
              </svg>
              {{ day.participantCount }}
            </span>
            <!-- Threshold met icon -->
            <span
              v-if="day.meetsThreshold"
              class="text-green-600 dark:text-green-400"
              :title="t('calendar.thresholdMet', 'Threshold met')"
            >
              <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
            </span>
          </div>
        </div>
      </button>
    </div>

    <!-- Legend -->
    <div v-if="actionableDays.length > 0" class="mt-4 flex flex-wrap gap-4 text-xs">
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
          class="h-3 w-3 rounded bg-orange-100 border border-orange-200 dark:bg-orange-900/40 dark:border-orange-700"
        />
        <span class="text-gray-600 dark:text-gray-400">{{
          t('calendar.publicHoliday', 'Public holiday')
        }}</span>
      </div>
      <div v-if="allowHolidayEves" class="flex items-center gap-1">
        <div
          class="h-3 w-3 rounded bg-purple-100 border border-purple-200 dark:bg-purple-900/40 dark:border-purple-700"
        />
        <span class="text-gray-600 dark:text-gray-400">{{
          t('calendar.holidayEve', 'Holiday eve')
        }}</span>
      </div>
      <div
        v-if="props.highlightedDates && props.highlightedDates.size > 0"
        class="flex items-center gap-1"
      >
        <div
          class="h-3 w-3 rounded border border-purple-500 ring-2 ring-purple-500 bg-purple-100 dark:bg-purple-900/40"
        />
        <span class="text-gray-600 dark:text-gray-400">
          {{ t('participant.commonDates', 'Common dates') }}
        </span>
      </div>
    </div>

    <ParticipantDetailsPopup
      v-if="popupDate && popupAnchor"
      :calendar-token="calendarToken"
      :current-participant-id="currentParticipantId"
      :current-participant-name="currentParticipantName"
      :date="popupDate"
      :anchor-rect="popupAnchor"
      @close="onPopupClose"
      @availability-updated="onPopupAvailabilityUpdated"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatWeekday, formatFullDate } from '@/utils/dateFormatting';
import { formatDateISO, todayISO } from '@/utils/date/isoDate';
import { getHolidayIndex } from '@/utils/calendar/holidays';
import { buildCalendarRules, isDayOpen } from '@/utils/calendar/dateRules';
import type { Availability, RecurrenceWithExceptions } from '@/types';
import ParticipantDetailsPopup from './ParticipantDetailsPopup.vue';

interface MonthConfig {
  year: number;
  month: number;
}

interface WeekConfig {
  weekStartDate: Date;
}

interface Props {
  displayMode: 'month' | 'week';
  monthsToDisplay: MonthConfig[];
  weeksToDisplay: WeekConfig[];
  availabilities: Availability[];
  recurrences: RecurrenceWithExceptions[];
  participantCounts: Record<string, number>;
  threshold: number;
  allowedWeekdays?: number[];
  timezone?: string;
  holidaysPolicy?: string;
  allowHolidayEves?: boolean;
  startDate?: string;
  endDate?: string;
  displayedYear: number;
  displayedMonth: number;
  currentWeekStartDate: Date;
  currentParticipantId: string;
  currentParticipantName: string;
  calendarToken: string;
  highlightedDates?: Set<string>;
}

interface ListDay {
  dateString: string;
  dateObj: Date;
  dayOfWeek: number;
  dayName: string;
  dateFormatted: string;
  isToday: boolean;
  isHoliday: boolean;
  isHolidayEve: boolean;
  holidayName?: string;
  hasAvailability: boolean;
  hasRecurrence: boolean;
  meetsThreshold: boolean;
  participantCount: number;
  isHighlighted: boolean;
  timeLabel?: string;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (e: 'day-click', dateString: string): void;
  (e: 'month-change', year: number, month: number): void;
  (e: 'week-change', weekStartDate: Date): void;
  (e: 'view-style-change', style: 'classic' | 'compact' | 'list'): void;
  (e: 'availability-updated'): void;
}>();

// Long-press detection for opening the participant details popup
const LONG_PRESS_MS = 500;
const MOVE_THRESHOLD_PX = 8;
const popupDate = ref<string | null>(null);
const popupAnchor = ref<DOMRect | null>(null);
let holdTimer: number | null = null;
let longPressFired = false;
let pointerStartX = 0;
let pointerStartY = 0;

function clearHoldTimer() {
  if (holdTimer !== null) {
    window.clearTimeout(holdTimer);
    holdTimer = null;
  }
}

function onRowPointerDown(day: ListDay, event: PointerEvent) {
  if (!event.isPrimary) return;
  if (day.participantCount === 0) return;
  const target = event.currentTarget as HTMLElement;
  pointerStartX = event.clientX;
  pointerStartY = event.clientY;
  longPressFired = false;
  clearHoldTimer();
  holdTimer = window.setTimeout(() => {
    popupAnchor.value = target.getBoundingClientRect();
    popupDate.value = day.dateString;
    longPressFired = true;
    holdTimer = null;
  }, LONG_PRESS_MS);
}

function onRowPointerMove(event: PointerEvent) {
  if (holdTimer === null) return;
  const dx = event.clientX - pointerStartX;
  const dy = event.clientY - pointerStartY;
  if (Math.hypot(dx, dy) > MOVE_THRESHOLD_PX) {
    clearHoldTimer();
  }
}

function onRowPointerUp() {
  clearHoldTimer();
}

function onRowPointerCancel() {
  clearHoldTimer();
  longPressFired = false;
}

function onRowClick(day: ListDay, event: MouseEvent) {
  if (longPressFired) {
    event.preventDefault();
    event.stopPropagation();
    longPressFired = false;
    return;
  }
  emit('day-click', day.dateString);
}

function onPopupClose() {
  popupDate.value = null;
  popupAnchor.value = null;
}

function onPopupAvailabilityUpdated() {
  emit('availability-updated');
}

const { t, locale } = useI18n();

// Handle view style selection (classic/compact are rendered by parent grid components)
function handleViewStyleSelect(event: Event) {
  const value = (event.target as HTMLSelectElement).value as 'classic' | 'compact' | 'list';
  if (value !== 'list') {
    // Reset dropdown back to 'list' visually, then emit the switch
    (event.target as HTMLSelectElement).value = 'list';
    emit('view-style-change', value);
  }
}
// Calendar constraints and holiday lookup, resolved once instead of per day.
const calendarTimezone = computed(() => props.timezone || 'Europe/Paris');
const holidayIndex = computed(() => getHolidayIndex(calendarTimezone.value, locale.value));
const calendarRules = computed(() =>
  buildCalendarRules({
    timeZone: calendarTimezone.value,
    todayISO: todayISO(calendarTimezone.value),
    allowedWeekdays: props.allowedWeekdays,
    holidaysPolicy: props.holidaysPolicy,
    allowHolidayEves: props.allowHolidayEves,
    startDate: props.startDate,
    endDate: props.endDate,
    threshold: props.threshold,
  })
);

// Range label for navigation header
const rangeLabel = computed(() => {
  const localeCode = locale.value === 'fr' ? 'fr-FR' : 'en-US';

  if (props.displayMode === 'week') {
    if (props.weeksToDisplay.length === 0) return '';
    const first = props.weeksToDisplay[0].weekStartDate;
    const last = props.weeksToDisplay[props.weeksToDisplay.length - 1].weekStartDate;
    const lastEnd = new Date(last);
    lastEnd.setDate(lastEnd.getDate() + 6);

    return `${first.toLocaleDateString(localeCode, { day: 'numeric', month: 'short' })} — ${lastEnd.toLocaleDateString(localeCode, { day: 'numeric', month: 'short', year: 'numeric' })}`;
  }

  // Month mode
  if (props.monthsToDisplay.length === 0) return '';
  const first = props.monthsToDisplay[0];
  const last = props.monthsToDisplay[props.monthsToDisplay.length - 1];
  const firstDate = new Date(first.year, first.month, 1);
  const lastDate = new Date(last.year, last.month, 1);

  if (first.year === last.year && first.month === last.month) {
    return firstDate.toLocaleDateString(localeCode, { month: 'long', year: 'numeric' });
  }

  return `${firstDate.toLocaleDateString(localeCode, { month: 'short' })} — ${lastDate.toLocaleDateString(localeCode, { month: 'short', year: 'numeric' })}`;
});

// Generate all dates in the displayed period, then filter to actionable ones
const actionableDays = computed((): ListDay[] => {
  const allDates: Date[] = [];

  if (props.displayMode === 'month') {
    for (const mc of props.monthsToDisplay) {
      const daysInMonth = new Date(mc.year, mc.month + 1, 0).getDate();
      for (let d = 1; d <= daysInMonth; d++) {
        allDates.push(new Date(mc.year, mc.month, d));
      }
    }
  } else {
    for (const wc of props.weeksToDisplay) {
      for (let i = 0; i < 7; i++) {
        const date = new Date(wc.weekStartDate);
        date.setDate(wc.weekStartDate.getDate() + i);
        allDates.push(date);
      }
    }
  }

  // "Today" follows the calendar's timezone, not the viewer's browser.
  const rules = calendarRules.value;
  const today = rules.todayISO;
  const holidays = holidayIndex.value;

  // Deduplicate dates (month ranges may overlap with week ranges)
  const seen = new Set<string>();
  const days: ListDay[] = [];

  for (const dateObj of allDates) {
    dateObj.setHours(0, 0, 0, 0);
    const dateString = formatDateISO(dateObj);

    if (seen.has(dateString)) continue;
    seen.add(dateString);

    const dayOfWeek = dateObj.getDay();
    const isHoliday = holidays.isHoliday(dateString);
    const isHolidayEve = holidays.isHolidayEve(dateString);

    // Filter: not past, within the calendar range, and accepted by the weekday
    // and holiday rules.
    if (!isDayOpen({ date: dateString, dayOfWeek, isHoliday, isHolidayEve }, rules)) continue;

    // Check availability
    const dateAvailabilities = (props.availabilities || []).filter(a => a.date === dateString);
    const hasAvailability = dateAvailabilities.length > 0;

    // Check recurrence
    let hasRecurrence = false;
    (props.recurrences || []).some(rec => {
      if (rec.day_of_week !== dayOfWeek) return false;
      if (dateString < rec.start_date) return false;
      if (rec.end_date && dateString > rec.end_date) return false;
      const isException = rec.exceptions?.some(ex => ex.excluded_date === dateString);
      if (!isException) {
        hasRecurrence = true;
        return true;
      }
      return false;
    });

    const participantCount = props.participantCounts?.[dateString] || 0;
    const meetsThreshold = participantCount >= (props.threshold || 1);

    // Build time label from availabilities
    let timeLabel: string | undefined;
    if (hasAvailability && dateAvailabilities.length > 0) {
      const firstAvail = dateAvailabilities[0];
      if (firstAvail.start_time && firstAvail.end_time) {
        timeLabel = `${firstAvail.start_time} - ${firstAvail.end_time}`;
      }
    }

    days.push({
      dateString,
      dateObj: new Date(dateObj),
      dayOfWeek,
      dayName: formatWeekday(dateObj, locale.value, 'long'),
      dateFormatted: formatFullDate(dateObj, locale.value),
      isToday: dateString === today,
      isHoliday,
      isHolidayEve,
      holidayName: holidays.getName(dateString) ?? undefined,
      hasAvailability,
      hasRecurrence,
      meetsThreshold,
      participantCount,
      isHighlighted: props.highlightedDates?.has(dateString) || false,
      timeLabel,
    });
  }

  // Sort by date
  days.sort((a, b) => a.dateObj.getTime() - b.dateObj.getTime());

  return days;
});

function navigatePrevious() {
  if (props.displayMode === 'week') {
    const newStart = new Date(props.currentWeekStartDate);
    newStart.setDate(newStart.getDate() - 7);
    emit('week-change', newStart);
  } else {
    const newDate = new Date(props.displayedYear, props.displayedMonth - 1, 1);
    emit('month-change', newDate.getFullYear(), newDate.getMonth());
  }
}

function navigateNext() {
  if (props.displayMode === 'week') {
    const newStart = new Date(props.currentWeekStartDate);
    newStart.setDate(newStart.getDate() + 7);
    emit('week-change', newStart);
  } else {
    const newDate = new Date(props.displayedYear, props.displayedMonth + 1, 1);
    emit('month-change', newDate.getFullYear(), newDate.getMonth());
  }
}
</script>
