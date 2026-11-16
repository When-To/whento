/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * Resolve the theme before the first paint. Loaded from index.html as a plain,
 * blocking <script src>: it has to be a file, not an inline block, because the
 * CSP is script-src 'self' and the browser refuses inline scripts outright.
 *
 * This must stay in step with resolveTheme() in src/utils/theme.ts.
 */
(function () {
  try {
    var stored = localStorage.getItem('theme');
    var dark =
      stored === 'dark' ||
      (stored !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    if (dark) document.documentElement.classList.add('dark');
  } catch {
    /* private mode / storage disabled: fall through to the light default */
  }
})();
