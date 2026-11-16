/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { ref, type Ref } from 'vue';

/**
 * Local account generation — the third of the PR-7 session concepts.
 *
 * The three concepts that describe a browser session are deliberately kept apart:
 *
 *  1. The **server family/session id** (`session_id` on every auth response). This is
 *     the identity of a refreshable login session: stable across refresh, brand-new on
 *     a new login. The API client holds it in memory and shares it over the channel;
 *     it is what tells "a refresh of the session I have" from "a new login".
 *  2. The **access token + absolute expiry**. Short-lived credential state, in memory
 *     only, replaced whenever a newer token for the same family arrives.
 *  3. The **local account generation**, this module's counter: an in-memory fence that
 *     invalidates old async work after a logout or an account replacement.
 *
 * Both the first and the third stay *unchanged* on a same-family token rotation — a
 * refresh of the session this tab already holds is not a new login, must not clear the
 * account stores, must not re-read /auth/me, and must not bump the fence.
 *
 * A genuinely different finalized family, or an explicit logout, advances the fence:
 * account-scoped async actions capture the current value when they start and only
 * commit once that value is still current. A stable user id cannot tell "same account,
 * same session" from "same account, replacement session", so a delayed response from
 * an old session must not overwrite newer state loaded by the replacement.
 *
 * This module deliberately imports nothing of the project (only Vue's reactivity, so
 * route loaders can react to a generation change) — stores import it without a cycle.
 */
let accountGeneration = 0;

/** Reactive mirror of the counter, so views and account loaders can react to a change. */
const accountGenerationRef: Ref<number> = ref(0);

/** The current account-boundary value. */
export function currentAccountGeneration(): number {
  return accountGeneration;
}

/** Advance the account-boundary fence and return the new value. */
export function bumpAccountGeneration(): number {
  accountGeneration += 1;
  accountGenerationRef.value = accountGeneration;
  return accountGeneration;
}

/**
 * Reactive access to the account generation for route loaders.
 *
 * A loader that watches this ref re-runs whenever the fence advances — after a
 * logout-then-login, a remote account replacement, or a refresh that discovered a
 * different server family — so it can clear and reload only its own account data
 * instead of the whole document (the reload-loop behaviour this PR deliberately does
 * not reproduce).
 */
export function useAccountGeneration(): Ref<number> {
  return accountGenerationRef;
}
