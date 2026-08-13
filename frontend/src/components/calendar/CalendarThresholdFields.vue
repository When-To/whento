<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <!-- Threshold -->
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('calendar.threshold') }}
      <span class="text-danger-600">*</span>
    </label>
    <input
      v-model.number="threshold"
      type="number"
      min="1"
      :max="allowAnonymousParticipants ? undefined : participantCount || undefined"
      class="input"
      :class="{ 'border-danger-500': thresholdError }"
      required
    />
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      {{ t('calendar.thresholdHelp') }}
      <span v-if="!allowAnonymousParticipants && participantCount > 0">
        ({{ t('common.max') }}: {{ participantCount }})</span
      >
    </p>
    <p v-if="thresholdError" class="mt-1 text-sm text-danger-600">
      {{ thresholdError }}
    </p>
  </div>

  <!-- Minimum Duration -->
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('calendar.minDurationHours') }}
    </label>
    <input v-model.number="minDurationHours" type="number" min="0" class="input" />
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      {{ t('calendar.minDurationHelp') }}
    </p>
  </div>
</template>

<script setup lang="ts">
/**
 * How many participants make a date count, and how long a slot has to be.
 *
 * The two views differ only in where the participant count comes from — a local array
 * being built in the create form, the server's list in the settings form — so that is
 * the one thing passed in rather than read here.
 */
import { useI18n } from 'vue-i18n';

withDefaults(
  defineProps<{
    /** Participants the calendar has; caps the threshold unless anonymous joining is on. */
    participantCount: number;
    allowAnonymousParticipants: boolean;
    thresholdError?: string;
  }>(),
  { thresholdError: '' }
);

const threshold = defineModel<number>('threshold', { required: true });
const minDurationHours = defineModel<number>('minDurationHours', { required: true });

const { t } = useI18n();
</script>
