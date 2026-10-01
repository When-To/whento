/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import { validatePassword } from './password';

/**
 * Must mirror the backend `strongpassword` rule (pkg/validator): at least 12
 * characters with at least one uppercase, one lowercase, one digit and one
 * special character. The server stays the authority; these tests pin the
 * client-side pre-check to the same rule so registration and bootstrap do not
 * submit a password the backend will refuse.
 */
describe('validatePassword', () => {
  it('accepts a strong password', () => {
    expect(validatePassword('Str0ng!Passw0rd')).toBeNull();
  });

  it('rejects fewer than 12 characters', () => {
    expect(validatePassword('Str0ng!Pa')).not.toBeNull();
  });

  it('rejects a missing uppercase letter', () => {
    expect(validatePassword('str0ng!password')).not.toBeNull();
  });

  it('rejects a missing lowercase letter', () => {
    expect(validatePassword('STR0NG!PASSWORD')).not.toBeNull();
  });

  it('rejects a missing digit', () => {
    expect(validatePassword('Strpng!Password')).not.toBeNull();
  });

  it('rejects a missing special character', () => {
    expect(validatePassword('Str0ngPassword1')).not.toBeNull();
  });

  it('returns the too-short translation key so the form shows the real rule', () => {
    expect(validatePassword('short')).toBe('errors.passwordTooShort');
  });

  it('counts the minimum in code points, not UTF-16 code units', () => {
    // 4 emoji (1 code point but 2 UTF-16 code units each) + 8 ASCII = exactly 12
    // code points but 16 UTF-16 code units: the old `string.length` check would
    // have counted 16, so only a code-point count can tell this apart.
    const password = '😀'.repeat(4) + 'abcdeA1!';
    expect(password.length).toBe(16); // UTF-16 code units
    expect(Array.from(password).length).toBe(12); // code points
    expect(validatePassword(password)).toBeNull();
  });

  it('enforces the 72-UTF-8-byte ceiling with the too-long translation key', () => {
    // 25 × 3-byte '€' + the classes = 75+ bytes: 25 code points, well under any
    // character ceiling, but over bcrypt's 72-byte limit.
    const over = '€'.repeat(25) + 'A-a1!';
    const bytes = new TextEncoder().encode(over).byteLength;
    expect(bytes).toBeGreaterThan(72);
    expect(validatePassword(over)).toBe('errors.passwordTooLong');
  });

  it('accepts the largest password that fits in 72 UTF-8 bytes', () => {
    // 22 × 3-byte '€' + 5 single-byte = 71 bytes, with all classes present.
    const under = '€'.repeat(22) + 'A-a1!';
    expect(new TextEncoder().encode(under).byteLength).toBeLessThanOrEqual(72);
    expect(validatePassword(under)).toBeNull();
  });
});
