/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * Licensed under the Business Source License 1.1
 * See LICENSE file for details
 */

import js from '@eslint/js';
import pluginVue from 'eslint-plugin-vue';
import pluginVueA11y from 'eslint-plugin-vuejs-accessibility';
import tseslint from 'typescript-eslint';
import vueParser from 'vue-eslint-parser';
import globals from 'globals';
import eslintConfigPrettier from 'eslint-config-prettier';

/*
 * ---------------------------------------------------------------------------
 * The debt ratchet
 * ---------------------------------------------------------------------------
 *
 * Three rule families below are switched on as `error` even though the tree does
 * not satisfy them yet:
 *
 *   @typescript-eslint/no-explicit-any   120 occurrences
 *   no-console                            31 occurrences
 *   vuejs-accessibility/*                155 occurrences across 8 rules
 *
 * Turning them on as plain errors would fail every build; leaving them `off` — as
 * `no-explicit-any` and `no-console` were until now — means nobody ever sees them
 * and the counts only grow. Setting them to `warn` under a single global
 * `--max-warnings N` would cap the total, but the budget is fungible: deleting an
 * `any` in one file pays for a new `console.log` in another, and a brand-new file
 * can arrive dirty as long as someone cleaned up elsewhere.
 *
 * So the counts above are not enforced by a number in package.json. They are
 * recorded per file and per rule in `eslint-suppressions.json` (ESLint's native
 * bulk-suppression mechanism), and `npm run lint` runs with `--max-warnings 0`
 * against that inventory. The consequences:
 *
 *   - a violation in a file that has no entry for that rule fails immediately,
 *     so new and already-clean files are held to the real rule;
 *   - one more violation in a file that already has entries fails too, because
 *     the recorded count is exact;
 *   - removing a violation also fails, with ESLint telling you to run
 *     `npm run lint:prune`. That is deliberate: it forces the shrunken inventory
 *     into the diff, which is what makes the debt visible and one-directional.
 *
 * The inventory can therefore only go down. Regenerating it wholesale
 * (`npm run lint:suppress`) is possible but shows up as a large diff in review,
 * which is the point.
 *
 * The 31 `console.*` calls are almost all `console.error`, and in MFA enrolment,
 * passkey management and the admin views they are currently the *only* place a
 * real user-facing failure is reported. They are flagged here so they get a
 * proper destination (toast, error state); they must not simply be deleted.
 *
 * Fixing the accessibility findings is out of scope for this pass — the goal here
 * is that the number is known and cannot grow.
 */

export default [
  // Ignore patterns (replaces .eslintignore)
  {
    ignores: [
      'dist/**',
      'node_modules/**',
      // Generated run output. `coverage/` in particular ships bundled vendor JS
      // that ESLint happily lints, which would make CI depend on whether someone
      // had run `npm run test:coverage` first.
      'coverage/**',
      'test-results/**',
      'playwright-report/**',
      'blob-report/**',
      '*.d.ts',
    ],
  },

  // Base JS config
  js.configs.recommended,

  // TypeScript config
  ...tseslint.configs.recommended,

  // Vue config
  ...pluginVue.configs['flat/recommended'],

  // Vue accessibility config (applies to **/*.vue only)
  ...pluginVueA11y.configs['flat/recommended'],

  // Custom rules for Vue/TS files
  {
    files: ['**/*.vue', '**/*.ts', '**/*.tsx'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: tseslint.parser,
        ecmaVersion: 'latest',
        sourceType: 'module',
      },
      globals: {
        ...globals.browser,
      },
    },
    rules: {
      // Vue specific
      'vue/multi-word-component-names': 'off',
      'vue/no-v-html': 'off',
      'vue/require-default-prop': 'off',
      'vue/no-deprecated-filter': 'off', // False positives with TypeScript union types in templates

      // TypeScript specific
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_',
        },
      ],
      // Ratcheted — see the note at the top of this file.
      '@typescript-eslint/no-explicit-any': 'error',

      // General
      // Ratcheted — see the note at the top of this file. No `allow` list on
      // purpose: `console.error` is the majority of the debt and the part that
      // matters, since it is standing in for real error handling.
      'no-console': 'error',
      'no-debugger': 'error',
    },
  },

  // Node-side files: build tooling, Playwright configs and specs, the dev harness
  // entrypoint. They run under Node, not in the browser.
  {
    files: [
      '*.config.js',
      '*.config.ts',
      'playwright.config.ts',
      'playwright.backend.config.ts',
      'e2e/**/*.ts',
      'e2e-backend/**/*.ts',
    ],
    languageOptions: {
      globals: {
        ...globals.node,
      },
    },
  },

  // Prettier config - MUST be last to disable conflicting ESLint rules
  // @ts-expect-error - eslintConfigPrettier is used in the config array
  eslintConfigPrettier,
];
