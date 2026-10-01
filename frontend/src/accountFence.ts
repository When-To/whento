/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Origin-tab-wide counter for a "session / account boundary" fence.
 *
 * Bumped every time the signed-in session is reset (local or remote logout) or a new
 * session is established (login, registration, bootstrap, password reset, MFA/passkey,
 * /auth/me hydration). An account-scoped async action captures the current value when
 * it starts and only commits once that value is still current — a stable user id cannot
 * tell "same account, same session" from "same account, replacement session", so a
 * delayed response from an old session must not overwrite newer state loaded by the
 * replacement.
 *
 * Deliberately dependency-free (like `sessionEvents.ts`) so the auth store can import
 * it without a cycle.
 */
let accountGeneration = 0;

/** The current account-boundary value. */
export function currentAccountGeneration(): number {
  return accountGeneration;
}

/** Advance the account-boundary fence and return the new value. */
export function bumpAccountGeneration(): number {
  accountGeneration += 1;
  return accountGeneration;
}
