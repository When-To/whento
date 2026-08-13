<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <!-- Date Range -->
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('calendar.calendarDateRange') }}
    </label>
    <div class="grid grid-cols-2 gap-4">
      <div>
        <input
          v-model="startDate"
          type="date"
          class="input"
          :placeholder="t('calendar.calendarStartDatePlaceholder')"
        />
      </div>
      <div>
        <input
          v-model="endDate"
          type="date"
          class="input"
          :placeholder="t('calendar.calendarEndDatePlaceholder')"
        />
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
    <div class="grid grid-cols-[auto_repeat(7,minmax(0,1fr))] gap-2 items-center overflow-x-auto">
      <!-- Row 1: Label "Jour" + Day buttons -->
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap pr-2">
        {{ locale === 'fr' ? 'Jour' : 'Day' }}
      </label>
      <button
        v-for="day in weekdays"
        :key="day.value"
        type="button"
        class="rounded-lg border-2 px-2 py-2 text-sm font-medium transition-colors"
        :class="{
          'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300':
            allowedWeekdays.includes(day.value),
          'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700':
            !allowedWeekdays.includes(day.value),
        }"
        @click.prevent="handleToggleWeekday(day.value)"
      >
        {{ day.short }}
      </button>

      <!-- Row 2: Label + Start times -->
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap pr-2">
        {{ t('availability.startTime') }}
      </label>
      <TimeSelect
        v-for="day in weekdays"
        :key="`min-${day.value}`"
        v-model="weekdayTimes[day.value].min_time"
        :disabled="!allowedWeekdays.includes(day.value)"
        class="text-sm min-w-0"
        :class="{
          'opacity-50 cursor-not-allowed': !allowedWeekdays.includes(day.value),
        }"
        placeholder="--:--"
      />

      <!-- Row 3: Label + End times -->
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap pr-2">
        {{ t('availability.endTime') }}
      </label>
      <TimeSelect
        v-for="day in weekdays"
        :key="`max-${day.value}`"
        v-model="weekdayTimes[day.value].max_time"
        :disabled="!allowedWeekdays.includes(day.value)"
        class="text-sm min-w-0"
        :class="{
          'opacity-50 cursor-not-allowed': !allowedWeekdays.includes(day.value),
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
          <label for="holidays-policy" class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('calendar.holidaysPolicy') }}
          </label>
          <select id="holidays-policy" v-model="holidaysPolicy" class="input w-48">
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
          v-model="holidayMinTime"
          :disabled="holidaysPolicy !== 'allow'"
          class="w-32 text-sm"
          :class="{
            'opacity-50 cursor-not-allowed': holidaysPolicy !== 'allow',
          }"
          placeholder="Min"
        />
        <span class="text-gray-500 dark:text-gray-400">-</span>
        <TimeSelect
          v-model="holidayMaxTime"
          :disabled="holidaysPolicy !== 'allow'"
          class="w-32 text-sm"
          :class="{
            'opacity-50 cursor-not-allowed': holidaysPolicy !== 'allow',
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
          v-model="allowHolidayEves"
          type="checkbox"
          :disabled="allWeekdaysSelected"
          class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed dark:border-gray-600 dark:bg-gray-700"
        />
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
          v-model="holidayEveMinTime"
          :disabled="!allowHolidayEves || allWeekdaysSelected"
          class="w-32 text-sm"
          :class="{
            'opacity-50 cursor-not-allowed': !allowHolidayEves || allWeekdaysSelected,
          }"
          placeholder="Min"
        />
        <span class="text-gray-500 dark:text-gray-400">-</span>
        <TimeSelect
          v-model="holidayEveMaxTime"
          :disabled="!allowHolidayEves || allWeekdaysSelected"
          class="w-32 text-sm"
          :class="{
            'opacity-50 cursor-not-allowed': !allowHolidayEves || allWeekdaysSelected,
          }"
          placeholder="Max"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * "Allow/block days/hours": the date range, the per-weekday opening times, and the two
 * holiday policies.
 *
 * This block was duplicated verbatim between the create and the settings view — around
 * two hundred lines each, including the `--:--` / `Min` / `Max` placeholders that the
 * audit picked up as identical. Nothing about it depends on whether the calendar exists
 * yet, so unlike the participants panel it merges without either side carrying the
 * other's needs. Each view still supplies its own `CollapsibleSection` wrapper and its
 * own save button, which is where the two genuinely diverge.
 */
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { resolveWeekStart } from '@/utils/weekStart';
import { toggleWeekday, type WeekdayTimes } from '@/utils/calendar/weekdayTimes';
import TimeSelect from '@/components/TimeSelect.vue';

const props = defineProps<{
  /** The calendar's timezone, which decides where the weekday row starts. */
  timezone: string;
}>();

const startDate = defineModel<string>('startDate', { required: true });
const endDate = defineModel<string>('endDate', { required: true });
const allowedWeekdays = defineModel<number[]>('allowedWeekdays', { required: true });
const weekdayTimes = defineModel<WeekdayTimes>('weekdayTimes', { required: true });
const holidaysPolicy = defineModel<'ignore' | 'allow' | 'block'>('holidaysPolicy', {
  required: true,
});
const holidayMinTime = defineModel<string>('holidayMinTime', { required: true });
const holidayMaxTime = defineModel<string>('holidayMaxTime', { required: true });
const allowHolidayEves = defineModel<boolean>('allowHolidayEves', { required: true });
const holidayEveMinTime = defineModel<string>('holidayEveMinTime', { required: true });
const holidayEveMaxTime = defineModel<string>('holidayEveMaxTime', { required: true });

const { t, locale } = useI18n();

// Weekdays (0=Sunday, 6=Saturday)
// Order follows the calendar's timezone, resolved through CLDR.
const weekdays = computed(() => {
  const days = [
    { value: 0, short: t('weekdays.short.sunday') },
    { value: 1, short: t('weekdays.short.monday') },
    { value: 2, short: t('weekdays.short.tuesday') },
    { value: 3, short: t('weekdays.short.wednesday') },
    { value: 4, short: t('weekdays.short.thursday') },
    { value: 5, short: t('weekdays.short.friday') },
    { value: 6, short: t('weekdays.short.saturday') },
  ];

  // Rotated rather than special-cased: CLDR has weeks starting on Saturday in fourteen
  // countries and on Friday in the Maldives, which a Sunday-or-Monday branch cannot
  // express. The selector follows the timezone being chosen, so picking a German city
  // reorders it to Monday-first straight away.
  const first = resolveWeekStart(props.timezone, locale.value);

  return [...days.slice(first), ...days.slice(0, first)];
});

// Check if all weekdays are selected
const allWeekdaysSelected = computed(() => allowedWeekdays.value.length === 7);

function handleToggleWeekday(day: number) {
  toggleWeekday(allowedWeekdays.value, day);
}
</script>
