/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Shared scaffolding for component and composable tests.
 *
 * The real locale files are used rather than a stub dictionary: a test that asserts on
 * `t('availability.allDay')` returning some placeholder proves nothing, and using the
 * shipped messages means a deleted key fails a test instead of silently rendering its
 * own name.
 */

import { createI18n } from 'vue-i18n';
import { defineComponent, type App } from 'vue';
import { mount, type VueWrapper } from '@vue/test-utils';
import en from '@/locales/en.json';
import fr from '@/locales/fr.json';

export type TestLocale = 'en' | 'fr';

/** A fresh vue-i18n instance. Never shared, so one test cannot leak a locale into another. */
export function createTestI18n(locale: TestLocale = 'en') {
  return createI18n({
    legacy: false,
    locale,
    fallbackLocale: 'en',
    messages: { en, fr },
  });
}

/** What `withComposable` hands back: the composable's return value, plus the host. */
export interface ComposableHarness<T> {
  readonly result: T;
  readonly wrapper: VueWrapper;
  /** Unmount the host, running every cleanup hook the composable registered. */
  unmount(): void;
}

/**
 * Run a composable inside a real component instance.
 *
 * `useI18n`, `onMounted` and `watch` all need one; an `effectScope` alone is not enough
 * for anything that reaches for the current instance.
 */
export function withComposable<T>(
  setup: () => T,
  options: { locale?: TestLocale; plugins?: unknown[] } = {}
): ComposableHarness<T> {
  let result!: T;

  const host = defineComponent({
    setup() {
      result = setup();
      return () => null;
    },
  });

  const wrapper = mount(host, {
    global: {
      plugins: [
        createTestI18n(options.locale),
        ...((options.plugins ?? []) as Array<(app: App) => void>),
      ],
    },
  });

  return { result, wrapper, unmount: () => wrapper.unmount() };
}

/**
 * The payload of the most recent `event` emission, or `undefined` if it never fired.
 *
 * A model that is set twice emits twice; asserting on the first payload would pass while
 * the component ends up in the wrong state.
 */
export function lastEmit(wrapper: VueWrapper, event: string): unknown[] | undefined {
  const emissions = wrapper.emitted(event);
  return emissions === undefined ? undefined : emissions[emissions.length - 1];
}

/** Mount a component with i18n installed, which every component in this app needs. */
export function mountWithI18n(
  component: Parameters<typeof mount>[0],
  options: Record<string, unknown> = {},
  locale: TestLocale = 'en'
): VueWrapper {
  const globalOptions = (options.global ?? {}) as Record<string, unknown>;
  return mount(component, {
    ...options,
    global: {
      ...globalOptions,
      plugins: [
        createTestI18n(locale),
        ...((globalOptions.plugins ?? []) as Array<(app: App) => void>),
      ],
    },
  } as Parameters<typeof mount>[1]);
}
