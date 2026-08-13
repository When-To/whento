/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import en from '@/locales/en.json';
import {
  GENERIC_ERROR_KEY,
  translateErrorMessage,
  translateValidationError,
} from './errorTranslator';

/** Resolve a dotted i18n key against a locale file, or undefined if absent. */
function lookup(messages: unknown, key: string): unknown {
  return key
    .split('.')
    .reduce<unknown>(
      (node, part) =>
        typeof node === 'object' && node !== null
          ? (node as Record<string, unknown>)[part]
          : undefined,
      messages
    );
}

describe('translateErrorMessage', () => {
  describe('mapping the structured code', () => {
    it.each([
      ['BAD_REQUEST', 'errors.badRequest'],
      ['UNAUTHORIZED', 'errors.unauthorized'],
      ['FORBIDDEN', 'errors.forbidden'],
      ['NOT_FOUND', 'errors.notFound'],
      ['CONFLICT', 'errors.conflict'],
      ['VALIDATION_ERROR', 'errors.validationError'],
      ['RATE_LIMITED', 'errors.rateLimited'],
      ['TOO_MANY_REQUESTS', 'errors.rateLimited'],
      ['INTERNAL_ERROR', 'errors.serverError'],
      ['SERVICE_UNAVAILABLE', 'errors.serviceUnavailable'],
      ['ERR_NETWORK', 'errors.network'],
      ['ECONNABORTED', 'errors.timeout'],
      ['UNKNOWN_ERROR', 'errors.generic'],
    ])('%s -> %s', (code, expected) => {
      expect(translateErrorMessage({ code, message: 'whatever' })).toBe(expected);
    });

    it('ignores the message entirely', () => {
      // The regression this module exists to prevent: the previous implementation
      // matched substrings of the backend's English prose, so a French user saw
      // "Invalid credentials" whenever the guesses missed — which was most calls.
      const key = translateErrorMessage({
        code: 'NOT_FOUND',
        message: 'sql: no rows in result set',
      });

      expect(key).toBe('errors.notFound');
      expect(key).not.toContain('sql');
    });
  });

  describe('every key it can return exists in the locale files', () => {
    // A key that resolves to nothing is worse than the old behaviour: vue-i18n
    // renders the key itself, so the user would read "errors.serviceUnavailable".
    const codes = [
      'BAD_REQUEST',
      'UNAUTHORIZED',
      'FORBIDDEN',
      'NOT_FOUND',
      'CONFLICT',
      'VALIDATION_ERROR',
      'RATE_LIMITED',
      'TOO_MANY_REQUESTS',
      'INTERNAL_ERROR',
      'SERVICE_UNAVAILABLE',
      'ERR_NETWORK',
      'ERR_CANCELED',
      'ECONNABORTED',
      'ETIMEDOUT',
      'UNKNOWN_ERROR',
    ];

    it.each(codes)('%s', code => {
      expect(typeof lookup(en, translateErrorMessage({ code }))).toBe('string');
    });

    it('and so does the default', () => {
      expect(typeof lookup(en, GENERIC_ERROR_KEY)).toBe('string');
    });
  });

  describe('falling back', () => {
    it('uses the caller fallback for an unmapped code', () => {
      expect(
        translateErrorMessage({ code: 'SOMETHING_NEW' }, { fallback: 'calendar.fetchError' })
      ).toBe('calendar.fetchError');
    });

    it('uses the caller fallback when there is no code at all', () => {
      expect(translateErrorMessage(new Error('boom'), { fallback: 'calendar.deleteError' })).toBe(
        'calendar.deleteError'
      );
    });

    it.each([[undefined], [null], ['a string'], [42], [{}], [{ code: '' }], [{ code: 7 }]])(
      'survives %s and returns the generic key',
      value => {
        expect(translateErrorMessage(value)).toBe(GENERIC_ERROR_KEY);
      }
    );
  });

  describe('per-call overrides', () => {
    it('wins over the shared table', () => {
      // A 401 from /auth/login means "wrong password", not "you are signed out".
      expect(
        translateErrorMessage(
          { code: 'UNAUTHORIZED' },
          { overrides: { UNAUTHORIZED: 'auth.invalidCredentials' } }
        )
      ).toBe('auth.invalidCredentials');
    });

    it('leaves codes it does not name alone', () => {
      expect(
        translateErrorMessage(
          { code: 'NOT_FOUND' },
          { overrides: { UNAUTHORIZED: 'auth.invalidCredentials' } }
        )
      ).toBe('errors.notFound');
    });

    it('does not stop the fallback applying to unknown codes', () => {
      expect(
        translateErrorMessage(
          { code: 'MYSTERY' },
          { fallback: 'auth.loginError', overrides: { UNAUTHORIZED: 'auth.invalidCredentials' } }
        )
      ).toBe('auth.loginError');
    });
  });
});

describe('translateValidationError', () => {
  // Field-level details carry only {field, message}; the go-playground tag that
  // produced them is not serialised, so prose matching is the only signal here.
  it.each([
    ['email', 'This field is required', 'validation.fields.email.required'],
    ['email', 'Must be a valid email address', 'validation.fields.email.email'],
    ['role', 'must be one of [user admin]', 'validation.fields.role.oneof'],
    ['timezone', 'must be a valid timezone', 'validation.fields.timezone.timezone'],
  ])('%s / %s -> %s', (field, message, expected) => {
    expect(translateValidationError(field, message).key).toBe(expected);
  });

  it('extracts the bound from a minimum-length message', () => {
    expect(translateValidationError('display_name', 'must be at least 2 characters')).toEqual({
      key: 'validation.fields.display_name.min',
      params: { count: '2' },
    });
  });

  it.each(['must be at most 100 characters', 'must not exceed 100 characters'])(
    'extracts the bound from %s',
    message => {
      expect(translateValidationError('title', message)).toEqual({
        key: 'validation.fields.title.max',
        params: { count: '100' },
      });
    }
  );

  it('reports an empty count when the message carries no number', () => {
    expect(translateValidationError('title', 'must be at least this long')).toEqual({
      key: 'validation.fields.title.min',
      params: { count: '' },
    });
  });

  it('is case-insensitive', () => {
    expect(translateValidationError('email', 'REQUIRED').key).toBe(
      'validation.fields.email.required'
    );
  });

  it('hands back the message when nothing matches', () => {
    expect(translateValidationError('email', 'Some brand new rule').key).toBe(
      'Some brand new rule'
    );
  });
});
