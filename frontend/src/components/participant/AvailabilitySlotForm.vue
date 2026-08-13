<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="card">
    <div class="mb-4 flex items-baseline gap-2">
      <h2 class="font-display text-xl font-semibold text-gray-900 dark:text-white">
        {{ t('availability.timeSlot', 'Plage horaire') }}
      </h2>
      <span
        v-if="minDurationHours && minDurationHours > 0"
        class="text-sm text-gray-600 dark:text-gray-400"
      >
        ({{ t('calendar.minDurationHours') }}: {{ minDurationHours }}h)
      </span>
    </div>

    <div class="space-y-3">
      <!-- All Day Checkbox -->
      <div class="flex items-center">
        <input
          id="allDay"
          v-model="allDay"
          type="checkbox"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        />
        <label for="allDay" class="ml-2 text-sm text-gray-700 dark:text-gray-300">
          {{ t('availability.allDay') }}
        </label>
      </div>

      <!-- Time Range -->
      <div class="grid grid-cols-2 gap-2">
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
            {{ t('availability.startTime') }}
          </label>
          <TimeSelect
            v-model="startTime"
            class="text-sm"
            :disabled="allDay"
            :max="endTime || undefined"
          />
        </div>
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
            {{ t('availability.endTime') }}
          </label>
          <TimeSelect
            v-model="endTime"
            class="text-sm"
            :disabled="allDay"
            :min="startTime || undefined"
          />
        </div>
      </div>

      <!-- Note -->
      <div>
        <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
          {{ t('availability.note') }}
        </label>
        <textarea
          v-model="note"
          rows="2"
          class="input text-sm"
          :placeholder="t('availability.note')"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * The default time slot applied to the next day the participant clicks.
 *
 * It is not a form that submits anything: it only holds the values the calendar's click
 * and drag handlers read when they create availabilities, which is why it has no button
 * and why the parent keeps the request object.
 */
import { useI18n } from 'vue-i18n';
import TimeSelect from '@/components/TimeSelect.vue';

defineProps<{
  /** The calendar's minimum slot length, shown next to the heading when it is set. */
  minDurationHours?: number;
}>();

const allDay = defineModel<boolean>('allDay', { required: true });
const startTime = defineModel<string | undefined>('startTime');
const endTime = defineModel<string | undefined>('endTime');
const note = defineModel<string | undefined>('note');

const { t } = useI18n();
</script>
