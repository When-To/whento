/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { readonly, ref, type DeepReadonly, type Ref } from 'vue';

/**
 * The replacement for `window.confirm`.
 *
 * Thirteen destructive actions used the native dialog: unstyled, unthemed, with
 * OS-language "OK"/"Cancel" buttons that no amount of vue-i18n can reach, and
 * synchronously blocking the main thread. This keeps the same call shape — ask a
 * question, get a boolean — so a call site changes from
 *
 *     if (!confirm(t('calendar.confirmDelete'))) return;
 *
 * to
 *
 *     if (!(await confirm({ message: t('calendar.confirmDelete') }))) return;
 *
 * and gains a themed, translated, keyboard-accessible dialog.
 *
 * State lives at module scope and is rendered by the single `<ConfirmDialog />`
 * mounted in App.vue, so no caller has to place a component or own an `open` flag.
 */

export interface ConfirmOptions {
  /** The question. Required, and already translated by the caller. */
  message: string;
  /** Heading. Defaults to `common.confirmTitle`. */
  title?: string;
  /** Extra detail rendered under the message, e.g. "this cannot be undone". */
  detail?: string;
  /** Confirm button label. Defaults to `common.confirm`. */
  confirmLabel?: string;
  /** Cancel button label. Defaults to `common.cancel`. */
  cancelLabel?: string;
  /**
   * `'danger'` (the default) paints the confirm button red and focuses *cancel*
   * on open, so a stray Enter keypress cannot delete anything. `'primary'` focuses
   * the confirm button, for non-destructive questions.
   */
  tone?: 'danger' | 'primary';
}

interface PendingRequest {
  readonly options: ConfirmOptions;
  readonly settle: (confirmed: boolean) => void;
}

const pending = ref<PendingRequest | null>(null);

/**
 * Ask the user to confirm. Resolves `true` if they confirmed, `false` if they
 * cancelled, pressed Escape, or a second request superseded this one.
 *
 * Never rejects, so `await confirm(...)` needs no try/catch.
 */
export function confirm(options: ConfirmOptions): Promise<boolean> {
  // A second request while one is open cancels the first rather than queueing or
  // dropping it: its awaiting caller must not be left hanging forever.
  pending.value?.settle(false);

  return new Promise<boolean>(resolve => {
    let settled = false;
    pending.value = {
      options,
      settle(confirmed: boolean) {
        if (settled) return;
        settled = true;
        resolve(confirmed);
      },
    };
  });
}

/** Answer the open request and close the dialog. Used by ConfirmDialog.vue. */
export function resolveConfirm(confirmed: boolean): void {
  const request = pending.value;
  pending.value = null;
  request?.settle(confirmed);
}

export interface ConfirmController {
  /** The open request, or null when no dialog is showing. */
  readonly pending: DeepReadonly<Ref<PendingRequest | null>>;
  confirm: typeof confirm;
  resolveConfirm: typeof resolveConfirm;
}

export function useConfirm(): ConfirmController {
  return {
    pending: readonly(pending) as DeepReadonly<Ref<PendingRequest | null>>,
    confirm,
    resolveConfirm,
  };
}
