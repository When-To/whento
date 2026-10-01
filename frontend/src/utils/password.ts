/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * The client-side half of the backend's `strongpassword` and `maxbytes` rules
 * (pkg/validator/validateStrongPassword, validateMaxBytes). The server remains
 * the authority — this is here so a form does not submit a password the server
 * will immediately refuse, which used to cost a full round trip on registration,
 * bootstrap, password reset, and password change. Keeping one validator behind
 * a `password` translation key means the forms cannot drift apart again: if the
 * backend rule changes, this file and its tests are where that has to be
 * reflected.
 *
 * Length units match the backend exactly:
 * - the minimum ("12 characters") counts code points, like Go's
 *   utf8.RuneCountInString (and unlike `string.length`, which counts UTF-16
 *   code units);
 * - the maximum counts UTF-8 bytes, like Go's `maxbytes` — bcrypt's real
 *   ceiling is 72 bytes, and a code-point count would let a password made of
 *   multibyte characters through to a bcrypt internal error.
 *
 * Returns the i18n key of the first failing rule, or null when the password
 * satisfies every requirement.
 */
export function validatePassword(password: string): string | null {
  // Minimum 12 characters, in code points — the unit a user actually counts.
  if (Array.from(password).length < 12) {
    return 'errors.passwordTooShort';
  }
  // Bcrypt accepts at most 72 bytes. This is the binding ceiling; see above.
  if (new TextEncoder().encode(password).byteLength > 72) {
    return 'errors.passwordTooLong';
  }
  if (!/[A-Z]/.test(password)) {
    return 'errors.passwordTooShort';
  }
  if (!/[a-z]/.test(password)) {
    return 'errors.passwordTooShort';
  }
  if (!/[0-9]/.test(password)) {
    return 'errors.passwordTooShort';
  }
  // Mirrors pkg/validator's special-character class. `[` and `/` are literal
  // inside a character class, so no escapes there.
  if (!/[!@#$%^&*(),.?":{}|<>_\-+=[\]\\/;'~`]/.test(password)) {
    return 'errors.passwordTooShort';
  }
  return null;
}
