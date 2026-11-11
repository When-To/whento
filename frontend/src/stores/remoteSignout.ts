/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { useAuthStore } from './auth';
import { useCalendarStore } from './calendar';
import { useUnifiedFeedStore } from './unifiedFeed';
import { REMOTE_SIGNOUT_EVENT, REMOTE_SESSION_EVENT } from '@/sessionEvents';

/**
 * Drop everything that belonged to the signed-in account.
 *
 * Triggered when *another* tab reports a sign-out. The API client cannot do this
 * itself — resetting `stores/auth` from `api/client` would form an import cycle —
 * so it raises a window event and this module, wired up from `main.ts`, performs the
 * reset. This is what stops a tab sitting on a public calendar link from keeping the
 * previous account's user, owner list and pre-rendered participant ids after a logout
 * that happened in a different tab.
 */
export function resetAccountScopedState() {
  useAuthStore().resetSession();
  useCalendarStore().clearCalendars();
  useUnifiedFeedStore().clearConfig();
}

/**
 * Subscribe the account-scoped reset to the cross-tab sign-out event.
 *
 * Returns an unsubscribe function for tests and hot reloads. In the running
 * application the listener lives for the life of the page.
 */
export function registerRemoteSignoutListener(): () => void {
  if (typeof window === 'undefined') return () => undefined;
  const handler = () => resetAccountScopedState();
  window.addEventListener(REMOTE_SIGNOUT_EVENT, handler);
  return () => window.removeEventListener(REMOTE_SIGNOUT_EVENT, handler);
}

/**
 * Bring a tab that just accepted another tab's fresh login into agreement with it.
 *
 * The HTTP client installed the new access token but pinia still describes the
 * previous (or no) account. Drop the old account-scoped state, then hydrate /auth/me
 * so the UI and router guard reflect the account whose token outgoing requests now
 * carry.
 *
 * The accepted server family travels with the event and fences the round trip: `/auth/me`
 * commits — and on failure clears the token — only while that family is still the accepted
 * one, so a slower response for an older session cannot overwrite (or evict) a newer one.
 *
 * Deliberately **no document reload** (the fork's approach caused reload loops on
 * session-restored broadcasts, audit B5/B5q). Dropping the owner-scoped stores makes
 * the change visible immediately — public/participant pages lose the previous owner's
 * cached data on the spot — and the account-generation bump the reset performs is what
 * account-scoped route loaders (dashboard calendars/feed, calendar settings, admin
 * lists) watch to clear and reload only their own data once the new identity confirms.
 * Protected-route auth checks are unchanged.
 */
export async function handleRemoteSessionRestored(family: string | null): Promise<void> {
  resetAccountScopedState();
  await useAuthStore()
    .fetchUser(family)
    .catch(() => {});
}

/**
 * Subscribe the account hydration to the cross-tab fresh-session event.
 *
 * Returns an unsubscribe function for tests and hot reloads. In the running
 * application the listener lives for the life of the page.
 */
export function registerRemoteSessionListener(): () => void {
  if (typeof window === 'undefined') return () => undefined;
  const handler = (event: Event) => {
    const detail = (event as CustomEvent<{ family?: string | null }>).detail;
    void handleRemoteSessionRestored(detail?.family ?? null);
  };
  window.addEventListener(REMOTE_SESSION_EVENT, handler);
  return () => window.removeEventListener(REMOTE_SESSION_EVENT, handler);
}
