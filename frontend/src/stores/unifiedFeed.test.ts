/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 *
 * The regressions this file guards: the unified-feed config is account-scoped (its
 * `ics_token` is a bearer-like capability URL), but a deliberate logout never cleared
 * it, and every config request committed its response unconditionally. A request
 * started by account A could therefore repopulate the cleared slot after A logged out
 * and B signed in. `clearConfig()` bumps an epoch and the commits are bound to the
 * epoch and user that started them.
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import type { UnifiedFeedConfig, User } from '@/types';

const authApi = {
  login: vi.fn(),
  register: vi.fn(),
  bootstrap: vi.fn(),
  bootstrapStatus: vi.fn(),
  getMe: vi.fn(),
  updateProfile: vi.fn(),
  updatePassword: vi.fn(),
  logout: vi.fn(),
  forgotPassword: vi.fn(),
  resetPassword: vi.fn(),
  checkMagicLinkAvailable: vi.fn(),
};

const apiClient = {
  setToken: vi.fn(),
  clearToken: vi.fn(),
  hasSession: vi.fn(() => false),
  signOut: vi.fn(),
  logoutThroughLock: vi.fn((run: () => unknown) => run()),
};

const calendarsApi = {
  getAll: vi.fn(),
};

const unifiedFeedApi = {
  getConfig: vi.fn(),
  create: vi.fn(),
  updateCalendars: vi.fn(),
  regenerateToken: vi.fn(),
};

vi.mock('@/api/auth', () => ({ authApi }));
vi.mock('@/api/client', () => ({ apiClient }));
vi.mock('@/api/calendars', () => ({ calendarsApi }));
vi.mock('@/api/unifiedFeed', () => ({ unifiedFeedApi }));

const { useAuthStore } = await import('./auth');
const { useUnifiedFeedStore } = await import('./unifiedFeed');

/** A promise with its resolvers exposed, so a test can control when it settles. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function user(id: string): User {
  return { id, email: `${id}@x.test`, display_name: id, role: 'user' } as User;
}

function config(id: string): UnifiedFeedConfig {
  return { configured: true, ics_token: `token-${id}`, included_calendar_ids: [] } as never;
}

function feedConfig(ids: string[]): UnifiedFeedConfig {
  return { configured: true, ics_token: 't', included_calendar_ids: ids } as never;
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  setActivePinia(createPinia());
  authApi.logout.mockResolvedValue(undefined);
});

describe('unified feed store', () => {
  it('commits the fetched config for the signed-in user', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    unifiedFeedApi.getConfig.mockResolvedValue(config('a'));
    const store = useUnifiedFeedStore();

    await store.fetchConfig();

    expect(store.config?.ics_token).toBe('token-a');
  });

  it('discards a config response once the account has been cleared and another signs in', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const getConfig = deferred<UnifiedFeedConfig>();
    unifiedFeedApi.getConfig.mockReturnValue(getConfig.promise);
    const store = useUnifiedFeedStore();

    // Account A starts a config read; the response is still in flight.
    const pending = store.fetchConfig();

    // A signs out (clearing the feed) and B signs in before it settles.
    store.clearConfig();
    authStore.user = user('u-b');

    getConfig.resolve(config('a'));

    await pending;

    // A's (token-bearing) config must not come back.
    expect(store.config).toBeNull();
  });

  it('discards a config response even when the same account signs straight back in', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const getConfig = deferred<UnifiedFeedConfig>();
    unifiedFeedApi.getConfig.mockReturnValue(getConfig.promise);
    const store = useUnifiedFeedStore();

    const pending = store.fetchConfig();

    // A logs out and back in. The user id is unchanged, so only the epoch bumped by
    // clearConfig can tell the stale response apart from a fresh one.
    store.clearConfig();
    authStore.user = user('u-a');

    getConfig.resolve(config('a'));
    await pending;

    expect(store.config).toBeNull();
  });

  it('clears the stored feed when the user logs out in the same tab', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = config('a');

    // A deliberate logout routes through the same account-scoped reset as a remote
    // logout, so the same-tab clear is not skipped because BroadcastChannel never
    // echoes the logout back to the tab that posted it.
    await authStore.logout();

    expect(store.config).toBeNull();
    expect(authStore.user).toBeNull();
  });

  it('does not let a late createFeed response repopulate the cleared feed', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const create = deferred<UnifiedFeedConfig>();
    unifiedFeedApi.create.mockReturnValue(create.promise);
    const store = useUnifiedFeedStore();

    const pending = store.createFeed();
    store.clearConfig();

    create.resolve(config('a'));
    await pending;

    expect(store.config).toBeNull();
  });

  it('does not lose a rapid second toggle while the first write is in flight', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = feedConfig([]);

    const first = deferred<void>();
    const second = deferred<void>();
    unifiedFeedApi.updateCalendars
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const toggleA = store.toggleCalendar('a');
    const toggleB = store.toggleCalendar('b');

    // Optimistic: the second toggle built on the first's intended set, so both
    // selections are already reflected before any write settles.
    expect(store.config?.included_calendar_ids).toEqual(['a', 'b']);

    first.resolve();
    await toggleA;
    // The PATCHes are serialised: [a] then [a, b] — never two reads of [a] → [a] and
    // [b], which would make the backend replace one by the other.
    expect(unifiedFeedApi.updateCalendars).toHaveBeenNthCalledWith(1, ['a']);
    expect(unifiedFeedApi.updateCalendars).toHaveBeenNthCalledWith(2, ['a', 'b']);
    // The first write's response must not revert the optimistic selection.
    expect(store.config?.included_calendar_ids).toEqual(['a', 'b']);

    second.resolve();
    await toggleB;
    expect(store.config?.included_calendar_ids).toEqual(['a', 'b']);
  });

  it('rolls back an optimistic toggle when the latest write fails', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = feedConfig([]);

    unifiedFeedApi.updateCalendars.mockRejectedValueOnce(new Error('offline'));

    await expect(store.toggleCalendar('a')).rejects.toThrow();

    // The server refused the write, so the checkbox must not keep claiming the
    // optimistic selection it never accepted.
    expect(store.config?.included_calendar_ids).toEqual([]);
    expect(store.error).not.toBeNull();
  });

  it('does not let a superseded failure leave its error authoritative', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = feedConfig([]);

    const first = deferred<void>();
    unifiedFeedApi.updateCalendars
      .mockReturnValueOnce(first.promise) // [a]
      .mockResolvedValueOnce(undefined); // [a, b]

    const toggleA = store.toggleCalendar('a');
    const toggleB = store.toggleCalendar('b');
    expect(store.config?.included_calendar_ids).toEqual(['a', 'b']);

    // The now-superseded first write fails; the latest ([a, b]) is still queued
    // behind it and must decide.
    first.reject(new Error('offline'));
    await toggleA;
    await toggleB;

    expect(store.config?.included_calendar_ids).toEqual(['a', 'b']);
    expect(store.error).toBeNull();
  });

  it('does not fire a queued write after the account has been reset', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = feedConfig(['a', 'b']);

    const first = deferred<void>();
    unifiedFeedApi.updateCalendars.mockReturnValueOnce(first.promise);

    // A checks, then unchecks both: the second replacement is an empty set, which is
    // valid server-side and would clear the next account's feed if it ever fired.
    const toggleA = store.toggleCalendar('a'); // queued: [b]
    const toggleB = store.toggleCalendar('b'); // queued: []
    expect(store.config?.included_calendar_ids).toEqual([]);

    // A signs out and B signs in while the first write is still in flight.
    store.clearConfig();
    authStore.user = user('u-b');

    first.resolve();
    await toggleA;
    // The empty replacement queued under A must never fire under B's token.
    expect(unifiedFeedApi.updateCalendars).toHaveBeenCalledTimes(1);

    await toggleB;
    expect(unifiedFeedApi.updateCalendars).toHaveBeenCalledTimes(1);
  });

  it('rolls back a pair of failures to the last server-confirmed selection, not an optimistic intermediate', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = feedConfig([]);

    const first = deferred<void>();
    const second = deferred<void>();
    unifiedFeedApi.updateCalendars
      .mockReturnValueOnce(first.promise) // [a]
      .mockReturnValueOnce(second.promise); // [a, b]

    const toggleA = store.toggleCalendar('a');
    const toggleB = store.toggleCalendar('b');
    expect(store.config?.included_calendar_ids).toEqual(['a', 'b']);

    // Both writes fail. The older one is superseded and swallowed; the latest must
    // roll back to what the server actually has (the initial empty set) — not to the
    // optimistic [a] that no write ever accepted.
    first.reject(new Error('offline'));
    await toggleA;
    second.reject(new Error('offline'));
    await expect(toggleB).rejects.toThrow();

    expect(store.config?.included_calendar_ids).toEqual([]);
    expect(store.error).not.toBeNull();
  });

  it('rolls back a failed newest write to the last confirmed success across three writes', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = feedConfig([]);

    const first = deferred<void>(); // [a]
    const third = deferred<void>(); // [a, b, c]
    unifiedFeedApi.updateCalendars
      .mockReturnValueOnce(first.promise) // [a] — fails
      .mockResolvedValueOnce(undefined) // [a, b] — succeeds
      .mockReturnValueOnce(third.promise); // [a, b, c] — fails

    const toggleA = store.toggleCalendar('a');
    const toggleB = store.toggleCalendar('b');
    const toggleC = store.toggleCalendar('c');
    expect(store.config?.included_calendar_ids).toEqual(['a', 'b', 'c']);

    first.reject(new Error('offline'));
    await toggleA;
    await toggleB;
    // The middle write confirmed [a, b] server-side, but the UI keeps the newest
    // optimistic [a, b, c] until it settles.
    expect(store.config?.included_calendar_ids).toEqual(['a', 'b', 'c']);
    expect(store.error).toBeNull();

    third.reject(new Error('offline'));
    await expect(toggleC).rejects.toThrow();
    // Rolled back to what the server confirmed, not to an optimistic intermediate.
    expect(store.config?.included_calendar_ids).toEqual(['a', 'b']);
    expect(store.error).not.toBeNull();
  });

  it('does not let a delayed config read overwrite a selection written after it started', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();

    const getConfig = deferred<UnifiedFeedConfig>();
    unifiedFeedApi.getConfig.mockReturnValue(getConfig.promise);
    const pendingRead = store.fetchConfig();

    // A toggle lands, and its write reaches the server, while the GET is still up.
    unifiedFeedApi.updateCalendars.mockResolvedValue(undefined);
    store.config = feedConfig([]);
    await store.toggleCalendar('a');
    expect(store.config?.included_calendar_ids).toEqual(['a']);

    // The earlier GET finally answers with the pre-toggle state: it must not restore it.
    getConfig.resolve(config('a'));
    await pendingRead;
    expect(store.config?.included_calendar_ids).toEqual(['a']);
  });

  it('lets a selection write supersede a config read that landed first', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = feedConfig([]);

    const write = deferred<void>();
    unifiedFeedApi.updateCalendars.mockReturnValueOnce(write.promise);
    const toggle = store.toggleCalendar('a');

    // A read starts after the write and lands before it, with the pre-write server
    // state. It commits (it is the newest operation), but the write's reconciliation —
    // the user's intent — must win.
    const getConfig = deferred<UnifiedFeedConfig>();
    unifiedFeedApi.getConfig.mockReturnValue(getConfig.promise);
    const pendingRead = store.fetchConfig();
    getConfig.resolve(config('a'));
    await pendingRead;
    expect(store.config?.included_calendar_ids).toEqual([]);

    write.resolve();
    await toggle;
    expect(store.config?.included_calendar_ids).toEqual(['a']);
  });

  it('does not let a delayed config read resurrect a dead ICS token after regeneration', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = config('a'); // token-a is on screen and about to be replaced

    const getConfig = deferred<UnifiedFeedConfig>();
    unifiedFeedApi.getConfig.mockReturnValue(getConfig.promise);
    const pendingRead = store.fetchConfig();

    // Regeneration succeeds and invalidates token-a while the GET is still in flight.
    unifiedFeedApi.regenerateToken.mockResolvedValue({ ics_token: 'token-new' });
    await store.regenerateToken();
    expect(store.config?.ics_token).toBe('token-new');

    // The earlier read lands with the now-dead token: it must not come back.
    getConfig.resolve(config('a'));
    await pendingRead;
    expect(store.config?.ics_token).toBe('token-new');
  });

  it('does not let a read started during regeneration restore the pre-rotation token', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = config('a');

    const regenerate = deferred<{ ics_token: string }>();
    unifiedFeedApi.regenerateToken.mockReturnValue(regenerate.promise);
    const pendingRegen = store.regenerateToken();

    const getConfig = deferred<UnifiedFeedConfig>();
    unifiedFeedApi.getConfig.mockReturnValue(getConfig.promise);
    const pendingRead = store.fetchConfig();

    regenerate.resolve({ ics_token: 'token-new' });
    await pendingRegen;
    expect(store.config?.ics_token).toBe('token-new');

    getConfig.resolve(config('a'));
    await pendingRead;
    expect(store.config?.ics_token).toBe('token-new');
  });

  it('keeps a regenerated token even when a read lands during it', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = config('a');

    const regenerate = deferred<{ ics_token: string }>();
    unifiedFeedApi.regenerateToken.mockReturnValue(regenerate.promise);
    const pendingRegen = store.regenerateToken();

    // A read, started after the regeneration, answers the old token while the
    // regeneration is still pending.
    const getConfig = deferred<UnifiedFeedConfig>();
    unifiedFeedApi.getConfig.mockReturnValue(getConfig.promise);
    const pendingRead = store.fetchConfig();
    getConfig.resolve(config('a'));
    await pendingRead;
    expect(store.config?.ics_token).toBe('token-a');

    // The regeneration is the freshest capability: it must still win.
    regenerate.resolve({ ics_token: 'token-new' });
    await pendingRegen;
    expect(store.config?.ics_token).toBe('token-new');
  });

  it("does not use a late old-account feed success as the new account's rollback baseline", async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = feedConfig(['a']);

    // A toggles another calendar; its PATCH stays on the wire.
    const aWrite = deferred<void>();
    unifiedFeedApi.updateCalendars.mockReturnValueOnce(aWrite.promise);
    const toggleA = store.toggleCalendar('b');

    // A signs out and B signs in, loading B's own feed while A's PATCH is still up.
    store.clearConfig();
    authStore.user = user('u-b');
    store.config = feedConfig(['x']);

    // B's write also stays on the wire for now.
    const bWrite = deferred<void>();
    unifiedFeedApi.updateCalendars.mockReturnValueOnce(bWrite.promise);
    const toggleB = store.toggleCalendar('y');

    // A's old PATCH succeeds late — after the store already belongs to B.
    aWrite.resolve();
    await toggleA;

    // B's write then fails: its rollback must land on B's confirmed selection, not on
    // the ghost of A's late success.
    bWrite.reject(new Error('offline'));
    await expect(toggleB).rejects.toThrow();

    expect(store.config?.included_calendar_ids).toEqual(['x']);
  });

  it('shares a single in-flight regeneration so two rotations cannot leave a dead token', async () => {
    const authStore = useAuthStore();
    authStore.user = user('u-a');
    const store = useUnifiedFeedStore();
    store.config = config('a');

    const first = deferred<{ ics_token: string }>();
    unifiedFeedApi.regenerateToken.mockReturnValue(first.promise);

    // A double activation while the first rotation is in flight shares it: no second
    // rotation is issued, so the store's token can never be the one a later backend
    // commit invalidated.
    const firstCall = store.regenerateToken();
    const secondCall = store.regenerateToken();
    expect(unifiedFeedApi.regenerateToken).toHaveBeenCalledTimes(1);

    first.resolve({ ics_token: 'token-new' });
    await firstCall;
    await secondCall;
    expect(store.config?.ics_token).toBe('token-new');
  });
});
