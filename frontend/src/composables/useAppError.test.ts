/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest';
import { clearFatalError, reportFatalError, useAppError } from './useAppError';

const { failed, reference } = useAppError();

/** The console is the deliberate destination for the technical detail. */
let errorSpy: MockInstance<typeof console.error>;

beforeEach(() => {
  // Silenced so a passing run is quiet, while still asserting on what was written.
  errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  clearFatalError();
  vi.restoreAllMocks();
});

describe('useAppError', () => {
  it('starts clean', () => {
    expect(failed.value).toBe(false);
    expect(reference.value).toBe('');
  });

  it('raises the fallback when something throws', () => {
    reportFatalError(new Error('render blew up'), 'boundary:render');

    expect(failed.value).toBe(true);
    expect(reference.value).not.toBe('');
  });

  it('returns the reference it assigned', () => {
    const ref = reportFatalError(new Error('boom'), 'vue:render');

    expect(ref).toBe(reference.value);
  });

  it('issues a fresh reference per failure', () => {
    const first = reportFatalError(new Error('one'), 'vue:render');
    const second = reportFatalError(new Error('two'), 'vue:render');

    expect(second).not.toBe(first);
  });

  it('logs the error and its context for developers', () => {
    const cause = new Error('render blew up');
    const ref = reportFatalError(cause, 'boundary:render function');

    expect(errorSpy).toHaveBeenCalledWith(`[whento:${ref}] boundary:render function`, cause);
  });

  it('keeps the technical detail out of the reactive state', () => {
    // Everything the fallback screen renders comes from this state, so nothing here
    // may carry a message, a stack, or an internal identifier.
    reportFatalError(new Error('pq: relation "users" does not exist'), 'vue:render');

    expect(reference.value).not.toContain('pq:');
    expect(reference.value).toMatch(/^[0-9A-Z]{1,6}$/);
  });

  it('survives a thrown non-Error', () => {
    expect(() => reportFatalError('just a string', 'vue:render')).not.toThrow();
    expect(failed.value).toBe(true);
  });

  it('clears on recovery', () => {
    reportFatalError(new Error('boom'), 'vue:render');
    clearFatalError();

    expect(failed.value).toBe(false);
    expect(reference.value).toBe('');
  });

  it('exposes read-only state', () => {
    // The fallback must only ever be raised through reportFatalError, so that the
    // logging and the reference cannot be bypassed. Vue reports the refused write on
    // console.warn, which is the assertion below.
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const state = useAppError();

    (state.failed as unknown as { value: boolean }).value = true;

    expect(failed.value).toBe(false);
    expect(warn).toHaveBeenCalled();
  });
});
