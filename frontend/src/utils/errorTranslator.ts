/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Translates backend validation error messages to i18n translation keys
 *
 * Field-level details are the one place where substring matching is unavoidable:
 * `validator.ValidationError` carries only `{field, message}` over the wire — the
 * go-playground tag that produced it is not serialised — so the English prose is
 * the only signal available. Everything below that granularity (`translateErrorMessage`)
 * uses the structured `ApiError.code` instead.
 */
export function translateValidationError(
  field: string,
  message: string
): { key: string; params?: Record<string, string> } {
  // Extract validation type from backend message
  const messageLower = message.toLowerCase();

  // Map common validation patterns to translation keys
  if (messageLower.includes('required') || messageLower.includes('is required')) {
    return { key: `validation.fields.${field}.required` };
  }

  if (messageLower.includes('valid email') || messageLower.includes('must be a valid email')) {
    return { key: `validation.fields.${field}.email` };
  }

  if (messageLower.includes('at least')) {
    // Extract number from message like "must be at least 2 characters"
    const match = message.match(/(\d+)/);
    const count = match ? match[1] : '';
    return { key: `validation.fields.${field}.min`, params: { count } };
  }

  if (messageLower.includes('at most') || messageLower.includes('must not exceed')) {
    // Extract number from message like "must be at most 100 characters"
    const match = message.match(/(\d+)/);
    const count = match ? match[1] : '';
    return { key: `validation.fields.${field}.max`, params: { count } };
  }

  if (messageLower.includes('one of')) {
    return { key: `validation.fields.${field}.oneof` };
  }

  if (messageLower.includes('timezone')) {
    return { key: `validation.fields.${field}.timezone` };
  }

  // If no specific pattern matched, return the original message
  return { key: message };
}

/**
 * Every code the backend can put in the `{data, error}` envelope, plus the
 * transport codes `ApiClient.normalizeError` falls back to when there is no
 * envelope at all (a DNS failure, a timeout, a proxy 502 with an HTML body).
 *
 * Source of truth: the `ErrCode*` constants in `pkg/httputil/response.go`.
 * Matching on the code rather than on the message is what makes this work for a
 * French user: the backend's `message` is English prose written for developers,
 * and the previous implementation showed it verbatim whenever its substring
 * guesses missed — which was most of the time, since the guesses only covered
 * five phrases out of the entire API surface.
 */
const ERROR_CODE_KEYS: Readonly<Record<string, string>> = {
  // pkg/httputil/response.go
  BAD_REQUEST: 'errors.badRequest',
  UNAUTHORIZED: 'errors.unauthorized',
  FORBIDDEN: 'errors.forbidden',
  NOT_FOUND: 'errors.notFound',
  CONFLICT: 'errors.conflict',
  VALIDATION_ERROR: 'errors.validationError',
  RATE_LIMITED: 'errors.rateLimited',
  INTERNAL_ERROR: 'errors.serverError',
  // pkg/middleware
  TOO_MANY_REQUESTS: 'errors.rateLimited',
  SERVICE_UNAVAILABLE: 'errors.serviceUnavailable',
  // axios transport codes, surfaced by ApiClient.normalizeError
  ERR_NETWORK: 'errors.network',
  ERR_CANCELED: 'errors.network',
  ECONNABORTED: 'errors.timeout',
  ETIMEDOUT: 'errors.timeout',
  UNKNOWN_ERROR: 'errors.generic',
};

/** The default i18n key for anything this module cannot place. */
export const GENERIC_ERROR_KEY = 'errors.generic';

export interface ApiErrorKeyOptions {
  /** Key to use when the error carries no code this module knows. */
  fallback?: string;
  /**
   * Per-call refinements of the shared table, for the cases where one code means
   * something more specific in context — `UNAUTHORIZED` on the login form is
   * "wrong email or password", not the generic "you are not signed in".
   */
  overrides?: Readonly<Record<string, string>>;
}

/** Narrow an unknown catch binding to something with a `code`. */
function codeOf(error: unknown): string | undefined {
  if (typeof error !== 'object' || error === null) return undefined;
  const code = (error as { code?: unknown }).code;
  return typeof code === 'string' && code.length > 0 ? code : undefined;
}

/**
 * Resolve a caught API error to an i18n key.
 *
 * Always returns a key that exists in the locale files, so the caller can hand the
 * result straight to `t()` without comparing it against the raw message first. No
 * backend prose ever reaches the user through this path.
 */
export function translateErrorMessage(error: unknown, options: ApiErrorKeyOptions = {}): string {
  const { fallback = GENERIC_ERROR_KEY, overrides } = options;
  const code = codeOf(error);
  if (code === undefined) return fallback;
  return overrides?.[code] ?? ERROR_CODE_KEYS[code] ?? fallback;
}
