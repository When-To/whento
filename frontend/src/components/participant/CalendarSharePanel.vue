<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="card">
    <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
      {{ t('calendar.sharingLinks', 'Sharing Links') }}
    </h2>
    <div class="space-y-3">
      <!-- Public Link -->
      <div v-if="showPublicLink">
        <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
          {{ t('calendar.publicLink', 'Public link') }}
        </label>
        <div class="flex gap-2">
          <input :value="publicLink" readonly class="input flex-1 text-sm" />
          <button
            class="btn btn-secondary"
            :title="t('calendar.copyLink', 'Copy link')"
            @click="emit('copy', publicLink)"
          >
            <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
              />
            </svg>
          </button>
        </div>
      </div>

      <!-- ICS Link -->
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
          {{ t('calendar.icsLink', 'iCal subscription link') }}
        </label>
        <div class="flex gap-2">
          <input :value="icsLink" readonly class="input flex-1 text-sm" />
          <button
            class="btn btn-secondary"
            :title="t('calendar.copyLink', 'Copy link')"
            @click="emit('copy', icsLink)"
          >
            <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
              />
            </svg>
          </button>
        </div>
      </div>

      <!-- Settings Link (Owner/Admin only) -->
      <div v-if="settingsPath">
        <router-link :to="settingsPath" class="btn btn-ghost w-full justify-center">
          <svg class="mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
            />
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
            />
          </svg>
          {{ t('calendar.editCalendar', 'Edit calendar') }}
        </router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * The links out of a calendar: the public invitation, the iCal subscription feed, and —
 * for whoever may manage it — the settings page.
 *
 * Purely presentational. Copying is emitted rather than performed here: the toast that
 * confirms it belongs to the page, and a panel that reached for the clipboard itself
 * could not be rendered in a test without stubbing `navigator`.
 */
import { useI18n } from 'vue-i18n';

defineProps<{
  publicLink: string;
  icsLink: string;
  /** Hidden on calendars with a locked roster: there is no self-service link to give out. */
  showPublicLink: boolean;
  /** Route to the settings page, or null when the reader cannot manage the calendar. */
  settingsPath?: string | null;
}>();

const emit = defineEmits<{
  copy: [text: string];
}>();

const { t } = useI18n();
</script>
