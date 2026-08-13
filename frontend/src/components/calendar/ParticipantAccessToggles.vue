<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <!-- Lock Participants Toggle (hidden when anonymous registration is enabled) -->
  <div
    v-if="!allowAnonymousParticipants"
    class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
  >
    <div class="flex items-start">
      <input
        :id="`lock-participants${idSuffix}`"
        v-model="lockParticipants"
        type="checkbox"
        class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
        @change="handleLockChange"
      />
      <label
        :for="`lock-participants${idSuffix}`"
        class="ml-2 text-sm text-gray-700 dark:text-gray-300"
      >
        <span class="font-medium">{{ t('calendar.lockParticipants') }}</span>
        <p class="text-gray-500 dark:text-gray-400">
          {{ t('calendar.lockParticipantsHelp') }}
        </p>
      </label>
    </div>
  </div>

  <!-- Allow Anonymous Participants Toggle (hidden when lock participants is enabled) -->
  <div
    v-if="!lockParticipants"
    class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
  >
    <div class="flex items-start">
      <input
        :id="`allow-anonymous-participants${idSuffix}`"
        v-model="allowAnonymousParticipants"
        type="checkbox"
        class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
        @change="handleAnonymousChange"
      />
      <label
        :for="`allow-anonymous-participants${idSuffix}`"
        class="ml-2 text-sm text-gray-700 dark:text-gray-300"
      >
        <span class="font-medium">{{ t('calendar.allowAnonymousParticipants') }}</span>
        <p class="text-gray-500 dark:text-gray-400">
          {{ t('calendar.allowAnonymousParticipantsHelp') }}
        </p>
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * The two mutually exclusive ways a calendar admits participants: a fixed roster
 * (`lock_participants`) or open self-registration (`allow_anonymous_participants`).
 *
 * The exclusion rule lives here because it is a property of the pair, not of either
 * view — both forms enforced it separately, one inline in the template and one inside a
 * save handler. What differs is what happens *next*: the create form is only collecting
 * values, while the settings form patches the calendar immediately, so each toggle
 * emits its own event and the settings view keeps its own handler.
 */
import { useI18n } from 'vue-i18n';

withDefaults(
  defineProps<{
    /**
     * Appended to the checkbox ids so the create form keeps the `-create` suffix it
     * has always rendered.
     */
    idSuffix?: string;
  }>(),
  { idSuffix: '' }
);

const emit = defineEmits<{
  'change:lock': [];
  'change:anonymous': [];
}>();

const lockParticipants = defineModel<boolean>('lockParticipants', { required: true });
const allowAnonymousParticipants = defineModel<boolean>('allowAnonymousParticipants', {
  required: true,
});

const { t } = useI18n();

function handleLockChange() {
  if (lockParticipants.value) allowAnonymousParticipants.value = false;
  emit('change:lock');
}

function handleAnonymousChange() {
  if (allowAnonymousParticipants.value) lockParticipants.value = false;
  emit('change:anonymous');
}
</script>
