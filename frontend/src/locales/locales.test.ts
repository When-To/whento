/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * The i18n ratchet.
 *
 * Every rule below corresponds to a defect the audit found in the tree, and exists so
 * that the defect cannot come back quietly:
 *
 *  - `en.json` and `fr.json` drifting apart, so a French user silently reads English
 *    through `fallbackLocale`;
 *  - `t('some.key')` pointing at a key that no locale defines, which renders the key
 *    path itself on the page;
 *  - keys nobody references piling up — 14% of the file at the worst point;
 *  - user-visible English written straight into a template, either as a literal
 *    attribute or as a `locale === 'fr' ? … : …` ternary that a third language can
 *    never satisfy;
 *  - the backend's English developer prose (`err.message`) being preferred over the
 *    translated string in a `catch`.
 *
 * These are source-text assertions rather than runtime ones on purpose: they hold for
 * code paths no test happens to execute, which is exactly where this debt accumulated.
 */

import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const LOCALES = ['en', 'fr'] as const;

type Messages = { [key: string]: string | Messages };

function load(locale: string): Messages {
  return JSON.parse(fs.readFileSync(path.join(SRC, 'locales', `${locale}.json`), 'utf8'));
}

/** `{ a: { b: 'x' } }` -> `{ 'a.b': 'x' }`. */
function flatten(messages: Messages, prefix = '', out: Record<string, string> = {}) {
  for (const [key, value] of Object.entries(messages)) {
    if (typeof value === 'string') out[prefix + key] = value;
    else flatten(value, `${prefix}${key}.`, out);
  }
  return out;
}

/** Every `.ts` / `.vue` file under `src/`, excluding the locale JSON itself. */
function sourceFiles(dir = SRC): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry: fs.Dirent) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(full);
    if (!/\.(ts|vue)$/.test(entry.name)) return [];
    if (full.includes(`${path.sep}locales${path.sep}`)) return [];
    return [full];
  });
}

/**
 * Drop whole-line comments.
 *
 * `stores/asyncAction.ts` documents the very anti-patterns asserted against below by
 * quoting them, so scanning raw text would report the explanation as an offence.
 */
function stripComments(text: string): string {
  return text
    .split('\n')
    .map(line => (/^\s*(\/\/|\/\*|\*)/.test(line) ? '' : line))
    .join('\n');
}

const FILES = sourceFiles();
const SOURCES = FILES.map(
  file => [path.relative(SRC, file), stripComments(fs.readFileSync(file, 'utf8'))] as const
);
const ALL_SOURCE = SOURCES.map(([, text]) => text).join('\n');

const en = flatten(load('en'));
const fr = flatten(load('fr'));
const MESSAGES: Record<string, Record<string, string>> = { en, fr };

/**
 * Key paths built at runtime instead of written out, which the literal scan below cannot
 * see. Each entry needs a reference to the code that builds it.
 *
 * `validation.fields.<field>.<rule>` — `utils/errorTranslator.ts` interpolates the field
 * name the backend reports and the rule it inferred from the message, so any field × rule
 * pair under this prefix is live even though no file spells it out.
 */
const DYNAMIC_KEY_PREFIXES = ['validation.fields.'];

/**
 * Values that are legitimately identical in English and French: loanwords ("Discord"),
 * proper nouns, and locale-neutral input masks. A ratchet rather than an allowlist, so it
 * cannot grow without someone changing this number and explaining why in review.
 */
const MAX_IDENTICAL_VALUES = 37;

describe('locale files', () => {
  it('define exactly the same key paths in every locale', () => {
    const reference = Object.keys(en).sort();
    for (const locale of LOCALES) {
      expect(Object.keys(MESSAGES[locale]).sort(), `${locale}.json key paths`).toEqual(reference);
    }
  });

  it('has no blank message', () => {
    for (const locale of LOCALES) {
      const blank = Object.entries(MESSAGES[locale])
        .filter(([, value]) => value.trim() === '')
        .map(([key]) => key);
      expect(blank, `blank messages in ${locale}.json`).toEqual([]);
    }
  });

  it('uses the same interpolation placeholders in every locale', () => {
    // A translator dropping `{count}` produces a message that renders a literal hole.
    const named = (value: string) =>
      [...value.matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort();

    const mismatched = Object.keys(en).filter(
      key => named(en[key]).join(',') !== named(fr[key]).join(',')
    );
    expect(mismatched, 'keys whose placeholders differ between en and fr').toEqual([]);
  });

  it('uses the same number of plural branches in every locale', () => {
    const branches = (value: string) => value.split('|').length;
    const mismatched = Object.keys(en).filter(key => branches(en[key]) !== branches(fr[key]));
    expect(mismatched, 'keys whose plural branches differ between en and fr').toEqual([]);
  });

  it('keeps French an actual translation rather than a copy of English', () => {
    const identical = Object.keys(en).filter(key => en[key] === fr[key]);
    expect(
      identical.length,
      `keys with identical en/fr values (ratchet ${MAX_IDENTICAL_VALUES}):\n${identical.join('\n')}`
    ).toBeLessThanOrEqual(MAX_IDENTICAL_VALUES);
  });
});

describe('translation keys used by the app', () => {
  /** `t('a.b')` and `$t('a.b')`, but not `foo.t('a.b')` or `somethingt('a.b')`. */
  const CALL = /(?<![A-Za-z0-9_$.])\$?t\(\s*'([a-zA-Z][\w.]*)'/g;

  it('all resolve in every locale', () => {
    const missing: string[] = [];
    for (const [file, text] of SOURCES) {
      for (const match of text.matchAll(CALL)) {
        const key = match[1];
        if (!key.includes('.')) continue; // not a locale path
        for (const locale of LOCALES) {
          if (MESSAGES[locale][key] === undefined) missing.push(`${file}: ${key} (${locale})`);
        }
      }
    }
    expect(missing, 't() calls with no matching entry').toEqual([]);
  });

  it('cover every key the locale files define', () => {
    const orphans = Object.keys(en).filter(
      key =>
        !ALL_SOURCE.includes(key) && !DYNAMIC_KEY_PREFIXES.some(prefix => key.startsWith(prefix))
    );
    expect(
      orphans,
      'keys no source file references; delete them, or add the prefix that builds them to DYNAMIC_KEY_PREFIXES'
    ).toEqual([]);
  });

  it('never pass a default message as the second argument', () => {
    // `t(key, 'Some text')` is a real vue-i18n overload — the string is used when the key
    // is missing. Since the test above proves no key is missing, every such default is
    // dead weight, and several used to carry French, which hid the intent entirely.
    const offenders: string[] = [];
    for (const [file, text] of SOURCES) {
      for (const match of text.matchAll(
        /(?<![A-Za-z0-9_$.])\$?t\(\s*'[^']+'\s*,\s*'(?:[^'\\]|\\.)*'\s*\)/g
      )) {
        offenders.push(`${file}: ${match[0].replace(/\s+/g, ' ')}`);
      }
    }
    expect(offenders, 'redundant default-message arguments').toEqual([]);
  });
});

describe('no user-visible text outside the locale files', () => {
  it('branches on no locale in a template', () => {
    // `locale === 'fr' ? 'Jour' : 'Day'` cannot express a third language.
    const offenders: string[] = [];
    for (const [file, text] of SOURCES) {
      if (file.endsWith('.test.ts')) continue;
      for (const match of text.matchAll(/locale(?:\.value)?\s*===\s*'[a-z]{2}'\s*\?/g)) {
        offenders.push(`${file}: ${match[0]}`);
      }
    }
    expect(offenders, 'hard-coded per-language ternaries').toEqual([]);
  });

  it('writes no literal text into a translatable attribute', () => {
    // Only the bound forms (`:alt`, `:aria-label`, …) can be translated. `alt=""` is
    // allowed: an empty alt is the correct markup for a decorative image.
    const ATTRIBUTE = /(?<![:\w-])(alt|aria-label|aria-description|placeholder|title)="([^"]+)"/g;
    const offenders: string[] = [];
    for (const [file, text] of SOURCES) {
      if (!file.endsWith('.vue')) continue;
      const template = text.slice(0, text.indexOf('<script'));
      for (const match of template.matchAll(ATTRIBUTE)) {
        offenders.push(`${file}: ${match[0]}`);
      }
    }
    expect(offenders, 'literal attributes that should go through t()').toEqual([]);
  });

  it('never prefers the backend message over the translated one', () => {
    // `err.message || t('…')` renders the API's English developer prose whenever it is
    // present, which is almost always. `translateErrorMessage` maps the structured code
    // instead — see stores/asyncAction.ts.
    const offenders: string[] = [];
    for (const [file, text] of SOURCES) {
      if (file === 'utils/errorTranslator.ts' || file === 'api/client.ts') continue;
      if (file.endsWith('.test.ts')) continue;
      for (const match of text.matchAll(/\w+(?:\?)?\.message\s*\|\|/g)) {
        offenders.push(`${file}: ${match[0]}`);
      }
    }
    expect(offenders, 'untranslated backend messages shown to the user').toEqual([]);
  });
});
