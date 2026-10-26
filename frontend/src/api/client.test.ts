/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 *
 * The PR-7 client suite. The fork it derives from serialised refreshes with epoch
 * watermarks and a lease-based Web-Locks fallback; this branch deliberately does not
 * reproduce either (audit B5/B5q: the epoch model caused reload loops). Instead the
 * three session concepts are held apart:
 *
 *  1. server family/session id — identity of a refreshable login (stable across refresh,
 *     new on a new login), shared over the channel and stored only as non-secret
 *     metadata;
 *  2. access token + absolute expiry — in memory only, never in storage;
 *  3. local account generation — an in-memory fence advanced on logout/replacement.
 *
 * Without Web Locks the client serialises in-process only; cross-tab Set-Cookie
 * ordering is simply not guaranteed, the server's family + refresh-grace protection is
 * the authority, and a late response that conflicts with a finalized family is dropped
 * (this is asserted below, including the PR-163 seed-163 no-Web-Locks logout race).
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AxiosAdapter, AxiosInstance, AxiosRequestConfig } from 'axios';

// The client imports the router at module scope, which would drag in every view and the
// auth store. It reads currentRoute.meta.public, .fullPath and .name, so a stub is
// enough — and it lets the tests choose the route the visitor is being thrown out of.
const routeMeta = { public: false as boolean };
const currentRoute = { fullPath: '/dashboard', name: 'dashboard' as string | undefined };
vi.mock('@/router', () => ({
  default: {
    currentRoute: {
      value: {
        get meta() {
          return routeMeta;
        },
        get fullPath() {
          return currentRoute.fullPath;
        },
        get name() {
          return currentRoute.name;
        },
      },
    },
  },
}));

const { apiClient, ApiClient } = await import('./client');
const { authApi } = await import('./auth');

/** Reach the private axios instance so a fake adapter can stand in for the network. */
function instance(): AxiosInstance {
  return (apiClient as unknown as { client: AxiosInstance }).client;
}

/** Reach a non-singleton client's private axios instance (multi-tab fencing tests). */
function instanceOf(client: typeof apiClient): AxiosInstance {
  return (client as unknown as { client: AxiosInstance }).client;
}

/** Reach a client's private BroadcastChannel message handler. */
function channelHandler(client: typeof apiClient): ((event: MessageEvent) => void) | undefined {
  const channel = (client as unknown as { channel: BroadcastChannel | null }).channel;
  return channel?.onmessage as unknown as ((event: MessageEvent) => void) | undefined;
}

interface Recorded {
  url: string;
  method: string;
  auth?: string;
}

interface Responder {
  (config: AxiosRequestConfig, callNumber: number): { status: number; data?: unknown };
}

/**
 * Install an adapter that records every request and answers from `responder`.
 *
 * This exercises the real interceptors — the retry, the refresh and the logout all run
 * as they do in the browser; only the socket is replaced.
 */
function withAdapter(responder: Responder): Recorded[] {
  const seen: Recorded[] = [];

  const adapter: AxiosAdapter = async config => {
    const url = config.url ?? '';
    const callNumber = seen.filter(r => r.url === url).length;

    seen.push({
      url,
      method: (config.method ?? 'get').toLowerCase(),
      auth: (config.headers as Record<string, string> | undefined)?.Authorization,
    });

    const { status, data } = responder(config, callNumber);
    const response = {
      data,
      status,
      statusText: String(status),
      headers: {},
      config: config as never,
    };

    if (status >= 200 && status < 300) return response as never;

    const error = new Error(`Request failed with status code ${status}`) as Error & {
      response?: unknown;
      config?: unknown;
      isAxiosError?: boolean;
    };
    error.response = response;
    error.config = config;
    error.isAxiosError = true;
    throw error;
  };

  instance().defaults.adapter = adapter;

  return seen;
}

const ok = (data: unknown) => ({ status: 200, data: { success: true, data } });
const unauthorized = () => ({
  status: 401,
  data: { success: false, error: { code: 'UNAUTHORIZED', message: 'nope' } },
});

let live: Array<{ dispose(): void }> = [];

/**
 * Drain broadcast deliveries a preceding test may have queued.
 *
 * A logout broadcast sent by an earlier test on the shared `whento.auth` channel is
 * delivered asynchronously (as a task). If one lands *inside* this test's await window,
 * the singleton's channel handler clears the very session we are about to assert on —
 * a test-isolation artifact, not a client defect. Consuming the queue at the start of
 * the test while no session exists keeps the assertions deterministic.
 */
async function drainChannel() {
  for (let i = 0; i < 25; i++) {
    await new Promise(resolve => setTimeout(resolve, 0));
  }
}

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

describe('apiClient', () => {
  beforeEach(async () => {
    live.forEach(c => c.dispose());
    live = [];
    // Node delivers BroadcastChannel posts as tasks, including posts whose sender
    // has since closed. Drain them before installing the next test's credentials.
    apiClient.clearToken();
    await drainChannel();
    localStorage.clear();
    apiClient.clearToken();
    // The singleton is one long-lived module instance across the whole suite; its
    // in-memory signed-out families must not leak from a test that signed a family out
    // into one that logs the same family in again.
    (apiClient as unknown as { signedOutFamilies: Set<string> }).signedOutFamilies.clear();
    routeMeta.public = false;
    currentRoute.fullPath = '/dashboard';
    currentRoute.name = 'dashboard';
    // jsdom refuses real navigation; forceLogout assigns to it.
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { href: '' },
    });
  });

  afterEach(() => {
    live.forEach(c => c.dispose());
    live = [];
    vi.restoreAllMocks();
    // restoreAllMocks does not undo stubGlobal, and a stubbed navigator carrying a
    // fake Web Lock would follow us into the next test.
    vi.unstubAllGlobals();
  });

  describe('tokens', () => {
    it.each([false, true])(
      'persists only non-secret recovery metadata through sign-out (MFA: %s)',
      async requireMfa => {
        const recoveryProof = 'r'.repeat(64);
        const newPassword = 'Recovery-only-password!42';
        const family = '4bafab71-cd3f-4926-a0b5-c119c234e371';
        const accessToken = 'recovery-access-token';
        const refreshToken = 'recovery-refresh-token';
        const tempToken = 'recovery-mfa-challenge';
        const writes = vi.spyOn(Storage.prototype, 'setItem');
        const response = requireMfa
          ? { require_mfa: true, temp_token: tempToken }
          : {
              access_token: accessToken,
              refresh_token: refreshToken,
              expires_in: 900,
              session_id: family,
              // Even an unexpected secret field echoed by a faulty server must
              // not be serialized along with the session metadata.
              new_password: newPassword,
            };
        withAdapter(config => {
          expect(config.url).toBe('/auth/reset-password');
          expect(JSON.parse(config.data as string)).toEqual({
            token: recoveryProof,
            new_password: newPassword,
          });
          return ok(response);
        });

        await expect(authApi.completeAccountRecovery(recoveryProof, newPassword)).resolves.toEqual(
          response
        );
        expect(apiClient.getFamilyId()).toBe(requireMfa ? null : family);
        expect(apiClient.hasSession()).toBe(!requireMfa);
        apiClient.signOut();

        // Inspect every write, not just the final snapshot: a transient write
        // followed by removeItem would still expose a password or token.
        expect(writes.mock.calls).toEqual(
          requireMfa
            ? []
            : [
                ['whento.activeFamily', family],
                ['whento.session', '1'],
                ['whento.loggedOutFamily', family],
              ]
        );
        for (const secret of [newPassword, recoveryProof, accessToken, refreshToken, tempToken]) {
          expect(writes.mock.calls.some(([, value]) => value.includes(secret))).toBe(false);
          expect(Object.values(localStorage)).not.toContain(secret);
          expect(Object.values(sessionStorage)).not.toContain(secret);
        }
      }
    );

    it('does not persist recovery data when the proof is rejected', async () => {
      const writes = vi.spyOn(Storage.prototype, 'setItem');
      withAdapter(() => unauthorized());

      await expect(
        authApi.completeAccountRecovery('rejected-proof', 'Unstored-password!42')
      ).rejects.toMatchObject({ code: 'UNAUTHORIZED' });
      expect(writes).not.toHaveBeenCalled();
      expect(apiClient.hasSession()).toBe(false);
    });

    it.each([
      '/auth/login',
      '/auth/register',
      '/auth/bootstrap',
      '/auth/mfa/verify',
      '/auth/passkey/login/finish',
      '/auth/magic-link/verify',
      '/auth/reset-password',
    ])('installs finalized %s credentials before releasing the cookie lock', async path => {
      const lock = vi.fn(async (_name: string, operation: () => Promise<unknown>) => {
        const result = await operation();
        expect(apiClient.getFamilyId()).toBe('family-finalized');
        return result;
      });
      vi.stubGlobal('navigator', { ...navigator, locks: { request: lock } });
      withAdapter(() =>
        ok({ access_token: 'finalized-token', expires_in: 900, session_id: 'family-finalized' })
      );
      await apiClient.post(path, {});
      expect(lock).toHaveBeenCalledTimes(1);
    });

    it('keeps the access token out of storage entirely', () => {
      apiClient.setToken('a-token');

      // The whole point of the change: a script that can read stored data must not
      // find the token there. Only the flag and the non-secret logout marker survive.
      expect(Object.values(localStorage)).not.toContain('a-token');
      expect(localStorage.getItem('whento.session')).toBe('1');
    });

    it('uses the in-memory token for requests', async () => {
      apiClient.setToken('a-token');

      const seen = withAdapter(() => ok({}));
      await apiClient.get('/anything');

      expect(seen[0].auth).toBe('Bearer a-token');
    });

    it('drops the token and the flag on clear', async () => {
      apiClient.setToken('a-token');
      apiClient.clearToken();

      expect(apiClient.hasSession()).toBe(false);
      expect(localStorage.getItem('whento.session')).toBeNull();

      const seen = withAdapter(() => ok({}));
      await apiClient.get('/anything');

      expect(seen[0].auth).toBeUndefined();
    });

    it('reports a session so a cold load knows to refresh', () => {
      expect(apiClient.hasSession()).toBe(false);

      apiClient.setToken('a-token');

      expect(apiClient.hasSession()).toBe(true);
    });

    it('sends no Authorization header when there is no token', async () => {
      const seen = withAdapter(() => ok({}));

      await apiClient.get('/anything');

      expect(seen[0].auth).toBeUndefined();
    });
  });

  describe('unwrapping', () => {
    it('returns response.data.data rather than the envelope', async () => {
      withAdapter(() => ok({ id: 7, name: 'Calendar' }));

      await expect(apiClient.get('/calendars/7')).resolves.toEqual({ id: 7, name: 'Calendar' });
    });

    it('unwraps for every verb', async () => {
      withAdapter(() => ok('payload'));

      await expect(apiClient.post('/x', {})).resolves.toBe('payload');
      await expect(apiClient.patch('/x', {})).resolves.toBe('payload');
      await expect(apiClient.delete('/x')).resolves.toBe('payload');
    });
  });

  describe('signing out across tabs', () => {
    it.each(['/auth/bootstrap', '/auth/bootstrap?source=setup'])(
      'preserves the session and original wrong-key error from %s',
      async endpoint => {
        apiClient.setToken('existing-session');
        const posted = vi.spyOn(BroadcastChannel.prototype, 'postMessage');
        const wrongKey = { code: 'UNAUTHORIZED', message: 'Invalid bootstrap key' };
        const seen = withAdapter(() => ({
          status: 401,
          data: { success: false, error: wrongKey },
        }));

        await expect(apiClient.post(endpoint, { boot_key: 'wrong-key' })).rejects.toBe(wrongKey);

        expect(seen.map(request => request.url)).toEqual([endpoint]);
        expect(apiClient.hasSession()).toBe(true);
        expect(posted).not.toHaveBeenCalled();
        expect(window.location.href).toBe('');
        const protectedRequests = withAdapter(() => ok({}));
        await apiClient.get('/protected');
        expect(protectedRequests[0].auth).toBe('Bearer existing-session');
      }
    );

    it('tells the other tabs when the user signs out', () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('a-token', undefined, 'family-a');

      apiClient.signOut();

      expect(apiClient.hasSession()).toBe(false);
      const logout = posted.find(m => (m as { type?: string })?.type === 'logout');
      expect(logout).toBeTruthy();
      // The logout names the family that died, so a delayed token for it — and a
      // delayed logout for any *other* family — is told apart from the current one.
      expect((logout as { family?: string | null }).family).toBe('family-a');
    });

    it('records the signed-out family in the shared logout marker', () => {
      apiClient.setToken('a-token', undefined, 'family-a');
      apiClient.signOut();

      expect(localStorage.getItem('whento.loggedOutFamily')).toBe('family-a');
    });

    it('stays quiet when a request merely fails', () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('a-token');
      posted.length = 0;

      // The error paths — a refused /auth/me, a failed restore — go through
      // clearToken. A transient failure in one tab must not sign the others out.
      apiClient.clearToken();

      expect(posted).toEqual([]);
    });

    it('clearing the token does not move the logout marker', () => {
      apiClient.setToken('a-token', undefined, 'family-a');
      apiClient.clearToken();

      expect(localStorage.getItem('whento.loggedOutFamily')).toBeNull();
    });

    it('on a public route, resets the account-scoped stores when another tab signs out', async () => {
      // A real browser: A's user and calendar list (capability ids included) live in
      // this tab's stores, and the BroadcastChannel reports that another tab signed
      // A out. The receiving side must drop the token *and* the stores, because on a
      // public calendar link there is no navigation to wipe them.
      const { createPinia, setActivePinia } = await import('pinia');
      const { useAuthStore } = await import('@/stores/auth');
      const { useCalendarStore } = await import('@/stores/calendar');
      const { registerRemoteSignoutListener } = await import('@/stores/remoteSignout');

      setActivePinia(createPinia());
      const authStore = useAuthStore();
      authStore.user = {
        id: 'u-a',
        email: 'a@example.test',
        display_name: 'A',
        role: 'user',
      } as never;
      const calendarStore = useCalendarStore();
      calendarStore.calendars = [{ id: 'c-1', name: 'A-owned', participants: [] }] as never;
      calendarStore.calendarsForUser = 'u-a';

      routeMeta.public = true;
      apiClient.setToken('a-token', undefined, 'family-a');

      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-signout', listener);
      const unsubscribe = registerRemoteSignoutListener();
      try {
        const onMessage = channelHandler(apiClient);
        onMessage?.({ data: { type: 'logout', family: 'family-a' } } as MessageEvent);

        expect(apiClient.hasSession()).toBe(false);
        expect(events).toContain('whento:remote-signout');
        // The previous account's state is gone, so a later visit cannot reuse it.
        expect(authStore.user).toBeNull();
        expect(calendarStore.calendars).toEqual([]);
        expect(calendarStore.calendarsForUser).toBeNull();
        // A public calendar link has no account to sign back in to: no navigation.
        expect(window.location.href).toBe('');
      } finally {
        window.removeEventListener('whento:remote-signout', listener);
        unsubscribe();
      }
    });

    it('on a public route, does not dispatch the app event when the message is a token', () => {
      routeMeta.public = true;
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-signout', listener);
      try {
        const onMessage = channelHandler(apiClient);
        onMessage?.({ data: { type: 'token', token: 'x', family: 'family-b' } } as MessageEvent);

        expect(events).toEqual([]);
      } finally {
        window.removeEventListener('whento:remote-signout', listener);
      }
    });
  });

  describe('cross-tab session restoration', () => {
    it('rejects delayed tokens after a local A-to-B replacement without logout', async () => {
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(() => {});
      const tab = new ApiClient();
      live.push(tab);
      tab.setToken('a-token', 900, 'family-a');
      tab.setToken('b-token', 900, 'family-b');
      channelHandler(tab)?.({
        data: {
          type: 'token',
          token: 'late-a',
          expiresAt: Date.now() + 900_000,
          family: 'family-a',
        },
      } as MessageEvent);

      expect(tab.getFamilyId()).toBe('family-b');
      const seen: string[] = [];
      instanceOf(tab).defaults.adapter = async config => {
        seen.push(String(config.headers.Authorization));
        return {
          data: { success: true, data: {} },
          status: 200,
          statusText: 'OK',
          headers: {},
          config,
        };
      };
      await tab.get('/auth/me');
      expect(seen).toEqual(['Bearer b-token']);
    });

    it('rejects a delayed old family in a cold tab that only observed the latest login', () => {
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(() => {});
      const sender = new ApiClient();
      const cold = new ApiClient();
      live.push(sender, cold);
      sender.setToken('b-token', 900, 'family-b');
      channelHandler(cold)?.({
        data: {
          type: 'token',
          token: 'late-a',
          expiresAt: Date.now() + 900_000,
          family: 'family-a',
        },
      } as MessageEvent);
      expect(cold.getFamilyId()).toBeNull();
      channelHandler(cold)?.({
        data: {
          type: 'token',
          token: 'b-token',
          expiresAt: Date.now() + 900_000,
          family: 'family-b',
        },
      } as MessageEvent);
      expect(cold.getFamilyId()).toBe('family-b');
    });

    it('fences the old account through storage when BroadcastChannel is unavailable', () => {
      vi.stubGlobal('BroadcastChannel', undefined);
      const tab = new ApiClient();
      live.push(tab);
      tab.setToken('a-token', 900, 'family-a');
      const listener = vi.fn();
      window.addEventListener('whento:remote-session', listener);
      try {
        localStorage.setItem('whento.activeFamily', 'family-b');
        window.dispatchEvent(
          new StorageEvent('storage', {
            key: 'whento.activeFamily',
            oldValue: 'family-a',
            newValue: 'family-b',
          })
        );
        expect(tab.getFamilyId()).toBe('family-b');
        expect((tab as unknown as { accessToken: string | null }).accessToken).toBeNull();
        expect(tab.hasSession()).toBe(true);
        expect(listener).toHaveBeenCalledTimes(1);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('retains the replacement fence in memory when storage is denied', () => {
      vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
        throw new Error('denied');
      });
      vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw new Error('denied');
      });
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(() => {});
      const tab = new ApiClient();
      live.push(tab);
      tab.setToken('a-token', 900, 'family-a');
      channelHandler(tab)?.({
        data: { type: 'token', token: 'b-token', expiresAt: null, family: 'family-b' },
      } as MessageEvent);
      channelHandler(tab)?.({
        data: { type: 'token', token: 'late-a', expiresAt: null, family: 'family-a' },
      } as MessageEvent);
      expect(tab.getFamilyId()).toBe('family-b');
    });

    it('raises the session-restored event when a fresh-session token is accepted', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        const onMessage = channelHandler(apiClient);
        // This tab had no session, so another tab's login token is a brand-new session:
        // the app must hydrate the account stores it does not yet have.
        onMessage?.({
          data: { type: 'token', token: 'b-token', expiresAt: null, family: 'family-b' },
        } as MessageEvent);

        expect(apiClient.hasSession()).toBe(true);
        expect(events).toEqual(['whento:remote-session']);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('does not raise the event for a same-session refresh', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        // Give this tab a session first; a token stamped with the same family is merely
        // a refresh of that session, so no app-level hydration is needed.
        apiClient.setToken('a-token', undefined, 'family-a');
        const onMessage = channelHandler(apiClient);
        onMessage?.({
          data: { type: 'token', token: 'a-token-refreshed', expiresAt: null, family: 'family-a' },
        } as MessageEvent);

        expect(events).toEqual([]);
        expect((apiClient as unknown as { accessToken: string | null }).accessToken).toBe(
          'a-token-refreshed'
        );
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('ignores an unidentified peer token once the current server family is known', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        apiClient.setToken('a-token', undefined, 'family-a');
        const onMessage = channelHandler(apiClient);
        onMessage?.({
          data: { type: 'token', token: 'a-token-refreshed', expiresAt: null },
        } as MessageEvent);

        expect(events).toEqual([]);
        expect((apiClient as unknown as { accessToken: string | null }).accessToken).toBe(
          'a-token'
        );
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('rejects a stale token from a signed-out family', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        // A token for the family this browser deliberately signed out of must never
        // resurrect the session, even if the logout message itself is long gone.
        apiClient.setToken('a-token', undefined, 'family-a');
        apiClient.signOut();
        const onMessage = channelHandler(apiClient);
        onMessage?.({
          data: { type: 'token', token: 'stale', expiresAt: null, family: 'family-a' },
        } as MessageEvent);

        expect(apiClient.hasSession()).toBe(false);
        expect(events).toEqual([]);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('rejects a token for the shared logged-out family on a cold tab that just loaded', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        // This tab signed out in a previous life and reloaded cold: it has no in-memory
        // knowledge, but the shared non-secret logout marker still names the family.
        localStorage.setItem('whento.loggedOutFamily', 'family-a');
        const onMessage = channelHandler(apiClient);
        onMessage?.({
          data: { type: 'token', token: 'stale', expiresAt: null, family: 'family-a' },
        } as MessageEvent);

        expect(apiClient.hasSession()).toBe(false);
        expect(events).toEqual([]);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('rejects a token that arrived already expired', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        const onMessage = channelHandler(apiClient);
        onMessage?.({
          data: { type: 'token', token: 'dead', expiresAt: Date.now() - 1000, family: 'family-x' },
        } as MessageEvent);

        expect(apiClient.hasSession()).toBe(false);
        expect(events).toEqual([]);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('rejects token messages that carry no token at all', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        const onMessage = channelHandler(apiClient);
        onMessage?.({ data: { type: 'token', token: '', family: 'family-x' } } as MessageEvent);

        expect(apiClient.hasSession()).toBe(false);
        expect(events).toEqual([]);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('a receiver never echoes a received token', () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      const api = new ApiClient();
      live.push(api);
      const onMessage = channelHandler(api);
      onMessage?.({
        data: { type: 'token', token: 'from-other-tab', family: 'family-b' },
      } as MessageEvent);

      expect(posted).toEqual([]);
    });

    it('adopts a different-family token as a replacement session and advances the fence', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        const api = new ApiClient();
        live.push(api);
        api.setToken('a-token', undefined, 'family-a');
        const genBefore = (api as unknown as { sessionGeneration: number }).sessionGeneration;

        localStorage.setItem('whento.activeFamily', 'family-b');
        const onMessage = channelHandler(api);
        onMessage?.({
          data: { type: 'token', token: 'b-token', expiresAt: null, family: 'family-b' },
        } as MessageEvent);

        expect((api as unknown as { accessToken: string | null }).accessToken).toBe('b-token');
        expect((api as unknown as { familyId: string | null }).familyId).toBe('family-b');
        // The generation moved — every pending request/refresh from family A is fenced.
        expect((api as unknown as { sessionGeneration: number }).sessionGeneration).toBe(
          genBefore + 1
        );
        expect(events).toEqual(['whento:remote-session']);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('does not adopt a same-family token twice (no re-broadcast, no fence move)', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        apiClient.setToken('a-token', undefined, 'family-a');
        const genBefore = (apiClient as unknown as { sessionGeneration: number }).sessionGeneration;

        const onMessage = channelHandler(apiClient);
        onMessage?.({
          data: { type: 'token', token: 'a-token-refreshed', expiresAt: null, family: 'family-a' },
        } as MessageEvent);

        expect((apiClient as unknown as { sessionGeneration: number }).sessionGeneration).toBe(
          genBefore
        );
        expect(events).toEqual([]);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('does not replay the provoking request when refresh adopts another family', async () => {
      const api = new ApiClient();
      live.push(api);
      const seen: string[] = [];
      instanceOf(api).defaults.adapter = (async config => {
        seen.push(config.url ?? '');
        if (config.url === '/auth/refresh') {
          return {
            data: {
              success: true,
              data: { access_token: 'b-token', expires_in: 900, session_id: 'family-b' },
            },
            status: 200,
            statusText: '200',
            headers: {},
            config: config as never,
          } as never;
        }
        const response = {
          data: { success: false, error: { code: 'UNAUTHORIZED', message: 'nope' } },
          status: 401,
          statusText: '401',
          headers: {},
          config: config as never,
        } as never;
        const error = new Error('Request failed with status code 401') as Error & {
          response?: unknown;
          config?: unknown;
          isAxiosError?: boolean;
        };
        error.response = response;
        error.config = config;
        error.isAxiosError = true;
        throw error;
      }) as AxiosAdapter;

      api.setToken('a-token', undefined, 'family-a');
      await api.get('/calendars').catch(() => {});

      expect(seen.filter(url => url === '/calendars')).toHaveLength(1);
      expect((api as unknown as { familyId: string | null }).familyId).toBe('family-b');
      expect((api as unknown as { accessToken: string | null }).accessToken).toBe('b-token');
    });

    it('treats a same-family token as a plain refresh', () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        const api = new ApiClient();
        live.push(api);
        api.setToken('a-token', undefined, 'family-a');
        const genBefore = (api as unknown as { sessionGeneration: number }).sessionGeneration;

        // Same family: the same session family, so a refresh, not a replacement — no
        // event, no generation bump.
        channelHandler(api)?.({
          data: { type: 'token', token: 'a-token-refreshed', expiresAt: null, family: 'family-a' },
        } as MessageEvent);

        expect((api as unknown as { accessToken: string | null }).accessToken).toBe(
          'a-token-refreshed'
        );
        expect((api as unknown as { sessionGeneration: number }).sessionGeneration).toBe(genBefore);
        expect(events).toEqual([]);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });
  });

  describe('cross-tab token expiration', () => {
    it('broadcasts the absolute expiry along with the token', () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));

      apiClient.setToken('a-token', 900);

      const message = posted.find(
        (m): m is { type: string } => (m as { type?: string })?.type === 'token'
      );
      expect(message).toBeTruthy();
      const expiresAt = (message as { expiresAt?: number }).expiresAt;
      expect(expiresAt).toBeTypeOf('number');
      // ~15 minutes from now, as a single absolute instant all tabs can schedule from.
      expect(expiresAt! - Date.now()).toBeGreaterThan(899_000);
      expect(expiresAt! - Date.now()).toBeLessThan(901_000);
    });

    it("reschedules proactive refresh from an incoming token's expiry", async () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));
        // A nearly-dead token: its proactive refresh is already due within the 5s floor.
        apiClient.setToken('nearly-dead', 60);

        const onMessage = channelHandler(apiClient);
        // Another tab hands us a token valid for 15 minutes from now.
        onMessage?.({
          data: { type: 'token', token: 'fresh-from-other-tab', expiresAt: Date.now() + 900_000 },
        } as MessageEvent);

        // The old (5s) timer must have been replaced, not left to fire a redundant
        // refresh that would spend the single-use refresh cookie.
        await vi.advanceTimersByTimeAsync(60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);

        // At the new token's own lead window the refresh does fire, with the received
        // token in hand.
        await vi.advanceTimersByTimeAsync(14 * 60_000);
        const refreshes = seen.filter(r => r.url === '/auth/refresh');
        expect(refreshes).toHaveLength(1);
      } finally {
        vi.useRealTimers();
      }
    });

    it('does not refresh on wake-up when the received token still has life', async () => {
      const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));
      apiClient.setToken('nearly-dead', 30);

      const onMessage = channelHandler(apiClient);
      onMessage?.({
        data: { type: 'token', token: 'fresh-from-other-tab', expiresAt: Date.now() + 900_000 },
      } as MessageEvent);

      Object.defineProperty(document, 'hidden', { configurable: true, value: false });
      document.dispatchEvent(new Event('visibilitychange'));
      await new Promise(resolve => setTimeout(resolve, 10));

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
    });
  });

  describe('logout versus an in-flight refresh', () => {
    /**
     * An adapter for the race tests: `/auth/refresh` answers only once its deferred
     * has been resolved, and every other request 401s (so the first GET provokes the
     * refresh).
     */
    const deferredRefreshAdapter =
      (
        refresh: { promise: Promise<{ access_token: string; expires_in?: number }> },
        onRefreshStarted?: () => void
      ): AxiosAdapter =>
      async config => {
        const base = { headers: {}, config: config as never };
        if (config.url === '/auth/refresh') {
          onRefreshStarted?.();
          const data = await refresh.promise;
          return {
            data: { success: true, data },
            status: 200,
            statusText: '200',
            ...base,
          } as never;
        }
        const response = {
          data: { success: false, error: { code: 'UNAUTHORIZED', message: 'nope' } },
          status: 401,
          statusText: '401',
          ...base,
        } as never;
        const error = new Error('Request failed with status code 401') as Error & {
          response?: unknown;
          config?: unknown;
          isAxiosError?: boolean;
        };
        error.response = response;
        error.config = config;
        error.isAxiosError = true;
        throw error;
      };

    /**
     * A 401-triggered (or proactive) refresh POST that is still on the wire when the
     * session is ended must not be allowed to re-seed it. Each sign-out bumps the
     * session generation; a refresh that captured an older generation discards its
     * response instead of storing and broadcasting it.
     */
    it('a local sign-out voids the response of a refresh that was already in flight', async () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      const refresh = deferred<{ access_token: string; expires_in?: number }>();
      let refreshStarted = false;
      instance().defaults.adapter = deferredRefreshAdapter(refresh, () => {
        refreshStarted = true;
      });

      apiClient.setToken('about-to-expire');

      const outcome = apiClient.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );

      await vi.waitFor(() => expect(refreshStarted).toBe(true));

      posted.length = 0;
      apiClient.signOut();
      expect(apiClient.hasSession()).toBe(false);

      refresh.resolve({ access_token: 'rotated-after-logout', expires_in: 900 });
      await outcome;

      expect(apiClient.hasSession()).toBe(false);
      expect(posted.filter(m => (m as { type?: string })?.type === 'token')).toEqual([]);
    });

    it('a received cross-tab logout voids the response of an in-flight refresh', async () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      const refresh = deferred<{ access_token: string; expires_in?: number }>();
      let refreshStarted = false;
      instance().defaults.adapter = deferredRefreshAdapter(refresh, () => {
        refreshStarted = true;
      });
      routeMeta.public = true;

      apiClient.setToken('about-to-expire', undefined, 'family-a');
      const outcome = apiClient.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );

      await vi.waitFor(() => expect(refreshStarted).toBe(true));
      const onMessage = channelHandler(apiClient);
      posted.length = 0;
      onMessage?.({ data: { type: 'logout', family: 'family-a' } } as MessageEvent);
      expect(apiClient.hasSession()).toBe(false);

      refresh.resolve({ access_token: 'rotated-after-logout', expires_in: 900 });
      await outcome;

      expect(apiClient.hasSession()).toBe(false);
      expect(posted.filter(m => (m as { type?: string })?.type === 'token')).toEqual([]);
    });

    /**
     * Record-and-defer adapter suitable for no-Web-Locks tests: `/auth/refresh` stays
     * pending, `/auth/logout` answers immediately, everything else 401s, and every
     * request is recorded so tests can assert what reached the wire.
     */
    const recordingAdapter =
      (
        refresh: { promise: Promise<{ access_token: string; expires_in?: number }> },
        seen: string[],
        onRefreshStarted?: () => void
      ): AxiosAdapter =>
      async config => {
        const base = { headers: {}, config: config as never };
        seen.push(config.url ?? '');
        if (config.url === '/auth/refresh') {
          onRefreshStarted?.();
          const data = await refresh.promise;
          return {
            data: { success: true, data },
            status: 200,
            statusText: '200',
            ...base,
          } as never;
        }
        if (config.url === '/auth/logout') {
          return {
            data: { success: true, data: {} },
            status: 200,
            statusText: '200',
            ...base,
          } as never;
        }
        const response = {
          data: { success: false, error: { code: 'UNAUTHORIZED', message: 'nope' } },
          status: 401,
          statusText: '401',
          ...base,
        } as never;
        const error = new Error('Request failed with status code 401') as Error & {
          response?: unknown;
          config?: unknown;
          isAxiosError?: boolean;
        };
        error.response = response;
        error.config = config;
        error.isAxiosError = true;
        throw error;
      };

    it('serialises a deliberate logout behind an in-flight refresh when Web Locks are absent', async () => {
      // jsdom has no navigator.locks, so this exercises the in-process gate: the
      // deliberate logout must queue behind the refresh within this client instead of
      // racing it onto the single-use refresh cookie.
      const seen: string[] = [];
      const refresh = deferred<{ access_token: string; expires_in?: number }>();
      let refreshStarted = false;
      instance().defaults.adapter = recordingAdapter(refresh, seen, () => {
        refreshStarted = true;
      });

      apiClient.setToken('about-to-expire');
      const outcome = apiClient.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );
      await vi.waitFor(() => expect(refreshStarted).toBe(true));

      let loggedOut = false;
      const logoutDone = apiClient.logoutThroughLock(async () => {
        await apiClient.post('/auth/logout');
        loggedOut = true;
      });

      await new Promise(resolve => setTimeout(resolve, 10));
      expect(loggedOut).toBe(false);
      expect(seen).not.toContain('/auth/logout');

      refresh.resolve({ access_token: 'rotated', expires_in: 900 });
      await logoutDone;
      await outcome;

      expect(loggedOut).toBe(true);
      expect(seen.indexOf('/auth/refresh')).toBeLessThan(seen.indexOf('/auth/logout'));
    });

    it('rejects a request whose refresh was voided, instead of replaying it as the next session', async () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      const seen: string[] = [];
      const refresh = deferred<{ access_token: string; expires_in?: number }>();
      let refreshStarted = false;
      instance().defaults.adapter = recordingAdapter(refresh, seen, () => {
        refreshStarted = true;
      });

      apiClient.setToken('a-token');
      const outcome = apiClient.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );
      await vi.waitFor(() => expect(refreshStarted).toBe(true));

      // The session dies while the refresh is on the wire — a forced expiry or a
      // cross-tab sign-out bumps the generation without going through the lock — and
      // account B signs back in before A's delayed response is delivered.
      apiClient.clearToken();
      apiClient.setToken('b-token', undefined, 'family-b');
      posted.length = 0;

      refresh.resolve({ access_token: 'a-rotated', expires_in: 900 });
      await outcome;

      expect(apiClient.hasSession()).toBe(true);
      expect(posted.filter(m => (m as { type?: string })?.type === 'token')).toEqual([]);
      expect(seen.filter(url => url === '/calendars')).toHaveLength(1);
    });

    /**
     * PR-163 seed 163, no-Web-Locks variant.
     *
     * Two tabs (two ApiClient instances sharing one jsdom window, no navigator.locks):
     * tabA has a refresh on the wire; tabB deliberately signs out, so a logout message
     * reaches tabA while its refresh is still pending; then the stale refresh succeeds.
     * Without Web Locks there is no cross-tab queue to make the logout wait — ordering
     * is explicitly not guaranteed — so the fence has to do the work: the received
     * logout must clear tabA (generation bump), and the delayed old-family refresh must
     * be discarded rather than re-seeding the session or re-broadcasting a token.
     */
    it('discards a delayed old-family refresh after a logout from another tab (seed 163, no Web Locks)', async () => {
      const tabA = new ApiClient();
      const tabB = new ApiClient();
      live.push(tabA, tabB);
      // jsdom has no navigator.locks — the no-Web-Locks fallback path.
      expect(typeof navigator.locks).toBe('undefined');

      const seenA: string[] = [];
      const refresh = deferred<{
        access_token: string;
        expires_in?: number;
        session_id?: string;
      }>();
      let refreshStarted = false;
      instanceOf(tabA).defaults.adapter = recordingAdapter(refresh, seenA, () => {
        refreshStarted = true;
      });

      tabA.setToken('about-to-expire', undefined, 'family-s163');
      const heldRefresh = tabA.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );
      await vi.waitFor(() => expect(refreshStarted).toBe(true));

      // Tab B deliberately signs out the shared session.
      const postedA: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => postedA.push(m));
      tabB.setToken('about-to-expire', undefined, 'family-s163');
      tabB.signOut();
      expect(tabB.hasSession()).toBe(false);

      // The logout message reaches tabA (channel delivery, unaffected by Web Locks).
      const seen = postedA.filter(m => (m as { type?: string }).type === 'logout');
      expect(seen).toHaveLength(1);
      channelHandler(tabA)?.({ data: seen[seen.length - 1] } as MessageEvent);
      // The received logout cleared tabA's in-memory session for the shared family.
      expect((tabA as unknown as { accessToken: string | null }).accessToken).toBeNull();
      expect(tabA.getFamilyId()).toBeNull();
      // tabB's own broadcasts before this point are irrelevant; only what is posted
      // from here on can be a re-seed attempt by tabA.
      postedA.length = 0;
      // Forget what tabB broadcast before the logout settled; only broadcasts from
      // here on are a re-seed attempt by tabA.
      postedA.length = 0;

      // The stale refresh resolves after the logout — and must not resurrect tabA.
      refresh.resolve({
        access_token: 'rotated-after-logout',
        expires_in: 900,
        session_id: 'family-s163',
      });
      await heldRefresh;
      expect((tabA as unknown as { accessToken: string | null }).accessToken).toBeNull();
      expect(tabA.getFamilyId()).toBeNull();
      // tabA never echoed the stale token back.
      expect(postedA.filter(m => (m as { type?: string }).type === 'token')).toEqual([]);
    });

    it('rejects a delayed token broadcast from a session family that has already logged out', async () => {
      const tabA = new ApiClient();
      const tabB = new ApiClient();
      const tabC = new ApiClient();
      live.push(tabA, tabB, tabC);

      // A signs in; C (another tab) follows through the channel.
      tabA.setToken('a-token', undefined, 'family-one');
      await new Promise(resolve => setTimeout(resolve, 20));
      expect(tabC.hasSession()).toBe(true);

      // B deliberately signs out family one.
      tabB.signOut();
      await new Promise(resolve => setTimeout(resolve, 20));
      expect(tabA.hasSession()).toBe(false);
      expect(tabC.hasSession()).toBe(false);

      // An out-of-order token from the dead session arrives at C *after* the logout.
      channelHandler(tabC)?.({
        data: {
          type: 'token',
          token: 'stale-from-dead-session',
          expiresAt: null,
          family: 'family-one',
        },
      } as MessageEvent);

      // C must remain signed out: the stale token never resurrects its session.
      expect(tabC.hasSession()).toBe(false);
      const seen: Array<{ url: string; auth?: string }> = [];
      instanceOf(tabC).defaults.adapter = async config => {
        seen.push({
          url: config.url ?? '',
          auth: (config.headers as Record<string, string> | undefined)?.Authorization,
        });
        const response = {
          data: { success: false, error: { code: 'UNAUTHORIZED', message: 'nope' } },
          status: 401,
          statusText: '401',
          headers: {},
          config: config as never,
        } as never;
        const error = new Error('Request failed with status code 401') as Error & {
          response?: unknown;
          config?: unknown;
          isAxiosError?: boolean;
        };
        error.response = response;
        error.config = config;
        error.isAxiosError = true;
        throw error;
      };
      await tabC.get('/calendars').catch(() => {});
      expect(seen[0]?.auth).toBeUndefined();
    });

    it("ignores an older sender's delayed logout after a newer session was accepted", async () => {
      // BroadcastChannel preserves a *sender's* ordering, not a total order across
      // senders, so a tab can legitimately receive the new session's token before the
      // old session's logout. Suppress auto-delivery so each message is placed by hand.
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));

      const tabA = new ApiClient();
      const tabB = new ApiClient();
      const tabC = new ApiClient();
      live.push(tabA, tabB, tabC);

      // All three tabs share session family one; B and C follow A's token through the
      // channel.
      tabA.setToken('session-1-token', undefined, 'family-one');
      const aToken = posted[posted.length - 1] as {
        type: string;
        token: string;
        family: string | null;
      };
      expect(aToken.type).toBe('token');
      channelHandler(tabB)?.({
        data: { type: 'token', token: aToken.token, expiresAt: null, family: aToken.family },
      } as MessageEvent);
      channelHandler(tabC)?.({
        data: { type: 'token', token: aToken.token, expiresAt: null, family: aToken.family },
      } as MessageEvent);
      expect(tabC.hasSession()).toBe(true);

      // A logs out session one; B observes the logout (clearing its copy of the
      // session), then signs in again as a *new* family.
      tabA.signOut();
      const aLogout = posted[posted.length - 1] as { type: string; family: string | null };
      expect(aLogout.type).toBe('logout');
      channelHandler(tabB)?.({
        data: { type: 'logout', family: aLogout.family },
      } as MessageEvent);
      expect(tabB.hasSession()).toBe(false);

      tabB.setToken('session-2-token', undefined, 'family-two');
      const bToken = posted[posted.length - 1] as {
        type: string;
        token: string;
        family: string | null;
      };
      expect(bToken.type).toBe('token');
      expect(bToken.family).toBe('family-two');

      // C receives B's newer-session token *before* A's delayed logout.
      channelHandler(tabC)?.({
        data: { type: 'token', token: bToken.token, expiresAt: null, family: bToken.family },
      } as MessageEvent);
      expect(tabC.hasSession()).toBe(true);

      // The delayed logout belongs to an older family than the one C accepted, so it
      // must not clear the newer session.
      channelHandler(tabC)?.({
        data: { type: 'logout', family: aLogout.family },
      } as MessageEvent);
      expect(tabC.hasSession()).toBe(true);

      // And the newer session is genuinely installed: its token rides C's requests.
      const seen: Array<{ url: string; auth?: string }> = [];
      instanceOf(tabC).defaults.adapter = async config => {
        seen.push({
          url: config.url ?? '',
          auth: (config.headers as Record<string, string> | undefined)?.Authorization,
        });
        return {
          data: { success: true, data: { ok: true } },
          status: 200,
          statusText: '200',
          headers: {},
          config: config as never,
        } as never;
      };
      await expect(tabC.get('/calendars')).resolves.toEqual({ ok: true });
      expect(seen[0]?.auth).toBe('Bearer session-2-token');
    });

    it('never replays a request whose refresh was invalidated while queued behind the cookie lock', async () => {
      const holder = new ApiClient();
      const queued = new ApiClient();
      live.push(holder, queued);

      // Holder occupies the cookie lock with a delayed refresh (fake Web Locks).
      const fakeLocks = (() => {
        const queue: Array<() => void> = [];
        let held = false;
        const request = async (_name: string, fn: () => Promise<unknown>) => {
          if (held) {
            await new Promise<void>(resolve => queue.push(resolve));
          }
          held = true;
          try {
            return await fn();
          } finally {
            held = false;
            const next = queue.shift();
            next?.();
          }
        };
        return { request };
      })();
      vi.stubGlobal('navigator', { ...navigator, locks: fakeLocks });

      const seenHolder: string[] = [];
      const refresh = deferred<{ access_token: string; expires_in?: number }>();
      let refreshStarted = false;
      instanceOf(holder).defaults.adapter = recordingAdapter(refresh, seenHolder, () => {
        refreshStarted = true;
      });
      holder.setToken('a-token');
      const holdingRefresh = holder.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );
      await vi.waitFor(() => expect(refreshStarted).toBe(true));

      // Queued's request 401s and its refresh queues behind the holder's lock.
      const seenQueued: string[] = [];
      instanceOf(queued).defaults.adapter = async config => {
        seenQueued.push(config.url ?? '');
        const response = {
          data: { success: false, error: { code: 'UNAUTHORIZED', message: 'nope' } },
          status: 401,
          statusText: '401',
          headers: {},
          config: config as never,
        } as never;
        const error = new Error('Request failed with status code 401') as Error & {
          response?: unknown;
          config?: unknown;
          isAxiosError?: boolean;
        };
        error.response = response;
        error.config = config;
        error.isAxiosError = true;
        throw error;
      };
      queued.setToken('a-token');
      const outcome = queued.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );
      await new Promise(resolve => setTimeout(resolve, 50));

      // The session is replaced while the refresh is still queued.
      queued.clearToken();
      queued.setToken('b-token', undefined, 'family-b');

      // The lock frees; the queued refresh runs its generation guard and voids itself.
      refresh.resolve({ access_token: 'a-rotated', expires_in: 900 });
      await holdingRefresh;
      await outcome;

      // The provoking request was never sent a second time, and never with B's token.
      expect(seenQueued.filter(url => url === '/calendars')).toHaveLength(1);
      expect(seenQueued).not.toContain('/auth/refresh');
    });
  });

  describe('refreshing before the token dies', () => {
    it('re-arms a browser timer that fires just before the lead window', async () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 120 }));
        apiClient.setToken('current', 120);
        const early = vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 59_999);
        await vi.advanceTimersByTimeAsync(60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
        early.mockRestore();
        await vi.advanceTimersByTimeAsync(5_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1);
      } finally {
        vi.useRealTimers();
      }
    });

    it('schedules a refresh a minute short of expiry', async () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));

        apiClient.setToken('current', 900);

        vi.advanceTimersByTime(13 * 60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);

        await vi.advanceTimersByTimeAsync(2 * 60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1);
      } finally {
        vi.useRealTimers();
      }
    });

    it('does nothing when the token carries no expiry', () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh' }));

        apiClient.setToken('current');

        vi.advanceTimersByTime(60 * 60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
      } finally {
        vi.useRealTimers();
      }
    });

    it('drops the pending refresh when the session ends', () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));
        apiClient.setToken('current', 900);
        expect(vi.getTimerCount()).toBeGreaterThan(0);

        apiClient.clearToken();

        vi.advanceTimersByTime(60 * 60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
        expect(vi.getTimerCount()).toBe(0);
      } finally {
        vi.useRealTimers();
      }
    });

    it('replaces the old refresh timer instead of leaking a second one', () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));
        apiClient.setToken('first', 900);
        expect(vi.getTimerCount()).toBe(1);

        apiClient.setToken('second', 900);
        expect(vi.getTimerCount()).toBe(1);

        apiClient.setToken('third');
        expect(vi.getTimerCount()).toBe(0);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
      } finally {
        vi.useRealTimers();
      }
    });

    it('refreshes on the way back from a sleeping tab', async () => {
      const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));

      apiClient.setToken('nearly-dead', 30);

      Object.defineProperty(document, 'hidden', { configurable: true, value: false });
      document.dispatchEvent(new Event('visibilitychange'));

      await vi.waitFor(() => expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1));
    });

    it('leaves a healthy token alone when the tab comes back', async () => {
      const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));
      apiClient.setToken('plenty-of-life', 900);

      Object.defineProperty(document, 'hidden', { configurable: true, value: false });
      document.dispatchEvent(new Event('visibilitychange'));
      window.dispatchEvent(new Event('online'));
      await new Promise(resolve => setTimeout(resolve, 10));

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
    });
  });

  describe('the 401 refresh', () => {
    it('refreshes once and replays the original request', async () => {
      apiClient.setToken('expired');

      const seen = withAdapter((config, callNumber) => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'fresh' });
        return callNumber === 0 ? unauthorized() : ok({ ok: true });
      });

      await expect(apiClient.get('/calendars')).resolves.toEqual({ ok: true });

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1);
      // The replay carries the new token, not the expired one.
      const replay = seen.filter(r => r.url === '/calendars');
      expect(replay).toHaveLength(2);
      expect(replay[1].auth).toBe('Bearer fresh');
      expect(apiClient).toBeTruthy();
    });

    it('issues one refresh for several concurrent 401s', async () => {
      apiClient.setToken('expired');

      const seen = withAdapter((config, callNumber) => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'fresh' });
        return callNumber === 0 ? unauthorized() : ok({ url: config.url });
      });

      const results = await Promise.all([
        apiClient.get('/calendars'),
        apiClient.get('/availabilities'),
        apiClient.get('/quota/limits'),
      ]);

      expect(results).toHaveLength(3);
      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1);
    });

    it('starts a fresh refresh after the previous one has settled', async () => {
      apiClient.setToken('expired');

      const seen = withAdapter((config, callNumber) => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'fresh' });
        return callNumber % 2 === 0 ? unauthorized() : ok({});
      });

      await apiClient.get('/first');
      await apiClient.get('/second');

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(2);
    });

    it('does not retry a request that already retried once', async () => {
      apiClient.setToken('expired');

      const seen = withAdapter(config => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'fresh' });
        return unauthorized();
      });

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      // Two attempts at the original, one refresh — not an endless loop.
      expect(seen.filter(r => r.url === '/calendars')).toHaveLength(2);
      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1);
    });

    it('never tries to refresh for the auth endpoints themselves', async () => {
      const seen = withAdapter(() => unauthorized());

      await expect(apiClient.post('/auth/login', {})).rejects.toBeTruthy();

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
    });

    it('does not sign out or bounce on a rejected /auth/bootstrap key', async () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('still-valid');
      posted.length = 0;

      withAdapter(config => {
        if (config.url === '/auth/bootstrap') return unauthorized();
        return ok({});
      });

      await expect(apiClient.post('/auth/bootstrap', { key: 'wrong' })).rejects.toBeTruthy();

      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(posted).toEqual([]);
    });

    it('does not sign out or bounce on a rejected login or register', async () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('still-valid');
      posted.length = 0;

      withAdapter(() => unauthorized());

      await expect(apiClient.post('/auth/login', {})).rejects.toBeTruthy();
      await expect(apiClient.post('/auth/register', {})).rejects.toBeTruthy();

      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(posted).toEqual([]);
    });

    it('does not refresh, sign out or bounce on an invalid magic-link verification', async () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('still-valid');
      posted.length = 0;

      const seen = withAdapter(config => {
        if (config.url === '/auth/magic-link/verify') return unauthorized();
        return ok({});
      });

      await expect(
        apiClient.post(
          '/auth/magic-link/verify',
          { token: 'bad' },
          {
            headers: { 'X-Whento-Auth-Intent': 'magic-link' },
          }
        )
      ).rejects.toBeTruthy();

      // No refresh was attempted (it runs under the same cookie lock as the verify,
      // so an attempt would deadlock — this asserts the deadlock cannot happen).
      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(posted).toEqual([]);
    });

    it('returns an invalid MFA code 401 to the form, preserving the pending login', async () => {
      const { createPinia, setActivePinia } = await import('pinia');
      const { useAuthStore } = await import('@/stores/auth');
      const { registerRemoteSignoutListener } = await import('@/stores/remoteSignout');

      setActivePinia(createPinia());
      const authStore = useAuthStore();
      authStore.user = null;
      authStore.setTempToken('valid-pending-token');
      const unsubscribe = registerRemoteSignoutListener();

      try {
        routeMeta.public = true;

        let mfaCalls = 0;
        const seen = withAdapter(config => {
          if (config.url === '/auth/mfa/verify') {
            mfaCalls += 1;
            return mfaCalls === 1
              ? unauthorized()
              : ok({
                  access_token: 'access-token',
                  expires_in: 3600,
                  session_id: 'family-mfa',
                  user: { id: 'u-1' },
                });
          }
          if (config.url === '/auth/refresh') return unauthorized();
          return ok({});
        });

        await expect(
          apiClient.post('/auth/mfa/verify', { temp_token: 'valid-pending-token', code: '000000' })
        ).rejects.toBeTruthy();

        expect(seen.map(r => r.url)).toEqual(['/auth/mfa/verify']);
        expect(authStore.tempToken).toBe('valid-pending-token');
        expect(window.location.href).toBe('');
        expect(apiClient.hasSession()).toBe(false);
        expect(authStore.user).toBeNull();

        await expect(
          apiClient.post('/auth/mfa/verify', { temp_token: 'valid-pending-token', code: '123456' })
        ).resolves.toEqual({
          access_token: 'access-token',
          expires_in: 3600,
          session_id: 'family-mfa',
          user: { id: 'u-1' },
        });
        expect(seen.map(r => r.url)).toEqual(['/auth/mfa/verify', '/auth/mfa/verify']);
      } finally {
        unsubscribe();
      }
    });

    it.each([false, true])(
      'returns a rejected passkey assertion 401 to the login flow (healthy session=%s)',
      async healthy => {
        const posted: unknown[] = [];
        vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
        if (healthy) apiClient.setToken('healthy-token', undefined, 'healthy-family');

        routeMeta.public = true;

        const seen = withAdapter(config => {
          if (config.url === '/auth/refresh' && healthy) {
            return ok({
              access_token: 'rotated-healthy',
              session_id: 'healthy-family',
              expires_in: 900,
            });
          }
          if (config.url === '/auth/refresh') return unauthorized();
          return unauthorized();
        });

        await expect(
          apiClient.post(
            '/auth/passkey/login/finish',
            { id: 'rejected-credential' },
            { headers: { 'X-Challenge-ID': 'valid-challenge' } }
          )
        ).rejects.toBeTruthy();

        expect(seen.map(r => r.url)).toEqual(['/auth/passkey/login/finish']);
        expect(posted.filter(m => (m as { type?: string }).type === 'logout')).toEqual([]);
        expect(window.location.href).toBe('');
      }
    );

    it('logs out when the refresh itself fails', async () => {
      apiClient.setToken('expired');

      withAdapter(config => (config.url === '/auth/refresh' ? unauthorized() : unauthorized()));

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      expect(apiClient.hasSession()).toBe(false);
      expect(window.location.href).toBe('/login?redirect=%2Fdashboard');
    });

    it('preserves the session when the refresh fails transiently (500 from the backend)', async () => {
      await drainChannel();
      Object.defineProperty(window, 'location', { configurable: true, value: { href: '' } });
      apiClient.clearToken();
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('expired');

      routeMeta.public = false;

      let refreshes = 0;
      withAdapter(config => {
        if (config.url === '/auth/refresh') {
          refreshes += 1;
          return {
            status: 500,
            data: { success: false, error: { code: 'INTERNAL', message: 'boom' } },
          };
        }
        return unauthorized();
      });

      await expect(apiClient.get('/calendars')).rejects.toMatchObject({ code: 'INTERNAL' });

      expect(refreshes).toBe(1);
      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(posted.filter(m => (m as { type?: string })?.type === 'logout')).toEqual([]);
    });

    it('preserves the session when the refresh fails with a network transport error', async () => {
      await drainChannel();
      Object.defineProperty(window, 'location', { configurable: true, value: { href: '' } });
      apiClient.clearToken();
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('expired');

      withAdapter(config => {
        if (config.url === '/auth/refresh') {
          const error = new Error('Network Error') as Error & {
            config?: unknown;
            isAxiosError?: boolean;
            code?: string;
          };
          error.config = config;
          error.code = 'ERR_NETWORK';
          error.isAxiosError = true;
          throw error;
        }
        return unauthorized();
      });

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(posted.filter(m => (m as { type?: string })?.type === 'logout')).toEqual([]);
    });

    it('preserves the session when a direct/proactive refresh fails with a 500', async () => {
      await drainChannel();
      Object.defineProperty(window, 'location', { configurable: true, value: { href: '' } });
      apiClient.clearToken();
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('expired');

      withAdapter(config => {
        if (config.url === '/auth/refresh') {
          return {
            status: 500,
            data: { success: false, error: { code: 'INTERNAL', message: 'boom' } },
          };
        }
        return unauthorized();
      });

      await expect(apiClient.refreshToken()).rejects.toMatchObject({ code: 'INTERNAL' });

      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(posted.filter(m => (m as { type?: string })?.type === 'logout')).toEqual([]);
    });

    it('carries the page they were thrown out of into the login URL', async () => {
      currentRoute.fullPath = '/calendars/abc-123/settings';
      currentRoute.name = 'calendar-settings';
      apiClient.setToken('expired');

      withAdapter(() => unauthorized());

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      expect(window.location.href).toBe('/login?redirect=%2Fcalendars%2Fabc-123%2Fsettings');
    });

    it('does not ask to be sent back to the login page', async () => {
      currentRoute.fullPath = '/login';
      currentRoute.name = 'login';
      apiClient.setToken('expired');

      withAdapter(() => unauthorized());

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();
      expect(window.location.href).toBe('/login');
    });

    it('skips the call when another tab refreshed while we queued', async () => {
      apiClient.setToken('expired');

      const locks = {
        request: async (_name: string, fn: () => Promise<void>) => {
          apiClient.setToken('from-another-tab');
          return fn();
        },
      };
      vi.stubGlobal('navigator', { ...navigator, locks });

      const seen = withAdapter((config, callNumber) => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'rotated-away' });
        return callNumber === 0 ? unauthorized() : ok({ ok: true });
      });

      await expect(apiClient.get('/calendars')).resolves.toEqual({ ok: true });

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
      const replay = seen.filter(r => r.url === '/calendars');
      expect(replay[replay.length - 1].auth).toBe('Bearer from-another-tab');
    });

    it('does not redirect away from a public route', async () => {
      routeMeta.public = true;
      apiClient.setToken('expired');

      withAdapter(() => unauthorized());

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      expect(apiClient.hasSession()).toBe(false);
      expect(window.location.href).toBe('');
    });

    it('resets the account stores on a public route when the refresh itself fails', async () => {
      const { createPinia, setActivePinia } = await import('pinia');
      const { useAuthStore } = await import('@/stores/auth');
      const { useCalendarStore } = await import('@/stores/calendar');
      const { useUnifiedFeedStore } = await import('@/stores/unifiedFeed');
      const { registerRemoteSignoutListener } = await import('@/stores/remoteSignout');

      setActivePinia(createPinia());
      const authStore = useAuthStore();
      authStore.user = { id: 'u-1', email: 'a@x.test', display_name: 'A', role: 'user' } as never;
      const calendarStore = useCalendarStore();
      calendarStore.calendars = [{ id: 'c-1', name: 'A-owned', participants: [] }] as never;
      calendarStore.calendarsForUser = 'u-1';
      const feedStore = useUnifiedFeedStore();
      feedStore.config = { configured: true } as never;
      const unsubscribe = registerRemoteSignoutListener();
      try {
        routeMeta.public = true;
        apiClient.setToken('expired');

        withAdapter(() => unauthorized());

        await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

        expect(apiClient.hasSession()).toBe(false);
        expect(window.location.href).toBe('');
        expect(authStore.user).toBeNull();
        expect(calendarStore.calendars).toEqual([]);
        expect(calendarStore.calendarsForUser).toBeNull();
        expect(feedStore.config).toBeNull();
      } finally {
        unsubscribe();
      }
    });
  });

  describe('session boundary on delayed responses', () => {
    function immediateLocks() {
      vi.stubGlobal('navigator', {
        locks: { request: (_name: string, fn: () => unknown) => fn() },
      });
    }

    function makeResponse(config: AxiosRequestConfig, data: unknown) {
      return {
        status: 200,
        statusText: 'OK',
        headers: {},
        config: config as never,
        data: { success: true, data },
      } as never;
    }

    function make401(config: AxiosRequestConfig) {
      const response = {
        status: 401,
        statusText: 'Unauthorized',
        headers: {},
        config: config as never,
        data: { success: false, error: { code: 'UNAUTHORIZED', message: 'expired' } },
      } as never;
      const error = new Error('Request failed with status code 401') as Error & {
        response?: unknown;
        config?: unknown;
        isAxiosError?: boolean;
      };
      error.response = response;
      error.config = config;
      error.isAxiosError = true;
      return error;
    }

    function make500(config: AxiosRequestConfig) {
      const response = {
        status: 500,
        statusText: 'Internal Server Error',
        headers: {},
        config: config as never,
        data: { success: false, error: { code: 'INTERNAL', message: 'boom' } },
      } as never;
      const error = new Error('Request failed with status code 500') as Error & {
        response?: unknown;
        config?: unknown;
        isAxiosError?: boolean;
      };
      error.response = response;
      error.config = config;
      error.isAxiosError = true;
      return error;
    }

    it('does not sign out the replacement session when an old refresh fails with a non-401 error', async () => {
      routeMeta.public = true;
      immediateLocks();

      const held = deferred<void>();
      const wireOrder: string[] = [];
      const logoutBroadcasts: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => {
        logoutBroadcasts.push(m);
      });
      instance().defaults.adapter = (async config => {
        wireOrder.push(config.url ?? '');
        if (config.url === '/auth/refresh') {
          await held.promise;
          throw make500(config);
        }
        throw make401(config);
      }) as AxiosAdapter;

      apiClient.setToken('a-token', undefined, 'family-a');
      const pending = apiClient.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );
      await vi.waitFor(() => expect(wireOrder).toContain('/auth/refresh'));

      apiClient.clearToken();
      apiClient.setToken('b-token', undefined, 'family-b');
      logoutBroadcasts.length = 0;
      held.resolve();

      await pending;

      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(logoutBroadcasts.filter(m => (m as { type?: string })?.type === 'logout')).toEqual([]);
    });

    it('never replays an old-account mutation when its first 401 arrives after replacement', async () => {
      routeMeta.public = true;
      immediateLocks();

      const held = deferred<void>();
      const started = deferred<void>();
      const seen: Array<{ url: string | undefined; auth: string | undefined }> = [];

      instance().defaults.adapter = (async config => {
        seen.push({
          url: config.url,
          auth: (config.headers as Record<string, string> | undefined)?.Authorization,
        });
        if (seen.length === 1) {
          started.resolve();
          await held.promise;
          throw make401(config);
        }
        if (config.url === '/auth/refresh') {
          return makeResponse(config, {
            access_token: 'b-rotated',
            session_id: 'family-b',
            expires_in: 900,
          });
        }
        return makeResponse(config, { id: 'created-for-b' });
      }) as AxiosAdapter;

      apiClient.setToken('a-token', undefined, 'family-a');
      const pending = apiClient.post('/calendars', { name: 'Account A calendar' }).then(
        () => 'resolved',
        () => 'rejected'
      );
      await started.promise;

      apiClient.clearToken();
      apiClient.setToken('b-token', undefined, 'family-b');
      held.resolve();

      const outcome = await pending;
      expect({ outcome, seen }).toEqual({
        outcome: 'rejected',
        seen: [{ url: '/calendars', auth: 'Bearer a-token' }],
      });
    });

    it('does not sign out the replacement session when an old refresh answers 401', async () => {
      routeMeta.public = true;
      immediateLocks();

      const held = deferred<void>();
      const started = deferred<void>();
      const logoutBroadcasts: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => {
        logoutBroadcasts.push(m);
      });
      instance().defaults.adapter = (async config => {
        started.resolve();
        await held.promise;
        throw make401(config);
      }) as AxiosAdapter;

      apiClient.setToken('a-token', undefined, 'family-a');
      const pending = apiClient.refreshToken().catch(() => undefined);
      await started.promise;

      apiClient.clearToken();
      apiClient.setToken('b-token', undefined, 'family-b');
      logoutBroadcasts.length = 0;
      held.resolve();

      await pending;

      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(logoutBroadcasts.filter(m => (m as { type?: string })?.type === 'logout')).toEqual([]);
    });

    it('does not sign out a replacement session when a cold-start refresh answers 401', async () => {
      routeMeta.public = true;
      immediateLocks();

      const started = deferred<void>();
      const held = deferred<void>();
      const logoutBroadcasts: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => {
        logoutBroadcasts.push(m);
      });
      const onMessage = channelHandler(apiClient);
      expect(onMessage).toBeTypeOf('function');

      instance().defaults.adapter = (async config => {
        started.resolve();
        await held.promise;
        throw make401(config);
      }) as AxiosAdapter;

      const pending = apiClient.refreshToken().catch(() => undefined);
      await started.promise;

      // A remote tab (or a restore) installs a valid session while the refresh is on
      // the wire: the real token-message handler advances the session generation.
      onMessage?.({
        data: {
          type: 'token',
          token: 'b-token',
          family: 'family-b',
          expiresAt: Date.now() + 900_000,
        },
      } as MessageEvent);
      logoutBroadcasts.length = 0;
      held.resolve();

      await pending;

      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(logoutBroadcasts.filter(m => (m as { type?: string })?.type === 'logout')).toEqual([]);
    });

    it('cancels a queued replay when the session is replaced before its final dispatch', async () => {
      routeMeta.public = true;
      immediateLocks();

      const replayQueued = deferred<void>();
      const release = deferred<void>();
      const seen: Array<{ url: string | undefined; auth: string | undefined }> = [];
      instance().interceptors.request.use(async config => {
        if ((config as { _retry?: boolean })._retry) {
          replayQueued.resolve();
          await release.promise;
        }
        return config;
      });
      instance().defaults.adapter = (async config => {
        seen.push({
          url: config.url,
          auth: (config.headers as Record<string, string> | undefined)?.Authorization,
        });
        if (seen.length === 1) {
          throw make401(config);
        }
        if (config.url === '/auth/refresh') {
          return makeResponse(config, {
            access_token: 'a-rotated',
            session_id: 'family-a',
            expires_in: 900,
          });
        }
        return makeResponse(config, { id: 'created' });
      }) as AxiosAdapter;

      apiClient.setToken('a-token', undefined, 'family-a');
      const pending = apiClient.post('/calendars', { name: 'A calendar' }).then(
        () => 'resolved',
        () => 'rejected'
      );
      await replayQueued.promise;

      apiClient.clearToken();
      apiClient.setToken('b-token', undefined, 'family-b');
      release.resolve();

      const outcome = await pending;
      expect({ outcome, seen }).toEqual({
        outcome: 'rejected',
        seen: [
          { url: '/calendars', auth: 'Bearer a-token' },
          { url: '/auth/refresh', auth: 'Bearer a-token' },
        ],
      });
    });
  });

  describe('token family stability (PR-163 acceptance 7)', () => {
    it('keeps the family stable across refreshes and distinct across new sessions', async () => {
      const api = new ApiClient();
      live.push(api);
      let refreshCount = 0;
      instanceOf(api).defaults.adapter = (async config => {
        if (config.url === '/auth/refresh') {
          refreshCount += 1;
          return {
            data: {
              success: true,
              data: {
                access_token: `rotated-${refreshCount}`,
                expires_in: 900,
                session_id: 'family-stable',
              },
            },
            status: 200,
            statusText: '200',
            headers: {},
            config: config as never,
          } as never;
        }
        return {
          data: { success: true, data: {} },
          status: 200,
          statusText: '200',
          headers: {},
          config: config as never,
        } as never;
      }) as AxiosAdapter;

      // A new login establishes a distinct family.
      api.setToken('login-token', 900, 'family-stable');
      expect(api.getFamilyId()).toBe('family-stable');

      // Several refreshes keep the same family.
      await api.refreshToken();
      await api.refreshToken();
      expect(api.getFamilyId()).toBe('family-stable');
      expect(refreshCount).toBe(2);

      // A brand-new login gets a brand-new family — never the epoch-style check.
      api.setToken('next-login-token', 900, 'family-next');
      expect(api.getFamilyId()).toBe('family-next');
    });
  });

  describe('disposal', () => {
    it('closes the channel, cancels timers and clears the session', () => {
      const api = new ApiClient();
      live.push(api);
      const close = vi.spyOn(BroadcastChannel.prototype as never as { close: () => void }, 'close');
      api.setToken('a-token', 900);

      api.dispose();

      expect(close).toHaveBeenCalled();
      expect(api.hasSession()).toBe(false);
      expect(api.isDisposed).toBe(true);
    });

    it('removes its storage listener', () => {
      const remove = vi.spyOn(window, 'removeEventListener');
      const api = new ApiClient();
      live.push(api);

      api.dispose();

      expect(remove).toHaveBeenCalledWith('storage', expect.any(Function));
    });

    it('stops reacting after disposal', () => {
      const api = new ApiClient();
      live.push(api);
      api.setToken('a-token', undefined, 'family-a');
      routeMeta.public = true;
      api.dispose();

      window.dispatchEvent(
        new StorageEvent('storage', {
          key: 'whento.loggedOutFamily',
          newValue: 'family-a',
          oldValue: null,
        })
      );

      // A disposed client must not clear or navigate — nothing to assert beyond it not
      // touching the flag; the point is the listener is gone.
      expect(api.hasSession()).toBe(false);
    });

    it('without BroadcastChannel, a storage logout notification still signs the tab out', () => {
      // The fallback: no channel to pass tokens, but a deliberate sign-out must still
      // reach the other tabs. Hello the minimum storage notification: the shared
      // logged-out-family marker, observed through a window 'storage' event.
      vi.stubGlobal('BroadcastChannel', undefined as never);
      const api = new ApiClient();
      live.push(api);
      api.setToken('a-token', undefined, 'family-a');
      routeMeta.public = true;

      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-signout', listener);
      try {
        window.dispatchEvent(
          new StorageEvent('storage', {
            key: 'whento.loggedOutFamily',
            newValue: 'family-a',
            oldValue: null,
          })
        );

        expect(api.hasSession()).toBe(false);
        // The public-route reset still fires — the account stores get wiped in place.
        expect(events).toContain('whento:remote-signout');
      } finally {
        window.removeEventListener('whento:remote-signout', listener);
      }
    });

    it('a logout notification for an unrelated family does not clear the current one', () => {
      const api = new ApiClient();
      live.push(api);
      api.setToken('a-token', undefined, 'family-a');
      routeMeta.public = true;

      window.dispatchEvent(
        new StorageEvent('storage', {
          key: 'whento.loggedOutFamily',
          newValue: 'family-old',
          oldValue: null,
        })
      );

      // The current client still holds its own family's session; only the shared
      // session flag is a storage artifact (removed by an unrelated cold tab), which
      // production only reads once at cold load.
      expect((api as unknown as { accessToken: string | null }).accessToken).toBe('a-token');
      expect(api.getFamilyId()).toBe('family-a');
    });
  });
});
