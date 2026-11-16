/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Whether a normalized failure is a *definitive* rejection of the current session
 * rather than a transient fault.
 *
 * The 401 interceptor normalizes refresh and replay failures to the API error
 * envelope; a definitive session rejection always carries the backend's
 * `UNAUTHORIZED` code (as does a replay of a protected request under a freshly
 * rotated token that the backend still refuses, and the stale-session sentinel).
 * Anything else — a 5xx, a network error, a timeout — is transient and must not
 * destroy a possibly still-valid session.
 *
 * Kept dependency-free (like `sessionEvents.ts`) so both the API client's 401
 * interceptor and the auth store can apply the same verdict: the store clears a
 * session only for a definitive rejection, never for a transient fault that a
 * retry could recover from.
 */
export function isDefinitiveRejection(failure: unknown): boolean {
  return (
    typeof failure === 'object' &&
    failure !== null &&
    (failure as { code?: string }).code === 'UNAUTHORIZED'
  );
}
