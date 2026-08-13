/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { describe, expect, it } from 'vitest';
import { i18n } from '@/i18n';
import { useAsyncActions } from './asyncAction';

/** A promise with its resolvers exposed, so a test can control when it settles. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('useAsyncActions', () => {
  describe('loading', () => {
    it('is false before anything runs', () => {
      expect(useAsyncActions().loading.value).toBe(false);
    });

    it('is true while an action is in flight and false once it settles', async () => {
      const actions = useAsyncActions();
      const gate = deferred<string>();

      const call = actions.run('errors.generic', () => gate.promise);
      expect(actions.loading.value).toBe(true);

      gate.resolve('done');
      await expect(call).resolves.toBe('done');
      expect(actions.loading.value).toBe(false);
    });

    it('stays true until the *last* concurrent action settles', async () => {
      // The defect this replaces: calendar.ts had one `loading` flag that twelve
      // actions each set and cleared, so the first call to finish turned the spinner
      // off while the second was still writing.
      const actions = useAsyncActions();
      const first = deferred<void>();
      const second = deferred<void>();

      const a = actions.run('errors.generic', () => first.promise);
      const b = actions.run('errors.generic', () => second.promise);
      expect(actions.pending.value).toBe(2);

      first.resolve();
      await a;
      expect(actions.loading.value).toBe(true);
      expect(actions.pending.value).toBe(1);

      second.resolve();
      await b;
      expect(actions.loading.value).toBe(false);
      expect(actions.pending.value).toBe(0);
    });

    it('clears even when the action rejects', async () => {
      const actions = useAsyncActions();

      await expect(
        actions.run('errors.generic', () => Promise.reject(new Error('boom')))
      ).rejects.toThrow('boom');

      expect(actions.loading.value).toBe(false);
      expect(actions.pending.value).toBe(0);
    });

    it('is not left stuck by a concurrent failure', async () => {
      const actions = useAsyncActions();
      const ok = deferred<void>();

      const failing = actions.run('errors.generic', () => Promise.reject(new Error('boom')));
      const succeeding = actions.run('errors.generic', () => ok.promise);

      await expect(failing).rejects.toThrow('boom');
      expect(actions.loading.value).toBe(true);

      ok.resolve();
      await succeeding;
      expect(actions.loading.value).toBe(false);
    });
  });

  describe('error', () => {
    it('is null on success', async () => {
      const actions = useAsyncActions();
      await actions.run('errors.generic', () => Promise.resolve(1));
      expect(actions.error.value).toBeNull();
    });

    it('translates the API error code rather than showing the backend message', async () => {
      const actions = useAsyncActions();

      await expect(
        actions.run('calendar.fetchError', () =>
          Promise.reject({ code: 'NOT_FOUND', message: 'sql: no rows in result set' })
        )
      ).rejects.toBeDefined();

      expect(actions.error.value).toBe(i18n.global.t('errors.notFound'));
      expect(actions.error.value).not.toContain('sql');
    });

    it('falls back to the key the action names for itself', async () => {
      const actions = useAsyncActions();

      await expect(
        actions.run('calendar.deleteError', () => Promise.reject(new Error('socket hang up')))
      ).rejects.toThrow();

      expect(actions.error.value).toBe(i18n.global.t('calendar.deleteError'));
    });

    it('never surfaces a raw i18n key', async () => {
      const actions = useAsyncActions();
      await expect(
        actions.run('calendar.createError', () => Promise.reject({ code: 'CONFLICT' }))
      ).rejects.toBeDefined();

      expect(actions.error.value).not.toMatch(/^errors\./);
      expect(actions.error.value).not.toMatch(/^calendar\./);
    });

    it('honours per-call overrides', async () => {
      const actions = useAsyncActions();

      await expect(
        actions.run('auth.loginError', () => Promise.reject({ code: 'UNAUTHORIZED' }), {
          overrides: { UNAUTHORIZED: 'auth.invalidCredentials' },
        })
      ).rejects.toBeDefined();

      expect(actions.error.value).toBe(i18n.global.t('auth.invalidCredentials'));
    });

    it('is cleared when the next action starts', async () => {
      const actions = useAsyncActions();
      await expect(
        actions.run('errors.generic', () => Promise.reject(new Error('boom')))
      ).rejects.toThrow();
      expect(actions.error.value).not.toBeNull();

      await actions.run('errors.generic', () => Promise.resolve());
      expect(actions.error.value).toBeNull();
    });

    it('is cleared by clearError', async () => {
      const actions = useAsyncActions();
      await expect(
        actions.run('errors.generic', () => Promise.reject(new Error('boom')))
      ).rejects.toThrow();

      actions.clearError();
      expect(actions.error.value).toBeNull();
    });
  });

  it('re-throws so callers can still branch on the failure', async () => {
    const actions = useAsyncActions();
    const cause = { code: 'FORBIDDEN', message: 'nope' };

    await expect(actions.run('errors.generic', () => Promise.reject(cause))).rejects.toBe(cause);
  });

  it('passes the resolved value straight through', async () => {
    const actions = useAsyncActions();
    await expect(
      actions.run('errors.generic', () => Promise.resolve({ id: 'x' }))
    ).resolves.toEqual({ id: 'x' });
  });

  it('gives each store its own state', async () => {
    const a = useAsyncActions();
    const b = useAsyncActions();
    const gate = deferred<void>();

    const call = a.run('errors.generic', () => gate.promise);
    expect(a.loading.value).toBe(true);
    expect(b.loading.value).toBe(false);

    gate.resolve();
    await call;
  });
});
