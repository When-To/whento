/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Window event carrying the fact that *another tab* signed this one out.
 *
 * Kept dependency-free so the API client can dispatch it without importing any store
 * (which would form an import cycle); the reset handler lives in
 * `stores/remoteSignout.ts`.
 */
export const REMOTE_SIGNOUT_EVENT = 'whento:remote-signout';

/**
 * Window event carrying the fact that *another tab* started a brand-new session.
 *
 * The receiving tab has just accepted a fresh login's access token over the channel
 * but its Pinia stores still describe the previous (or no) account. The handler in
 * `stores/remoteSignout.ts` resets account-scoped state and hydrates `/auth/me`, so
 * the UI/router agree with the HTTP client about who is signed in.
 */
export const REMOTE_SESSION_EVENT = 'whento:remote-session';
