/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { describe, expect, it } from 'vitest';
import { lastEmit, mountWithI18n } from '@/test/harness';
import ParticipantAccessToggles from './ParticipantAccessToggles.vue';

/**
 * A calendar either has a fixed roster or lets anyone add themselves. It cannot do both,
 * and the create form and the settings form each used to enforce that separately — one
 * in an inline template handler, the other inside a save handler.
 *
 * The exclusion is now a property of the pair. What the two views still decide for
 * themselves is whether a change is persisted immediately, which is why each toggle
 * emits its own event.
 */

function mountToggles(props: Record<string, unknown> = {}) {
  return mountWithI18n(ParticipantAccessToggles, {
    props: {
      lockParticipants: false,
      allowAnonymousParticipants: false,
      ...props,
    },
  });
}

describe('ParticipantAccessToggles', () => {
  it('shows both choices while neither is taken', () => {
    const wrapper = mountToggles();
    expect(wrapper.find('#lock-participants').exists()).toBe(true);
    expect(wrapper.find('#allow-anonymous-participants').exists()).toBe(true);
  });

  /*
   * The exclusion is enforced by rendering, not by trusting the handler: while one
   * option is on, the other's checkbox is not on the page at all, so there is no click
   * that could turn both on. The handler still clears the counterpart defensively, for
   * the case where the server hands back a calendar with both flags set.
   */
  it('hides the lock while anonymous joining is on, and the reverse', () => {
    const anonymous = mountToggles({ allowAnonymousParticipants: true });
    expect(anonymous.find('#lock-participants').exists()).toBe(false);
    expect(anonymous.find('#allow-anonymous-participants').exists()).toBe(true);

    const locked = mountToggles({ lockParticipants: true });
    expect(locked.find('#lock-participants').exists()).toBe(true);
    expect(locked.find('#allow-anonymous-participants').exists()).toBe(false);
  });

  it('reports the lock being turned on, so the settings view can persist it', async () => {
    const wrapper = mountToggles();

    await wrapper.find('#lock-participants').setValue(true);

    expect(lastEmit(wrapper, 'update:lockParticipants')).toEqual([true]);
    expect(wrapper.emitted('change:lock')).toHaveLength(1);
    expect(wrapper.emitted('change:anonymous')).toBeUndefined();
  });

  it('reports anonymous joining being turned on', async () => {
    const wrapper = mountToggles();

    await wrapper.find('#allow-anonymous-participants').setValue(true);

    expect(lastEmit(wrapper, 'update:allowAnonymousParticipants')).toEqual([true]);
    expect(wrapper.emitted('change:anonymous')).toHaveLength(1);
    expect(wrapper.emitted('change:lock')).toBeUndefined();
  });

  it('reports a setting being turned back off', async () => {
    const wrapper = mountToggles({ lockParticipants: true });

    await wrapper.find('#lock-participants').setValue(false);

    expect(lastEmit(wrapper, 'update:lockParticipants')).toEqual([false]);
    expect(wrapper.emitted('change:lock')).toHaveLength(1);
  });

  /*
   * The create form has always rendered these with a `-create` suffix. Two components
   * are never on screen at once today, but the ids are what the labels point at, so the
   * suffix stays configurable rather than hard-coded.
   */
  it('suffixes the ids, and keeps the labels pointing at them', () => {
    const wrapper = mountToggles({ idSuffix: '-create' });

    expect(wrapper.find('#lock-participants-create').exists()).toBe(true);
    expect(wrapper.find('label[for="lock-participants-create"]').exists()).toBe(true);
    expect(wrapper.find('label[for="allow-anonymous-participants-create"]').exists()).toBe(true);
  });
});
