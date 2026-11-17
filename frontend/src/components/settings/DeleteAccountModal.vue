<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-[60] flex items-center justify-center overflow-y-auto bg-gray-900/60 p-4"
    >
      <div
        ref="panel"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        class="w-full max-w-lg rounded-lg border border-gray-200 bg-white p-6 shadow-xl dark:border-gray-700 dark:bg-gray-900"
      >
        <h2
          :id="titleId"
          class="mb-4 font-display text-lg font-semibold text-gray-900 dark:text-white"
        >
          {{ t('settings.deleteAccount.title') }}
        </h2>

        <div class="space-y-3 text-sm text-gray-700 dark:text-gray-300">
          <template v-if="calendars === null">
            <p>{{ t('settings.deleteAccount.impactGeneric') }}</p>
          </template>
          <template v-else-if="calendars.length === 0">
            <p>{{ t('settings.deleteAccount.noCalendars') }}</p>
          </template>
          <template v-else>
            <div>
              <p>{{ t('settings.deleteAccount.calendarsDeleted', calendars.length) }}</p>
              <ul class="mt-1 list-inside list-disc" data-testid="calendar-names">
                <li v-for="calendar in shownCalendars" :key="calendar.id">{{ calendar.name }}</li>
                <li v-if="hiddenCount > 0" class="list-none">
                  {{ t('settings.deleteAccount.andOthers', hiddenCount) }}
                </li>
              </ul>
            </div>
            <p>{{ t('settings.deleteAccount.participantsErased', participantCount) }}</p>
          </template>
          <p>{{ t('settings.deleteAccount.securityDeleted') }}</p>

          <div>
            <h3 class="font-medium text-gray-900 dark:text-white">
              {{ t('settings.deleteAccount.notDeletedTitle') }}
            </h3>
            <p>{{ t('settings.deleteAccount.notDeleted') }}</p>
            <p>{{ t('settings.deleteAccount.backups') }}</p>
          </div>

          <p
            v-if="participantCount > 0"
            class="rounded-md bg-yellow-50 p-3 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200"
          >
            {{ t('settings.deleteAccount.notifyParticipants') }}
          </p>

          <p class="font-semibold text-danger-700 dark:text-danger-400">
            {{ t('settings.deleteAccount.irreversible') }}
          </p>
        </div>

        <form class="mt-4 space-y-4" @submit.prevent="submit">
          <label for="delete-account-password" class="block">
            <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('settings.deleteAccount.passwordLabel') }}
            </span>
            <input
              id="delete-account-password"
              v-model="password"
              type="password"
              autocomplete="current-password"
              class="input"
              :aria-invalid="passwordError !== null"
              :aria-describedby="passwordError ? passwordErrorId : undefined"
              @input="passwordError = null"
            />
          </label>
          <p
            v-if="passwordError"
            :id="passwordErrorId"
            class="text-sm text-danger-600 dark:text-danger-400"
          >
            {{ passwordError }}
          </p>

          <label
            for="delete-account-confirm"
            class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300"
          >
            <input id="delete-account-confirm" v-model="understood" type="checkbox" class="mt-1" />
            <span>{{ t('settings.deleteAccount.confirmCheckbox') }}</span>
          </label>

          <p v-if="lastAdmin" role="alert" class="text-sm text-danger-600 dark:text-danger-400">
            {{ t('settings.deleteAccount.lastAdmin') }}
          </p>

          <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <button
              ref="cancelButton"
              type="button"
              class="btn btn-ghost"
              :disabled="deleting"
              @click="close"
            >
              {{ t('common.cancel') }}
            </button>
            <button
              type="submit"
              class="btn btn-danger"
              data-testid="confirm-delete"
              :disabled="!canSubmit"
            >
              {{
                deleting
                  ? t('settings.deleteAccount.deleting')
                  : t('settings.deleteAccount.confirmButton')
              }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
/**
 * The confirmation for deleting one's own account. ConfirmDialog cannot carry a
 * password field or the impact summary, so this is its own dialog, with the same
 * keyboard behaviour from useFocusTrap.
 *
 * Guards against a deletion by accident: the button stays disabled until the box
 * is ticked and a password is typed, focus opens on Cancel so a reflexive Enter
 * deletes nothing, a backdrop click does not close, and the button cannot be
 * pressed twice while the request is in flight.
 */
import { computed, ref, useId, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { calendarsApi } from '@/api/calendars';
import { useFocusTrap } from '@/composables/useFocusTrap';
import { useAuthStore } from '@/stores/auth';
import { useToastStore } from '@/stores/toast';
import type { CalendarWithParticipants } from '@/types';
import { translateErrorMessage } from '@/utils/errorTranslator';

const MAX_NAMES = 5;

const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ close: [] }>();

const { t } = useI18n();
const router = useRouter();
const authStore = useAuthStore();
const toast = useToastStore();

const titleId = useId();
const passwordErrorId = useId();
const panel = ref<HTMLElement | null>(null);
const cancelButton = ref<HTMLButtonElement | null>(null);

/** null when the list could not be loaded: the summary then stays generic. */
const calendars = ref<CalendarWithParticipants[] | null>(null);
const password = ref('');
const understood = ref(false);
const deleting = ref(false);
const lastAdmin = ref(false);
const passwordError = ref<string | null>(null);

const shownCalendars = computed(() => calendars.value?.slice(0, MAX_NAMES) ?? []);
const hiddenCount = computed(() => Math.max((calendars.value?.length ?? 0) - MAX_NAMES, 0));
const participantCount = computed(
  // The list omits `participants` on a calendar that has none (omitempty).
  () => calendars.value?.reduce((sum, c) => sum + (c.participants?.length ?? 0), 0) ?? 0
);
const canSubmit = computed(
  () => understood.value && password.value !== '' && !deleting.value && !lastAdmin.value
);

useFocusTrap(() => props.open, {
  container: panel,
  onEscape: () => close(),
  initialFocus: () => cancelButton.value,
});

watch(
  () => props.open,
  async open => {
    if (!open) return;
    password.value = '';
    understood.value = false;
    lastAdmin.value = false;
    passwordError.value = null;
    calendars.value = null;
    try {
      // An owner of no calendar gets `data: null`, not [] (same guard as the calendar store).
      const result = await calendarsApi.getAll();
      calendars.value = Array.isArray(result) ? result : [];
    } catch {
      // The deletion stays possible; the impact is just not counted.
    }
  },
  { immediate: true }
);

function close() {
  if (deleting.value) return;
  emit('close');
}

async function submit() {
  if (!canSubmit.value) return;
  deleting.value = true;
  try {
    await authStore.deleteAccount(password.value);
    toast.success(t('settings.deleteAccount.success'));
    emit('close');
    await router.push('/');
  } catch (err) {
    const code = (err as { code?: string }).code;
    if (code === 'BAD_REQUEST' || code === 'VALIDATION_ERROR') {
      passwordError.value = t('settings.deleteAccount.wrongPassword');
      password.value = '';
    } else if (code === 'CONFLICT') {
      lastAdmin.value = true;
    } else {
      toast.error(t(translateErrorMessage(err, { fallback: 'settings.deleteAccount.error' })));
    }
  } finally {
    deleting.value = false;
  }
}
</script>
