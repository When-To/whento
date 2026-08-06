<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { DisplayMode, ViewStyle } from '@/composables/calendar/useCalendarViewState';

/**
 * The two orthogonal view switches, rendered once by the parent.
 *
 * Each of the three grids used to render its own copy of this select, and none of them
 * could render the others, so they emitted a "switch to that view" event and reset
 * their own select afterwards — three variants of the same trick. The classic/compact
 * choice is gone entirely: the month grid transposes itself in CSS at narrow widths, so
 * there is nothing left to choose.
 */

const displayMode = defineModel<DisplayMode>('displayMode', { required: true });
const viewStyle = defineModel<ViewStyle>('viewStyle', { required: true });

const { t } = useI18n();
</script>

<template>
  <div class="flex flex-wrap items-end gap-3">
    <label class="flex flex-col gap-1 text-xs text-gray-600 dark:text-gray-400">
      {{ t('calendar.displayMode') }}
      <select v-model="displayMode" class="input py-1 text-sm">
        <option value="month">{{ t('calendar.monthView') }}</option>
        <option value="week">{{ t('calendar.weekView') }}</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-xs text-gray-600 dark:text-gray-400">
      {{ t('calendar.viewClassic') }}
      <select v-model="viewStyle" class="input py-1 text-sm">
        <option value="grid">{{ t('calendar.viewClassic') }}</option>
        <option value="list">{{ t('calendar.listView') }}</option>
      </select>
    </label>
  </div>
</template>
