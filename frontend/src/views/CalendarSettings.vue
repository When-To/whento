<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-6xl">
      <!-- Loading State -->
      <div v-if="loading && !calendar" class="flex items-center justify-center py-12">
        <svg class="h-8 w-8 animate-spin text-primary-600" fill="none" viewBox="0 0 24 24">
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
      </div>

      <!-- Calendar Settings -->
      <template v-else-if="calendar">
        <!-- Header -->
        <div class="mb-8">
          <router-link
            to="/dashboard"
            class="mb-4 inline-flex items-center text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
          >
            <svg class="mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M15 19l-7-7 7-7"
              />
            </svg>
            {{ t('common.back') }}
          </router-link>
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ t('calendar.editCalendar') }}
          </h1>
        </div>

        <div class="space-y-6">
          <!-- Calendar Info Card -->
          <div class="card">
            <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('calendar.calendarInfo') }}
            </h2>

            <form class="space-y-4" @submit.prevent="handleUpdate">
              <CalendarInfoFields
                v-model:name="form.name"
                v-model:description="form.description"
                v-model:timezone="form.timezone"
                :name-error="errors.name"
              />

              <!-- Actions -->
              <div class="flex items-center justify-end">
                <button
                  type="submit"
                  :disabled="
                    updating || !calendar.participants || calendar.participants.length === 0
                  "
                  class="btn btn-primary"
                >
                  <svg
                    v-if="updating"
                    class="mr-2 h-5 w-5 animate-spin"
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
                  {{ updating ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </form>
          </div>

          <!-- Sharing Links Card -->
          <div class="card">
            <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('calendar.sharingLinks') }}
            </h2>

            <div class="space-y-4">
              <!-- Public Link (hidden when participants are locked) -->
              <!--
                A `<span id>` + `aria-labelledby` rather than a `<label for>`: the copy
                button shares the row with the field, and a `<label>` may not contain
                interactive content other than the control it labels. The accessible name
                is the same either way.
              -->
              <div v-if="!form.lock_participants">
                <span
                  id="calendar-public-link-label"
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t('calendar.publicLink') }}
                </span>
                <div class="flex gap-2">
                  <input
                    id="calendar-public-link"
                    :value="publicUrl"
                    type="text"
                    class="input flex-1"
                    aria-labelledby="calendar-public-link-label"
                    readonly
                  />
                  <button
                    type="button"
                    :aria-label="t('a11y.copyPublicLink')"
                    class="btn btn-secondary"
                    @click="copyToClipboard(publicUrl)"
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      aria-hidden="true"
                    >
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
                <span
                  id="calendar-ics-link-label"
                  class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ t('calendar.icsLink') }}
                </span>
                <div class="flex gap-2">
                  <input
                    id="calendar-ics-link"
                    :value="icsUrl"
                    type="text"
                    class="input flex-1"
                    aria-labelledby="calendar-ics-link-label"
                    readonly
                  />
                  <button
                    type="button"
                    :aria-label="t('a11y.copyIcsLink')"
                    class="btn btn-secondary"
                    @click="copyToClipboard(icsUrl)"
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      aria-hidden="true"
                    >
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
            </div>
          </div>

          <!-- Participants Card -->
          <CollapsibleSection :title="t('calendar.participants')" :default-open="false">
            <!-- Participants List -->
            <div
              v-if="calendar.participants && calendar.participants.length > 0"
              class="mb-4 space-y-2"
            >
              <div
                v-for="participant in calendar.participants"
                :key="participant.id"
                class="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-4 py-3 dark:border-gray-700 dark:bg-gray-800"
              >
                <!-- Edit mode -->
                <template v-if="editingParticipantId === participant.id">
                  <input
                    :id="`participant-name-${participant.id}`"
                    v-model="editingParticipantName"
                    type="text"
                    class="input flex-1"
                    :aria-label="t('a11y.participantName')"
                    @keyup.enter="handleSaveParticipant(participant.id)"
                    @keyup.esc="cancelEditParticipant"
                  />
                  <button
                    type="button"
                    class="text-primary-600 hover:text-primary-700 dark:text-primary-400"
                    :title="t('common.save')"
                    :aria-label="t('common.save')"
                    @click="handleSaveParticipant(participant.id)"
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      aria-hidden="true"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M5 13l4 4L19 7"
                      />
                    </svg>
                  </button>
                  <button
                    type="button"
                    class="text-gray-600 hover:text-gray-700 dark:text-gray-400"
                    :title="t('common.cancel')"
                    :aria-label="t('common.cancel')"
                    @click="cancelEditParticipant"
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      aria-hidden="true"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M6 18L18 6M6 6l12 12"
                      />
                    </svg>
                  </button>
                </template>

                <!-- View mode -->
                <template v-else>
                  <span class="flex-1 text-gray-900 dark:text-white">{{ participant.name }}</span>
                  <button
                    type="button"
                    class="text-primary-600 hover:text-primary-700 dark:text-primary-400"
                    :title="t('calendar.copyParticipantLink')"
                    :aria-label="t('calendar.copyParticipantLink')"
                    @click="copyParticipantLink(participant.id!)"
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      aria-hidden="true"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"
                      />
                    </svg>
                  </button>
                  <button
                    type="button"
                    class="text-gray-600 hover:text-gray-700 dark:text-gray-400"
                    :title="t('common.edit')"
                    :aria-label="t('common.edit')"
                    @click="startEditParticipant(participant.id!, participant.name)"
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      aria-hidden="true"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                      />
                    </svg>
                  </button>
                  <button
                    v-if="calendar.participants.length > 1"
                    type="button"
                    class="text-danger-600 hover:text-danger-700 dark:text-danger-400"
                    :title="t('common.delete')"
                    :aria-label="t('common.delete')"
                    @click="handleDeleteParticipant(participant.id!, participant.name)"
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      aria-hidden="true"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                      />
                    </svg>
                  </button>
                </template>
              </div>
            </div>

            <!-- Empty State -->
            <div
              v-else
              class="mb-4 rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 p-6 text-center dark:border-gray-700 dark:bg-gray-800"
            >
              <svg
                class="mx-auto h-12 w-12 text-gray-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
                />
              </svg>
              <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
                {{ t('calendar.noParticipants') }}
              </p>
            </div>

            <!-- Add Participant -->
            <form class="flex gap-2 mb-4" @submit.prevent="handleAddParticipant">
              <input
                id="new-participant-name"
                v-model="newParticipantName"
                type="text"
                class="input flex-1"
                :aria-label="t('a11y.participantName')"
                :placeholder="t('calendar.participantNamePlaceholder')"
              />
              <button
                type="submit"
                :disabled="!newParticipantName.trim() || addingParticipant"
                class="btn btn-secondary"
              >
                <svg class="mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M12 4v16m8-8H4"
                  />
                </svg>
                {{ t('calendar.addParticipant') }}
              </button>
            </form>

            <ParticipantAccessToggles
              v-model:lock-participants="form.lock_participants"
              v-model:allow-anonymous-participants="form.allow_anonymous_participants"
              @change:lock="handleLockParticipantsChange"
              @change:anonymous="handleAllowAnonymousParticipantsChange"
            />

            <!-- Errors and Warnings -->
            <div v-if="!calendar.participants || calendar.participants.length === 0" class="mt-4">
              <!-- Warning Message - No Participants -->
              <div class="rounded-lg bg-orange-50 p-4 dark:bg-orange-900/20">
                <div class="flex">
                  <svg
                    class="h-5 w-5 text-orange-600 dark:text-orange-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                    />
                  </svg>
                  <p class="ml-3 text-sm text-orange-600 dark:text-orange-400">
                    {{ t('calendar.noParticipantsWarningUpdate') }}
                  </p>
                </div>
              </div>
            </div>
          </CollapsibleSection>

          <!-- Participant threshold and minimum duration -->
          <CollapsibleSection :title="t('calendar.sectionThreshold')" :default-open="false">
            <CalendarThresholdFields
              v-model:threshold="form.threshold"
              v-model:min-duration-hours="form.min_duration_hours"
              :participant-count="calendar.participants?.length ?? 0"
              :allow-anonymous-participants="form.allow_anonymous_participants"
              :threshold-error="errors.threshold"
            />

            <!-- Actions -->
            <div class="flex items-center justify-end">
              <button
                type="button"
                :disabled="updating || !calendar.participants || calendar.participants.length === 0"
                class="btn btn-primary"
                @click="handleUpdate"
              >
                <svg
                  v-if="updating"
                  class="mr-2 h-5 w-5 animate-spin"
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
                {{ updating ? t('common.saving') : t('common.save') }}
              </button>
            </div>
          </CollapsibleSection>

          <!-- Allow/block days/hours -->
          <CollapsibleSection :title="t('calendar.sectionSchedule')" :default-open="false">
            <CalendarScheduleFields
              v-model:start-date="form.start_date"
              v-model:end-date="form.end_date"
              v-model:allowed-weekdays="form.allowed_weekdays"
              v-model:weekday-times="form.weekday_times"
              v-model:holidays-policy="form.holidays_policy"
              v-model:holiday-min-time="form.holiday_min_time"
              v-model:holiday-max-time="form.holiday_max_time"
              v-model:allow-holiday-eves="form.allow_holiday_eves"
              v-model:holiday-eve-min-time="form.holiday_eve_min_time"
              v-model:holiday-eve-max-time="form.holiday_eve_max_time"
              :timezone="form.timezone"
            />

            <!-- Actions -->
            <div class="flex items-center justify-end">
              <button
                type="button"
                :disabled="updating || !calendar.participants || calendar.participants.length === 0"
                class="btn btn-primary"
                @click="handleUpdate"
              >
                <svg
                  v-if="updating"
                  class="mr-2 h-5 w-5 animate-spin"
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
                {{ updating ? t('common.saving') : t('common.save') }}
              </button>
            </div>
          </CollapsibleSection>

          <!-- Notifications -->
          <NotificationSettings
            v-model="notifyConfig"
            :smtp-configured="smtpConfigured"
            @save="handleSaveNotifications"
          />

          <!-- Danger Zone -->
          <CollapsibleSection
            :title="t('common.dangerZone')"
            :default-open="false"
            variant="danger"
          >
            <div class="space-y-4">
              <!-- Regenerate Tokens -->
              <div
                class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
              >
                <h3 class="mb-2 font-semibold text-gray-900 dark:text-white">
                  {{ t('calendar.regenerateTokens') }}
                </h3>
                <p class="mb-3 text-sm text-gray-600 dark:text-gray-400">
                  {{ t('calendar.regenerateTokensHelp') }}
                </p>
                <div class="flex gap-2">
                  <button
                    type="button"
                    class="btn btn-ghost text-orange-600 hover:bg-orange-50 dark:text-orange-400 dark:hover:bg-orange-900/20"
                    @click="handleRegenerateToken('public')"
                  >
                    {{ t('calendar.regeneratePublicToken') }}
                  </button>
                  <button
                    type="button"
                    class="btn btn-ghost text-orange-600 hover:bg-orange-50 dark:text-orange-400 dark:hover:bg-orange-900/20"
                    @click="handleRegenerateToken('ics')"
                  >
                    {{ t('calendar.regenerateICSToken') }}
                  </button>
                </div>
              </div>

              <!-- Delete Calendar -->
              <div
                class="rounded-lg border border-danger-200 bg-danger-50 p-4 dark:border-danger-900 dark:bg-danger-900/20"
              >
                <h3 class="mb-2 font-semibold text-danger-600 dark:text-danger-400">
                  {{ t('calendar.deleteCalendar') }}
                </h3>
                <p class="mb-3 text-sm text-danger-600 dark:text-danger-400">
                  {{ t('calendar.deleteCalendarHelp') }}
                </p>
                <button
                  type="button"
                  :disabled="deleting"
                  class="btn bg-danger-600 text-white hover:bg-danger-700 dark:bg-danger-600 dark:hover:bg-danger-700"
                  @click="handleDelete"
                >
                  {{ deleting ? t('common.deleting') : t('calendar.deleteCalendar') }}
                </button>
              </div>
            </div>
          </CollapsibleSection>
        </div>

        <!--
          The delete confirmation used to be a fourth hand-rolled modal here: no
          `role="dialog"`, no focus trap, no Escape, dismissable only by clicking the
          backdrop. It is now the shared `confirm()` from `useConfirm`, which this view
          already used for the other five destructive actions on the page.
        -->
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue';
import { useRouter, useRoute, onBeforeRouteLeave } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useCalendarStore } from '@/stores/calendar';
import { useToastStore } from '@/stores/toast';
import { confirm } from '@/composables/useConfirm';
import CollapsibleSection from '@/components/CollapsibleSection.vue';
import NotificationSettings from '@/components/NotificationSettings.vue';
import CalendarInfoFields from '@/components/calendar/CalendarInfoFields.vue';
import CalendarThresholdFields from '@/components/calendar/CalendarThresholdFields.vue';
import CalendarScheduleFields from '@/components/calendar/CalendarScheduleFields.vue';
import ParticipantAccessToggles from '@/components/calendar/ParticipantAccessToggles.vue';
import { translateErrorMessage } from '@/utils/errorTranslator';
import {
  createEmptyWeekdayTimes,
  normalizeTime,
  prepareWeekdayTimes,
} from '@/utils/calendar/weekdayTimes';
import {
  getNotifyConfig,
  updateNotifyConfig,
  getDefaultNotifyConfig,
  type NotifyConfig,
} from '@/api/notify';
import { authApi } from '@/api/auth';

const router = useRouter();
const route = useRoute();
const { t } = useI18n();
const calendarStore = useCalendarStore();
const toastStore = useToastStore();

/** The calendar being edited, as a *computed* of the route id. */
const calendarId = computed(() => route.params.id as string);
/** Invocation counter so a stale id's load cannot clobber a newer one. */
let loadVersion = 0;
/** Per-action sequences so one save cannot strand another's busy flag. */
let saveSeq = 0;
let participantSeq = 0;
/** Serialises the two access toggles so rapid opposite clicks cannot finish out of order. */
let accessToggleChain: Promise<unknown> = Promise.resolve();
/** The id the form was last populated for, or null; a change resets id-scoped state. */
let loadedForId: string | null = null;

const loading = ref(true);
const updating = ref(false);
const addingParticipant = ref(false);
const deleting = ref(false);
const newParticipantName = ref('');

// Participant editing
const editingParticipantId = ref<string | null>(null);
const editingParticipantName = ref('');

// Form state
const form = reactive({
  name: '',
  description: '',
  threshold: 1,
  allowed_weekdays: [0, 1, 2, 3, 4, 5, 6] as number[],
  min_duration_hours: 0,
  timezone: 'Europe/Paris',
  holidays_policy: 'ignore' as 'ignore' | 'allow' | 'block',
  allow_holiday_eves: false,
  lock_participants: false,
  allow_anonymous_participants: false,
  weekday_times: createEmptyWeekdayTimes(),
  holiday_min_time: '',
  holiday_max_time: '',
  holiday_eve_min_time: '',
  holiday_eve_max_time: '',
  start_date: '',
  end_date: '',
});

const originalForm = reactive({
  name: '',
  description: '',
  threshold: 1,
  allowed_weekdays: [0, 1, 2, 3, 4, 5, 6] as number[],
  min_duration_hours: 0,
  timezone: 'Europe/Paris',
  holidays_policy: 'ignore' as 'ignore' | 'allow' | 'block',
  allow_holiday_eves: false,
  lock_participants: false,
  allow_anonymous_participants: false,
  weekday_times: createEmptyWeekdayTimes(),
  holiday_min_time: '',
  holiday_max_time: '',
  holiday_eve_min_time: '',
  holiday_eve_max_time: '',
  start_date: '',
  end_date: '',
});

// Notification config state
const notifyConfig = ref<NotifyConfig>(getDefaultNotifyConfig());
// Email notification options depend on the instance actually having SMTP
// configured; default to hidden until the backend confirms otherwise.
const smtpConfigured = ref(false);

// Track if form has unsaved changes
const hasUnsavedChanges = computed(() => {
  return (
    form.name !== originalForm.name ||
    form.description !== originalForm.description ||
    form.threshold !== originalForm.threshold ||
    form.min_duration_hours !== originalForm.min_duration_hours ||
    form.timezone !== originalForm.timezone ||
    form.holidays_policy !== originalForm.holidays_policy ||
    form.allow_holiday_eves !== originalForm.allow_holiday_eves ||
    form.lock_participants !== originalForm.lock_participants ||
    form.allow_anonymous_participants !== originalForm.allow_anonymous_participants ||
    form.holiday_min_time !== originalForm.holiday_min_time ||
    form.holiday_max_time !== originalForm.holiday_max_time ||
    form.holiday_eve_min_time !== originalForm.holiday_eve_min_time ||
    form.holiday_eve_max_time !== originalForm.holiday_eve_max_time ||
    form.start_date !== originalForm.start_date ||
    form.end_date !== originalForm.end_date ||
    JSON.stringify(form.allowed_weekdays) !== JSON.stringify(originalForm.allowed_weekdays) ||
    JSON.stringify(form.weekday_times) !== JSON.stringify(originalForm.weekday_times)
  );
});

const errors = reactive({
  name: '',
  threshold: '',
});

/** Reset every id-scoped setting (form, original snapshot, notify config, edits). */
function resetIdScopedState() {
  Object.assign(form, {
    name: '',
    description: '',
    threshold: 1,
    allowed_weekdays: [0, 1, 2, 3, 4, 5, 6],
    min_duration_hours: 0,
    timezone: 'Europe/Paris',
    holidays_policy: 'ignore',
    allow_holiday_eves: false,
    lock_participants: false,
    allow_anonymous_participants: false,
    weekday_times: createEmptyWeekdayTimes(),
    holiday_min_time: '',
    holiday_max_time: '',
    holiday_eve_min_time: '',
    holiday_eve_max_time: '',
    start_date: '',
    end_date: '',
  });
  Object.assign(originalForm, {
    name: '',
    description: '',
    threshold: 1,
    allowed_weekdays: [0, 1, 2, 3, 4, 5, 6],
    min_duration_hours: 0,
    timezone: 'Europe/Paris',
    holidays_policy: 'ignore',
    allow_holiday_eves: false,
    lock_participants: false,
    allow_anonymous_participants: false,
    weekday_times: createEmptyWeekdayTimes(),
    holiday_min_time: '',
    holiday_max_time: '',
    holiday_eve_min_time: '',
    holiday_eve_max_time: '',
    start_date: '',
    end_date: '',
  });
  notifyConfig.value = getDefaultNotifyConfig();
  newParticipantName.value = '';
  editingParticipantId.value = null;
  editingParticipantName.value = '';
  errors.name = '';
  errors.threshold = '';
}

const calendar = computed(() => calendarStore.currentCalendar);

const publicUrl = computed(() => {
  if (!calendar.value) return '';
  return `${window.location.origin}/c/${calendar.value.public_token}`;
});

const icsUrl = computed(() => {
  if (!calendar.value) return '';
  return `${window.location.origin}/api/v1/ics/feed/${calendar.value.ics_token}.ics`;
});

async function loadCalendar() {
  // Bind every continuation to the id and version this load started under: a slow
  // response for calendar A must not populate the form, notification config or
  // navigation for calendar B after a same-component route change.
  const version = ++loadVersion;
  const id = calendarId.value;
  if (loadedForId !== id) {
    loadedForId = id;
    resetIdScopedState();
  }
  loading.value = true;

  try {
    await calendarStore.fetchCalendar(id);
    if (version !== loadVersion) return;

    if (calendar.value) {
      form.name = calendar.value.name;
      form.description = calendar.value.description || '';
      form.threshold = calendar.value.threshold;
      form.allowed_weekdays = calendar.value.allowed_weekdays || [0, 1, 2, 3, 4, 5, 6];
      form.min_duration_hours = calendar.value.min_duration_hours || 0;
      form.timezone = calendar.value.timezone || 'Europe/Paris';
      form.holidays_policy = calendar.value.holidays_policy || 'ignore';
      form.allow_holiday_eves = calendar.value.allow_holiday_eves || false;
      form.lock_participants = (calendar.value as any).lock_participants || false;
      form.allow_anonymous_participants =
        (calendar.value as any).allow_anonymous_participants || false;

      // Initialize weekday_times from calendar data (if available)
      if ((calendar.value as any).weekday_times) {
        form.weekday_times = (calendar.value as any).weekday_times;
      }

      // Initialize holiday times from calendar data (if available)
      form.holiday_min_time = (calendar.value as any).holiday_min_time || '';
      form.holiday_max_time = (calendar.value as any).holiday_max_time || '';
      form.holiday_eve_min_time = (calendar.value as any).holiday_eve_min_time || '';
      form.holiday_eve_max_time = (calendar.value as any).holiday_eve_max_time || '';

      // Initialize date range from calendar data (if available)
      form.start_date = (calendar.value as any).start_date
        ? new Date((calendar.value as any).start_date).toISOString().split('T')[0]
        : '';
      form.end_date = (calendar.value as any).end_date
        ? new Date((calendar.value as any).end_date).toISOString().split('T')[0]
        : '';

      // Save original values
      originalForm.name = calendar.value.name;
      originalForm.description = calendar.value.description || '';
      originalForm.threshold = calendar.value.threshold;
      originalForm.allowed_weekdays = calendar.value.allowed_weekdays || [0, 1, 2, 3, 4, 5, 6];
      originalForm.min_duration_hours = calendar.value.min_duration_hours || 0;
      originalForm.timezone = calendar.value.timezone || 'Europe/Paris';
      originalForm.holidays_policy = calendar.value.holidays_policy || 'ignore';
      originalForm.allow_holiday_eves = calendar.value.allow_holiday_eves || false;
      originalForm.lock_participants = (calendar.value as any).lock_participants || false;
      originalForm.allow_anonymous_participants =
        (calendar.value as any).allow_anonymous_participants || false;

      // Save original weekday_times
      if ((calendar.value as any).weekday_times) {
        originalForm.weekday_times = JSON.parse(
          JSON.stringify((calendar.value as any).weekday_times)
        );
      }

      // Save original holiday times
      originalForm.holiday_min_time = (calendar.value as any).holiday_min_time || '';
      originalForm.holiday_max_time = (calendar.value as any).holiday_max_time || '';
      originalForm.holiday_eve_min_time = (calendar.value as any).holiday_eve_min_time || '';
      originalForm.holiday_eve_max_time = (calendar.value as any).holiday_eve_max_time || '';

      // Save original date range
      originalForm.start_date = form.start_date;
      originalForm.end_date = form.end_date;

      // Load notification config
      try {
        const notify = await getNotifyConfig(id);
        if (version !== loadVersion) return;
        notifyConfig.value = notify;
      } catch (_error) {
        // If notify config doesn't exist, use default
        if (version !== loadVersion) return;
        notifyConfig.value = getDefaultNotifyConfig();
      }
    }
  } catch (error: any) {
    // A superseded id's failure must not redirect the newer route or toast about it.
    if (version !== loadVersion) return;
    toastStore.error(t(translateErrorMessage(error, { fallback: 'calendar.fetchError' })));
    // Redirect to dashboard on error
    router.push('/dashboard');
  } finally {
    if (version === loadVersion) {
      loading.value = false;
    }
  }
}

/** True while this calendar and load are still the ones on screen. */
function routeStill(id: string, version: number): boolean {
  return calendarId.value === id && loadVersion === version;
}

function validateForm(): boolean {
  errors.name = '';
  errors.threshold = '';

  let isValid = true;

  if (!form.name.trim()) {
    errors.name = t('errors.required');
    isValid = false;
  }

  if (!calendar.value?.participants || calendar.value.participants.length === 0) {
    toastStore.error(t('calendar.participantsRequired'));
    isValid = false;
  }

  if (!form.threshold || form.threshold < 1) {
    errors.threshold = t('calendar.thresholdMinError');
    isValid = false;
  }

  if (
    !form.allow_anonymous_participants &&
    calendar.value?.participants &&
    form.threshold > calendar.value.participants.length
  ) {
    errors.threshold = t('calendar.thresholdMaxError');
    isValid = false;
  }

  return isValid;
}

async function handleUpdate() {
  if (!validateForm()) {
    return;
  }

  updating.value = true;
  const id = calendarId.value;
  const version = loadVersion;
  const op = ++saveSeq;

  try {
    // Normalize 00:00 times to empty (not meaningful as restrictions)
    const normalizedHolidayMinTime = normalizeTime(form.holiday_min_time);
    const normalizedHolidayMaxTime = normalizeTime(form.holiday_max_time);
    const normalizedHolidayEveMinTime = normalizeTime(form.holiday_eve_min_time);
    const normalizedHolidayEveMaxTime = normalizeTime(form.holiday_eve_max_time);
    const rawHolidayMin = form.holiday_min_time;
    const rawHolidayMax = form.holiday_max_time;
    const rawHolidayEveMin = form.holiday_eve_min_time;
    const rawHolidayEveMax = form.holiday_eve_max_time;

    const submitted = {
      name: form.name.trim(),
      description: form.description.trim(),
      threshold: form.threshold,
      allowed_weekdays: [...form.allowed_weekdays],
      min_duration_hours: form.min_duration_hours,
      timezone: form.timezone,
      holidays_policy: form.holidays_policy,
      allow_holiday_eves: form.allow_holiday_eves,
      lock_participants: form.lock_participants,
      allow_anonymous_participants: form.allow_anonymous_participants,
      weekday_times: prepareWeekdayTimes(form.weekday_times),
      holiday_min_time: normalizedHolidayMinTime,
      holiday_max_time: normalizedHolidayMaxTime,
      holiday_eve_min_time: normalizedHolidayEveMinTime,
      holiday_eve_max_time: normalizedHolidayEveMaxTime,
      start_date: form.start_date,
      end_date: form.end_date,
    };
    await calendarStore.updateCalendar(id, {
      ...submitted,
      description: submitted.description || undefined,
      weekday_times: submitted.weekday_times,
      holiday_min_time: submitted.holiday_min_time,
      holiday_max_time: submitted.holiday_max_time,
      holiday_eve_min_time: submitted.holiday_eve_min_time,
      holiday_eve_max_time: submitted.holiday_eve_max_time,
      start_date: submitted.start_date || undefined,
      end_date: submitted.end_date || undefined,
    } as any);

    // Advance only the snapshot that was sent. Edits made while the request was in
    // flight stay in the live form and remain unsaved. A completion for a calendar
    // the visitor left must not touch the form now on screen.
    if (!routeStill(id, version)) return;
    originalForm.name = submitted.name;
    originalForm.description = submitted.description;
    originalForm.threshold = submitted.threshold;
    originalForm.allowed_weekdays = submitted.allowed_weekdays;
    originalForm.min_duration_hours = submitted.min_duration_hours;
    originalForm.timezone = submitted.timezone;
    originalForm.holidays_policy = submitted.holidays_policy;
    originalForm.allow_holiday_eves = submitted.allow_holiday_eves;
    originalForm.lock_participants = submitted.lock_participants;
    originalForm.allow_anonymous_participants = submitted.allow_anonymous_participants;
    originalForm.weekday_times = JSON.parse(JSON.stringify(submitted.weekday_times));
    originalForm.holiday_min_time = rawHolidayMin;
    originalForm.holiday_max_time = rawHolidayMax;
    originalForm.holiday_eve_min_time = rawHolidayEveMin;
    originalForm.holiday_eve_max_time = rawHolidayEveMax;
    originalForm.start_date = submitted.start_date;
    originalForm.end_date = submitted.end_date;
  } catch (error: any) {
    if (!routeStill(id, version)) return;
    toastStore.error(t(translateErrorMessage(error, { fallback: 'calendar.updateError' })));
  } finally {
    if (saveSeq === op) {
      updating.value = false;
    }
  }
}

async function handleSaveNotifications(config: NotifyConfig) {
  const id = calendarId.value;
  const version = loadVersion;
  try {
    // An instance without SMTP cannot ever deliver the email channel; persist
    // that truthfully instead of saving an "enabled" flag that says it can.
    const saved = { ...config };
    if (!smtpConfigured.value) {
      saved.channels = { ...saved.channels, email: { ...saved.channels.email, enabled: false } };
    }
    await updateNotifyConfig(id, saved);
    if (!routeStill(id, version)) return;
    toastStore.success(t('calendar.settingsSaved'));
  } catch (error: any) {
    if (!routeStill(id, version)) return;
    toastStore.error(t(translateErrorMessage(error, { fallback: 'notifications.saveError' })));
  }
}

async function handleAddParticipant() {
  if (!newParticipantName.value.trim()) {
    return;
  }

  addingParticipant.value = true;
  const id = calendarId.value;
  const version = loadVersion;
  const op = ++participantSeq;
  const name = newParticipantName.value.trim();

  try {
    await calendarStore.addParticipant(id, { name });
    if (!routeStill(id, version)) return;
    newParticipantName.value = '';
    // No need to reload - the store updates currentCalendar automatically
  } catch (error: any) {
    if (!routeStill(id, version)) return;
    toastStore.error(t(translateErrorMessage(error, { fallback: 'calendar.addParticipantError' })));
  } finally {
    if (participantSeq === op) {
      addingParticipant.value = false;
    }
  }
}

function startEditParticipant(participantId: string, participantName: string) {
  editingParticipantId.value = participantId;
  editingParticipantName.value = participantName;
}

function cancelEditParticipant() {
  editingParticipantId.value = null;
  editingParticipantName.value = '';
}

async function handleSaveParticipant(participantId: string) {
  if (!editingParticipantName.value.trim()) {
    return;
  }

  const id = calendarId.value;
  const version = loadVersion;
  const name = editingParticipantName.value.trim();
  try {
    await calendarStore.updateParticipant(id, participantId, { name });
    if (!routeStill(id, version)) return;
    cancelEditParticipant();
    // No need to reload - the store updates currentCalendar automatically
  } catch (error: any) {
    if (!routeStill(id, version)) return;
    toastStore.error(t(translateErrorMessage(error, { fallback: 'calendar.updateError' })));
  }
}

async function handleDeleteParticipant(participantId: string, participantName: string) {
  // Capture the target before the (awaiting) confirmation dialog.
  const id = calendarId.value;
  const version = loadVersion;
  if (
    !(await confirm({ message: t('calendar.confirmDeleteParticipant', { name: participantName }) }))
  ) {
    return;
  }
  if (!routeStill(id, version)) return;

  try {
    await calendarStore.deleteParticipant(id, participantId);
    if (!routeStill(id, version)) return;

    // Automatically adjust threshold if necessary
    if (calendar.value?.participants) {
      const newParticipantCount = calendar.value.participants.length;
      if (form.threshold > newParticipantCount) {
        form.threshold = newParticipantCount;
      }
    }
    // No need to reload - the store updates currentCalendar automatically
  } catch (error: any) {
    if (!routeStill(id, version)) return;
    toastStore.error(
      t(translateErrorMessage(error, { fallback: 'calendar.deleteParticipantError' }))
    );
  }
}

async function handleRegenerateToken(tokenType: 'public' | 'ics') {
  const confirmMessage =
    tokenType === 'public'
      ? t('calendar.confirmRegeneratePublic')
      : t('calendar.confirmRegenerateICS');

  // Capture the target before the (awaiting) confirmation dialog.
  const id = calendarId.value;
  const version = loadVersion;
  if (!(await confirm({ message: confirmMessage }))) {
    return;
  }
  if (!routeStill(id, version)) return;

  try {
    if (tokenType === 'public') {
      await calendarStore.regeneratePublicToken(id);
    } else {
      await calendarStore.regenerateICSToken(id);
    }
    if (!routeStill(id, version)) return;
    // No need to reload - the store updates currentCalendar automatically
  } catch (error: any) {
    if (!routeStill(id, version)) return;
    toastStore.error(t(translateErrorMessage(error, { fallback: 'calendar.regenerateError' })));
  }
}

async function handleDelete() {
  // Capture the target before the (awaiting) confirmation dialog.
  const id = calendarId.value;
  const version = loadVersion;
  const confirmed = await confirm({
    title: t('calendar.deleteCalendar'),
    message: t('calendar.confirmDelete'),
    detail: t('calendar.confirmDeleteMessage'),
    confirmLabel: t('common.delete'),
  });
  if (!confirmed) {
    return;
  }
  // The dialog outlives this view. Confirming it after navigating away must not
  // delete the calendar the dialog was opened for.
  if (!routeStill(id, version)) return;

  deleting.value = true;

  try {
    await calendarStore.deleteCalendar(id);
    // A delete that finishes after the visitor left this calendar must not yank
    // them off whatever route they are on now.
    if (calendarId.value !== id || loadVersion !== version) return;
    router.push('/dashboard');
  } catch (error: any) {
    if (calendarId.value !== id || loadVersion !== version) return;
    toastStore.error(t(translateErrorMessage(error, { fallback: 'calendar.deleteError' })));
  } finally {
    if (calendarId.value === id && loadVersion === version) {
      deleting.value = false;
    }
  }
}

let toggleIntent = 0;

async function persistAccessToggles(submitted: {
  lock_participants: boolean;
  allow_anonymous_participants: boolean;
}) {
  const id = calendarId.value;
  const version = loadVersion;
  const intent = ++toggleIntent;
  const write = accessToggleChain.then(() => calendarStore.updateCalendar(id, submitted as any));
  accessToggleChain = write.catch(() => {});
  try {
    await write;
    if (!routeStill(id, version)) return;
    originalForm.lock_participants = submitted.lock_participants;
    originalForm.allow_anonymous_participants = submitted.allow_anonymous_participants;
    // An older success must not clobber a newer optimistic intent. Only the latest
    // intent reconciles the live controls (an older failure may have wiped them).
    if (intent !== toggleIntent) return;
    form.lock_participants = submitted.lock_participants;
    form.allow_anonymous_participants = submitted.allow_anonymous_participants;
    toastStore.success(t('calendar.calendarUpdated'));
  } catch (error: any) {
    if (!routeStill(id, version) || intent !== toggleIntent) return;
    form.lock_participants = originalForm.lock_participants;
    form.allow_anonymous_participants = originalForm.allow_anonymous_participants;
    toastStore.error(t(translateErrorMessage(error, { fallback: 'calendar.updateError' })));
  }
}

async function handleLockParticipantsChange() {
  // Mutually exclusive: disable anonymous registration when locking
  if (form.lock_participants) {
    form.allow_anonymous_participants = false;
  }
  await persistAccessToggles({
    lock_participants: form.lock_participants,
    allow_anonymous_participants: form.allow_anonymous_participants,
  });
}

async function handleAllowAnonymousParticipantsChange() {
  // Mutually exclusive: disable lock when allowing anonymous registration
  if (form.allow_anonymous_participants) {
    form.lock_participants = false;
  }
  await persistAccessToggles({
    lock_participants: form.lock_participants,
    allow_anonymous_participants: form.allow_anonymous_participants,
  });
}

function copyParticipantLink(participantId: string) {
  if (!calendar.value) return;

  const link = `${window.location.origin}/c/${calendar.value.public_token}/p/${participantId}`;
  navigator.clipboard.writeText(link);
  toastStore.success(t('calendar.participantLinkCopied'));
}

function copyToClipboard(text: string) {
  navigator.clipboard.writeText(text);
  toastStore.success(t('calendar.linkCopied'));
}

// Warn user about unsaved changes before leaving
const handleBeforeUnload = (e: BeforeUnloadEvent) => {
  if (hasUnsavedChanges.value) {
    e.preventDefault();
    e.returnValue = '';
  }
};

// Vue Router navigation guard
onBeforeRouteLeave(async (_to, _from, next) => {
  if (hasUnsavedChanges.value) {
    const answer = await confirm({ message: t('calendar.confirmUnsavedChanges') });
    next(answer ? undefined : false);
  } else {
    next();
  }
});

// Reload whenever the route's calendar id changes (including the initial mount): Vue
// Router reuses this component when only `:id` changes, so a one-time onMounted load
// would leave the view editing calendar A while the URL says B. Each load is fenced by
// `loadVersion`, so a slower A response can never overwrite B.
watch(
  () => calendarId.value,
  () => loadCalendar(),
  { immediate: true }
);

onMounted(() => {
  // Email options follow the instance's actual SMTP configuration.
  // /auth/magic-link/available is the existing endpoint for exactly this answer.
  authApi
    .checkMagicLinkAvailable()
    .then(result => {
      smtpConfigured.value = result.available;
    })
    .catch(() => {
      // Best-effort: on failure the safe answer is "no SMTP", so email options
      // stay hidden instead of being offered for mails that could never leave.
      smtpConfigured.value = false;
    });
  // Add beforeunload listener
  window.addEventListener('beforeunload', handleBeforeUnload);
});

onBeforeUnmount(() => {
  // Remove beforeunload listener
  window.removeEventListener('beforeunload', handleBeforeUnload);
});
</script>
