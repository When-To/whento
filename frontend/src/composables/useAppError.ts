/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { readonly, ref, type DeepReadonly, type Ref } from 'vue';

/**
 * The application's error boundary state.
 *
 * Before this existed there was no `onErrorCaptured`, no `app.config.errorHandler`,
 * no `<Suspense>` and no `unhandledrejection` listener anywhere in the tree. A throw
 * during any component's render unmounted the whole app and left a white page — the
 * only trace being a stack in the console, which no user reads.
 *
 * Module-level state on purpose: `main.ts` (the global handler) and `App.vue` (the
 * boundary) both have to reach it, and it must survive the component tree being torn
 * down, which is exactly the situation it exists to report.
 */

const failed = ref(false);
const reference = ref('');

/**
 * A short, opaque reference shown to the user and printed next to the stack in the
 * console. It carries no technical detail by itself, but it lets a user quote
 * something a maintainer can grep for in a session recording or a support thread.
 */
function newReference(): string {
  return Math.random().toString(36).slice(2, 8).toUpperCase();
}

/**
 * Record a fatal error and switch the app over to the fallback screen.
 *
 * `context` is developer-facing only (Vue's `info` string, or the name of the
 * handler that caught it). It is never rendered — the user sees a translated,
 * generic message and the reference, and nothing else. Leaking `err.message` here
 * would put backend prose, stack fragments and internal identifiers on screen.
 */
export function reportFatalError(error: unknown, context: string): string {
  reference.value = newReference();
  failed.value = true;

  // The deliberate destination for the technical detail. `no-console` is an error
  // in this repo precisely because `console.error` was standing in for real error
  // handling elsewhere; here it *is* the error handling — this is the last place a
  // maintainer can see what actually threw, and it is paired with a user-facing
  // fallback rather than replacing one.
  // eslint-disable-next-line no-console
  console.error(`[whento:${reference.value}] ${context}`, error);

  return reference.value;
}

/** Leave the fallback screen. Called when the user navigates or retries. */
export function clearFatalError(): void {
  failed.value = false;
  reference.value = '';
}

export interface AppErrorState {
  /** Whether the fallback screen should be shown instead of the app. */
  readonly failed: DeepReadonly<Ref<boolean>>;
  /** Reference for the last fatal error, or '' when there is none. */
  readonly reference: DeepReadonly<Ref<string>>;
  reportFatalError: typeof reportFatalError;
  clearFatalError: typeof clearFatalError;
}

export function useAppError(): AppErrorState {
  return {
    failed: readonly(failed),
    reference: readonly(reference),
    reportFatalError,
    clearFatalError,
  };
}
