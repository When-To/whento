/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { applyTheme, resolveTheme, THEME_STORAGE_KEY } from './theme';

/** jsdom has no matchMedia; every test states the OS preference it assumes. */
function systemPrefersDark(dark: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockReturnValue({ matches: dark, media: '(prefers-color-scheme: dark)' })
  );
}

beforeEach(() => {
  localStorage.clear();
  document.documentElement.classList.remove('dark');
  systemPrefersDark(false);
});

describe('resolveTheme', () => {
  it.each(['dark', 'light'] as const)('honours the stored choice: %s', stored => {
    // The stored choice wins over the OS, in both directions: someone who picked
    // light on a dark-themed machine must not be flipped back on every load.
    systemPrefersDark(stored === 'light');
    localStorage.setItem(THEME_STORAGE_KEY, stored);

    expect(resolveTheme()).toBe(stored);
  });

  it.each([true, false])('falls back to the OS preference (dark=%s)', dark => {
    systemPrefersDark(dark);

    expect(resolveTheme()).toBe(dark ? 'dark' : 'light');
  });

  it('ignores a stored value that is neither theme', () => {
    systemPrefersDark(true);
    localStorage.setItem(THEME_STORAGE_KEY, 'solarized');

    expect(resolveTheme()).toBe('dark');
  });

  it('agrees with the pre-paint snippet in index.html', () => {
    // The snippet is duplicated on purpose — it has to run before the bundle exists —
    // so the two must not drift. This mirrors its logic exactly.
    const snippet = () => {
      const stored = localStorage.getItem('theme');
      return stored === 'dark' ||
        (stored !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)
        ? 'dark'
        : 'light';
    };

    for (const dark of [true, false]) {
      systemPrefersDark(dark);
      for (const stored of [null, 'dark', 'light', 'nonsense']) {
        localStorage.clear();
        if (stored !== null) localStorage.setItem(THEME_STORAGE_KEY, stored);

        expect(resolveTheme()).toBe(snippet());
      }
    }
  });
});

describe('applyTheme', () => {
  it('adds the class for dark', () => {
    applyTheme('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('removes it for light', () => {
    document.documentElement.classList.add('dark');
    applyTheme('light');

    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('is idempotent', () => {
    applyTheme('dark');
    applyTheme('dark');

    expect(document.documentElement.className.split(/\s+/).filter(c => c === 'dark')).toHaveLength(
      1
    );
  });
});
