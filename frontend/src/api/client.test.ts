/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
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

/**
 * Drain broadcast deliveries a preceding test may have queued.
 *
 * A logout broadcast sent by an earlier test on the shared `whento.auth` channel is
 * delivered asynchronously (as a task). If one lands *inside* this test's await window,
 * the singleton's channel handler clears the very session we are about to assert on —
 * a test-isolation artifact, not a client defect (in production a received logout is
 * supposed to sign the session out). Consuming the queue at the start of the test while
 * no session exists keeps the assertions deterministic. Delivery happens on idle turns,
 * so a small fixed number of turns is a deterministic drain for any bounded backlog.
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
  beforeEach(() => {
    localStorage.clear();
    apiClient.clearToken();
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
    vi.restoreAllMocks();
    // restoreAllMocks does not undo stubGlobal, and a stubbed navigator carrying a
    // fake Web Lock would follow us into the next test.
    vi.unstubAllGlobals();
  });

  describe('tokens', () => {
    it('keeps the access token out of storage entirely', () => {
      apiClient.setToken('a-token');

      // The whole point of the change: a script that can read stored data must not
      // find the token there. Only the flag survives, and it is not a credential.
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
    it('tells the other tabs when the user signs out', () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('a-token');

      apiClient.signOut();

      expect(apiClient.hasSession()).toBe(false);
      const logout = posted.find(m => (m as { type?: string })?.type === 'logout');
      expect(logout).toBeTruthy();
      // The logout carries the epoch a stale token message must be rejected against.
      expect((logout as { epoch?: number }).epoch).toBeTypeOf('number');
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
      apiClient.setToken('a-token');

      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-signout', listener);
      const unsubscribe = registerRemoteSignoutListener();
      try {
        const channel = (apiClient as unknown as { channel: BroadcastChannel | null }).channel;
        const onMessage = channel?.onmessage as unknown as
          ((event: MessageEvent) => void) | undefined;
        onMessage?.({ data: { type: 'logout' } } as MessageEvent);

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
        const channel = (apiClient as unknown as { channel: BroadcastChannel | null }).channel;
        const onMessage = channel?.onmessage as unknown as
          ((event: MessageEvent) => void) | undefined;
        onMessage?.({ data: { type: 'token', token: 'x' } } as MessageEvent);

        expect(events).toEqual([]);
      } finally {
        window.removeEventListener('whento:remote-signout', listener);
      }
    });
  });

  describe('cross-tab session restoration', () => {
    it('raises the session-restored event when a fresh-session token is accepted', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        const channel = (apiClient as unknown as { channel: BroadcastChannel | null }).channel;
        const onMessage = channel?.onmessage as unknown as
          ((event: MessageEvent) => void) | undefined;
        // This tab had no session, so another tab's login token is a brand-new session:
        // the app must hydrate the account stores it does not yet have.
        onMessage?.({
          data: { type: 'token', token: 'b-token', expiresAt: null, epoch: 5 },
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
        // Give this tab a session first; a token stamped with the same epoch is merely
        // a refresh of the same session family, so no app-level hydration is needed.
        apiClient.setToken('a-token');
        const epoch = (apiClient as unknown as { sessionEpoch: number | null }).sessionEpoch;
        const channel = (apiClient as unknown as { channel: BroadcastChannel | null }).channel;
        const onMessage = channel?.onmessage as unknown as
          ((event: MessageEvent) => void) | undefined;
        onMessage?.({
          data: { type: 'token', token: 'a-token-refreshed', expiresAt: null, epoch },
        } as MessageEvent);

        expect(events).toEqual([]);
        expect((apiClient as unknown as { accessToken: string | null }).accessToken).toBe(
          'a-token-refreshed'
        );
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('does not raise the event for a stale token from a logged-out family', () => {
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        // A token at or below the logged-out watermark is rejected outright; it must
        // not look like a new session that needs hydrating.
        apiClient.setToken('a-token');
        apiClient.signOut();
        const channel = (apiClient as unknown as { channel: BroadcastChannel | null }).channel;
        const onMessage = channel?.onmessage as unknown as
          ((event: MessageEvent) => void) | undefined;
        onMessage?.({
          data: { type: 'token', token: 'stale', expiresAt: null, epoch: 1 },
        } as MessageEvent);

        expect(apiClient.hasSession()).toBe(false);
        expect(events).toEqual([]);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('does not install a colliding equal-epoch token; refresh converges on the cookie family', async () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        const tabA = new ApiClient();
        const tabB = new ApiClient();
        tabA.setToken('a-token', undefined, 'family-a');
        tabB.setToken('b-token', undefined, 'family-b');
        // Force the collided epoch the non-atomic counter can produce.
        const epoch = (tabA as unknown as { sessionEpoch: number | null }).sessionEpoch;
        (tabB as unknown as { sessionEpoch: number | null }).sessionEpoch = epoch;

        // Cross-deliver both login messages at the same epoch. Neither tab may adopt
        // the other's token — that swap leaves each UI describing the wrong account.
        const seenA: string[] = [];
        const seenB: string[] = [];
        const refreshBody = {
          access_token: 'cookie-token',
          expires_in: 900,
          session_id: 'cookie-family',
        };
        const adapter = (seen: string[]) => async (config: { url?: string }) => {
          seen.push(config.url ?? '');
          return {
            data: { success: true, data: refreshBody },
            status: 200,
            statusText: '200',
            headers: {},
            config: config as never,
          } as never;
        };
        instanceOf(tabA).defaults.adapter = adapter(seenA) as AxiosAdapter;
        instanceOf(tabB).defaults.adapter = adapter(seenB) as AxiosAdapter;

        channelHandler(tabA)?.({
          data: { type: 'token', token: 'b-token', expiresAt: null, epoch, nonce: 'family-b' },
        } as MessageEvent);
        channelHandler(tabB)?.({
          data: { type: 'token', token: 'a-token', expiresAt: null, epoch, nonce: 'family-a' },
        } as MessageEvent);

        expect((tabA as unknown as { accessToken: string | null }).accessToken).toBe('a-token');
        expect((tabB as unknown as { accessToken: string | null }).accessToken).toBe('b-token');

        await vi.waitFor(() => expect(seenA).toContain('/auth/refresh'));
        await vi.waitFor(() => expect(seenB).toContain('/auth/refresh'));
        await Promise.resolve();
        await Promise.resolve();

        expect((tabA as unknown as { accessToken: string | null }).accessToken).toBe(
          'cookie-token'
        );
        expect((tabB as unknown as { accessToken: string | null }).accessToken).toBe(
          'cookie-token'
        );
        expect((tabA as unknown as { sessionNonce: string | null }).sessionNonce).toBe(
          'cookie-family'
        );
        expect((tabB as unknown as { sessionNonce: string | null }).sessionNonce).toBe(
          'cookie-family'
        );
        expect(events.length).toBeGreaterThan(0);
      } finally {
        window.removeEventListener('whento:remote-session', listener);
      }
    });

    it('does not replay the provoking request when refresh adopts another family', async () => {
      const api = new ApiClient();
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
      expect((api as unknown as { sessionNonce: string | null }).sessionNonce).toBe('family-b');
      expect((api as unknown as { accessToken: string | null }).accessToken).toBe('b-token');
    });

    it('treats an equal-epoch token with the same nonce as a plain refresh', () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      const events: string[] = [];
      const listener = (event: Event) => events.push(event.type);
      window.addEventListener('whento:remote-session', listener);
      try {
        const tab = new ApiClient();
        tab.setToken('a-token');
        const epoch = (tab as unknown as { sessionEpoch: number | null }).sessionEpoch;
        const nonce = (tab as unknown as { sessionNonce: string | null }).sessionNonce;

        // Same epoch AND same nonce: the same session family, so a refresh, not a
        // replacement — no event, no generation bump.
        channelHandler(tab)?.({
          data: { type: 'token', token: 'a-token-refreshed', expiresAt: null, epoch, nonce },
        } as MessageEvent);

        expect((tab as unknown as { accessToken: string | null }).accessToken).toBe(
          'a-token-refreshed'
        );
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

        const channel = (apiClient as unknown as { channel: BroadcastChannel | null }).channel;
        const onMessage = channel?.onmessage as unknown as
          ((event: MessageEvent) => void) | undefined;
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
      // An old, nearly-dead token — if expiry were not transferred, the wake-up would
      // refresh it immediately.
      apiClient.setToken('nearly-dead', 30);

      const channel = (apiClient as unknown as { channel: BroadcastChannel | null }).channel;
      const onMessage = channel?.onmessage as unknown as
        ((event: MessageEvent) => void) | undefined;
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
     * refresh). Mirrors the response/error shape `withAdapter` produces, which is what
     * the real interceptor expects.
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
      // Lets the test wait until /auth/refresh is genuinely on the wire before logging
      // out; the axios adapter chain is deferred to microtasks, so without this the
      // interceptor's refresh token capture would happen after the sign-out and the
      // race the test cares about would never actually be exercised.
      let refreshStarted = false;
      instance().defaults.adapter = deferredRefreshAdapter(refresh, () => {
        refreshStarted = true;
      });

      apiClient.setToken('about-to-expire');

      // The GET 401s and the interceptor starts a refresh that stays in flight.
      const outcome = apiClient.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );

      await vi.waitFor(() => expect(refreshStarted).toBe(true));

      // The user logs out while /auth/refresh is still on the wire.
      // (The initial setToken above legitimately broadcast the token; what matters is
      // that nothing is broadcast from here on — against the refresh response.)
      posted.length = 0;
      apiClient.signOut();
      expect(apiClient.hasSession()).toBe(false);

      // The refresh now succeeds — but it belongs to a session that is over.
      refresh.resolve({ access_token: 'rotated-after-logout', expires_in: 900 });
      await outcome;

      expect(apiClient.hasSession()).toBe(false);
      // Nothing was broadcast either: the other tabs must not be re-seeded.
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
      // A public route so the incoming logout handler resets state without navigating.
      routeMeta.public = true;

      apiClient.setToken('about-to-expire');
      const outcome = apiClient.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );

      // Wait for the refresh to be on the wire, then have another tab report a sign-out.
      await vi.waitFor(() => expect(refreshStarted).toBe(true));
      const channel = (apiClient as unknown as { channel: BroadcastChannel | null }).channel;
      const onMessage = channel?.onmessage as unknown as
        ((event: MessageEvent) => void) | undefined;
      // The initial setToken broadcast is irrelevant; only broadcasts from here on are.
      posted.length = 0;
      onMessage?.({ data: { type: 'logout' } } as MessageEvent);
      expect(apiClient.hasSession()).toBe(false);

      // The stale refresh resolves afterwards.
      refresh.resolve({ access_token: 'rotated-after-logout', expires_in: 900 });
      await outcome;

      expect(apiClient.hasSession()).toBe(false);
      expect(posted.filter(m => (m as { type?: string })?.type === 'token')).toEqual([]);
    });

    /**
     * An adapter for the no-lock tests: `/auth/refresh` stays pending until its
     * deferred resolves, `/auth/logout` answers immediately, and everything else 401s.
     * Every request is recorded so tests can assert what reached the wire and in what
     * order. Mirrors the response/error shape `withAdapter` produces.
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
      // jsdom has no navigator.locks, so this exercises the in-process fallback: the
      // deliberate logout must queue behind the refresh instead of racing it onto the
      // single-use refresh cookie (a refresh that won the race could rotate the cookie
      // just as the logout cleared it).
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

      // The real logout method drives /auth/logout through the same lock as the
      // refresh, so it must wait its turn.
      let loggedOut = false;
      const logoutDone = apiClient.logoutThroughLock(async () => {
        await apiClient.post('/auth/logout');
        loggedOut = true;
      });

      // Give queued microtasks a chance: the logout must still be parked behind the
      // in-flight refresh, not on the wire yet.
      await new Promise(resolve => setTimeout(resolve, 10));
      expect(loggedOut).toBe(false);
      expect(seen).not.toContain('/auth/logout');

      refresh.resolve({ access_token: 'rotated', expires_in: 900 });
      await logoutDone;
      await outcome;

      // Cookie operations stayed ordered at the adapter boundary: the refresh settled
      // before the logout was allowed to clear the cookie.
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
      apiClient.setToken('b-token');
      posted.length = 0;

      refresh.resolve({ access_token: 'a-rotated', expires_in: 900 });
      await outcome;

      // B's session is untouched: nothing from A's stale refresh was stored/broadcast.
      expect(apiClient.hasSession()).toBe(true);
      expect(posted.filter(m => (m as { type?: string })?.type === 'token')).toEqual([]);
      // A's provoked request was never replayed — B's token never rode it anywhere.
      expect(seen.filter(url => url === '/calendars')).toHaveLength(1);
    });

    /**
     * Rebuilds the record-and-defer adapter for an independent client, so multi-tab
     * fencing tests can see exactly what each tab sent and in what order.
     */
    const recordingAdapterFor = (
      client: typeof apiClient,
      refresh: { promise: Promise<{ access_token: string; expires_in?: number }> },
      seen: string[],
      onRefreshStarted?: () => void
    ) => {
      instanceOf(client).defaults.adapter = recordingAdapter(refresh, seen, onRefreshStarted);
    };

    it('serialises a deliberate logout in one tab behind an in-flight refresh in another (no Web Locks)', async () => {
      const tabA = new ApiClient();
      const tabB = new ApiClient();

      // Tab A holds the origin lock while its refresh stays on the wire.
      const wireOrder: string[] = [];
      const seenA: string[] = [];
      const refresh = deferred<{ access_token: string; expires_in?: number }>();
      let refreshStarted = false;
      recordingAdapterFor(tabA, refresh, seenA, () => {
        refreshStarted = true;
        // Shared origin clock: record when the refresh actually reached the wire.
        wireOrder.push('/auth/refresh');
      });
      tabA.setToken('a-token');
      const holdingRefresh = tabA.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );
      await vi.waitFor(() => expect(refreshStarted).toBe(true));

      // Tab B tries to log out while A holds the lock: its /auth/logout must wait
      // behind A's refresh, not race it onto the single-use refresh cookie.
      const seenB: string[] = [];
      instanceOf(tabB).defaults.adapter = async config => {
        seenB.push(config.url ?? '');
        wireOrder.push(config.url ?? '');
        return {
          data: { success: true, data: {} },
          status: 200,
          statusText: '200',
          headers: {},
          config: config as never,
        } as never;
      };
      let loggedOut = false;
      const logoutDone = tabB.logoutThroughLock(async () => {
        await tabB.post('/auth/logout');
        loggedOut = true;
      });

      // Give the poller a few laps: the logout must still be parked behind A.
      await new Promise(resolve => setTimeout(resolve, 100));
      expect(loggedOut).toBe(false);
      expect(seenB).not.toContain('/auth/logout');

      refresh.resolve({ access_token: 'a-rotated', expires_in: 900 });
      await logoutDone;
      await holdingRefresh;

      // Cookie operations stayed ordered across the whole origin: A's refresh settled
      // before B's logout was allowed to clear the cookie.
      expect(loggedOut).toBe(true);
      expect(wireOrder.indexOf('/auth/refresh')).toBeLessThan(wireOrder.indexOf('/auth/logout'));
    });

    it('renews the fallback lock lease, so a slow-but-live refresh is never stolen', async () => {
      vi.useFakeTimers();
      try {
        const holder = new ApiClient();
        const waiter = new ApiClient();

        // Holder's refresh stays on the wire (the deferred keeps it parked far past the
        // lease). If the lease were static, the marker would lapse and the waiter could
        // steal a borrow that is still in use — recreating the single-use refresh-cookie
        // race. The holder must keep re-stamping it for as long as the request is live.
        const wireOrder: string[] = [];
        const refresh = deferred<{ access_token: string; expires_in?: number }>();
        recordingAdapterFor(holder, refresh, wireOrder);
        holder.setToken('a-token');
        const holdingRefresh = holder.get('/calendars').then(
          () => 'resolved',
          () => 'rejected'
        );
        // Let the holder acquire the lock (contention-window yield + poll) and reach
        // the wire with its refresh.
        await vi.advanceTimersByTimeAsync(200);
        expect(wireOrder).toContain('/auth/refresh');

        const seenWaiter: string[] = [];
        instanceOf(waiter).defaults.adapter = async config => {
          seenWaiter.push(config.url ?? '');
          wireOrder.push(config.url ?? '');
          return {
            data: { success: true, data: {} },
            status: 200,
            statusText: '200',
            headers: {},
            config: config as never,
          } as never;
        };
        let loggedOut = false;
        const logoutDone = waiter.logoutThroughLock(async () => {
          await waiter.post('/auth/logout');
          loggedOut = true;
        });

        // Advance well past the lease holding live. The renewal heartbeat must keep the
        // marker fresh, so the waiter stays parked instead of stealing the borrow.
        await vi.advanceTimersByTimeAsync(60_000);
        expect(seenWaiter).not.toContain('/auth/logout');
        expect(loggedOut).toBe(false);

        // The on-disk marker was re-stamped while held: far younger than the lease.
        const marker = localStorage.getItem('whento.authLock') ?? '';
        const markerAge = Date.now() - Number(marker.slice(marker.lastIndexOf(':') + 1));
        expect(markerAge).toBeLessThan(25_000);

        refresh.resolve({ access_token: 'a-rotated', expires_in: 900 });
        // The holder's release marks the lock free; under fake timers the waiting tab's
        // poll needs a clock tick to observe it and acquire.
        await vi.advanceTimersByTimeAsync(200);
        await holdingRefresh;
        await logoutDone;

        // Cookie operations stayed ordered; the slow holder finished before the waiter.
        expect(wireOrder.indexOf('/auth/refresh')).toBeLessThan(wireOrder.indexOf('/auth/logout'));
        expect(loggedOut).toBe(true);
      } finally {
        vi.useRealTimers();
      }
    });

    it('does not force a logout when a losing refresh gets a 401 after the fallback lock was taken over', async () => {
      // Two tabs dual-acquire the non-atomic fallback lock. The loser's /auth/refresh
      // is on the wire; the winner takes the marker over, then the loser's refresh
      // 401s. That 401 must NOT be read as "your session died": the 401 is a
      // side-effect of the race (the winner rotated the cookie), and forceLogout would
      // sign out the very session the winner is managing. It must instead discard the
      // refresh as stale without touching the session.
      const api = new ApiClient();
      const seen: string[] = [];
      const refreshFail = deferred<void>();
      instanceOf(api).defaults.adapter = (async config => {
        seen.push(config.url ?? '');
        const base = { headers: {}, config: config as never };
        const make401 = () => {
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
          return error;
        };
        if (config.url === '/auth/refresh') {
          await refreshFail.promise;
          throw make401();
        }
        throw make401();
      }) as AxiosAdapter;

      api.setToken('a-token');
      const outcome = api.get('/calendars').then(
        () => 'resolved',
        () => 'rejected'
      );
      // Let the losing client reach the wire with its refresh (holding the lock).
      await vi.waitFor(() => expect(seen).toContain('/auth/refresh'));

      // Another context takes the marker over while the refresh is on the wire.
      localStorage.setItem('whento.authLock', `another-tab:${Date.now()}`);

      refreshFail.resolve();
      await outcome;

      // The session survived: no forceLogout redirect, no token dropped, no replay.
      expect(api.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(seen.filter(url => url === '/calendars')).toHaveLength(1);
    });

    it('rejects a delayed token broadcast from a session family that has already logged out', async () => {
      const tabA = new ApiClient();
      const tabB = new ApiClient();
      const tabC = new ApiClient();

      // A signs in; C (another tab) follows through the channel.
      tabA.setToken('a-token');
      await new Promise(resolve => setTimeout(resolve, 20));
      expect(tabC.hasSession()).toBe(true);

      // B deliberately signs out: the origin-wide watermark rises above the session
      // epoch A's tokens were stamped with.
      tabB.signOut();
      await new Promise(resolve => setTimeout(resolve, 20));
      expect(tabA.hasSession()).toBe(false);
      expect(tabC.hasSession()).toBe(false);

      // An out-of-order token from the dead session arrives at C *after* the logout.
      channelHandler(tabC)?.({
        data: { type: 'token', token: 'stale-from-dead-session', expiresAt: null, epoch: 1 },
      } as MessageEvent);

      // C must remain signed out: the stale token never resurrects its session.
      expect(tabC.hasSession()).toBe(false);
      // And it cannot ride a request — no access token was re-installed.
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

      // All three tabs share session family 1; B and C follow A's token through the
      // channel (the way a tab only ever learns of a session is via setToken's broadcast).
      tabA.setToken('session-1-token');
      const aToken = posted[posted.length - 1] as { type: string; token: string; epoch: number };
      expect(aToken.type).toBe('token');
      channelHandler(tabB)?.({
        data: { type: 'token', token: aToken.token, expiresAt: null, epoch: aToken.epoch },
      } as MessageEvent);
      channelHandler(tabC)?.({
        data: { type: 'token', token: aToken.token, expiresAt: null, epoch: aToken.epoch },
      } as MessageEvent);
      expect(tabC.hasSession()).toBe(true);

      // A logs out session 1; B observes the logout (clearing its copy of the session),
      // then signs in again, allocating a fresh epoch above the logged-out watermark.
      tabA.signOut();
      const aLogout = posted[posted.length - 1] as { type: string; epoch: number };
      expect(aLogout.type).toBe('logout');
      channelHandler(tabB)?.({
        data: { type: 'logout', epoch: aLogout.epoch },
      } as MessageEvent);
      expect(tabB.hasSession()).toBe(false);

      tabB.setToken('session-2-token');
      const bToken = posted[posted.length - 1] as { type: string; token: string; epoch: number };
      expect(bToken.type).toBe('token');
      expect(bToken.epoch).toBeGreaterThan(aLogout.epoch);

      // C receives B's newer-session token *before* A's delayed logout — the valid
      // cross-sender reordering described above.
      channelHandler(tabC)?.({
        data: { type: 'token', token: bToken.token, expiresAt: null, epoch: bToken.epoch },
      } as MessageEvent);
      expect(tabC.hasSession()).toBe(true);

      // The delayed logout belongs to an older session family than the one C accepted,
      // so it must not clear the newer session.
      channelHandler(tabC)?.({
        data: { type: 'logout', epoch: aLogout.epoch },
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

    it('never replays a request whose refresh was invalidated while queued behind the lock', async () => {
      const holder = new ApiClient();
      const queued = new ApiClient();

      // Holder occupies the origin lock with a delayed refresh.
      const seenHolder: string[] = [];
      const refresh = deferred<{ access_token: string; expires_in?: number }>();
      let refreshStarted = false;
      recordingAdapterFor(holder, refresh, seenHolder, () => {
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
      // Let the queued refresh park itself behind the held lock.
      await new Promise(resolve => setTimeout(resolve, 100));

      // The session is replaced while the refresh is still queued.
      queued.clearToken();
      queued.setToken('b-token');

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
    it('schedules a refresh a minute short of expiry', async () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));

        // A fifteen-minute token: the refresh is due at fourteen.
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

        // The MFA and passkey paths used to hand over a token without one. The 401
        // path still covers those; this is an optimisation, not the mechanism.
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
        // clearToken must also dispose of the timer handle, not just let it expire.
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

        // A local token replacement (login, refresh) reschedules the proactive refresh.
        // The stale timer handle must be cleared, so only one stays pending.
        apiClient.setToken('second', 900);
        expect(vi.getTimerCount()).toBe(1);

        apiClient.setToken('third');
        expect(vi.getTimerCount()).toBe(0);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
      } finally {
        vi.useRealTimers();
      }
    });

    // A backgrounded tab does not get its timers on time — browsers throttle them and
    // suspend them outright on a discarded tab — so the wake-up is what covers a tab
    // left alone for an hour. Without it the user's first click is a 401 and a replay.
    it('refreshes on the way back from a sleeping tab', async () => {
      const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));

      // A token already inside the lead window, as one restored from sleep would be.
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
        // The protected call fails the first time and succeeds after the refresh.
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

    /**
     * The reason refreshToken shares one in-flight promise.
     *
     * A page load fires several requests at once — the calendar issues three — and each
     * one that 401s used to start its own refresh. That is a burst against an endpoint
     * rate limited to 5 per minute per IP, and worse, refresh *rotates* the token: the
     * last response could invalidate the token an earlier one had just stored, logging
     * the user out mid-session.
     */
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
      // The in-flight promise must be cleared, or the second expiry would reuse a
      // resolved one and never refresh again.
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
        return unauthorized(); // still 401 after the refresh
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

    // A 401 from /auth/bootstrap is a *wrong boot key*, not an expired session: the
    // bootstrap page must be left alone to render the error, not sign the visitor
    // out and bounce them to a login screen. The same reasoning applies to a wrong
    // password on login and a duplicate on register — reject, keep the session.
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

    // A wrong second-factor code is a *rejection* of the submitted credential, not an
    // expired session: `/auth/mfa/verify` authenticates with the body-carried pending
    // token and code. A 401 must be returned straight to the form — refreshing (which,
    // with no refresh cookie yet, itself 401s) would force a logout that clears the
    // still-valid pending login and bounces the user back to the password/passkey stage.
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
        // /verify-mfa is a public route: the visitor has no access token yet and may not
        // even have a refresh cookie, so a refresh attempt would also 401.
        routeMeta.public = true;

        let mfaCalls = 0;
        const seen = withAdapter(config => {
          if (config.url === '/auth/mfa/verify') {
            mfaCalls += 1;
            // First submission is rejected as a wrong code; a corrected retry succeeds.
            return mfaCalls === 1
              ? unauthorized()
              : ok({ access_token: 'access-token', expires_in: 3600, user: { id: 'u-1' } });
          }
          if (config.url === '/auth/refresh') return unauthorized();
          return ok({});
        });

        await expect(
          apiClient.post('/auth/mfa/verify', { temp_token: 'valid-pending-token', code: '000000' })
        ).rejects.toBeTruthy();

        // No refresh was attempted, the pending login survived, and nothing navigated or
        // signed out.
        expect(seen.map(r => r.url)).toEqual(['/auth/mfa/verify']);
        expect(authStore.tempToken).toBe('valid-pending-token');
        expect(window.location.href).toBe('');
        expect(apiClient.hasSession()).toBe(false);
        expect(authStore.user).toBeNull();

        // A corrected code can still complete the same pending login.
        await expect(
          apiClient.post('/auth/mfa/verify', { temp_token: 'valid-pending-token', code: '123456' })
        ).resolves.toEqual({ access_token: 'access-token', expires_in: 3600, user: { id: 'u-1' } });
        expect(seen.map(r => r.url)).toEqual(['/auth/mfa/verify', '/auth/mfa/verify']);
      } finally {
        unsubscribe();
      }
    });

    // A rejected WebAuthn assertion on `/auth/passkey/login/finish` is a *rejection* of
    // the submitted credential, not an expired access session: the backend returns 401
    // for ErrPasskeyNotFound/ErrInvalidCredential. A 401 must be returned straight to
    // the login flow — refreshing cannot repair a rejected credential, and with no
    // refresh cookie the forced logout would raise the origin-wide logout watermark and
    // discard another tab's still-held session (plus clear local account state on the
    // public login route). With a healthy session it would needlessly rotate it and
    // resubmit the same rejected assertion.
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

        // Only the passkey finish was sent — no refresh, no replay — and nothing was
        // broadcast or signed out.
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

    // A refresh that answers with an HTTP 5xx (or a network error, or a timeout) is NOT
    // a session verdict: the backend only answers 401 for a genuinely refused refresh
    // cookie; anything else is a transient server fault. Signing out then would turn a
    // temporary outage into origin-wide session loss. The session must survive, the
    // triggering request must reject with the recoverable failure, and nothing may be
    // broadcast or navigated.
    it('preserves the session when the refresh fails transiently (500 from the backend)', async () => {
      await drainChannel();
      // A stale logout broadcast consumed above may have redirected; reset to a clean slate.
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

    // A *proactive* (direct) refresh — the one the timer or a cold load fires, not one
    // provoked by a 401 — that hits a transient backend fault must not destroy the
    // session either. Before this fix the 500 surfaced as a plain rejection the caller
    // could only swallow, which was fine; the danger was the ordinary-request retry path
    // forceLogouting on it. This pins the direct path: the awaiting request rejects with
    // the recoverable failure and the session survives.
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
      // Without the query, signing back in always landed on the dashboard however deep
      // the page the session expired on. The guard already produces this shape for an
      // anonymous visitor; a mid-session expiry now matches it.
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

      // Stand in for the Web Lock: while the second tab waits its turn, the winning
      // tab's token arrives over the channel. Spending the cookie again here would
      // rotate away the token we were just handed and sign both tabs out.
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
      // A participant following a calendar link is not signed in and must not be
      // bounced to a login page they have no account for.
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
        // A visitor on a public calendar link may still be holding a (now expired)
        // session. The refresh cookie is dead, but there is no account to sign back
        // in to, so the client must neither navigate nor leave the expired account
        // rendered as the owner.
        routeMeta.public = true;
        apiClient.setToken('expired');

        withAdapter(() => unauthorized());

        await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

        expect(apiClient.hasSession()).toBe(false);
        expect(window.location.href).toBe('');
        // The stale user id and its matching calendarsForUser marker would otherwise
        // still pass the public view's ownership check; the config would keep the old
        // account's capability URL. All of it must be gone.
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
    // A request that 401s after the session that issued it has been replaced must not
    // refresh the replacement session, replay the old request with its token, or sign
    // the replacement out. These tests hold the wire response open, swap the session
    // underneath, and only then release the error — the timing the in-flight
    // generation guards cannot see, because the session changed before the response
    // landed.

    function immediateLocks() {
      vi.stubGlobal('navigator', {
        locks: { request: (_name: string, fn: () => unknown) => fn() },
      });
    }

    /** A full 2xx Axios response, because the interceptor retries through real axios. */
    function makeResponse(config: AxiosRequestConfig, data: unknown) {
      return {
        status: 200,
        statusText: 'OK',
        headers: {},
        config: config as never,
        data: { success: true, data },
      } as never;
    }

    /** A full 401 Axios error standing in for the wire rejecting an expired token. */
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

    /** A full 500 Axios error standing in for a server fault (not an auth signal). */
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
      // Failure B analog for the catch path, not the 401 branch: A's request 401s and
      // provokes a refresh; while that refresh is on the wire the session is replaced
      // with B; then the refresh fails with a *server fault* (500, network). The
      // refresh failure belongs to the old session, so it must not reach the
      // interceptor's forceLogout and sign B out — it must be voided as stale instead.
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
      // Wait until A's refresh is on the wire before swapping the session.
      await vi.waitFor(() => expect(wireOrder).toContain('/auth/refresh'));

      apiClient.clearToken();
      apiClient.setToken('b-token', undefined, 'family-b');
      logoutBroadcasts.length = 0; // forget B's own token broadcast below
      held.resolve();

      await pending;

      // B's session survives the stale A 500: no forced logout, no logout broadcast,
      // no login redirect.
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
        // If the interceptor wrongly replays after a refresh, answer the refresh with
        // B's rotation and the replay with a created calendar so we can observe it.
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

      // Replace A's session with B while A's request is still on the wire.
      apiClient.clearToken();
      apiClient.setToken('b-token', undefined, 'family-b');
      held.resolve();

      const outcome = await pending;
      expect({ outcome, seen }).toEqual({
        // The old request is rejected, and never sent again — least of all with B's token.
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

      // Replace A's session with B while A's refresh is still on the wire.
      apiClient.clearToken();
      apiClient.setToken('b-token', undefined, 'family-b');
      logoutBroadcasts.length = 0; // forget B's own token broadcast below
      held.resolve();

      await pending;

      // B's session survives the stale A refresh: no forced logout, no logout broadcast,
      // no login redirect.
      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(logoutBroadcasts.filter(m => (m as { type?: string })?.type === 'logout')).toEqual([]);
    });

    it('does not sign out a replacement session when a cold-start refresh answers 401', async () => {
      // Audit 28 F1: a refresh whose *purpose* is to obtain the first in-memory token
      // (no access token exists yet) must still be fenced. With no token the request
      // interceptor used to leave /auth/refresh unstamped, so when another tab
      // installed a replacement session while it was pending, the delayed 401 reached
      // forceLogout and signed the replacement out. Every request is now stamped, so
      // the stale refresh is rejected without touching the replacement session.
      routeMeta.public = true;
      immediateLocks();

      const started = deferred<void>();
      const held = deferred<void>();
      const logoutBroadcasts: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => {
        logoutBroadcasts.push(m);
      });
      const channelHandler = (apiClient as unknown as { channel: BroadcastChannel }).channel
        .onmessage as ((event: MessageEvent) => void) | undefined;
      expect(channelHandler).toBeTypeOf('function');

      instance().defaults.adapter = (async config => {
        started.resolve();
        await held.promise;
        throw make401(config);
      }) as AxiosAdapter;

      // Cold start: no access token in memory, only the refresh cookie.
      const pending = apiClient.refreshToken().catch(() => undefined);
      await started.promise;

      // A remote tab (or a restore) installs a valid session while the refresh is on
      // the wire: the real token-message handler advances the session generation.
      channelHandler?.({
        data: { type: 'token', token: 'b-token', nonce: 'family-b', epoch: Date.now() + 1000 },
      } as MessageEvent);
      logoutBroadcasts.length = 0; // forget B's own token broadcast below
      held.resolve();

      await pending;

      // B's session survives the stale cold refresh: no forced logout, no logout
      // broadcast, no login redirect.
      expect(apiClient.hasSession()).toBe(true);
      expect(window.location.href).toBe('');
      expect(logoutBroadcasts.filter(m => (m as { type?: string })?.type === 'logout')).toEqual([]);
    });

    it('cancels a queued replay when the session is replaced before its final dispatch', async () => {
      // Defense in depth (audit 28 hardening observation): a retry that was queued
      // behind an asynchronous interceptor for session A must not be downgraded to
      // "current token" if B replaces A before the retry actually dispatches. The
      // saved stamp from the first dispatch is checked at the final token-attachment
      // point, so the stale replay is cancelled rather than sent with B's token.
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
      // A's request 401s and its retry is queued behind the async interceptor.
      await replayQueued.promise;

      // B replaces A while the retry is queued.
      apiClient.clearToken();
      apiClient.setToken('b-token', undefined, 'family-b');
      release.resolve();

      const outcome = await pending;
      expect({ outcome, seen }).toEqual({
        // The stale replay is cancelled: no third request with B's token.
        outcome: 'rejected',
        seen: [
          { url: '/calendars', auth: 'Bearer a-token' },
          { url: '/auth/refresh', auth: 'Bearer a-token' },
        ],
      });
    });
  });

  describe('error normalisation', () => {
    it('surfaces the server error envelope', async () => {
      withAdapter(() => ({
        status: 409,
        data: { success: false, error: { code: 'CONFLICT', message: 'Already exists' } },
      }));

      await expect(apiClient.post('/calendars', {})).rejects.toMatchObject({
        code: 'CONFLICT',
        message: 'Already exists',
      });
    });

    it('falls back when the response carries no envelope', async () => {
      withAdapter(() => ({ status: 500, data: 'plain text' }));

      await expect(apiClient.get('/calendars')).rejects.toMatchObject({
        message: expect.stringContaining('500'),
      });
    });
  });
});
