<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="card mb-6">
    <h3 class="mb-4 font-display text-lg font-semibold text-gray-900 dark:text-white">
      {{ t('notifications.emailVerification') }}
    </h3>

    <!-- No email added yet -->
    <div v-if="!email">
      <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
        {{ t('notifications.addEmail') }}
      </p>
      <form class="flex gap-2" @submit.prevent="handleAddEmail">
        <input
          v-model="emailInput"
          type="email"
          class="input flex-1"
          :placeholder="t('notifications.emailPlaceholder')"
          required
        />
        <button type="submit" class="btn btn-primary" :disabled="addingEmail">
          <svg v-if="addingEmail" class="mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
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
          {{ addingEmail ? t('common.saving') : t('common.save') }}
        </button>
      </form>
    </div>

    <!-- Email pending verification -->
    <div v-else-if="!emailVerified" class="space-y-3">
      <div v-if="!changingEmail" class="space-y-3">
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
              {{ t('notifications.emailPending', { email }) }}
            </p>
          </div>
        </div>
        <div class="flex gap-2">
          <button
            class="btn btn-ghost"
            :disabled="resendingEmail"
            @click="handleResendVerification"
          >
            {{ resendingEmail ? t('common.sending') : t('notifications.resendVerification') }}
          </button>
          <button class="btn btn-ghost" @click="changingEmail = true">
            {{ t('notifications.changeEmail') }}
          </button>
        </div>
      </div>

      <!-- Change email form -->
      <ParticipantEmailChangeForm
        v-else
        v-model="newEmailInput"
        :current-email="email"
        :saving="addingEmail"
        @submit="handleChangeEmail"
        @cancel="handleCancelChangeEmail"
      />
    </div>

    <!-- Email verified -->
    <div v-else class="space-y-3">
      <div v-if="!changingEmail" class="space-y-3">
        <div class="rounded-lg bg-success-50 p-4 dark:bg-success-900/20">
          <div class="flex">
            <svg
              class="h-5 w-5 text-success-600 dark:text-success-400"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <p class="ml-3 text-sm text-success-600 dark:text-success-400">
              {{ t('notifications.emailVerified', { email }) }}
            </p>
          </div>
        </div>
        <button class="btn btn-ghost" @click="changingEmail = true">
          {{ t('notifications.changeEmail') }}
        </button>
      </div>

      <!-- Change email form -->
      <ParticipantEmailChangeForm
        v-else
        v-model="newEmailInput"
        :current-email="email"
        :saving="addingEmail"
        @submit="handleChangeEmail"
        @cancel="handleCancelChangeEmail"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * E-mail enrolment for a participant: adding an address, resending its verification,
 * and replacing it.
 *
 * Three states, each with its own form, and none of them related to availability — which
 * is what made this the first 150 lines of a calendar view. The two "change address"
 * branches were byte-identical duplicates in the original; they are one child component
 * here.
 *
 * The stores are used directly rather than passed down. Re-reading the public calendar
 * after a successful write is what makes the panel switch state, and routing that
 * through the parent would either duplicate the store call or make the save spinner stop
 * before the new state is known.
 */
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCalendarStore } from '@/stores/calendar';
import { useToastStore } from '@/stores/toast';
import { addParticipantEmail, resendVerificationEmail } from '@/api/notify';
import ParticipantEmailChangeForm from './ParticipantEmailChangeForm.vue';

const props = defineProps<{
  /** The calendar's public token. */
  token: string;
  participantId: string;
  /** The address currently on file, if any. */
  email?: string;
  emailVerified?: boolean;
}>();

const { t } = useI18n();
const calendarStore = useCalendarStore();
const toastStore = useToastStore();

const emailInput = ref('');
const newEmailInput = ref('');
const addingEmail = ref(false);
const resendingEmail = ref(false);
const changingEmail = ref(false);

async function handleAddEmail() {
  if (!emailInput.value.trim() || !props.token || !props.participantId) {
    return;
  }

  addingEmail.value = true;

  try {
    await addParticipantEmail(props.token, props.participantId, emailInput.value.trim());
    toastStore.success(t('notifications.emailSent'));
    emailInput.value = '';
    // Reload calendar to get updated participant email info
    await calendarStore.fetchPublicCalendar(props.token, props.participantId);
  } catch (error: any) {
    toastStore.error(error.message || t('notifications.emailError'));
  } finally {
    addingEmail.value = false;
  }
}

async function handleResendVerification() {
  if (!props.token || !props.participantId) {
    return;
  }

  resendingEmail.value = true;

  try {
    await resendVerificationEmail(props.token, props.participantId);
    toastStore.success(t('notifications.emailSent'));
  } catch (error: any) {
    toastStore.error(error.message || t('notifications.emailError'));
  } finally {
    resendingEmail.value = false;
  }
}

async function handleChangeEmail() {
  if (!newEmailInput.value.trim() || !props.token || !props.participantId) {
    return;
  }

  addingEmail.value = true;

  try {
    await addParticipantEmail(props.token, props.participantId, newEmailInput.value.trim());
    toastStore.success(t('notifications.emailChanged'));
    newEmailInput.value = '';
    changingEmail.value = false;
    // Reload calendar to get updated participant email info
    await calendarStore.fetchPublicCalendar(props.token, props.participantId);
  } catch (error: any) {
    toastStore.error(error.message || t('notifications.emailError'));
  } finally {
    addingEmail.value = false;
  }
}

function handleCancelChangeEmail() {
  changingEmail.value = false;
  newEmailInput.value = '';
}
</script>
