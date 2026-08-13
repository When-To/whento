<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <CollapsibleSection :title="t('availability.recurrence')" :default-open="false">
    <!-- Add Recurrence Form -->
    <div
      class="mb-6 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
    >
      <h3 class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('availability.addRecurrence') }}
      </h3>
      <div class="space-y-3">
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
            {{ t('availability.dayOfWeek') }}
          </label>
          <select v-model.number="newRecurrence.day_of_week" class="input text-sm">
            <option v-for="day in weekDaysOptions" :key="day.value" :value="day.value">
              {{ day.label }}
            </option>
          </select>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
              {{ t('availability.startTime') }}
            </label>
            <TimeSelect
              v-model="newRecurrence.start_time"
              class="text-sm"
              :min="newRecurrenceMinTime"
              :max="newRecurrenceStartTimeMax"
            />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
              {{ t('availability.endTime') }}
            </label>
            <TimeSelect
              v-model="newRecurrence.end_time"
              class="text-sm"
              :min="newRecurrenceEndTimeMin"
              :max="newRecurrenceMaxTime"
            />
          </div>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
              {{ t('availability.startDate') }}
            </label>
            <input v-model="newRecurrence.start_date" type="date" class="input text-sm" required />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
              {{ t('availability.endDate') }}
            </label>
            <input v-model="newRecurrence.end_date" type="date" class="input text-sm" />
          </div>
        </div>
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
            {{ t('availability.note') }}
          </label>
          <textarea
            v-model="newRecurrence.note"
            rows="2"
            class="input text-sm"
            :placeholder="t('availability.note')"
          />
        </div>
        <!-- Error message for equal times -->
        <p v-if="hasEqualTimesNewRecurrence" class="text-sm text-danger-600 dark:text-danger-400">
          {{ t('availability.startEndTimeMustDiffer') }}
        </p>
        <button
          :disabled="cannotAddRecurrence"
          class="btn btn-primary w-full text-sm"
          @click="handleAddRecurrence"
        >
          <svg
            v-if="addingRecurrence"
            class="mr-2 h-4 w-4 animate-spin"
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
          {{ addingRecurrence ? t('common.creating') : t('common.create') }}
        </button>
      </div>
    </div>

    <!-- Recurrences List -->
    <div class="space-y-3">
      <div
        v-for="recurrence in recurrences"
        :key="recurrence.id"
        class="rounded-lg border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-800"
      >
        <!-- Editing Mode -->
        <div v-if="editingRecurrenceId === recurrence.id">
          <div class="space-y-3">
            <!-- Day of Week (read-only in edit mode) -->
            <div>
              <label class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                {{ t('availability.dayOfWeek', 'Day of week') }}
              </label>
              <div class="flex items-center gap-2 text-sm text-gray-900 dark:text-white py-2">
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
                    d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                  />
                </svg>
                {{ getDayName(editingRecurrence.day_of_week) }}
              </div>
            </div>

            <!-- Time Range -->
            <div class="grid grid-cols-2 gap-2">
              <div>
                <label class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  {{ t('availability.startTime', 'Start time') }}
                </label>
                <TimeSelect
                  v-model="editingRecurrence.start_time"
                  class="w-full"
                  :min="editingRecurrenceMinTime"
                  :max="editingRecurrenceStartTimeMax"
                />
              </div>
              <div>
                <label class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  {{ t('availability.endTime', 'End time') }}
                </label>
                <TimeSelect
                  v-model="editingRecurrence.end_time"
                  class="w-full"
                  :min="editingRecurrenceEndTimeMin"
                  :max="editingRecurrenceMaxTime"
                />
              </div>
            </div>

            <!-- Date Range -->
            <div class="grid grid-cols-2 gap-2">
              <div>
                <label class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  {{ t('availability.startDate', 'Start date') }}
                </label>
                <input
                  v-model="editingRecurrence.start_date"
                  type="date"
                  class="input w-full"
                  required
                />
              </div>
              <div>
                <label class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  {{ t('availability.endDate', 'End date') }}
                </label>
                <input v-model="editingRecurrence.end_date" type="date" class="input w-full" />
              </div>
            </div>

            <!-- Note -->
            <div>
              <label class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                {{ t('availability.note', 'Note') }}
              </label>
              <input
                v-model="editingRecurrence.note"
                type="text"
                class="input w-full"
                :placeholder="t('availability.note')"
              />
            </div>

            <!-- Error message for equal times -->
            <p
              v-if="hasEqualTimesEditingRecurrence"
              class="text-sm text-danger-600 dark:text-danger-400"
            >
              {{ t('availability.startEndTimeMustDiffer') }}
            </p>

            <!-- Action Buttons -->
            <div class="flex gap-2 justify-end">
              <button class="btn btn-ghost btn-sm" @click="resetEditing">
                {{ t('common.cancel', 'Cancel') }}
              </button>
              <button
                class="btn btn-primary btn-sm"
                :disabled="cannotSaveRecurrence"
                @click="handleSaveRecurrence"
              >
                {{ t('common.save', 'Save') }}
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
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                />
              </svg>
              <span class="text-sm font-medium text-gray-900 dark:text-white">
                {{ getDayName(recurrence.day_of_week) }}
              </span>
            </div>
            <div
              v-if="recurrence.start_time || recurrence.end_time"
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
                {{ formatTimeRange(recurrence.start_time, recurrence.end_time) }}
              </span>
            </div>
            <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ formatDate(recurrence.start_date) }}
              <span v-if="recurrence.end_date"> - {{ formatDate(recurrence.end_date) }}</span>
            </div>
            <p v-if="recurrence.note" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ recurrence.note }}
            </p>

            <!-- Exceptions -->
            <div v-if="recurrence.exceptions && recurrence.exceptions.length > 0" class="mt-2">
              <p class="text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('availability.exceptions') }}:
              </p>
              <div class="mt-1 flex flex-wrap gap-1">
                <span
                  v-for="exception in recurrence.exceptions"
                  :key="exception.id"
                  class="inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-gray-700 dark:text-gray-300"
                >
                  {{ formatDate(exception.excluded_date) }}
                  <button
                    class="hover:text-danger-600"
                    @click="handleRemoveException(recurrence.id, exception.excluded_date)"
                  >
                    <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M6 18L18 6M6 6l12 12"
                      />
                    </svg>
                  </button>
                </span>
              </div>
            </div>

            <!-- Add Exception Form -->
            <div class="mt-2 flex gap-2">
              <input
                v-model="exceptionDates[recurrence.id]"
                type="date"
                class="input flex-1 text-xs"
                :placeholder="t('availability.addException')"
              />
              <button
                :disabled="!exceptionDates[recurrence.id]"
                class="btn btn-secondary btn-sm"
                @click="handleAddException(recurrence.id)"
              >
                {{ t('availability.addException') }}
              </button>
            </div>
          </div>
          <div class="flex gap-2">
            <button
              class="text-gray-600 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
              :title="t('common.edit', 'Edit')"
              @click="startEditing(recurrence)"
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
              class="text-danger-600 hover:text-danger-700 dark:text-danger-400"
              :title="t('common.delete')"
              @click="handleDeleteRecurrence(recurrence.id)"
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
        v-if="recurrences.length === 0"
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
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {{ t('availability.addRecurrence') }}
        </p>
      </div>
    </div>
  </CollapsibleSection>
</template>

<script setup lang="ts">
/**
 * Create, edit, delete and except recurring availabilities — "every Tuesday from March".
 *
 * A self-contained CRUD screen that happened to live inside the participant view, where
 * it accounted for roughly four hundred lines of template and two hundred of script. The
 * form rules live in `useRecurrenceForm`; what is left here is the markup and the six
 * calls that write to the API.
 *
 * `reload` is a function prop rather than an event on purpose. Every mutation ends by
 * refetching both the rules and the range summary, and the "Creating…" spinner has to
 * stay up until that has finished — an event would let it stop a round trip early, which
 * is precisely the flicker the original code avoided by awaiting.
 */
import { computed, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { availabilitiesApi } from '@/api/availabilities';
import { confirm } from '@/composables/useConfirm';
import { useToastStore } from '@/stores/toast';
import CollapsibleSection from '@/components/CollapsibleSection.vue';
import TimeSelect from '@/components/TimeSelect.vue';
import {
  buildRecurrenceRequest,
  useRecurrenceForm,
} from '@/composables/calendar/useRecurrenceForm';
import { useAvailabilityLabels } from '@/composables/calendar/useAvailabilityLabels';
import type { PublicCalendar, RecurrenceWithExceptions } from '@/types';

const props = defineProps<{
  /** The calendar's public token. */
  token: string;
  participantId: string;
  /** The calendar being answered; supplies the weekday and time restrictions. */
  calendar: PublicCalendar | undefined;
  recurrences: RecurrenceWithExceptions[];
  /** Refetch the rules and the visible range. Awaited, so spinners can wait on it. */
  reload: () => Promise<void>;
}>();

const { t } = useI18n();
const toastStore = useToastStore();
const { formatDate, formatTimeRange, getDayName } = useAvailabilityLabels();

const {
  weekDaysOptions,
  newRecurrence,
  editingRecurrenceId,
  editingRecurrence,
  newRecurrenceMinTime,
  newRecurrenceMaxTime,
  newRecurrenceStartTimeMax,
  newRecurrenceEndTimeMin,
  editingRecurrenceMinTime,
  editingRecurrenceMaxTime,
  editingRecurrenceStartTimeMax,
  editingRecurrenceEndTimeMin,
  hasEqualTimesNewRecurrence,
  hasEqualTimesEditingRecurrence,
  resetNewRecurrence,
  startEditing,
  resetEditing,
} = useRecurrenceForm(computed(() => props.calendar));

const addingRecurrence = ref(false);
const exceptionDates = reactive<Record<string, string>>({});

const cannotAddRecurrence = computed(
  () =>
    newRecurrence.day_of_week === null ||
    !newRecurrence.start_date ||
    addingRecurrence.value ||
    hasEqualTimesNewRecurrence.value
);

const cannotSaveRecurrence = computed(
  () => !editingRecurrence.start_date || hasEqualTimesEditingRecurrence.value
);

async function handleAddRecurrence() {
  if (newRecurrence.day_of_week === null || !newRecurrence.start_date) return;

  addingRecurrence.value = true;
  try {
    await availabilitiesApi.createRecurrence(
      props.token,
      props.participantId,
      buildRecurrenceRequest(newRecurrence)
    );

    resetNewRecurrence();
    await props.reload();
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to add recurrence');
  } finally {
    addingRecurrence.value = false;
  }
}

async function handleSaveRecurrence() {
  if (
    editingRecurrence.day_of_week === null ||
    !editingRecurrence.start_date ||
    !editingRecurrenceId.value
  )
    return;

  try {
    await availabilitiesApi.updateRecurrence(
      props.token,
      props.participantId,
      editingRecurrenceId.value,
      buildRecurrenceRequest(editingRecurrence)
    );

    resetEditing();
    await props.reload();
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to update recurrence');
  }
}

async function handleDeleteRecurrence(recurrenceId: string) {
  if (!(await confirm({ message: t('availability.confirmDeleteRecurrence') }))) return;

  try {
    await availabilitiesApi.deleteRecurrence(props.token, props.participantId, recurrenceId);
    await props.reload();
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to delete recurrence');
  }
}

async function handleAddException(recurrenceId: string) {
  const date = exceptionDates[recurrenceId];
  if (!date) return;

  try {
    await availabilitiesApi.createException(props.token, props.participantId, recurrenceId, date);
    exceptionDates[recurrenceId] = '';
    await props.reload();
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to add exception');
  }
}

async function handleRemoveException(recurrenceId: string, date: string) {
  try {
    await availabilitiesApi.deleteException(props.token, props.participantId, recurrenceId, date);
    await props.reload();
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to remove exception');
  }
}
</script>
