<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <CollapsibleSection :title="t('availability.myAvailabilities')" :default-open="false">
    <!-- Availabilities List -->
    <div class="space-y-2">
      <div
        v-for="availability in sortedAvailabilities"
        :key="availability.id"
        class="rounded-lg border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-800"
      >
        <!-- Editing Mode -->
        <div v-if="editingDate === availability.date">
          <div class="space-y-3">
            <!-- Date (read-only) -->
            <div>
              <span class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                {{ t('availability.date') }}
              </span>
              <div class="flex items-center gap-2 text-sm text-gray-900 dark:text-white">
                <svg
                  class="h-4 w-4 text-gray-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                  />
                </svg>
                {{ formatDate(availability.date) }}
              </div>
            </div>

            <!-- Time Range -->
            <div class="grid grid-cols-2 gap-2">
              <div>
                <span class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  {{ t('availability.startTime') }}
                </span>
                <TimeSelect
                  v-model="editingAvailability.start_time"
                  class="w-full min-h-11"
                  :aria-label="t('availability.startTime')"
                  :max="editingAvailability.end_time || undefined"
                />
              </div>
              <div>
                <span class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  {{ t('availability.endTime') }}
                </span>
                <TimeSelect
                  v-model="editingAvailability.end_time"
                  class="w-full min-h-11"
                  :aria-label="t('availability.endTime')"
                  :min="editingAvailability.start_time || undefined"
                />
              </div>
            </div>

            <!-- Note -->
            <div>
              <label :for="`availability-note-${availability.date}`" class="block">
                <span class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  {{ t('availability.note') }}
                </span>
                <textarea
                  :id="`availability-note-${availability.date}`"
                  v-model="editingAvailability.note"
                  rows="2"
                  class="input w-full"
                  :placeholder="t('availability.note')"
                />
              </label>
            </div>

            <!-- Action Buttons -->
            <div class="flex flex-col md:flex-row gap-2 md:justify-end">
              <button class="btn btn-ghost btn-sm w-full md:w-auto min-h-11" @click="cancelEdit">
                {{ t('common.cancel') }}
              </button>
              <button class="btn btn-primary btn-sm w-full md:w-auto min-h-11" @click="handleSave">
                {{ t('common.save') }}
              </button>
            </div>
          </div>
        </div>

        <!-- Display Mode -->
        <div v-else class="flex items-start justify-between">
          <div class="flex-1">
            <div class="flex items-center gap-2">
              <svg
                class="h-4 w-4 text-gray-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                />
              </svg>
              <span class="text-sm font-medium text-gray-900 dark:text-white">
                {{ formatDate(availability.date) }}
              </span>
            </div>
            <div
              v-if="availability.start_time || availability.end_time"
              class="mt-1 flex items-center gap-2"
            >
              <svg
                class="h-4 w-4 text-gray-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
              <span class="text-xs text-gray-600 dark:text-gray-400">
                {{ formatTimeRange(availability.start_time, availability.end_time) }}
              </span>
            </div>
            <p v-if="availability.note" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ availability.note }}
            </p>
          </div>
          <div v-if="isDateInFuture(availability.date)" class="flex gap-2 shrink-0">
            <button
              class="p-2 min-h-11 min-w-11 md:min-h-0 md:min-w-0 md:p-0 flex items-center justify-center text-gray-600 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
              :title="t('common.edit')"
              @click="startEdit(availability)"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                />
              </svg>
            </button>
            <button
              class="p-2 min-h-11 min-w-11 md:min-h-0 md:min-w-0 md:p-0 flex items-center justify-center text-danger-600 hover:text-danger-700 dark:text-danger-400"
              :title="t('common.delete')"
              @click="handleDelete(availability.date)"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
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
      </div>

      <!-- Empty State -->
      <div
        v-if="sortedAvailabilities.length === 0"
        class="rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 p-6 text-center dark:border-gray-700 dark:bg-gray-800"
      >
        <svg
          class="mx-auto h-10 w-10 text-gray-400"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
          />
        </svg>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {{ t('availability.noAvailabilities') }}
        </p>
      </div>
    </div>
  </CollapsibleSection>
</template>

<script setup lang="ts">
/**
 * The participant's own answers, listed by date, each editable in place.
 *
 * Only future dates get edit and delete buttons: a past answer is a record of what
 * happened, and rewriting it would change a meeting that has already been held or missed.
 *
 * Saving deliberately does not refetch — the live stream reconciles it, and the original
 * code relied on that. Deleting does, because nothing else would notice the row leaving.
 */
import { computed, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { availabilitiesApi } from '@/api/availabilities';
import { confirm } from '@/composables/useConfirm';
import { useToastStore } from '@/stores/toast';
import CollapsibleSection from '@/components/CollapsibleSection.vue';
import TimeSelect from '@/components/TimeSelect.vue';
import { useAvailabilityLabels } from '@/composables/calendar/useAvailabilityLabels';
import type { Availability, CreateAvailabilityRequest } from '@/types';
import { translateErrorMessage } from '@/utils/errorTranslator';

const props = defineProps<{
  /** The calendar's public token. */
  token: string;
  participantId: string;
  availabilities: Availability[];
  /** Refetch the visible range. Awaited after a deletion. */
  reload: () => Promise<void>;
}>();

const { t } = useI18n();
const toastStore = useToastStore();
const { formatDate, formatTimeRange, isDateInFuture } = useAvailabilityLabels();

const editingDate = ref<string | null>(null);
const editingAvailability = reactive({
  start_time: '',
  end_time: '',
  note: '',
});

const sortedAvailabilities = computed(() =>
  [...props.availabilities].sort((a, b) => a.date.localeCompare(b.date))
);

function startEdit(availability: Availability) {
  editingDate.value = availability.date;
  editingAvailability.start_time = availability.start_time || '';
  editingAvailability.end_time = availability.end_time || '';
  editingAvailability.note = availability.note || '';
}

function cancelEdit() {
  editingDate.value = null;
  editingAvailability.start_time = '';
  editingAvailability.end_time = '';
  editingAvailability.note = '';
}

async function handleSave() {
  if (!editingDate.value) return;

  try {
    const data: Partial<CreateAvailabilityRequest> = {};

    // Include times even if empty (to allow clearing them)
    data.start_time = editingAvailability.start_time || undefined;
    data.end_time = editingAvailability.end_time || undefined;
    data.note = editingAvailability.note || undefined;

    await availabilitiesApi.update(props.token, props.participantId, editingDate.value, data);

    cancelEdit();

    // Participant counts will be automatically reloaded, which updates availabilityData
  } catch (err: any) {
    toastStore.error(t(translateErrorMessage(err, { fallback: 'availability.updateError' })));
  }
}

async function handleDelete(date: string) {
  if (!(await confirm({ message: t('availability.confirmDelete') }))) return;

  try {
    await availabilitiesApi.delete(props.token, props.participantId, date);
    await props.reload();
  } catch (err: any) {
    toastStore.error(t(translateErrorMessage(err, { fallback: 'availability.deleteError' })));
  }
}
</script>
