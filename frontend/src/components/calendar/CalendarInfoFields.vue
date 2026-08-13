<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <!-- Name -->
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('calendar.calendarName') }}
      <span class="text-danger-600">*</span>
    </label>
    <input
      v-model="name"
      type="text"
      class="input"
      :class="{ 'border-danger-500': nameError }"
      :placeholder="t('calendar.calendarNamePlaceholder')"
      required
    />
    <p v-if="nameError" class="mt-1 text-sm text-danger-600">
      {{ nameError }}
    </p>
  </div>

  <!-- Description -->
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('calendar.description') }}
    </label>
    <textarea
      v-model="description"
      rows="3"
      class="input"
      :placeholder="t('calendar.descriptionPlaceholder')"
    />
  </div>

  <!-- Timezone -->
  <div>
    <TimezoneSelector
      v-model="timezone"
      :label="t('calendar.timezone')"
      :help="t('calendar.timezoneHelp')"
    />
  </div>
</template>

<script setup lang="ts">
/**
 * Name, description and timezone — the three fields the create form and the settings
 * form always agreed on, down to the class list.
 *
 * Only the fields are shared. Each view keeps its own card, heading and submit button,
 * because creating and editing genuinely differ there: one posts a whole calendar, the
 * other patches an existing one and tracks unsaved changes.
 */
import { useI18n } from 'vue-i18n';
import TimezoneSelector from '@/components/TimezoneSelector.vue';

defineProps<{
  /** Validation message for the name field, or an empty string when it is valid. */
  nameError?: string;
}>();

const name = defineModel<string>('name', { required: true });
const description = defineModel<string>('description', { required: true });
const timezone = defineModel<string>('timezone', { required: true });

const { t } = useI18n();
</script>
