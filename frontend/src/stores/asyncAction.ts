/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { computed, ref, type ComputedRef, type Ref } from 'vue';
import { i18n } from '@/i18n';
import { translateErrorMessage, type ApiErrorKeyOptions } from '@/utils/errorTranslator';

/**
 * The `loading` / `error` / `try` / `catch` block every store action used to
 * open-code, in one place.
 *
 * Two defects are fixed by construction rather than by convention:
 *
 *  1. **The `loading` race.** `calendar.ts` had a single `loading` flag mutated by
 *     twelve actions, each with `finally { loading.value = false }`. Two concurrent
 *     calls — the dashboard fetches the calendar list while a participant PATCH is
 *     in flight, which is the normal case — meant the first one to settle cleared
 *     the flag while the second was still running, so spinners vanished early and
 *     "save" buttons re-enabled mid-write. `loading` is now derived from a counter
 *     of in-flight actions, so it stays true until the last one settles.
 *
 *  2. **English leaking to French users.** Each `catch (err: any)` wrote
 *     `err.message || 'Failed to fetch calendars'`: either the backend's English
 *     developer prose, or an English literal hard-coded in the store. Both were
 *     rendered verbatim. `run` resolves the error through `translateErrorMessage`,
 *     which maps the structured `ApiError.code`, and falls back to the i18n key the
 *     action names for itself.
 *
 * `error` is cleared when an action starts and set when one rejects. The rejection
 * is always re-thrown: callers that need to branch on the failure still can, and
 * nothing here swallows an error.
 */
export interface AsyncActions {
  /** True while at least one action started through `run` is still in flight. */
  readonly loading: ComputedRef<boolean>;
  /** Translated message for the last failure, or null. */
  readonly error: Ref<string | null>;
  /** How many actions are currently in flight. Exposed for tests. */
  readonly pending: ComputedRef<number>;
  /**
   * Run `fn` with loading/error bookkeeping.
   *
   * @param fallbackKey i18n key describing what failed, used when the error carries
   *                    no code the translator recognises.
   */
  run<T>(fallbackKey: string, fn: () => Promise<T>, options?: ApiErrorKeyOptions): Promise<T>;
  /** Clear the last error without running anything. */
  clearError(): void;
}

export function useAsyncActions(): AsyncActions {
  const inFlight = ref(0);
  const error = ref<string | null>(null);

  const loading = computed(() => inFlight.value > 0);
  const pending = computed(() => inFlight.value);

  async function run<T>(
    fallbackKey: string,
    fn: () => Promise<T>,
    options: ApiErrorKeyOptions = {}
  ): Promise<T> {
    inFlight.value++;
    error.value = null;

    try {
      return await fn();
    } catch (err) {
      error.value = i18n.global.t(
        translateErrorMessage(err, { fallback: fallbackKey, ...options })
      );
      throw err;
    } finally {
      inFlight.value--;
    }
  }

  function clearError() {
    error.value = null;
  }

  return { loading, error, pending, run, clearError };
}
