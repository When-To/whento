<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <Teleport to="body">
    <Transition name="confirm">
      <div
        v-if="request"
        class="fixed inset-0 z-[60] flex items-center justify-center bg-gray-900/60 p-4"
      >
        <div
          ref="panel"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          :aria-describedby="messageId"
          class="w-full max-w-md rounded-lg border border-gray-200 bg-white p-6 shadow-xl dark:border-gray-700 dark:bg-gray-900"
        >
          <h2
            :id="titleId"
            class="mb-2 font-display text-lg font-semibold text-gray-900 dark:text-white"
          >
            {{ request.options.title ?? t('common.confirmTitle') }}
          </h2>

          <p :id="messageId" class="text-sm text-gray-700 dark:text-gray-300">
            {{ request.options.message }}
          </p>

          <p v-if="request.options.detail" class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            {{ request.options.detail }}
          </p>

          <div class="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <button ref="cancelButton" type="button" class="btn btn-ghost" @click="cancel">
              {{ request.options.cancelLabel ?? t('common.cancel') }}
            </button>
            <button
              ref="confirmButton"
              type="button"
              :class="['btn', isDanger ? 'btn-danger' : 'btn-primary']"
              @click="accept"
            >
              {{ request.options.confirmLabel ?? t('common.confirm') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
/**
 * The single host for `useConfirm()`. Mounted once, in App.vue — call sites never
 * place this component, they just `await confirm({ message })`.
 *
 * Accessibility, all of which the native `window.confirm` it replaces got for free
 * and none of which the app had after moving to a custom dialog would have been
 * automatic:
 *
 *  - `role="dialog"` + `aria-modal="true"`, labelled by the heading and described
 *    by the message, so a screen reader announces the question on open;
 *  - focus moves into the dialog on open, to *cancel* for destructive questions so
 *    a reflexive Enter cannot confirm a deletion;
 *  - Tab and Shift+Tab are trapped inside the panel;
 *  - Escape cancels;
 *  - the element that had focus before the dialog opened gets it back on close,
 *    which is what lets a keyboard user carry on where they were.
 *
 * The last four now come from `useFocusTrap`, extracted from this component so the
 * app's other dialogs get the same behaviour instead of a fourth hand-rolled copy.
 *
 * Clicking the backdrop deliberately does *not* dismiss: every current caller is a
 * destructive action, and an accidental click outside should not count as an answer.
 */
import { computed, ref, useId } from 'vue';
import { useI18n } from 'vue-i18n';
import { useConfirm } from '@/composables/useConfirm';
import { useFocusTrap } from '@/composables/useFocusTrap';

const { t } = useI18n();
const { pending, resolveConfirm } = useConfirm();

const request = computed(() => pending.value);
const isDanger = computed(() => (request.value?.options.tone ?? 'danger') === 'danger');

const titleId = useId();
const messageId = useId();

const panel = ref<HTMLElement | null>(null);
const confirmButton = ref<HTMLButtonElement | null>(null);
const cancelButton = ref<HTMLButtonElement | null>(null);

useFocusTrap(() => request.value !== null, {
  container: panel,
  onEscape: () => cancel(),
  // Cancel for destructive questions, so a reflexive Enter cannot confirm a deletion.
  initialFocus: () => (isDanger.value ? cancelButton.value : confirmButton.value),
});

function accept() {
  resolveConfirm(true);
}

function cancel() {
  resolveConfirm(false);
}
</script>

<style scoped>
.confirm-enter-active,
.confirm-leave-active {
  transition: opacity 0.15s ease;
}

.confirm-enter-from,
.confirm-leave-to {
  opacity: 0;
}
</style>
