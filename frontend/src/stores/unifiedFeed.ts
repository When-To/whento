/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref } from 'vue';
import { unifiedFeedApi } from '@/api/unifiedFeed';
import { useAsyncActions } from '@/stores/asyncAction';
import { useAuthStore } from '@/stores/auth';
import type { UnifiedFeedConfig } from '@/types';

export const useUnifiedFeedStore = defineStore('unifiedFeed', () => {
  const config = ref<UnifiedFeedConfig | null>(null);
  const { loading, error, run, clearError } = useAsyncActions();

  /**
   * The config generation. `clearConfig()` bumps it so that any unified-feed request
   * still in flight when the account is cleared — a logout, an expiry, a cross-tab
   * sign-out — cannot commit its (possibly account-A, token-bearing `ics_token`)
   * response afterwards. Every asynchronous commit below is bound to the generation
   * and the signed-in user that started it, and is discarded when either changes.
   */
  let configGeneration = 0;

  /**
   * The monotonic operation sequence for reads. Every config operation advances it — a
   * fetch, a create, a selection write, a token regeneration. A read may only commit
   * its (possibly stale) response when no later operation has become authoritative: a
   * slow GET that started before a toggle or a regeneration must not overwrite the
   * newer result when it finally lands.
   */
  let configOpSeq = 0;

  /**
   * The newest queued selection write. A response may only reconcile the config when
   * it belongs to the latest write: an older response must not revert the optimistic
   * selection a later toggle already built on, and a superseded write must not surface
   * its failure as authoritative.
   */
  let selectionSeq = 0;

  /**
   * The newest token-creating operation. Creating the feed or regenerating the ICS
   * token invalidates any *older* write's token result, but a later plain read must
   * not: reads only observe server state, and the freshest regenerate is always more
   * current than anything a read can return, so it keeps its commit even when an
   * overlapping read lands first.
   */
  let tokenOpSeq = 0;
  /**
   * Bumped when a new ICS token is actually committed, not when regeneration starts.
   * A GET that began during regeneration captures the old value and must not write
   * its pre-rotation token over the one regeneration has since stored.
   */
  let tokenCommitSeq = 0;

  /**
   * The last selection the server has actually accepted. A successful serialised PATCH
   * advances it, and `fetchConfig`/`createFeed` initialise it from the fresh server
   * value. When the latest write fails, the UI rolls back here — never to an
   * optimistic intermediate the server may also have refused — so the checkboxes cannot
   * end up claiming a selection no write ever confirmed. `null` before anything is
   * confirmed; the first edit then seeds it from the pre-toggle value (the loaded
   * server state).
   */
  let confirmedSelection: string[] | null = null;

  /**
   * The serialised selection-write queue tail. The backend endpoint *replaces* the
   * complete set, so two rapid toggles reading the same committed array and both
   * sending their own replacement would lose one selection. Queuing the PATCHes — in
   * the order the toggles happened, with each in the chain building on the optimistic
   * set below — keeps only the latest desired selection authoritative.
   *
   * The idle marker is a Symbol, not `Promise.resolve()`: queueing onto a resolved
   * promise would defer the first write by a microtask, letting a same-tick account
   * reset void a write the user had already made. Idle makes the first write start
   * synchronously, so it is genuinely on the wire before any reset arrives.
   */
  const IDLE = Symbol('selection-write-idle');
  let selectionWrite: Promise<void> | typeof IDLE = IDLE;

  /**
   * Drop the stored unified-feed config, and invalidate every in-flight request.
   *
   * The config is account-scoped (its `ics_token` is a bearer-like capability URL),
   * so every account reset has to go through here — deliberate same-tab logout,
   * remote cross-tab logout and forced expiry all end up in this action. It must
   * bump the generation, not just null the config: a fetch started by the previous
   * account must not repopulate the cleared slot.
   */
  function clearConfig() {
    configGeneration += 1;
    configOpSeq += 1;
    selectionSeq += 1;
    tokenOpSeq += 1;
    confirmedSelection = null;
    // Detach the queue so no node queued by the previous account can fire under the
    // next one (each node also re-checks the epoch before calling the API).
    selectionWrite = IDLE;
    config.value = null;
    clearError();
  }

  async function fetchConfig() {
    return run('unifiedFeed.fetchError', async () => {
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = configGeneration;
      const opSeq = ++configOpSeq;
      const tokenCommit = tokenCommitSeq;
      const result = await unifiedFeedApi.getConfig();
      if (generation !== configGeneration) return;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
      // A stale read must not overwrite a newer result: reject the commit when any
      // later operation (a toggle, a regeneration, another fetch) became authoritative
      // while this GET was in flight.
      if (opSeq !== configOpSeq) return;
      // Regeneration may have committed a new token after this GET was issued. Keep
      // that token; the rest of the row is still this read's.
      if (tokenCommit !== tokenCommitSeq && config.value) {
        config.value = { ...result, ics_token: config.value.ics_token };
      } else {
        config.value = result;
      }
      confirmedSelection = result.included_calendar_ids ?? [];
    });
  }

  async function createFeed() {
    return run('unifiedFeed.createError', async () => {
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = configGeneration;
      const opSeq = ++configOpSeq;
      // A create hands out a brand-new ICS token: the freshest capability on record.
      tokenOpSeq += 1;
      const created = await unifiedFeedApi.create();
      if (generation !== configGeneration) return;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
      if (opSeq !== configOpSeq) return;
      config.value = created;
      tokenCommitSeq += 1;
      confirmedSelection = created.included_calendar_ids ?? [];
    });
  }

  /**
   * Persist a complete calendar selection for the unified feed.
   *
   * Writes are serialised through `selectionWrite`. Each queued node re-checks the
   * account epoch and initiating user *before* the API call is made — a write queued
   * by account A must never fire under account B's token, and a reset renders every
   * node from the previous account a no-op. Only the latest desired set may reconcile
   * the config afterwards.
   *
   * @param base what the selection was before this toggle's optimistic edit, so a
   *             failed latest write can roll the checkboxes back to it.
   */
  async function updateCalendars(calendarIds: string[], base: string[] = calendarIds) {
    return run('unifiedFeed.updateError', async () => {
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = configGeneration;
      const selSeq = ++selectionSeq;
      // A selection write is also a config operation: bumping the read sequence here
      // means any read started before this write is stale the moment the write is
      // issued, even while the PATCH is still queued or in flight.
      configOpSeq += 1;
      // The PATCH body. Each node re-checks the account epoch and initiating user
      // *before* the API call is made — a write queued by account A must never fire
      // under account B's token, and a reset renders every node from the previous
      // account a no-op.
      const writeBody = async () => {
        if (generation !== configGeneration) return;
        if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
        try {
          await unifiedFeedApi.updateCalendars(calendarIds);
          // The server accepted this replacement — but it belongs to this account and
          // this epoch. An account reset while the PATCH was on the wire must not let a
          // late account-A success overwrite the rollback baseline of the account (B)
          // that has since signed in and loaded: B's next failure would otherwise roll
          // back to A's calendar ids.
          if (generation !== configGeneration) return;
          if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
          // This is now the last confirmed state — even if a newer write has since
          // superseded it in the UI, this is what a failed newer write must fall back
          // to.
          confirmedSelection = [...calendarIds];
        } catch (err) {
          // If a newer write superseded this one mid-flight, its failure is not
          // authoritative — the queued latest write still decides — so neither roll
          // back nor surface it as an error.
          if (selSeq !== selectionSeq) return;
          // The latest write failed. Roll back to the last state the server confirmed
          // (falling back to the pre-edit value only on a store that never confirmed
          // anything), not to an optimistic intermediate it may also have refused, so
          // the UI never claims a selection no write accepted.
          const fallback = confirmedSelection ?? base;
          if (config.value) {
            config.value = { ...config.value, included_calendar_ids: fallback };
          }
          throw err;
        }
      };
      // Serialise: a toggle that fires while a write is in flight waits its turn (in
      // the order it happened) instead of racing the same base set. When the queue is
      // idle, though, start the write immediately — a same-account supersession later
      // in the same tick is handled by the catch's selSeq check, and an account reset
      // must not void a write that queued this tick.
      const write = selectionWrite === IDLE ? writeBody() : selectionWrite.then(writeBody);
      selectionWrite = write.catch(() => undefined);
      await write;
      if (generation !== configGeneration) return;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
      // Only the latest desired set may reconcile the config: an older write's
      // response must not revert the optimistic selection a later toggle built on.
      if (selSeq !== selectionSeq) return;
      if (config.value) {
        config.value.included_calendar_ids = calendarIds;
      }
    });
  }

  async function toggleCalendar(calendarId: string) {
    if (!config.value) return;

    // Optimistically reflect the toggle before the write lands, so a second rapid
    // toggle builds its full replacement on the intended set (not the committed one),
    // and the checkbox matches the user's intent immediately.
    const current = config.value.included_calendar_ids || [];
    // First edit after a load: the pre-toggle value is what the server just told us,
    // so it is the rollback baseline until a PATCH confirms a newer state.
    if (confirmedSelection === null) {
      confirmedSelection = [...current];
    }
    const newIds = current.includes(calendarId)
      ? current.filter(id => id !== calendarId)
      : [...current, calendarId];
    config.value = { ...config.value, included_calendar_ids: newIds };

    await updateCalendars(newIds, current);
  }

  /**
   * The in-flight regeneration for the current account generation, if any.
   *
   * Rotating the ICS token twice concurrently is not safe to resolve by client
   * invocation order: the backend serialises commits, so the *last* rotation to commit
   * server-side is the live one, and either invocation could be that one. Sharing a
   * single in-flight rotation per account (rather than firing two) keeps the store's
   * token equal to the server's final commit.
   */
  let regeneration: { generation: number; promise: Promise<unknown> } | null = null;

  async function regenerateToken() {
    const generation = configGeneration;
    if (regeneration !== null && regeneration.generation === generation) {
      return regeneration.promise;
    }

    const promise = run('unifiedFeed.regenerateError', async () => {
      const requestingUserId = useAuthStore().user?.id ?? null;
      // Regeneration is also a config operation: a read started before it must discard.
      configOpSeq += 1;
      const tokenSeq = ++tokenOpSeq;
      const { ics_token } = await unifiedFeedApi.regenerateToken();
      if (generation !== configGeneration) return;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
      // A read may not downgrade a fresher regenerate, so the token commit is gated on
      // the token revision (a create or a reset supersede it) — not on the read
      // sequence and not on client invocation order.
      if (tokenSeq !== tokenOpSeq) return;
      if (config.value) {
        config.value.ics_token = ics_token;
        tokenCommitSeq += 1;
      }
    });

    regeneration = { generation, promise };
    try {
      return await promise;
    } finally {
      if (regeneration?.generation === generation) {
        regeneration = null;
      }
    }
  }

  function isCalendarIncluded(calendarId: string): boolean {
    return config.value?.included_calendar_ids?.includes(calendarId) ?? false;
  }

  return {
    config,
    loading,
    error,
    fetchConfig,
    createFeed,
    updateCalendars,
    toggleCalendar,
    regenerateToken,
    isCalendarIncluded,
    /** Drops every account-scoped feed state from memory (used on account reset). */
    clearConfig,
    clearError,
  };
});
