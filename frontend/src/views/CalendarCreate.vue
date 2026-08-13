<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-6xl">
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
          {{ t('calendar.newCalendar') }}
        </h1>
      </div>

      <!-- Form -->
      <form class="space-y-6" @submit.prevent="handleSubmit">
        <!-- Calendar Info Card -->
        <div class="card">
          <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('calendar.calendarInfo') }}
          </h2>

          <div class="space-y-4">
            <CalendarInfoFields
              v-model:name="form.name"
              v-model:description="form.description"
              v-model:timezone="form.timezone"
              :name-error="errors.name"
            />
          </div>
        </div>

        <!-- Participants -->
        <CollapsibleSection :title="t('calendar.participants')" :default-open="true">
          <!-- Participants List -->
          <div v-if="participants.length > 0" class="mb-4 space-y-2">
            <div
              v-for="(_participant, index) in participants"
              :key="index"
              class="flex items-center gap-2"
            >
              <input
                v-model="participants[index]"
                type="text"
                class="input flex-1"
                :aria-label="t('calendar.participantName')"
                :placeholder="t('calendar.participantName')"
                required
              />
              <button
                v-if="participants.length > 1"
                type="button"
                class="btn btn-ghost text-danger-600 hover:bg-danger-50 dark:hover:bg-danger-900/20"
                :title="t('common.delete')"
                @click="removeParticipant(index)"
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
          <div class="flex gap-2 mb-4">
            <input
              v-model="newParticipantName"
              type="text"
              class="input flex-1"
              :aria-label="t('calendar.addParticipant')"
              :placeholder="t('calendar.participantNamePlaceholder')"
              @keyup.enter.prevent="addParticipant"
            />
            <button type="button" class="btn btn-secondary" @click="addParticipant">
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
          </div>

          <p v-if="errors.participants" class="mb-4 text-sm text-danger-600">
            {{ errors.participants }}
          </p>

          <ParticipantAccessToggles
            v-model:lock-participants="form.lock_participants"
            v-model:allow-anonymous-participants="form.allow_anonymous_participants"
            id-suffix="-create"
          />
        </CollapsibleSection>

        <!-- Participant threshold and minimum duration -->
        <CollapsibleSection :title="t('calendar.sectionThreshold')" :default-open="true">
          <CalendarThresholdFields
            v-model:threshold="form.threshold"
            v-model:min-duration-hours="form.min_duration_hours"
            :participant-count="participants.length"
            :allow-anonymous-participants="form.allow_anonymous_participants"
            :threshold-error="errors.threshold"
          />
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
        </CollapsibleSection>

        <!-- Notifications -->
        <NotificationSettings
          v-model="notifyConfig"
          :smtp-configured="smtpConfigured"
          :show-save-button="false"
        />

        <!-- Warning Message - No Participants -->
        <div
          v-if="!form.allow_anonymous_participants && participants.length === 0"
          class="rounded-lg bg-orange-50 p-4 dark:bg-orange-900/20"
        >
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
              {{ t('calendar.noParticipantsWarningCreate') }}
            </p>
          </div>
        </div>

        <!-- Error Message -->
        <div v-if="errorMessage" class="rounded-lg bg-danger-50 p-4 dark:bg-danger-900/20">
          <div class="flex">
            <svg
              class="h-5 w-5 text-danger-600 dark:text-danger-400"
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
            <p class="ml-3 text-sm text-danger-600 dark:text-danger-400">
              {{ errorMessage }}
            </p>
          </div>
        </div>

        <!-- Actions -->
        <div class="flex items-center justify-end gap-4">
          <router-link to="/dashboard" class="btn btn-ghost">
            {{ t('common.cancel') }}
          </router-link>
          <button
            type="submit"
            :disabled="loading || (!form.allow_anonymous_participants && participants.length === 0)"
            class="btn btn-primary"
          >
            <svg v-if="loading" class="mr-2 h-5 w-5 animate-spin" fill="none" viewBox="0 0 24 24">
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
            {{ loading ? t('common.creating') : t('common.create') }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { isSupportedLocale } from '@/i18n';
import { useCalendarStore } from '@/stores/calendar';
import { useAuthStore } from '@/stores/auth';
import { useToastStore } from '@/stores/toast';
import CollapsibleSection from '@/components/CollapsibleSection.vue';
import NotificationSettings from '@/components/NotificationSettings.vue';
import CalendarInfoFields from '@/components/calendar/CalendarInfoFields.vue';
import CalendarThresholdFields from '@/components/calendar/CalendarThresholdFields.vue';
import CalendarScheduleFields from '@/components/calendar/CalendarScheduleFields.vue';
import ParticipantAccessToggles from '@/components/calendar/ParticipantAccessToggles.vue';
import {
  createEmptyWeekdayTimes,
  normalizeTime,
  prepareWeekdayTimes,
} from '@/utils/calendar/weekdayTimes';
import { getDefaultNotifyConfig, updateNotifyConfig, type NotifyConfig } from '@/api/notify';
import { translateErrorMessage } from '@/utils/errorTranslator';

const router = useRouter();
const { t, locale } = useI18n();
const calendarStore = useCalendarStore();
const authStore = useAuthStore();
const toastStore = useToastStore();

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

// Notification config state
const notifyConfig = ref<NotifyConfig>(getDefaultNotifyConfig());
const smtpConfigured = ref(true); // TODO: Fetch from backend config

const participants = ref<string[]>([]);

// Automatically add the connected user as a default participant
// and initialize timezone with the user's timezone
onMounted(() => {
  if (authStore.user?.display_name) {
    participants.value.push(authStore.user.display_name);
  }
  if (authStore.user?.timezone) {
    form.timezone = authStore.user.timezone;
  }
});
const newParticipantName = ref('');
const loading = ref(false);
const errorMessage = ref('');
const errors = reactive({
  name: '',
  threshold: '',
  participants: '',
});

function addParticipant() {
  if (newParticipantName.value.trim()) {
    // Check for duplicates
    if (participants.value.includes(newParticipantName.value.trim())) {
      errors.participants = t('calendar.duplicateParticipant');
      return;
    }

    participants.value.push(newParticipantName.value.trim());
    newParticipantName.value = '';
    errors.participants = '';
  }
}

function removeParticipant(index: number) {
  participants.value.splice(index, 1);
}

function validateForm(): boolean {
  // Reset errors
  errors.name = '';
  errors.threshold = '';
  errors.participants = '';

  let isValid = true;

  if (!form.name.trim()) {
    errors.name = t('errors.required');
    isValid = false;
  }

  if (!form.allow_anonymous_participants && participants.value.length === 0) {
    errors.participants = t('calendar.participantsRequired');
    isValid = false;
  }

  if (!form.threshold || form.threshold < 1) {
    errors.threshold = t('calendar.thresholdMinError');
    isValid = false;
  }

  if (
    !form.allow_anonymous_participants &&
    participants.value.length > 0 &&
    form.threshold > participants.value.length
  ) {
    errors.threshold = t('calendar.thresholdMaxError');
    isValid = false;
  }

  return isValid;
}

async function handleSubmit() {
  errorMessage.value = '';

  if (!validateForm()) {
    return;
  }

  loading.value = true;

  try {
    // Create calendar with participants in a single atomic request
    // Normalize 00:00 times to empty (not meaningful as restrictions)
    const normalizedHolidayMinTime = normalizeTime(form.holiday_min_time);
    const normalizedHolidayMaxTime = normalizeTime(form.holiday_max_time);
    const normalizedHolidayEveMinTime = normalizeTime(form.holiday_eve_min_time);
    const normalizedHolidayEveMaxTime = normalizeTime(form.holiday_eve_max_time);

    const calendar = await calendarStore.createCalendar({
      name: form.name.trim(),
      description: form.description.trim() || undefined,
      threshold: form.threshold,
      allowed_weekdays: form.allowed_weekdays,
      min_duration_hours: form.min_duration_hours,
      timezone: form.timezone,
      holidays_policy: form.holidays_policy,
      allow_holiday_eves: form.allow_holiday_eves,
      lock_participants: form.lock_participants,
      allow_anonymous_participants: form.allow_anonymous_participants,
      weekday_times: prepareWeekdayTimes(form.weekday_times),
      // Send empty string (not undefined) for consistency with update
      holiday_min_time: normalizedHolidayMinTime,
      holiday_max_time: normalizedHolidayMaxTime,
      holiday_eve_min_time: normalizedHolidayEveMinTime,
      holiday_eve_max_time: normalizedHolidayEveMaxTime,
      start_date: form.start_date || undefined,
      end_date: form.end_date || undefined,
      notify_on_threshold: notifyConfig.value.enabled,
      // Narrowed rather than cast: the request only accepts a locale the backend
      // has email templates for, and `locale.value` is typed as a plain string.
      participant_locale: isSupportedLocale(locale.value) ? locale.value : undefined,
      participants: participants.value.filter(name => name.trim() !== ''),
    });

    // The creation endpoint deliberately ignores a notification configuration:
    // the backend only accepts one through PATCH /notify-config, which validates
    // the webhook URLs. This used to post a `notify_config` JSON string that the
    // API dropped on the floor, so notifications were never actually enabled on a
    // freshly created calendar.
    if (notifyConfig.value.enabled) {
      try {
        await updateNotifyConfig(calendar.id, notifyConfig.value);
      } catch {
        // The calendar itself exists; only its notification settings did not take.
        // Say so, and still send the user to the settings page to retry there,
        // rather than leaving them to create a second calendar.
        toastStore.error(t('notifications.saveError'));
      }
    }

    // Redirect to calendar management page
    router.push(`/calendars/${calendar.id}/settings`);
  } catch (error: any) {
    console.error('Error creating calendar:', error);
    errorMessage.value = t(translateErrorMessage(error, { fallback: 'calendar.createError' }));
  } finally {
    loading.value = false;
  }
}
</script>
