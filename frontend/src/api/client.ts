/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import axios, {
  type AxiosInstance,
  type AxiosError,
  type InternalAxiosRequestConfig,
  type AxiosRequestConfig,
} from 'axios';
import type { ApiResponse, ApiError } from '@/types';
import router from '@/router';
import { REMOTE_SIGNOUT_EVENT, REMOTE_SESSION_EVENT } from '@/sessionEvents';
import { isDefinitiveRejection } from '@/api/failureClassification';

/**
 * The three concepts that describe a browser session, kept deliberately apart:
 *
 *  1. **Server family / session id** (`session_id` on every auth response). Identity of
 *     a refreshable login session: stable across refresh, brand-new on a new login. The
 *     client holds it in memory, shares it over the BroadcastChannel, and it alone
 *     tells "a newer token for the session I already have" from "a new login".
 *  2. **Access token + absolute expiry**. Short-lived credential state, updated in
 *     memory when a token for the same family rotates; the proactive refresh timer is
 *     replaced with the newer token's due time.
 *  3. **Local account generation** (`sessionGeneration` here; `accountFence.ts` for the
 *     stores). An in-memory fence that invalidates old async work — queued retries,
 *     in-flight refreshes, delayed responses — after a logout or account replacement.
 *
 * A same-family rotation is *not* a new login: it replaces the token and restarts the
 * refresh timer without clearing the account stores, fetching /auth/me again, bumping
 * the generation, navigating or re-broadcasting. A different finalized family, or an
 * explicit logout, advances the generation and fences all pending work.
 */

/** Marks that this browser has a session; never holds the token itself. */
const SESSION_FLAG = 'whento.session';

/**
 * The family id this browser *deliberately signed out of*, the non-secret logout marker.
 *
 * Session identity is the server's family id, not a locally guessed counter, so a
 * single marker — the most recent deliberate logout — is enough for a cold tab to tell
 * "a delayed token from the session we signed out of" from "a brand-new login" (which,
 * on this server, always carries a freshly allocated family). It is not a credential:
 * knowing a family id does not refresh anything (the httpOnly cookie does), which is
 * why it may live in storage while the JWT may not.
 */
const LOGGED_OUT_FAMILY_KEY = 'whento.loggedOutFamily';

/** Latest server-confirmed login family. Non-secret; never an access token. */
const ACTIVE_FAMILY_KEY = 'whento.activeFamily';

function readActiveFamily(): string | null {
  if (typeof window === 'undefined') return null;
  try {
    return normalizeFamily(localStorage.getItem(ACTIVE_FAMILY_KEY));
  } catch {
    return null;
  }
}

function writeActiveFamily(family: string | null) {
  if (typeof window === 'undefined') return;
  try {
    if (family === null) localStorage.removeItem(ACTIVE_FAMILY_KEY);
    else localStorage.setItem(ACTIVE_FAMILY_KEY, family);
  } catch {
    // Storage denied: retired families still fence this tab in memory.
  }
}

/** Names the cross-tab channel and the cross-tab cookie lock. */
const AUTH_CHANNEL = 'whento.auth';
const COOKIE_LOCK = 'whento.cookie';

const SESSION_ISSUANCE_PATHS = new Set([
  '/auth/login',
  '/auth/register',
  '/auth/bootstrap',
  '/auth/mfa/verify',
  '/auth/passkey/login/finish',
  '/auth/magic-link/verify',
  '/auth/reset-password',
]);

/**
 * How far ahead of expiry to refresh.
 *
 * Wide enough that a slow round trip still lands before the token dies, narrow enough
 * that it is a rounding error against the fifteen-minute lifetime.
 */
const REFRESH_LEAD_MS = 60_000;

/**
 * The soonest a scheduled refresh may fire.
 *
 * A token already inside the lead window — or, through a misconfigured server, shorter
 * than it — would otherwise schedule at zero and spin.
 */
const MIN_REFRESH_DELAY_MS = 5_000;

type AuthMessage =
  | { type: 'token'; token: string; expiresAt?: number | null; family?: string | null }
  | { type: 'logout'; family?: string | null };

/** A family id is a non-empty string or nothing (an older peer, a token without one). */
function normalizeFamily(value: unknown): string | null {
  return typeof value === 'string' && value.length > 0 ? value : null;
}

/** The family this browser deliberately signed out of, if any. Never secret. */
function readLoggedOutFamily(): string | null {
  if (typeof window === 'undefined') return null;
  try {
    return localStorage.getItem(LOGGED_OUT_FAMILY_KEY);
  } catch {
    return null;
  }
}

/** Record a deliberate sign-out so late tokens for that family stay rejected. */
function writeLoggedOutFamily(family: string) {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(LOGGED_OUT_FAMILY_KEY, family);
  } catch {
    // Storage denied: the in-memory signed-out set still fences this tab.
  }
}

/**
 * The session-generation stamp carried on an outbound request.
 *
 * Set by the request interceptor on the first dispatch and preserved through a
 * retry, so a delayed 401 (or refresh failure) can be told apart from one that
 * belongs to the session which has since replaced it. `_retry` is pre-existing
 * axios-common retry bookkeeping.
 */
interface SessionStampedRequest extends InternalAxiosRequestConfig {
  _whentoSessionGeneration?: number;
  _retry?: boolean;
}

/**
 * Signals that a refresh was voided while it was in flight.
 *
 * Thrown by `performRefresh` when the session generation moved, or the server family
 * changed, between the moment the refresh captured them and the moment its response
 * landed — a deliberate logout, a cross-tab sign-out or a forced expiry. It is a
 * *cancellation*, not a failure: the 401 interceptor must neither replay the request
 * that provoked the refresh (that would attach the replacing session's token to a
 * request the old session issued) nor force the replacing session out.
 */
class StaleRefreshError extends Error {}

/**
 * Signals that a refresh failed for a *transient* reason — an HTTP 5xx, a network
 * error, a timeout — rather than because the session itself is dead.
 *
 * The backend maps a genuine rejection of the presented refresh cookie to 401;
 * everything else is infrastructure, and the cookie may still be valid. The
 * ordinary-request 401 retry path must therefore preserve the session on this error,
 * reject the triggering request with the recoverable failure, and let the caller
 * retry — signing a user out over a server outage turns an avoidable blip into
 * session loss.
 */
class TransientRefreshError extends Error {
  readonly originalError: unknown;

  constructor(originalError: unknown) {
    super(originalError instanceof Error ? originalError.message : String(originalError));
    this.name = 'TransientRefreshError';
    this.originalError = originalError;
  }
}

/**
 * What the 401 interceptor reports for a request whose refresh was voided by a session
 * change: the request genuinely never completed, and no usable token exists for it.
 */
const STALE_SESSION_ERROR: ApiError = {
  code: 'UNAUTHORIZED',
  message: 'The session changed while the request was pending',
};

/**
 * Credential-validation endpoints whose HTTP 401 is a *rejection* of the submitted
 * credential, not an expired access session: bad credentials on login, a duplicate on
 * register, a wrong boot key on bootstrap, a wrong second-factor code on MFA verify, a
 * rejected WebAuthn assertion on passkey login finish, an invalid/expired magic link or
 * password-reset token, or a logout whose refresh cookie was already gone. A 401 from
 * any of them must be returned straight to the form that sent it — never followed by a
 * refresh, a replay, or a sign-out. A failed register attempt in one tab must not log a
 * healthy other tab out; a mistyped MFA code must not clear the still-valid pending
 * login.
 *
 * That a 401 here must never refresh also matters because these endpoints run under
 * the same cookie-mutation lock as the refresh (they set or revoke the refresh cookie):
 * invoking the refresh interceptor from under the lock would deadlock.
 *
 * Matched by exact path (query string aside), not substring, so an endpoint that merely
 * shares a prefix, e.g. the protected `/auth/me`, is never exempted from a legitimate
 * access-session refresh.
 */
const CREDENTIAL_REJECTION_PATHS = new Set([
  '/auth/login',
  '/auth/register',
  '/auth/bootstrap',
  '/auth/logout',
  '/auth/mfa/verify',
  '/auth/passkey/login/finish',
  '/auth/magic-link/verify',
  '/auth/reset-password',
]);

/** The request's path with any query string removed, for exact-endpoint matching. */
function requestPath(url: string | undefined): string | undefined {
  if (!url) return undefined;
  return url.split('?')[0];
}

/**
 * The HTTP client. Exported so tests can construct isolated instances (multi-tab
 * session fencing) alongside the app-wide singleton below.
 */
export class ApiClient {
  private client: AxiosInstance;
  private accessToken: string | null = null;
  /** The one refresh in progress, shared by every caller that needs it. */
  private refreshInFlight: Promise<void> | null = null;
  private channel: BroadcastChannel | null = null;
  /** The pending proactive refresh, if the current token carried an expiry. */
  private refreshTimer: ReturnType<typeof setTimeout> | null = null;
  /** When the current token dies, so a tab returning from sleep can tell. */
  private expiresAt: number | null = null;
  /**
   * The server family/session id of the refresh cookie this tab's tokens belong to.
   * `null` until the first auth response carries one. Stable across refresh; new on a
   * new login. A token message whose family equals this is a rotation of the session
   * already held; any other (non-null) family is a replacement session.
   */
  private familyId: string | null = null;
  /**
   * The client session generation. Every sign-out and every adoption of a different
   * server family bumps it so that an in-flight refresh that started before the
   * boundary can never resurrect the old session: the refresh accepts and broadcasts
   * a token only if the generation it captured is still current when the response
   * lands, and a retry queued under the previous generation is never dispatched with
   * the replacement session's token.
   */
  private sessionGeneration = 0;
  /**
   * Families this tab has *signed out of* (its own sign-out, or a logout message from
   * another tab), or superseded by another login. A delayed token from one must never resurrect the
   * session, no matter how late it arrives.
   */
  private signedOutFamilies = new Set<string>();
  /**
   * In-process serialisation for every cookie-changing operation — the refresh, the
   * explicit login/bootstrap/register/MFA/magic-link/password-reset session issuance
   * and the deliberate logout. One operation at a time per client even where
   * `navigator.locks` does not exist; across tabs the server's session-family and
   * refresh-grace protection is the authority (cross-tab Set-Cookie ordering is simply
   * not guaranteed without Web Locks, so a late response is discarded against the
   * finalized family rather than committed).
   */
  private cookieMutex: Promise<unknown> = Promise.resolve();
  private removeStorageListener: (() => void) | null = null;
  private removeWakeListeners: (() => void) | null = null;
  private disposed = false;

  /** Whether this instance has been disposed (tests/HMR); nothing runs on a disposed client. */
  get isDisposed(): boolean {
    return this.disposed;
  }

  constructor() {
    this.client = axios.create({
      baseURL: '/api/v1',
      headers: {
        'Content-Type': 'application/json',
      },
      timeout: 30000,
      withCredentials: true,
    });

    this.setupInterceptors();
    this.setupChannel();
    this.removeStorageListener = this.setupStorageFallback();
    this.setupWakeUp();
  }

  /**
   * Refresh on the way back from sleep, rather than on the user's next click.
   *
   * A backgrounded tab does not get its timers on time — browsers throttle them heavily
   * and suspend them outright on a discarded tab — so a tab left alone for an hour wakes
   * with a token that expired long ago. Waiting for the next request means the first
   * thing the user does after coming back is a 401, a refresh and a replay.
   *
   * Both events are cheap and idempotent: refreshIfDue does nothing while the token has
   * life left in it.
   */
  private setupWakeUp() {
    if (typeof window === 'undefined') {
      return;
    }

    const onVisibility = () => {
      if (!document.hidden) {
        this.refreshIfDue();
      }
    };
    const onOnline = () => this.refreshIfDue();
    document.addEventListener('visibilitychange', onVisibility);
    window.addEventListener('online', onOnline);
    this.removeWakeListeners = () => {
      document.removeEventListener('visibilitychange', onVisibility);
      window.removeEventListener('online', onOnline);
    };
  }

  /** Refresh when the token is at or past its lead window. Silent on failure: the 401 path remains. */
  private refreshIfDue() {
    if (this.disposed || !this.accessToken || this.expiresAt === null) {
      return;
    }
    if (this.expiresAt - Date.now() > REFRESH_LEAD_MS) {
      return;
    }

    void this.refreshToken().catch(() => {
      // Already handled by the interceptor's forced sign-out; nothing to add here,
      // and an unhandled rejection would reach the global handler in main.ts.
    });
  }

  /**
   * Replace the pending proactive refresh.
   *
   * Cleared and reset on every token, so the timer always describes the token in hand
   * rather than the one before it.
   */
  private scheduleRefresh(expiresInSeconds?: number) {
    this.scheduleRefreshAt(expiresInSeconds ? Date.now() + expiresInSeconds * 1000 : null);
  }

  /**
   * Replace the pending proactive refresh from an absolute expiration time (ms).
   *
   * Shares the schedule with `scheduleRefresh`; the receiving side of the cross-tab
   * channel uses it because it is handed an absolute `expiresAt`, not seconds.
   * Clearing the old timer matters: a token just received over the channel has its own
   * due time, and the stale one must not fire a redundant refresh that would spend the
   * single-use refresh cookie against the token we were just given.
   */
  private scheduleRefreshAt(expiresAtMs: number | null) {
    if (this.refreshTimer !== null) {
      clearTimeout(this.refreshTimer);
      this.refreshTimer = null;
    }

    if (expiresAtMs === null || expiresAtMs <= Date.now()) {
      // No expiry (the MFA and passkey paths hand over a token without one) or already
      // dead. The 401 path covers both; this is an optimisation, not the mechanism.
      this.expiresAt = expiresAtMs;
      return;
    }

    this.expiresAt = expiresAtMs;
    const delay = Math.max(expiresAtMs - Date.now() - REFRESH_LEAD_MS, MIN_REFRESH_DELAY_MS);

    this.refreshTimer = setTimeout(() => {
      this.refreshTimer = null;
      // Browser timer rounding may wake us just before the lead window. Do not
      // lose proactive refresh permanently after that harmless early callback.
      if (this.expiresAt !== null && this.expiresAt - Date.now() > REFRESH_LEAD_MS) {
        this.scheduleRefreshAt(this.expiresAt);
        return;
      }
      this.refreshIfDue();
    }, delay);
  }

  private setSessionFlag() {
    if (typeof window === 'undefined') return;
    try {
      localStorage.setItem(SESSION_FLAG, '1');
    } catch {
      // Storage denied: hasSession() falls back to the in-memory token.
    }
  }

  private removeSessionFlag() {
    if (typeof window === 'undefined') return;
    try {
      localStorage.removeItem(SESSION_FLAG);
    } catch {
      // Storage denied.
    }
  }

  /**
   * Share tokens and sign-outs with the other tabs on this origin.
   *
   * Refresh tokens are single-use — the backend deletes the old one before issuing the
   * next (auth_service.go, `RefreshToken`). Tabs each hold their own in-memory access
   * token, so restoring a window full of pinned tabs used to fire one `/auth/refresh`
   * per tab against the same cookie: the first rotated it and the rest got a 401 and a
   * forced sign-out. Whichever tab wins the cookie lock now passes its result to the
   * others.
   *
   * The message carries the server family id and absolute expiry so a receiver can tell
   * a same-family rotation (replace the token, keep everything) from a replacement
   * session (advance the fence, reset account stores, re-read /auth/me) without ever
   * inventing an epoch. Nothing is persisted — tokens live in memory only.
   */
  private setupChannel() {
    if (typeof BroadcastChannel === 'undefined') {
      return;
    }

    this.channel = new BroadcastChannel(AUTH_CHANNEL);
    this.channel.onmessage = (event: MessageEvent<AuthMessage>) => {
      if (this.disposed) return;
      const message = event.data;
      if (!message || typeof message !== 'object') return;
      if (message.type === 'token') {
        this.acceptRemoteToken(message);
      } else if (message.type === 'logout') {
        this.acceptRemoteLogout(message);
      }
    };
  }

  /**
   * Accept a token broadcast by another tab.
   *
   * The incoming message is validated before anything is installed: it must be a
   * non-empty string token, not already expired, and not painted with a family this
   * browser has signed out of (or the shared logout marker). A token for the family we
   * already hold is a plain rotation — installed silently, no store reset, no /auth/me,
   * no generation bump, no navigation, no re-broadcast. A token for *any other* family
   * is a replacement session: the account generation is advanced (fencing pending
   * work), the token installed and the account stores are told to hydrate through the
   * server. A receiver never echoes a received message.
   */
  private acceptRemoteToken(message: Extract<AuthMessage, { type: 'token' }>) {
    // Validate presence: a malformed or empty token is not authorisation.
    if (typeof message.token !== 'string' || message.token.length === 0) {
      return;
    }
    // Validate expiration: never install what is already dead.
    if (
      typeof message.expiresAt === 'number' &&
      Number.isFinite(message.expiresAt) &&
      message.expiresAt <= Date.now()
    ) {
      return;
    }
    const family = normalizeFamily(message.family ?? null);

    // A cold tab may never have held the old family. The shared, server-confirmed
    // family also fences messages queued before the latest login in another tab.
    const activeFamily = readActiveFamily();
    if (activeFamily !== null && family !== activeFamily) return;
    if (this.familyId !== null && family === null) return;

    // A token for a family this browser deliberately signed out of must not resurrect
    // the session — even when it arrives long after the logout that ended it.
    if (family !== null) {
      if (this.signedOutFamilies.has(family)) return;
      const marker = readLoggedOutFamily();
      if (marker !== null && family === marker && family !== this.familyId) return;
    }

    // A tab with no session at all adopts the first valid token as a brand-new session.
    if (this.accessToken === null && this.familyId === null) {
      this.sessionGeneration += 1;
      this.familyId = family;
      this.accessToken = message.token;
      this.scheduleRefreshAt(message.expiresAt ?? null);
      this.setSessionFlag();
      this.dispatchRemoteSession(family);
      return;
    }

    // A different non-null family replaces this tab's session: advance the generation so
    // every request/refresh issued under the previous family is voided, then tell the
    // account stores to reset and confirm the identity through /auth/me exactly once.
    if (family !== null && family !== this.familyId) {
      if (this.familyId !== null) this.signedOutFamilies.add(this.familyId);
      this.sessionGeneration += 1;
      this.familyId = family;
      this.accessToken = message.token;
      this.scheduleRefreshAt(message.expiresAt ?? null);
      this.setSessionFlag();
      this.dispatchRemoteSession(family);
      return;
    }

    // Same family (or a family-less token while we hold a session): a plain rotation.
    this.accessToken = message.token;
    this.scheduleRefreshAt(message.expiresAt ?? null);
    this.setSessionFlag();
  }

  /**
   * Accept a logout broadcast by another tab.
   *
   * A logout for a family that is *not* the one this tab currently accepts is ignored:
   * a delayed logout from an older session must never clear a subsequently finalized
   * new one. A logout for the accepted family (or for none, while this tab holds one)
   * clears the local session — dropping the token, timer, family and session flag, and
   * advancing the generation so an in-flight refresh cannot re-seed it. The family is
   * remembered as signed out, and, on a public route, the account stores are reset in
   * place (there is no login page to reload into). No re-broadcast: the sender already
   * told everyone.
   */
  private acceptRemoteLogout(message: Extract<AuthMessage, { type: 'logout' }>) {
    const family = normalizeFamily(message.family ?? null);
    const activeFamily = readActiveFamily();
    if (family !== null && activeFamily !== null && family !== activeFamily) return;
    if (family !== null) {
      if (this.familyId !== null && family !== this.familyId) {
        return;
      }
      this.signedOutFamilies.add(family);
    }
    this.clearToken();
    if (router.currentRoute.value.meta.public === true) {
      this.resetLocalStateIfPublic();
      return;
    }
    this.redirectToLogin();
  }

  /**
   * Without a BroadcastChannel, peers still need to learn about a deliberate sign-out.
   *
   * The `whento.loggedOutFamily` marker doubles as a storage notification: when another
   * tab signs out it rewrites the marker, and every tab watching `storage` sees the
   * change and clears its own copy of that family. Token passing across tabs is not
   * possible without a channel, so the other tabs hydrate through the server the normal
   * way — the shared refresh cookie and the 401 path. No JWT ever touches storage.
   */
  private setupStorageFallback(): (() => void) | null {
    if (typeof window === 'undefined') return null;
    const onStorage = (event: StorageEvent) => {
      if (this.disposed) return;
      if (event.key === ACTIVE_FAMILY_KEY && this.channel === null) {
        const family = normalizeFamily(event.newValue);
        if (family === null || family === this.familyId) return;
        if (this.signedOutFamilies.has(family)) return;
        if (this.familyId !== null) this.signedOutFamilies.add(this.familyId);
        this.sessionGeneration += 1;
        this.familyId = family;
        this.accessToken = null;
        this.scheduleRefreshAt(null);
        this.setSessionFlag();
        // No credentials travel through storage. Reset the old account first;
        // the receiver hydrates using the shared httpOnly cookie and /auth/me.
        this.dispatchRemoteSession(family);
        return;
      }
      if (event.key !== LOGGED_OUT_FAMILY_KEY) return;
      const family = normalizeFamily(event.newValue ?? null);
      if (family === null || family === event.oldValue) return;
      const activeFamily = readActiveFamily();
      if (activeFamily !== null && activeFamily !== family) return;
      // Another tab deliberately signed that family out. If it is the family we hold
      // (or we hold none), end this tab's copy of the session without touching the
      // peer's newer one — a delayed logout for an old family cannot clear a new one.
      if (this.familyId !== null && family !== this.familyId) return;
      this.signedOutFamilies.add(family);
      this.clearToken();
      if (router.currentRoute.value.meta.public === true) {
        this.resetLocalStateIfPublic();
        return;
      }
      this.redirectToLogin();
    };
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }

  /**
   * Tell the application that a replacement session was accepted, so it can drop the
   * previous account's owner-scoped stores and confirm the identity through /auth/me.
   *
   * The awaited family travels with the event so the handler can fence its round trip
   * against the family it was issued for — a late /auth/me for an older session must
   * not overwrite a newer one that has since been accepted.
   */
  private dispatchRemoteSession(family: string | null) {
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent(REMOTE_SESSION_EVENT, { detail: { family } }));
    }
  }

  private broadcast(message: AuthMessage) {
    this.channel?.postMessage(message);
  }

  private setupInterceptors() {
    // Request interceptor - add auth token
    this.client.interceptors.request.use(
      (config: InternalAxiosRequestConfig) => {
        // Stamp *every* request — including the cold-start /auth/refresh whose purpose
        // is to obtain the first in-memory token — with the session generation it was
        // dispatched under. A delayed 401 for a request issued by a session that has
        // since been replaced must not refresh the replacement session, replay the old
        // request with its token, or (for a refresh) reach unconditional logout.
        const stamped = config as SessionStampedRequest;
        if (stamped._whentoSessionGeneration === undefined) {
          stamped._whentoSessionGeneration = this.sessionGeneration;
        } else if (stamped._whentoSessionGeneration !== this.sessionGeneration) {
          // A retry queued (e.g. behind an asynchronous interceptor) for a session that
          // has since been replaced must not be dispatched under the replacement
          // session's token.
          return Promise.reject(STALE_SESSION_ERROR);
        }
        if (this.accessToken && config.headers) {
          config.headers.Authorization = `Bearer ${this.accessToken}`;
        }
        return config;
      },
      error => Promise.reject(error)
    );

    // Response interceptor - handle errors
    this.client.interceptors.response.use(
      response => response,
      async (error: AxiosError<ApiResponse<never>>) => {
        const originalRequest = error.config;

        // Bootstrap authenticates the operator key, not the browser's session.
        // Its 401 must preserve both this tab's session and every other tab's.
        const path = requestPath(originalRequest?.url);
        const isRejectedAuth = path !== undefined && CREDENTIAL_REJECTION_PATHS.has(path);
        const isRefresh = path === '/auth/refresh';

        // If 401 and not already retrying, try to refresh token (except for auth endpoints)
        if (
          error.response?.status === 401 &&
          originalRequest &&
          !(originalRequest as SessionStampedRequest)._retry &&
          !isRejectedAuth &&
          !isRefresh
        ) {
          const sessionAtDispatch = (originalRequest as SessionStampedRequest)
            ?._whentoSessionGeneration;
          if (sessionAtDispatch !== undefined && sessionAtDispatch !== this.sessionGeneration) {
            return Promise.reject(STALE_SESSION_ERROR);
          }
          (originalRequest as SessionStampedRequest)._retry = true;

          try {
            await this.refreshToken();
            return this.client(originalRequest);
          } catch (failure) {
            // A refresh voided by a session change (logout or a new sign-in while it
            // was in flight) must neither replay the original request — replaying
            // would attach the replacing session's token to a request the previous
            // session issued — nor force the replacing session out.
            if (failure instanceof StaleRefreshError) {
              return Promise.reject(STALE_SESSION_ERROR);
            }
            // A transient refresh failure is not evidence the session died.
            if (failure instanceof TransientRefreshError) {
              return Promise.reject(failure.originalError);
            }
            if (isDefinitiveRejection(failure)) {
              this.forceLogout();
            }
            return Promise.reject(failure);
          }
        }

        // If 401 on /auth/refresh (or spawned by one), force logout
        if (error.response?.status === 401 && isRefresh) {
          const sessionAtDispatch = (originalRequest as SessionStampedRequest)
            ?._whentoSessionGeneration;
          if (sessionAtDispatch !== undefined && sessionAtDispatch !== this.sessionGeneration) {
            // The refresh answered for a session that has since been replaced: its 401
            // is a side-effect of that replacement (the old refresh cookie was already
            // rotated away), not evidence the replacement session died.
            throw new StaleRefreshError('refresh answered after the session changed');
          }
          this.forceLogout();
        }

        // Any failure of our own refresh that is *not* a definitive 401 is transient.
        if (isRefresh && error.response?.status !== 401) {
          return Promise.reject(new TransientRefreshError(this.normalizeError(error)));
        }

        return Promise.reject(this.normalizeError(error));
      }
    );
  }

  private forceLogout() {
    this.signOut();
    // BroadcastChannel does not echo a posted message back to the tab that posted it,
    // so this tab would never see the "logout" broadcast it just sent. On a public
    // route there is no login page to reload into, so the account stores have to be
    // reset in place — exactly as an incoming cross-tab logout resets them.
    this.resetLocalStateIfPublic();
    this.redirectToLogin();
  }

  /**
   * Raise the account-scoped reset event when the session dies on a public route.
   *
   * The reset itself lives above this client (stores/remoteSignout via main.ts).
   * On a private route it is not needed: `forceLogout` redirects to /login, and that
   * redirect is a full page load which drops every Pinia store with it.
   */
  private resetLocalStateIfPublic() {
    if (router.currentRoute.value.meta.public !== true) return;
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent(REMOTE_SIGNOUT_EVENT));
    }
  }

  /**
   * End the session in this tab and in every other one.
   *
   * The deliberate sign-out goes through here rather than through clearToken, which
   * stays local on purpose: the error paths call it too — a refused /auth/me, a
   * failed restore — and a transient failure in one tab must not sign the others out.
   * Choosing to log out is different, and has to reach them all at once. The backend
   * revocation (the actual /auth/logout) is the caller's job, done under the cookie
   * lock; this clears the local session, remembers the family as deliberately signed
   * out (in memory plus the shared non-secret marker) so a delayed token for it is
   * rejected everywhere, and then informs the peers.
   */
  signOut() {
    const family = this.familyId;
    this.clearToken();
    if (family !== null) {
      this.signedOutFamilies.add(family);
      writeLoggedOutFamily(family);
      if (readActiveFamily() === family) writeActiveFamily(null);
    }
    this.broadcast({ type: 'logout', family });
  }

  /**
   * Send the visitor to the login page, remembering where they were.
   *
   * Still a full page load rather than a router navigation. The reload is doing real
   * work: it drops every Pinia store with it, and this client cannot reset them itself
   * without importing the stores that import it. A soft navigation would leave the
   * previous account's user, calendars and settings in memory on the login screen.
   *
   * What was missing is the query. The guard sends an anonymous visitor to
   * `?redirect=<where they were going>` and Login.vue returns them there afterwards,
   * but an expiry mid-session went to a bare `/login` — so signing back in always
   * landed on the dashboard, however deep the page they were thrown out of.
   */
  private redirectToLogin() {
    if (this.disposed) return;
    // Only redirect to login if current route is not public: a participant following
    // a calendar link has no account to sign back in to.
    const currentRoute = router.currentRoute.value;
    if (currentRoute.meta.public === true) {
      return;
    }

    const target = currentRoute.fullPath;
    // The login route itself, and anything with no path to speak of, has nothing worth
    // coming back to.
    if (!target || target === '/' || currentRoute.name === 'login') {
      window.location.href = '/login';
      return;
    }

    window.location.href = `/login?redirect=${encodeURIComponent(target)}`;
  }

  private normalizeError(error: AxiosError<ApiResponse<never>>): ApiError {
    if (error.response?.data?.error) {
      return error.response.data.error;
    }

    return {
      code: error.code || 'UNKNOWN_ERROR',
      message: error.message || 'An unknown error occurred',
    };
  }

  /** The server family/session id this tab currently belongs to (or null). */
  getFamilyId(): string | null {
    return this.familyId;
  }

  /**
   * Install a token this tab itself obtained (login, register, bootstrap, MFA, magic
   * link, password reset, or a refresh).
   *
   * A token whose server family differs from the one already held is a *new* session:
   * the client generation advances so no pending request or refresh from the previous
   * family is replayed under this one. Whether new or a same-family rotation, the
   * token and absolute expiry are stored in memory only, the proactive refresh timer is
   * restarted from the new expiry, and the other tabs are told — receivers decide for
   * themselves whether the family is theirs.
   */
  setToken(token: string, expiresInSeconds?: number, sessionId?: string) {
    const family = normalizeFamily(sessionId ?? null);
    // Finalized credentials are installed under the cookie lock. Stores may repeat
    // the same installation as they commit the corresponding user payload.
    if (this.accessToken === token && this.familyId === family) return;
    const hadFamily = this.familyId !== null;
    const familyChanged = family !== null && hadFamily && family !== this.familyId;

    if (familyChanged) {
      if (this.familyId !== null) this.signedOutFamilies.add(this.familyId);
      this.sessionGeneration += 1;
      this.familyId = family;
    } else if (this.familyId === null) {
      this.familyId = family;
    }

    this.accessToken = token;
    this.scheduleRefresh(expiresInSeconds);
    writeActiveFamily(this.familyId);
    // A flag and the family id, never the token: anything persisted is readable by any
    // script that gets to run on the page. The refresh cookie is what actually proves
    // the session.
    this.setSessionFlag();
    this.broadcast({ type: 'token', token, expiresAt: this.expiresAt, family: this.familyId });
  }

  /**
   * Drop this tab's copy of the session without telling anyone.
   *
   * Local on purpose — the error paths (a refused /auth/me, a failed restore) call it,
   * and a transient failure in one tab must not sign the others out. It clears the
   * token, timer, family and session flag and advances the generation, so a refresh
   * that was in flight when the session ended cannot store its response afterwards.
   */
  clearToken() {
    const previousFamily = this.familyId;
    const activeFamily = readActiveFamily();
    this.accessToken = null;
    this.familyId = null;
    this.scheduleRefresh();
    if (previousFamily === null || activeFamily === null || previousFamily === activeFamily) {
      this.removeSessionFlag();
    }
    this.sessionGeneration += 1;
  }

  /** Whether this browser had a session, and so whether a cold load should refresh. */
  hasSession(): boolean {
    try {
      return typeof window !== 'undefined' && localStorage.getItem(SESSION_FLAG) !== null;
    } catch {
      return this.accessToken !== null;
    }
  }

  /**
   * Run a cookie-changing operation under the origin-wide cookie lock.
   *
   * Every operation that writes (or revokes) the refresh cookie — the refresh itself,
   * the session issuance of a login/bootstrap/register/MFA/magic-link/password-reset,
   * and the deliberate logout — serialises through this one lock, so a refresh cannot
   * rotate the cookie mid-login and a login cannot issue a cookie mid-refresh.
   *
   * In-process the operations always serialise (one at a time per client). Cross-tab,
   * `navigator.locks` queues the other tabs' operations. Without Web Locks there is no
   * atomic cross-tab mutex to stand in — a lease-style browser lock is precisely what
   * this PR refuses to re-invent — so the other tabs on the origin simply do not
   * serialise here: cross-tab Set-Cookie ordering is not guaranteed in that case, the
   * server's session-family and refresh-grace protection is the authority, and any late
   * response that conflicts with a finalized family is discarded (see `performRefresh`).
   */
  async withCookieLock<T>(fn: () => Promise<T>): Promise<T> {
    const run = () => {
      if (typeof navigator !== 'undefined' && navigator.locks) {
        return navigator.locks.request(COOKIE_LOCK, fn);
      }
      return fn();
    };
    const next = this.cookieMutex.then(run, run);
    this.cookieMutex = next.then(
      () => undefined,
      () => undefined
    );
    return next;
  }

  /**
   * The deliberate sign-out runs its backend revocation under the same cookie lock the
   * refresh uses, so a refresh that is already rotating the cookie cannot race the
   * logout's deletion.
   */
  async logoutThroughLock<T>(logoutRequest: () => Promise<T>): Promise<T> {
    return this.withCookieLock(logoutRequest);
  }

  /**
   * Refresh the access token, at most once at a time.
   *
   * Every request that 401s calls this, and a page load fires several at once — the
   * calendar alone issues three. Without the shared promise each of them started its
   * own `/auth/refresh`, so one expired token produced a burst of refreshes against a
   * rate-limited endpoint. Callers all await the same in-flight request and continue
   * with the token it stored.
   */
  async refreshToken(): Promise<void> {
    if (this.refreshInFlight) {
      return this.refreshInFlight;
    }

    this.refreshInFlight = this.performRefresh()
      .catch(err => {
        if (err instanceof TransientRefreshError) {
          throw err.originalError;
        }
        throw err;
      })
      .finally(() => {
        this.refreshInFlight = null;
      });

    return this.refreshInFlight;
  }

  private async performRefresh(): Promise<void> {
    // Whatever we were holding when we decided a refresh was needed. If it has changed
    // by the time the lock is ours, another tab refreshed while we queued and its token
    // is already ours via the channel.
    const staleToken = this.accessToken;
    // The session this refresh belongs to. A deliberate logout, a cross-tab sign-out or
    // an adopted replacement family bumps the generation; if it moved while the refresh
    // was in flight, the response must not be stored, broadcast, or used to resurrect
    // the session.
    const generation = this.sessionGeneration;
    const family = this.familyId;

    return this.withCookieLock(async () => {
      // Test the generation *before* the changed-token shortcut: after a logout or
      // account replacement a queued refresh must void itself, or the interceptor would
      // read its return as a success and replay the old request with the new token.
      if (generation !== this.sessionGeneration) {
        throw new StaleRefreshError('refresh voided by a session change');
      }

      if (this.accessToken !== staleToken) {
        return;
      }

      let response;
      try {
        response =
          await this.client.post<
            ApiResponse<{ access_token: string; expires_in?: number; session_id?: string }>
          >('/auth/refresh');
      } catch (err) {
        // A refresh that failed for a session that was replaced while it was on the wire
        // is the old session's result, not evidence the replacement is dead; void it.
        if (generation !== this.sessionGeneration) {
          throw new StaleRefreshError('refresh failed after the session changed');
        }
        throw err;
      }
      const newToken = response.data.data?.access_token;
      // The generation may have moved while `/auth/refresh` was on the wire; a logout
      // that landed then invalidates the result.
      if (generation !== this.sessionGeneration) {
        throw new StaleRefreshError('refresh voided by a session change');
      }
      if (newToken) {
        const newFamily = normalizeFamily(response.data.data?.session_id ?? null);
        const familyChanged = newFamily !== null && family !== null && newFamily !== family;
        const activeFamily = readActiveFamily();
        if (newFamily !== null && this.signedOutFamilies.has(newFamily)) {
          throw new StaleRefreshError('refresh belongs to a retired family');
        }
        if (activeFamily !== null && activeFamily !== family && newFamily !== activeFamily) {
          throw new StaleRefreshError('refresh conflicts with the latest login family');
        }
        this.setToken(newToken, response.data.data?.expires_in, response.data.data?.session_id);
        // A refresh that adopted a different server family means the shared refresh
        // cookie belonged to a different login than the one this tab described. That is
        // a replacement session: the account stores must reset and confirm identity
        // through /auth/me, and the request that provoked this refresh must not replay.
        if (familyChanged) {
          this.dispatchRemoteSession(newFamily);
        }
      }
      if (this.sessionGeneration !== generation) {
        throw new StaleRefreshError('refresh adopted a different session family');
      }
    });
  }

  /**
   * Tear the client down (tests/HMR): cancel timers, close the channel, remove the
   * storage and wake listeners, and drop the in-memory session. Production keeps one
   * long-lived client and never calls this.
   */
  dispose() {
    this.disposed = true;
    if (this.refreshTimer !== null) {
      clearTimeout(this.refreshTimer);
      this.refreshTimer = null;
    }
    this.channel?.close();
    this.channel = null;
    this.removeStorageListener?.();
    this.removeStorageListener = null;
    this.removeWakeListeners?.();
    this.removeWakeListeners = null;
    this.clearToken();
  }

  // Generic HTTP methods
  async get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    const response = await this.client.get<ApiResponse<T>>(url, config);
    return response.data.data as T;
  }

  async post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    const send = async () => {
      const response = await this.client.post<ApiResponse<T>>(url, data, config);
      const result = response.data.data as T;
      if (SESSION_ISSUANCE_PATHS.has(url) && result && typeof result === 'object') {
        const session = result as {
          access_token?: string;
          expires_in?: number;
          session_id?: string;
        };
        if (typeof session.access_token === 'string' && session.access_token.length > 0) {
          this.setToken(session.access_token, session.expires_in, session.session_id);
        }
      }
      return result;
    };
    return SESSION_ISSUANCE_PATHS.has(url) ? this.withCookieLock(send) : send();
  }

  async patch<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    const response = await this.client.patch<ApiResponse<T>>(url, data, config);
    return response.data.data as T;
  }

  async delete<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    const response = await this.client.delete<ApiResponse<T>>(url, config);
    return response.data.data as T;
  }
}

export const apiClient = new ApiClient();
