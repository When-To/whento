/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import axios, { type AxiosInstance, type AxiosError, type InternalAxiosRequestConfig } from 'axios';
import type { ApiResponse, ApiError } from '@/types';
import router from '@/router';

/** Marks that this browser has a session; never holds the token itself. */
const SESSION_FLAG = 'whento.session';

/** Names the cross-tab channel and the cross-tab lock. */
const AUTH_CHANNEL = 'whento.auth';
const REFRESH_LOCK = 'whento.refresh';

type AuthMessage = { type: 'token'; token: string } | { type: 'logout' };

class ApiClient {
  private client: AxiosInstance;
  private accessToken: string | null = null;
  /** The one refresh in progress, shared by every caller that needs it. */
  private refreshInFlight: Promise<void> | null = null;
  private channel: BroadcastChannel | null = null;

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
        // Set directly rather than through setToken, which would echo it back.
        this.accessToken = message.token;
        localStorage.setItem(SESSION_FLAG, '1');
      } else if (message?.type === 'logout') {
        // Another tab signed out. Dropping the token here is not enough: this tab
        // would keep rendering the account it no longer has a session for —
        // calendars, settings, the lot — until its next request happened to 401.
        // On a shared machine that is the thing signing out is meant to prevent.
        // No re-broadcast: the tab that sent this already told everyone.
        this.clearToken();
        this.redirectToLogin();
      }
    };
  }

  private broadcast(message: AuthMessage) {
    this.channel?.postMessage(message);
  }

  private setupInterceptors() {
    // Request interceptor - add auth token
    this.client.interceptors.request.use(
      (config: InternalAxiosRequestConfig) => {
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

        // Don't try to refresh token for auth endpoints (login, register, refresh)
        const isAuthEndpoint =
          originalRequest?.url?.includes('/auth/login') ||
          originalRequest?.url?.includes('/auth/register') ||
          originalRequest?.url?.includes('/auth/refresh');

        // If 401 and not already retrying, try to refresh token (except for auth endpoints)
        if (
          error.response?.status === 401 &&
          originalRequest &&
          !(originalRequest as any)._retry &&
          !isAuthEndpoint
        ) {
          (originalRequest as any)._retry = true;

          try {
            await this.refreshToken();
            return this.client(originalRequest);
          } catch (refreshError) {
            // Refresh failed - force full page redirect to login
            this.forceLogout();
            return Promise.reject(refreshError);
          }
        }

        // If 401 on auth endpoint (e.g., refresh failed), force logout
        if (error.response?.status === 401 && isAuthEndpoint) {
          this.forceLogout();
        }

        return Promise.reject(this.normalizeError(error));
      }
    );
  }

  private forceLogout() {
    this.signOut();
    this.redirectToLogin();
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
    this.broadcast({ type: 'logout' });
  }

  private redirectToLogin() {
    // Only redirect to login if current route is not public: a participant following
    // a calendar link has no account to sign back in to.
    const currentRoute = router.currentRoute.value;
    const isPublicRoute = currentRoute.meta.public === true;

    if (!isPublicRoute) {
      // Force full page reload to login to reset all UI state
      window.location.href = '/login';
    }
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

  setToken(token: string) {
    this.accessToken = token;
    // A flag, not a secret. The token itself never leaves memory: anything persisted
    // is readable by any script that gets to run on the page, which is exactly what
    // CodeQL's js/clear-text-storage-of-sensitive-data flagged when the JWT lived
    // here. All this records is "there was a session in this browser", so a cold load
    // knows whether to spend a `/auth/refresh` before deciding the visitor is
    // anonymous. The refresh cookie is what actually proves the session.
    localStorage.setItem(SESSION_FLAG, '1');
    this.broadcast({ type: 'token', token });
  }

  clearToken() {
    this.accessToken = null;
    localStorage.removeItem(SESSION_FLAG);
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

    this.refreshInFlight = this.performRefresh().finally(() => {
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

    // Serialise across tabs as well as within one. Web Locks queue rather than fail,
    // so a tab that arrives second waits here instead of racing the first onto a
    // refresh cookie that is already spent.
    return this.withRefreshLock(async () => {
      if (this.accessToken !== staleToken) {
        return;
      }

      const response =
        await this.client.post<ApiResponse<{ access_token: string }>>('/auth/refresh');
      const newToken = response.data.data?.access_token;
      if (newToken) {
        this.setToken(newToken);
      }
    });
  }

  /**
   * Run `fn` while holding the cross-tab refresh lock.
   *
   * jsdom implements neither `navigator.locks` nor a stand-in for it, so the tests run
   * the callback directly. That is the honest fallback: a single tab is exactly the
   * case the lock is not needed for, and `refreshInFlight` still serialises within it.
   */
  private withRefreshLock(fn: () => Promise<void>): Promise<void> {
    if (typeof navigator === 'undefined' || !navigator.locks) {
      return fn();
    }

    return navigator.locks.request(REFRESH_LOCK, fn);
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
