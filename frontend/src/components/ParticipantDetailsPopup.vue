<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div
    ref="tooltipRef"
    class="fixed z-50 flex max-h-[calc(100vh-1.25rem)] flex-col overflow-hidden rounded-lg border border-gray-200 bg-white p-4 shadow-xl pointer-events-auto max-w-[calc(100vw-2rem)] md:max-w-md md:p-6 dark:border-gray-700 dark:bg-gray-800"
    :style="{
      left: `${popupPosition.x}px`,
      top: `${popupPosition.y}px`,
    }"
  >
    <div class="flex min-h-0 flex-1 flex-col">
      <div class="mb-4 flex shrink-0 items-center justify-between">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('participant.participantsForDate', 'Participants for') }}
          {{ formatSelectedDate }}
        </h3>
        <button
          class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
          @click="emit('close')"
        >
          <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M6 18L18 6M6 6l12 12"
            />
          </svg>
        </button>
      </div>

      <div v-if="loadingDetails" class="min-h-0 flex-1 overflow-y-auto py-8 text-center">
        <div
          class="inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-primary-600 border-r-transparent"
        />
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {{ t('common.loading', 'Loading...') }}
        </p>
      </div>

      <div v-else-if="participantDetails" class="min-h-0 flex-1 overflow-y-auto">
        <div class="mb-4">
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ participantDetails.total_count }}
            {{
              participantDetails.total_count > 1
                ? t('calendar.participants', 'Participants')
                : t('calendar.participantCount', 'participant(s)')
            }}
          </p>
        </div>

        <div class="space-y-2">
          <div
            v-for="(participant, index) in participantDetails.participants"
            :key="participant.participant_id ?? `${participant.participant_name}-${index}`"
            :class="[
              'rounded-lg border p-3',
              participant.participant_name === props.currentParticipantName
                ? 'border-primary-300 bg-primary-50/50 dark:border-primary-700 dark:bg-primary-900/10'
                : 'border-gray-200 dark:border-gray-700',
            ]"
          >
            <div class="flex items-start justify-between">
              <div class="flex-1">
                <div class="flex items-center gap-2">
                  <div class="font-medium text-gray-900 dark:text-white">
                    {{ participant.participant_name }}
                  </div>
                  <span
                    v-if="participant.participant_name === props.currentParticipantName"
                    class="text-xs px-2 py-0.5 rounded-full bg-primary-100 text-primary-700 dark:bg-primary-900 dark:text-primary-300"
                  >
                    {{ t('common.you', 'You') }}
                  </span>
                </div>
                <div class="mt-1 text-sm text-gray-600 dark:text-gray-400">
                  {{ formatTimeRange(participant.start_time, participant.end_time) }}
                </div>

                <!-- Availability edit form -->
                <div
                  v-if="
                    participant.participant_name === props.currentParticipantName && editingNote
                  "
                  class="mt-2"
                >
                  <!-- Time Range -->
                  <div class="grid grid-cols-2 gap-2 mb-2">
                    <div>
                      <label
                        class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                      >
                        {{ t('availability.startTime', 'Start time') }}
                      </label>
                      <TimeSelect
                        v-model="editedStartTime"
                        class="w-full text-sm"
                        :max="editedEndTime || undefined"
                      />
                    </div>
                    <div>
                      <label
                        class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                      >
                        {{ t('availability.endTime', 'End time') }}
                      </label>
                      <TimeSelect
                        v-model="editedEndTime"
                        class="w-full text-sm"
                        :min="editedStartTime || undefined"
                      />
                    </div>
                  </div>

                  <!-- Note -->
                  <div class="mb-2">
                    <label class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                      {{ t('availability.note', 'Note') }}
                    </label>
                    <textarea
                      v-model="editedNote"
                      rows="2"
                      class="input w-full text-sm"
                      :placeholder="t('availability.note', 'Note')"
                    />
                  </div>

                  <!-- Action buttons -->
                  <div class="flex gap-2">
                    <button :disabled="savingNote" class="btn btn-primary btn-sm" @click="saveNote">
                      <svg
                        v-if="savingNote"
                        class="mr-1 h-3 w-3 animate-spin"
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
                      {{ t('common.save', 'Save') }}
                    </button>
                    <button :disabled="savingNote" class="btn btn-ghost btn-sm" @click="cancelEdit">
                      {{ t('common.cancel', 'Cancel') }}
                    </button>
                  </div>
                </div>
                <div
                  v-else-if="participant.note"
                  class="mt-1 text-sm text-gray-500 dark:text-gray-400 italic"
                >
                  {{ participant.note }}
                </div>
                <div
                  v-else-if="participant.participant_name === props.currentParticipantName"
                  class="mt-1 text-sm text-gray-400 dark:text-gray-500 italic"
                >
                  {{ t('availability.noNote', 'No note') }}
                </div>

                <!-- Says why there is nothing to edit here. -->
                <p
                  v-if="
                    participant.participant_name === props.currentParticipantName &&
                    props.fromRecurrence
                  "
                  class="mt-2 flex items-start gap-1.5 text-xs text-gray-500 dark:text-gray-400"
                >
                  <span class="cal-dot cal-dot--recurring mt-1" />
                  {{ t('availability.fromRecurrenceHint') }}
                </p>
              </div>

              <!-- Edit button for current participant -->
              <button
                v-if="
                  participant.participant_name === props.currentParticipantName &&
                  !editingNote &&
                  !props.fromRecurrence
                "
                class="ml-2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                :title="t('common.edit', 'Edit')"
                @click="
                  startEdit(participant.note || '', participant.start_time, participant.end_time)
                "
              >
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                  />
                </svg>
              </button>
            </div>
          </div>
        </div>
      </div>

      <div v-else class="min-h-0 flex-1 overflow-y-auto py-8 text-center">
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ t('availability.noAvailabilities', 'No availabilities') }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onUnmounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { availabilitiesApi } from '@/api/availabilities';
import TimeSelect from '@/components/TimeSelect.vue';
import type { DateAvailabilitySummary } from '@/types';

interface Props {
  calendarToken: string;
  currentParticipantId: string;
  currentParticipantName: string;
  date: string;
  anchorRect: DOMRect;
  /**
   * True when the participant is only available on this date because of a recurrence.
   * There is no one-off record to edit, so the form is replaced by an explanation —
   * saving would silently create a one-off availability shadowing the rule.
   */
  fromRecurrence?: boolean;
}

interface Emits {
  (e: 'close'): void;
  (e: 'availability-updated'): void;
}

const props = defineProps<Props>();
const emit = defineEmits<Emits>();
const { t, locale } = useI18n();

// Typed rather than `any`: the summary's `participant_id` is optional — a calendar
// with lock_participants withholds it for everyone but this caller — and `any` was
// hiding that from the template, which keys its rows on it.
const participantDetails = ref<DateAvailabilitySummary | null>(null);
const loadingDetails = ref(false);
const popupPosition = ref({ x: 0, y: 0 });
const tooltipRef = ref<HTMLElement | null>(null);

const editingNote = ref(false);
const editedNote = ref('');
const editedStartTime = ref('');
const editedEndTime = ref('');
const savingNote = ref(false);

const formatSelectedDate = computed(() => {
  if (!props.date) return '';
  const date = new Date(props.date);
  const localeCode = locale.value;
  return date.toLocaleDateString(localeCode, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  });
});

function isFullDayTime(startTime?: string, endTime?: string): boolean {
  if (!startTime || !endTime) return false;
  const start = startTime.substring(0, 5);
  const end = endTime.substring(0, 5);
  return start === '00:00' && end === '23:59';
}

function formatTimeRange(startTime?: string, endTime?: string): string {
  if (isFullDayTime(startTime, endTime)) {
    return t('availability.allDay', 'All day');
  }
  return `${startTime ?? '00:00'}-${endTime ?? '23:59'}`;
}

async function loadParticipantDetails() {
  loadingDetails.value = true;

  try {
    const details = await availabilitiesApi.getDateSummary(
      props.calendarToken,
      props.date,
      props.currentParticipantId
    );
    participantDetails.value = details;
  } catch (err) {
    console.error('Failed to load participant details:', err);
    participantDetails.value = null;
  } finally {
    loadingDetails.value = false;

    await nextTick();
    adjustTooltipPosition();
  }
}

/**
 * Keep the popup inside the viewport.
 *
 * It is `position: fixed`, so the page scrolling underneath it does not move it and
 * cannot reveal anything that hangs off the bottom of the screen. Clamping the
 * position is therefore not optional — it is the only way the content stays reachable,
 * together with the max-height and internal scroll on the element itself.
 */
function adjustTooltipPosition() {
  if (!tooltipRef.value) return;

  const rect = tooltipRef.value.getBoundingClientRect();
  const offset = 10;
  const viewportWidth = window.innerWidth;
  const viewportHeight = window.innerHeight;

  let { x, y } = popupPosition.value;

  x = Math.min(x, viewportWidth - rect.width - offset);
  y = Math.min(y, viewportHeight - rect.height - offset);
  x = Math.max(x, offset);
  y = Math.max(y, offset);

  if (x !== popupPosition.value.x || y !== popupPosition.value.y) {
    popupPosition.value = { x, y };
  }
}

function startEdit(currentNote: string, startTime?: string, endTime?: string) {
  editedNote.value = currentNote;
  editedStartTime.value = startTime || '';
  editedEndTime.value = endTime || '';
  editingNote.value = true;
}

function cancelEdit() {
  editingNote.value = false;
  editedNote.value = '';
  editedStartTime.value = '';
  editedEndTime.value = '';
}

async function saveNote() {
  if (!props.calendarToken || !props.currentParticipantId || !props.date) {
    return;
  }

  savingNote.value = true;

  try {
    await availabilitiesApi.update(props.calendarToken, props.currentParticipantId, props.date, {
      note: editedNote.value || undefined,
      start_time: editedStartTime.value || undefined,
      end_time: editedEndTime.value || undefined,
    });

    await loadParticipantDetails();

    editingNote.value = false;
    editedNote.value = '';
    editedStartTime.value = '';
    editedEndTime.value = '';

    emit('availability-updated');
  } catch (err) {
    console.error('Failed to update availability:', err);
  } finally {
    savingNote.value = false;
  }
}

/**
 * Whether an event started inside the popup.
 *
 * Uses `composedPath()` rather than `contains(event.target)`: a click that causes its
 * own target to be removed — pressing Edit hides the pencil via `v-if` — leaves the
 * target detached by the time this listener runs, and `contains` then reports it as
 * outside. That is what made edit mode impossible to enter: the popup closed on the
 * very click that opened the form. The composed path is captured at dispatch and
 * survives the removal.
 */
const startedInsidePopup = (event: Event): boolean => {
  const root = tooltipRef.value;
  if (!root) return false;
  const path = typeof event.composedPath === 'function' ? event.composedPath() : [];
  if (path.includes(root)) return true;
  const target = event.target as Node | null;
  // A target already detached from the document cannot be judged; treat it as ours.
  return !!target && (root.contains(target) || !(target as Element).isConnected);
};

const handleClickOutside = (event: Event) => {
  // While editing, only Escape and the form's own buttons close the popup. `TimeSelect`
  // teleports its dropdown to the body, so picking a time is an "outside" click by any
  // DOM measure — and losing an unsaved note to a stray click is its own bug.
  if (editingNote.value) return;
  if (!startedInsidePopup(event)) emit('close');
};

const handleScroll = () => {
  if (editingNote.value) return;
  emit('close');
};

const handleEscape = (event: KeyboardEvent) => {
  if (event.key === 'Escape') {
    emit('close');
  }
};

let attachTimer: number | null = null;
let resizeObserver: ResizeObserver | null = null;

onMounted(async () => {
  // Initial anchored position (below the row, clamped to viewport width)
  popupPosition.value = {
    x: Math.min(props.anchorRect.left + 16, window.innerWidth - 400),
    y: props.anchorRect.bottom + 8,
  };

  await loadParticipantDetails();

  // Entering edit mode adds two time pickers, a note field and two buttons, which can
  // push the popup off the bottom of the screen. Repositioning only after the initial
  // load left it hanging there, unreachable because scrolling the page does not move a
  // fixed element. Watching its own size covers every content change.
  if (tooltipRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => adjustTooltipPosition());
    resizeObserver.observe(tooltipRef.value);
  }

  // Delay attaching so the opening pointer event doesn't immediately close
  attachTimer = window.setTimeout(() => {
    document.addEventListener('click', handleClickOutside);
    document.addEventListener('touchstart', handleClickOutside);
    window.addEventListener('scroll', handleScroll, true);
    document.addEventListener('keydown', handleEscape);
  }, 100);
});

onUnmounted(() => {
  if (attachTimer !== null) {
    window.clearTimeout(attachTimer);
  }
  resizeObserver?.disconnect();
  resizeObserver = null;
  document.removeEventListener('click', handleClickOutside);
  document.removeEventListener('touchstart', handleClickOutside);
  window.removeEventListener('scroll', handleScroll, true);
  document.removeEventListener('keydown', handleEscape);
});

// Reload when the date prop changes (e.g., user long-presses a different row)
watch(
  () => props.date,
  async (newDate, oldDate) => {
    if (newDate && newDate !== oldDate) {
      editingNote.value = false;
      await loadParticipantDetails();
    }
  }
);
</script>
