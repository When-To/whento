/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { afterEach, describe, expect, it } from 'vitest';
import { confirm, resolveConfirm, useConfirm } from './useConfirm';

const { pending } = useConfirm();

afterEach(() => {
  // Never leave a request open for the next test: it would cancel on the first
  // `confirm()` there and make the failure look like it came from that test.
  if (pending.value) resolveConfirm(false);
});

describe('useConfirm', () => {
  it('starts with nothing open', () => {
    expect(pending.value).toBeNull();
  });

  it('publishes the request for the dialog to render', () => {
    void confirm({ message: 'Delete this calendar?', title: 'Careful' });

    expect(pending.value?.options).toMatchObject({
      message: 'Delete this calendar?',
      title: 'Careful',
    });
  });

  it('resolves true when confirmed', async () => {
    const answer = confirm({ message: 'Go ahead?' });
    resolveConfirm(true);

    await expect(answer).resolves.toBe(true);
  });

  it('resolves false when cancelled', async () => {
    const answer = confirm({ message: 'Go ahead?' });
    resolveConfirm(false);

    await expect(answer).resolves.toBe(false);
  });

  it('closes the dialog once answered', async () => {
    const answer = confirm({ message: 'Go ahead?' });
    resolveConfirm(true);
    await answer;

    expect(pending.value).toBeNull();
  });

  it('never rejects, so callers need no try/catch', async () => {
    const answer = confirm({ message: 'Go ahead?' });
    resolveConfirm(false);

    await expect(answer).resolves.toBe(false);
  });

  it('answers a superseded request rather than leaving its caller hanging', async () => {
    // Two destructive buttons clicked in quick succession. Dropping the first promise
    // would deadlock whatever was awaiting it, half-way through a delete.
    const first = confirm({ message: 'First?' });
    const second = confirm({ message: 'Second?' });

    await expect(first).resolves.toBe(false);
    expect(pending.value?.options.message).toBe('Second?');

    resolveConfirm(true);
    await expect(second).resolves.toBe(true);
  });

  it('ignores a second answer to the same request', async () => {
    const answer = confirm({ message: 'Go ahead?' });
    resolveConfirm(true);
    resolveConfirm(false);

    await expect(answer).resolves.toBe(true);
  });

  it('does nothing when answered with no request open', () => {
    expect(() => resolveConfirm(true)).not.toThrow();
    expect(pending.value).toBeNull();
  });

  it('carries the optional labels through untouched', () => {
    void confirm({
      message: 'Regenerate the token?',
      detail: 'The old link stops working.',
      confirmLabel: 'Regenerate',
      cancelLabel: 'Keep it',
      tone: 'primary',
    });

    expect(pending.value?.options).toEqual({
      message: 'Regenerate the token?',
      detail: 'The old link stops working.',
      confirmLabel: 'Regenerate',
      cancelLabel: 'Keep it',
      tone: 'primary',
    });
  });

  it('defaults tone to danger by omission, leaving the choice to the dialog', () => {
    // The dialog reads `tone ?? 'danger'`, which is what focuses *cancel* first so a
    // reflexive Enter cannot delete anything.
    void confirm({ message: 'Delete?' });

    expect(pending.value?.options.tone).toBeUndefined();
  });
});
