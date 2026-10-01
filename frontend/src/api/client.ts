/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import axios, { type AxiosInstance, type AxiosError, type InternalAxiosRequestConfig } from 'axios';
import type { ApiResponse, ApiError } from '@/types';
import router from '@/router';
import { REMOTE_SIGNOUT_EVENT, REMOTE_SESSION_EVENT } from '@/sessionEvents';
import { isDefinitiveRejection } from '@/api/failureClassification';

/** Marks that this browser has a session; never holds the token itself. */
const SESSION_FLAG = 'whento.session';

/**
 * Origin-wide markers for session fencing when Web Locks are unavailable.
 *
 * `whento.sessionEpoch` is a monotonic counter shared by every tab. Each fresh session
 * stamps its auth-channel broadcasts with an epoch taken from it; a deliberate sign-out
 * raises the `whento.loggedOutEpoch` watermark above everything issued so far. A token
 * broadcast that still carries an epoch at or below the watermark belongs to a session
 * family that was already logged out, and is rejected even if the logout message itself
 * has not been delivered in order yet.
 *
 * `whento.authLock` is a lease-based, origin-wide mutex for the fallback path (see
 * `withRefreshLock`): it serialises `/auth/refresh` and `/auth/logout` cookie operations
 * across tabs where `navigator.locks` does not exist.
 */
const SESSION_EPOCH_KEY = 'whento.sessionEpoch';
const LOGGED_OUT_EPOCH_KEY = 'whento.loggedOutEpoch';
const AUTH_LOCK_KEY = 'whento.authLock';

/** Names the cross-tab channel and the cross-tab lock. */
const AUTH_CHANNEL = 'whento.auth';
const REFRESH_LOCK = 'whento.refresh';

/**
 * Lease on the localStorage fallback lock, so a vanished tab cannot wedge the origin.
 *
 * Deliberately longer than the HTTP client's own 30s request timeout: a slow but
 * still-live auth request must never have its borrow stolen, because the old holder
 * would keep running and could commit a rotated cookie after a new holder started.
 * The lease is also renewed continuously while held (see `withRefreshLock`), so the
 * live holder outlives even the slowest operation.
 */
const AUTH_LOCK_LEASE_MS = 90_000;
/** How often a live holder re-stamps the fallback lock, keeping its lease current. */
const AUTH_LOCK_RENEW_MS = 25_000;
/** How long a waiting tab sleeps between polling attempts at the fallback lock. */
const AUTH_LOCK_POLL_MS = 20;

/**
 * Session epoch + family nonce carried on auth-channel messages.
 *
 * The epoch is a *best-effort* ordering value: two tabs can race the non-atomic
 * localStorage counter and allocate the same value in the same millisecond. Equality
 * therefore proves nothing about identity by itself — the nonce does. A token whose
 * epoch equals ours but whose nonce differs came from a *different* login of the same
 * brand-new epoch, and must be treated as a replacement session, not a same-family
 * refresh.
 */
type AuthMessage =
  | { type: 'token'; token: string; expiresAt?: number | null; epoch: number; nonce?: string }
  | { type: 'logout'; epoch: number };

/** Read an origin-wide numeric marker, tolerating an absent or corrupted value. */
function readMarker(key: string): number {
  if (typeof window === 'undefined') return 0;
  try {
    const value = Number(localStorage.getItem(key) ?? '0');
    return Number.isFinite(value) && value > 0 ? value : 0;
  } catch {
    return 0;
  }
}

/**
 * Next value of the session-epoch counter, shared by every tab.
 *
 * The counter is bumped to be strictly greater than both the previous allocation and
 * the clock (so two tabs that race the non-atomic read/increment/write almost always
 * land on different, strictly increasing values). Like every localStorage scheme this
 * is best-effort — the authoritative fence against a revoked session is the backend's
 * session family; the epoch only guards cross-tab message ordering.
 */
function nextSessionEpoch(): number {
  if (typeof window === 'undefined') return 1;
  const next = Math.max(readMarker(SESSION_EPOCH_KEY) + 1, Date.now());
  try {
    localStorage.setItem(SESSION_EPOCH_KEY, String(next));
  } catch {
    // Storage unavailable: fall back to the in-memory value; fencing degrades.
  }
  return next;
}

/** The highest epoch any tab has deliberately signed out of. */
function loggedOutEpoch(): number {
  return readMarker(LOGGED_OUT_EPOCH_KEY);
}

/**
 * Raise the logged-out watermark above every epoch issued so far.
 *
 * Returns the new watermark, which everything already broadcast (and everything still
 * to arrive with an older epoch) must now be rejected against.
 */
function markSessionLoggedOut(): number {
  const watermark = Math.max(loggedOutEpoch(), nextSessionEpoch());
  try {
    localStorage.setItem(LOGGED_OUT_EPOCH_KEY, String(watermark));
  } catch {
    // Storage unavailable: the in-memory marker carries the fence for this tab.
  }
  return watermark;
}

/** A unique owner token for the localStorage auth mutex. */
function makeLockToken(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

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
 * Thrown by `performRefresh` when the session generation moved between the moment the
 * refresh captured it and the moment its response landed — a deliberate logout, a
 * cross-tab sign-out or a forced expiry. It is a *cancellation*, not a failure: the 401
 * interceptor must neither replay the request that provoked the refresh (that would
 * attach the replacing session's token to a request the old session issued) nor force
 * the replacing session out (it is still alive and well).
 */
class StaleRefreshError extends Error {}

/**
 * Signals that a refresh failed for a *transient* reason — an HTTP 5xx, a network
 * error, a timeout — rather than because the session itself is dead.
 *
 * The backend maps a genuine rejection of the presented refresh cookie to 401 (see
 * `RefreshToken` and the refresh handler's error classification); everything else is
 * infrastructure, and the cookie may still be valid. The ordinary-request 401 retry
 * path must therefore preserve the session on this error, reject the triggering request
 * with the recoverable failure, and let the caller retry — signing a user out over a
 * server outage turns an avoidable blip into session loss.
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
 * rejected WebAuthn assertion on passkey login finish, or a logout whose refresh cookie
 * was already gone. A 401 from any of them must be returned straight to the form that
 * sent it — never followed by a refresh, a replay, or a sign-out. A failed register
 * attempt in one tab must not log a healthy other tab out; a mistyped MFA code or a
 * rejected passkey assertion must not clear the still-valid pending login and force the
 * user to redo the password/passkey stage.
 *
 * Matched by exact path (query string aside), not substring, so an endpoint that merely
 * shares a prefix, e.g. the protected `/auth/me`, is never exempted from a legitimate
 * access-session refresh — and protected passkey-management routes like
 * `/passkey/register/finish` or `/passkey/{id}` keep their normal refresh behavior.
 */
const CREDENTIAL_REJECTION_PATHS = new Set([
  '/auth/login',
  '/auth/register',
  '/auth/bootstrap',
  '/auth/logout',
  '/auth/mfa/verify',
  '/auth/passkey/login/finish',
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
   * The client session generation. Every sign-out bumps it so that an in-flight
   * refresh that started before the sign-out can never resurrect the session: the
   * refresh accepts and broadcasts a token only if the generation it captured is
   * still current when the response lands.
   */
  private sessionGeneration = 0;

  /**
   * The origin-wide session epoch this tab currently believes in. Stamped onto every
   * auth-channel broadcast; a token whose epoch sits at or below the shared logged-out
   * watermark belongs to a session family this origin already signed out of and is
   * ignored, even if its message arrives out of order.
   */
  private sessionEpoch: number | null = null;

  /**
   * The fallback lock owner token, while this tab holds it (see `withRefreshLock`).
   *
   * Set only on the no-Web-Locks path. It lets the holder detect that another context
   * took the marker over while an auth operation was on the wire — the localStorage
   * fallback is best-effort ordering, not a real mutex — and discard its result rather
   * than commit under a lost critical section.
   */
  private authLockOwner: string | null = null;

  /**
   * The unique nonce of the session family this tab belongs to, alongside `sessionEpoch`.
   *
   * The epoch alone is not collision-free (two tabs can allocate the same value in the
   * same millisecond), so equality of epochs must also match this nonce before a token
   * counts as a same-family refresh. A same-epoch/different-nonce token is a different
   * login — a replacement session, not a refresh.
   */
  private sessionNonce: string | null = null;

  /**
   * The epoch of the session family this tab currently accepts, or null when it has
   * none. Exposed so the account-restore path (stores/remoteSignout.ts) can fence a
   * `/auth/me` response against the session it was issued for.
   */
  getSessionEpoch(): number | null {
    return this.sessionEpoch;
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

    // visibilitychange is fired at the document, not the window.
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) {
        this.refreshIfDue();
      }
    });
    // Coming back from a dropped connection has the same shape: whatever the timer
    // tried to do while offline did not happen.
    window.addEventListener('online', () => this.refreshIfDue());
  }

  /** Refresh when the token is at or past its lead window. Silent on failure: the 401 path remains. */
  private refreshIfDue() {
    if (!this.accessToken || this.expiresAt === null) {
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

    this.refreshTimer = setTimeout(() => this.refreshIfDue(), delay);
  }

  /**
   * Share tokens and sign-outs with the other tabs on this origin.
   *
   * Refresh tokens are single-use — the backend deletes the old one before issuing the
   * next (auth_service.go, `RefreshToken`). Tabs each hold their own in-memory access
   * token, so restoring a window full of pinned tabs used to fire one `/auth/refresh`
   * per tab against the same cookie: the first rotated it and the rest got a 401 and a
   * forced sign-out. Whichever tab wins the lock now passes its result to the others.
   *
   * Nothing is persisted — the channel is same-origin and in-memory, so this does not
   * put the token back within reach of stored-data reads.
   */
  private setupChannel() {
    if (typeof BroadcastChannel === 'undefined') {
      return;
    }

    this.channel = new BroadcastChannel(AUTH_CHANNEL);
    this.channel.onmessage = (event: MessageEvent<AuthMessage>) => {
      const message = event.data;
      if (message?.type === 'token') {
        // A token from an *older* session family than the one this tab already
        // accepted must not be installed: afterwards the UI would describe the newer
        // account while requests authenticated as the older one. Only a token stamped
        // with the current (equal) or a strictly newer epoch may proceed.
        if (
          message.epoch !== undefined &&
          this.sessionEpoch !== null &&
          message.epoch < this.sessionEpoch
        ) {
          return;
        }
        // A token carrying an epoch at or below the origin-wide logged-out watermark is
        // from a session family this browser already deliberately signed out of. Accept
        // it and the logout is undone on the wire, even when the token message races
        // ahead of (or arrives long after) the logout message itself.
        if (message.epoch !== undefined && message.epoch <= loggedOutEpoch()) {
          return;
        }
        // Set directly rather than through setToken, which would echo it back. The
        // sender includes the token's absolute expiry so proactive refresh is
        // rescheduled from *this* token's due time: a tab keeping the previous one
        // would otherwise fire its old timer and spend the single-use refresh cookie
        // needlessly. No re-broadcast.
        const previousEpoch = this.sessionEpoch;
        const previousNonce = this.sessionNonce;
        // Equal epoch + different nonce is a colliding login, not a family we should
        // adopt. BroadcastChannel does not echo, so each tab installing the other's
        // message swaps families instead of converging, and the origin-wide refresh
        // cookie may belong to neither message. Do not install the foreign token;
        // refresh so the cookie's server session id becomes the one family every tab
        // adopts (setToken broadcasts it under a strictly newer epoch).
        const conflictingFamily =
          previousEpoch !== null &&
          message.epoch === previousEpoch &&
          message.nonce !== undefined &&
          previousNonce !== null &&
          message.nonce !== previousNonce;
        if (conflictingFamily) {
          void this.refreshToken().catch(() => {});
          return;
        }
        const isNewSession = previousEpoch === null || message.epoch > previousEpoch;
        if (isNewSession) {
          // A replacement session family must void every request or refresh that was
          // issued under the previous family, exactly as a sign-out voids them: bump
          // the generation here so a queued or in-flight `/auth/refresh` that captured
          // the older generation rejects with `StaleRefreshError` (its response is
          // discarded, and the 401 interceptor refuses to replay the older family's
          // request with this replacement token).
          this.sessionGeneration += 1;
        }
        // Adopt the session family the sender stamped — a strictly newer epoch (or a
        // different-nonce equal epoch) replaces ours, so this tab never broadcasts under
        // an older (already logged-out) identity it happened to hold.
        this.sessionEpoch = isNewSession ? message.epoch : previousEpoch;
        this.sessionNonce = isNewSession
          ? (message.nonce ?? this.sessionNonce ?? makeLockToken())
          : previousNonce;
        this.accessToken = message.token;
        this.scheduleRefreshAt(message.expiresAt ?? null);
        localStorage.setItem(SESSION_FLAG, '1');
        // A genuinely new session (the receiver had none, or is handed a newer (or
        // colliding) one) must be reflected in the account stores: reset the previous
        // account's state and hydrate /auth/me, or the UI keeps describing the old (or
        // no) account while outgoing requests carry the new account's token.
        if (isNewSession) {
          this.notifySessionRestored(this.sessionEpoch as number);
        }
      } else if (message?.type === 'logout') {
        // Ignore a logout that is older than the session this tab has already accepted:
        // after another tab signed in again (a newer epoch), a delayed logout from the
        // previous session must not clear the replacement session. Only a logout for the
        // current (or a newer) session is honoured.
        if (
          message.epoch !== undefined &&
          this.sessionEpoch !== null &&
          message.epoch < this.sessionEpoch
        ) {
          return;
        }
        // Another tab signed out (its epoch is already reflected in the shared
        // watermark before it broadcast). Dropping the token here is not enough: this
        // tab would keep rendering the account it no longer has a session for —
        // calendars, settings, the lot — until its next request happened to 401.
        // On a shared machine that is the thing signing out is meant to prevent.
        // No re-broadcast: the tab that sent this already told everyone.
        this.clearToken();
        if (router.currentRoute.value.meta.public === true) {
          // On a public calendar link there is no account to sign back in to, so no
          // navigation. But the app must still drop the previous account's in-memory
          // state — the user, the owner calendar list and the participant ids rendered
          // from it — or this tab keeps showing a signed-out session as signed in.
          this.resetLocalStateIfPublic();
          return;
        }
        this.redirectToLogin();
      }
    };
  }

  /**
   * Tell the application that a fresh session was established by another tab, so it
   * can reset the previous account's stores and hydrate the new one.
   *
   * The accepted epoch travels with the event so the handler can fence its `/auth/me`
   * round trip against the session it was issued for — a late response for an older
   * session must not overwrite a newer one that has since been accepted.
   */
  private notifySessionRestored(epoch: number) {
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent(REMOTE_SESSION_EVENT, { detail: { epoch } }));
    }
  }

  private broadcast(message: AuthMessage) {
    this.channel?.postMessage(message);
  }

  private setupInterceptors() {
    // Request interceptor - add auth token
    this.client.interceptors.request.use(
      (config: InternalAxiosRequestConfig) => {
        // Stamp *every* request — including the cold-start /auth/refresh whose
        // purpose is to obtain the first in-memory token — with the session
        // generation it was dispatched under. A delayed 401 for a request issued
        // by a session that has since been replaced must not refresh the
        // replacement session, replay the old request with its token, or (for a
        // refresh) reach unconditional logout. The stamp is captured on the first
        // dispatch and preserved through a retry, so the replay is fenced against
        // the same session that sent it.
        const stamped = config as InternalAxiosRequestConfig & {
          _whentoSessionGeneration?: number;
        };
        if (stamped._whentoSessionGeneration === undefined) {
          stamped._whentoSessionGeneration = this.sessionGeneration;
        } else if (stamped._whentoSessionGeneration !== this.sessionGeneration) {
          // A retry that was queued (e.g. behind an asynchronous interceptor) for
          // a session that has since been replaced must not be dispatched under
          // the replacement session's token. The stamp preserved from the first
          // dispatch no longer matches, so abort this dispatch instead of
          // attaching the current token.
          return Promise.reject(STALE_SESSION_ERROR);
        }
        // A token is attached only when there is one in memory; the stamp above is
        // independent of it, so an anonymous request still knows which session it
        // was issued under (and a request issued under none sees generation 0).
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

        // A 401 on the credential-validation endpoints is a *rejection*, not an expired
        // session: bad credentials on login, a duplicate on register, a wrong boot key on
        // bootstrap, or a wrong second-factor code on MFA verify. None of them should
        // refresh (the token is not the problem) and, crucially, none of them should
        // sign anyone out — a failed register attempt in one tab must not log a healthy
        // other tab out, and a mistyped MFA code must not clear the still-valid pending
        // login. Only a failed `/auth/refresh` means the session itself is dead; that
        // case is handled in its own branch below.
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
          // A 401 that answers a request dispatched under a *previous* session
          // generation means the session was replaced (or signed out) while that
          // request was in flight. Refreshing would refresh — and then replay the
          // old request with — the replacement session's token, and the request's
          // own generation was already consumed by the replacement. Reject it as
          // stale without touching the current session.
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
            // A refresh that failed for a *transient* reason (an HTTP 5xx from the
            // backend, a network error, a timeout) is not evidence the access session
            // died — the refresh cookie may still be perfectly valid and the server
            // simply could not answer. Signing out here would turn a temporary outage
            // into avoidable, origin-wide session loss. Preserve the session identity
            // and reject with the recoverable failure so the caller can retry.
            if (failure instanceof TransientRefreshError) {
              return Promise.reject(failure.originalError);
            }
            // A definitive 401 from our own refresh already forced the logout in the
            // isRefresh branch (and surfaces here as a StaleRefreshError); a replayed
            // request that is refused 401 even under a freshly rotated token is the
            // same verdict — the current session cannot authorise it. Either way the
            // session is genuinely dead and the full-page sign-out is right.
            if (isDefinitiveRejection(failure)) {
              this.forceLogout();
            }
            return Promise.reject(failure);
          }
        }

        // If 401 on /auth/refresh (or spawned by one), force logout
        if (error.response?.status === 401 && isRefresh) {
          // A refresh that answers for a *previous* session generation is stale: the
          // session it was issued for was replaced while the refresh was in flight.
          // Its 401 is a side-effect of that replacement (the old refresh cookie was
          // already rotated away), not evidence the replacement session died. Void it
          // so the caller rejects without forceLogout — which would sign out the very
          // session that has since replaced it.
          const sessionAtDispatch = (originalRequest as SessionStampedRequest)
            ?._whentoSessionGeneration;
          if (sessionAtDispatch !== undefined && sessionAtDispatch !== this.sessionGeneration) {
            throw new StaleRefreshError('refresh answered after the session changed');
          }
          // A 401 on our own refresh after the fallback lock was taken over by another
          // context is a *stale loser's* side-effect (the winner rotated the single-use
          // cookie), not evidence our session died. Do not force a logout — that would
          // sign out the very session the new lock owner is managing. It surfaces as a
          // StaleRefreshError, and the outer catch turns it into a stale-session
          // rejection with no logout.
          if (!this.authLockOwned()) {
            throw new StaleRefreshError('refresh lock lost to another context');
          }
          this.forceLogout();
        }

        // Any failure of our own refresh that is *not* a definitive 401 is transient:
        // an HTTP 5xx answered by the backend, a network error, a timeout. The backend
        // classifies a genuinely refused refresh cookie as 401; everything else means
        // the server could not service the request, not that the session died. Wrap it
        // so the 401-retry path preserves the session and surfaces a recoverable error
        // instead of signing the user (and every other tab) out over an outage.
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
    // so this tab would never see the "logout" broadcast it just sent. If the session
    // died on a public route there is no login page to reload into (the redirect below
    // is a no-op there), so the account stores have to be reset in place — exactly as
    // an incoming cross-tab logout resets them.
    this.resetLocalStateIfPublic();
    this.redirectToLogin();
  }

  /**
   * Raise the account-scoped reset event when the session dies on a public route.
   *
   * The reset itself lives above this client (stores/remoteSignout via main.ts),
   * because importing the stores from here would form an import cycle. On a private
   * route it is not needed: `forceLogout` redirects to /login, and that redirect is a
   * full page load which drops every Pinia store with it.
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
   * Choosing to log out is different, and has to reach them all at once.
   */
  signOut() {
    this.clearToken();
    // Raise the origin-wide logged-out watermark above every epoch issued so far, so a
    // delayed token broadcast by any other tab from this (now dead) session family is
    // rejected everywhere — even if that token's message races ahead of this one.
    const epoch = markSessionLoggedOut();
    this.broadcast({ type: 'logout', epoch });
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

  setToken(token: string, expiresInSeconds?: number, sessionId?: string) {
    // A refresh of the session this tab holds keeps its epoch and nonce. A server
    // session id that differs from the one we hold means the shared refresh cookie
    // belongs to a different login than the nonce we were broadcasting — adopt it,
    // under a strictly newer epoch, so every other tab converges on that family
    // instead of oscillating on a collided epoch.
    const incomingFamily = sessionId && sessionId.length > 0 ? sessionId : null;
    const hadFamily = this.sessionNonce !== null;
    const familyChanged =
      incomingFamily !== null && hadFamily && incomingFamily !== this.sessionNonce;
    if (familyChanged) {
      this.sessionGeneration += 1;
      this.sessionEpoch = nextSessionEpoch();
      this.sessionNonce = incomingFamily;
    } else if (this.sessionEpoch === null) {
      this.sessionEpoch = nextSessionEpoch();
      this.sessionNonce = incomingFamily ?? makeLockToken();
    } else if (this.sessionNonce === null) {
      this.sessionNonce = incomingFamily ?? makeLockToken();
    }
    this.accessToken = token;
    this.scheduleRefresh(expiresInSeconds);
    // A flag, not a secret. The token itself never leaves memory: anything persisted
    // is readable by any script that gets to run on the page, which is exactly what
    // CodeQL's js/clear-text-storage-of-sensitive-data flagged when the JWT lived
    // here. All this records is "there was a session in this browser", so a cold load
    // knows whether to spend a `/auth/refresh` before deciding the visitor is
    // anonymous. The refresh cookie is what actually proves the session.
    localStorage.setItem(SESSION_FLAG, '1');
    // Carry the absolute expiry across tabs so the receivers can reschedule their
    // proactive refresh instead of keeping the previous token's due time, and the
    // nonce so equal-epoch messages are told apart from a colliding session.
    this.broadcast({
      type: 'token',
      token,
      expiresAt: this.expiresAt,
      epoch: this.sessionEpoch,
      nonce: this.sessionNonce,
    });
    if (familyChanged) {
      this.notifySessionRestored(this.sessionEpoch as number);
    }
  }

  clearToken() {
    this.accessToken = null;
    this.sessionEpoch = null;
    this.sessionNonce = null;
    this.scheduleRefresh();
    localStorage.removeItem(SESSION_FLAG);
    // Invalidate any refresh already in flight: once the session is over, its
    // response must not be stored or broadcast back to the other tabs.
    this.sessionGeneration += 1;
  }

  /** Whether this browser had a session, and so whether a cold load should refresh. */
  hasSession(): boolean {
    return localStorage.getItem(SESSION_FLAG) !== null;
  }

  /**
   * Refresh the access token, at most once at a time.
   *
   * Every request that 401s calls this, and a page load fires several at once — the
   * calendar alone issues three. Without the shared promise each of them started its
   * own `/auth/refresh`, so one expired token produced a burst of refreshes against a
   * rate-limited endpoint. The later ones then raced: refresh rotates the token, so
   * whichever landed last could invalidate the token an earlier one had just stored,
   * logging the user out mid-session.
   *
   * Callers all await the same in-flight request and continue with the token it stored.
   * `performRefresh` extends the same guarantee across tabs.
   */
  async refreshToken(): Promise<void> {
    if (this.refreshInFlight) {
      return this.refreshInFlight;
    }

    this.refreshInFlight = this.performRefresh()
      // The transient-refresh marker is machinery internal to the request path; what a
      // caller (the 401 interceptor, the proactive timer, a cold load) observes is the
      // recoverable failure itself. Unwrap it here so the contract is uniform: every
      // caller sees the normalized error, and `isDefinitiveRejection` tells the
      // interceptor whether to sign out or preserve the session.
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
    // Whatever we were holding when we decided a refresh was needed. If it has
    // changed by the time the lock is ours, another tab refreshed while we queued
    // and its token is already ours via the channel — spending the cookie again
    // would rotate away the token we just received.
    const staleToken = this.accessToken;
    // The session this refresh belongs to. A deliberate logout or a cross-tab
    // sign-out bumps the generation; if it moved while the refresh was in flight,
    // the response must not be stored, broadcast, or used to resurrect the session.
    const generation = this.sessionGeneration;

    // Serialise across tabs as well as within one. Web Locks queue rather than fail,
    // so a tab that arrives second waits here instead of racing the first onto a
    // refresh cookie that is already spent.
    return this.withRefreshLock(async () => {
      // Test the generation *before* the changed-token shortcut: the shortcut is only
      // safe when the session is the same (another tab refreshed it). After a logout
      // or account replacement — even one that already installed a new token — a
      // queued refresh must void itself, or the interceptor would read its return as a
      // success and replay the old session's request with the new session's token.
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
        // A refresh that fails (e.g. a 401) because another context took the critical
        // section over must not report our own session as dead: the losing holder's
        // 401 is a side-effect of the race, not evidence our session expired. Void it
        // as a stale refresh so the interceptor rejects without calling forceLogout,
        // which would sign out the very session the new lock owner is managing.
        if (!this.authLockOwned()) {
          throw new StaleRefreshError('refresh lock lost to another context');
        }
        // The failure may be for a session that was replaced while the refresh was on
        // the wire — a 500, a network error, anything that is not a 401 (the 401 case
        // is caught by the response interceptor's isRefresh branch, but a non-401
        // error bypasses it and would otherwise fall through to the interceptor's
        // forceLogout). Its failure is the old session's result, not evidence the
        // replacement is dead; void it rather than sign the replacement out.
        if (generation !== this.sessionGeneration) {
          throw new StaleRefreshError('refresh failed after the session changed');
        }
        throw err;
      }
      const newToken = response.data.data?.access_token;
      // The generation may also have moved while `/auth/refresh` was on the wire; a
      // logout that landed then invalidates the result. Accepting it would re-seed
      // `whento.session`, reschedule a refresh timer and broadcast a fresh token to
      // every other tab — undoing the logout.
      if (generation !== this.sessionGeneration) {
        throw new StaleRefreshError('refresh voided by a session change');
      }
      // Loss-of-ownership fencing for the no-Web-Locks fallback: the localStorage
      // marker is not an atomic mutex, so a second context can take the critical
      // section over while this refresh is on the wire. We no longer own it; the
      // rotated result must not be committed, or it races the new owner's own
      // refresh/logout on the single-use cookie.
      if (!this.authLockOwned()) {
        throw new StaleRefreshError('refresh lock lost to another context');
      }
      if (newToken) {
        // expires_in was being discarded here, which is why every refresh had to be
        // provoked by a 401 rather than anticipated.
        this.setToken(newToken, response.data.data?.expires_in, response.data.data?.session_id);
      }
      // setToken bumps the generation when the cookie belongs to a different family.
      // The request that provoked this refresh was issued for the previous family and
      // must not be replayed with the replacement account's token.
      if (this.sessionGeneration !== generation) {
        throw new StaleRefreshError('refresh adopted a different session family');
      }
    });
  }

  /**
   * Run a server-side logout request while serialised against any in-flight refresh.
   *
   * The deliberate logout goes through the same Web Lock refresh uses, so a refresh
   * that is already rotating the refresh cookie cannot race the logout's cookie
   * deletion: whichever wins the lock, the other waits. The local sign-out still
   * happens after the request, and the session generation it bumps makes any refresh
   * that might have lost the race a no-op.
   */
  async logoutThroughLock(logoutRequest: () => Promise<unknown>): Promise<unknown> {
    return this.withRefreshLock(() => {
      // If another context took over the fallback lock while we were queued, our
      // deliberate logout would race the winner's refresh/logout on the single-use
      // cookie. Refuse to run it: the winner is handling the session.
      if (!this.authLockOwned()) {
        throw new StaleRefreshError('logout lock lost to another context');
      }
      return logoutRequest();
    });
  }

  /**
   * Run `fn` while holding the cross-tab refresh lock.
   *
   * `navigator.locks.request` queues rather than fails, so in every browser that
   * supports it the refresh and the deliberate logout serialise across tabs. Where Web
   * Locks is unavailable (it is a secure-context feature, and a self-hosted origin may
   * run on plain HTTP), a lease-based localStorage marker stands in: it is *best-effort
   * ordering*, not a true mutex — the platform offers no atomic cross-context
   * compare-and-set, so two contexts can still both believe they hold it. The holder
   * therefore watches for loss of ownership and cancels its commit (`performRefresh`
   * refuses to store a result once the marker was taken over), while the backend
   * session family remains the authoritative fence against a revoked session.
   */
  private async withRefreshLock<T>(fn: () => Promise<T>): Promise<T> {
    if (typeof navigator !== 'undefined' && navigator.locks) {
      return navigator.locks.request(REFRESH_LOCK, fn);
    }

    const { token, release } = await this.acquireAuthLock();
    this.authLockOwner = token;
    // Renew the lease while the operation is live: a slow but healthy request must not
    // have its borrow stolen by a timer lapsing mid-operation, even over the HTTP
    // client's 30s timeout (the lease is generous and renewal keeps it fresh).
    const renew = setInterval(() => {
      if (localStorage.getItem(AUTH_LOCK_KEY)?.startsWith(`${token}:`)) {
        localStorage.setItem(AUTH_LOCK_KEY, `${token}:${Date.now()}`);
      }
    }, AUTH_LOCK_RENEW_MS);
    try {
      return await fn();
    } finally {
      clearInterval(renew);
      this.authLockOwner = null;
      release();
    }
  }

  /**
   * Whether this tab still holds the fallback lock marker.
   *
   * On the Web Locks path (or without any lock in use) this is trivially true; on the
   * no-Web-Locks fallback it re-checks the shared marker so protected work can refuse
   * to commit once another context has taken the critical section over.
   */
  private authLockOwned(): boolean {
    if (this.authLockOwner === null) {
      return true;
    }
    try {
      return localStorage.getItem(AUTH_LOCK_KEY)?.startsWith(`${this.authLockOwner}:`) === true;
    } catch {
      return false;
    }
  }

  /**
   * Acquire the origin-wide auth serialisation mutex via localStorage.
   *
   * The holder writes a unique owner token plus a timestamp, yields the event loop so a
   * concurrent realm that also raced to write gets the chance to overwrite it, and only
   * then confirms ownership by re-reading the marker. Losers detect the takeover and
   * retry — a best-effort mutual exclusion; without Web Locks the platform has no atomic
   * cross-context compare-and-set, so the authoritative fence against a revoked session
   * remains the backend session family. A holder whose lease expired (a tab that
   * vanished mid-operation) can be taken over.
   */
  private async acquireAuthLock(): Promise<{ token: string; release: () => void }> {
    if (typeof window === 'undefined') {
      return { token: 'noop', release: () => {} };
    }
    const token = makeLockToken();
    for (;;) {
      const held = localStorage.getItem(AUTH_LOCK_KEY);
      if (held === null || this.authLockExpired(held)) {
        const value = `${token}:${Date.now()}`;
        localStorage.setItem(AUTH_LOCK_KEY, value);
        // Contention window: let any equally-raced writer publish, then confirm we still
        // own the marker. If another realm took it, restart.
        await new Promise(resolve => setTimeout(resolve, AUTH_LOCK_POLL_MS * 2));
        if (localStorage.getItem(AUTH_LOCK_KEY) === value) {
          return {
            token,
            release: () => {
              // Match by owner token, not by the full marker string: the holder renews
              // its lease by re-stamping Date.now(), which changes the recorded value.
              if (localStorage.getItem(AUTH_LOCK_KEY)?.startsWith(`${token}:`)) {
                localStorage.removeItem(AUTH_LOCK_KEY);
              }
            },
          };
        }
      }
      await new Promise(resolve => setTimeout(resolve, AUTH_LOCK_POLL_MS));
    }
  }

  /** Whether a lock marker was written so long ago its holder can be assumed gone. */
  private authLockExpired(held: string): boolean {
    const colon = held.lastIndexOf(':');
    if (colon === -1) return true;
    const start = Number(held.slice(colon + 1));
    return !Number.isFinite(start) || Date.now() - start > AUTH_LOCK_LEASE_MS;
  }

  // Generic HTTP methods
  async get<T>(url: string, config?: any): Promise<T> {
    const response = await this.client.get<ApiResponse<T>>(url, config);
    return response.data.data as T;
  }

  async post<T>(url: string, data?: any, config?: any): Promise<T> {
    const response = await this.client.post<ApiResponse<T>>(url, data, config);
    return response.data.data as T;
  }

  async patch<T>(url: string, data?: any, config?: any): Promise<T> {
    const response = await this.client.patch<ApiResponse<T>>(url, data, config);
    return response.data.data as T;
  }

  async delete<T>(url: string, config?: any): Promise<T> {
    const response = await this.client.delete<ApiResponse<T>>(url, config);
    return response.data.data as T;
  }
}

export const apiClient = new ApiClient();
