/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

export type Theme = 'light' | 'dark';

/** The localStorage key. Duplicated in the pre-paint snippet in index.html. */
export const THEME_STORAGE_KEY = 'theme';

/**
 * The theme to render with: an explicit choice if the user made one, otherwise the
 * operating system preference.
 *
 * The exact same rule is inlined in `index.html` and runs before the bundle is even
 * fetched. That duplication is the point: applying the class from `onMounted`, as
 * this app used to, means every dark-theme user gets a full light-coloured first
 * paint before Vue hydrates. The two must stay in step — change one, change both.
 */
export function resolveTheme(): Theme {
  const stored = localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === 'dark' || stored === 'light') {
    return stored;
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

/** Reflect the theme onto `<html>`, which is what Tailwind's `dark:` variant reads. */
export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle('dark', theme === 'dark');
}
