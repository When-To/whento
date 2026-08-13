/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { describe, expect, it } from 'vitest';
import { mountWithI18n } from '@/test/harness';
import CalendarSharePanel from './CalendarSharePanel.vue';

/**
 * Which links a participant is shown is an access-control decision rendered as markup:
 * the public link is an open invitation, and a calendar with a locked roster must not
 * hand one out. The settings link is only for someone who may actually manage it.
 */

const LINKS = {
  publicLink: 'https://whento.test/c/tok-1',
  icsLink: 'https://whento.test/api/v1/ics/feed/ics-1.ics',
};

function mountPanel(props: Record<string, unknown> = {}) {
  return mountWithI18n(CalendarSharePanel, {
    props: { ...LINKS, showPublicLink: true, ...props },
    global: { stubs: { RouterLink: { template: '<a><slot /></a>', props: ['to'] } } },
  });
}

describe('CalendarSharePanel', () => {
  it('shows both links when the roster is open', () => {
    const values = mountPanel()
      .findAll('input')
      .map(i => (i.element as HTMLInputElement).value);

    expect(values).toEqual([LINKS.publicLink, LINKS.icsLink]);
  });

  /*
   * A locked calendar has a fixed list of participants; anyone following a public link
   * would be unable to claim a name, so the link is withheld rather than shown broken.
   */
  it('withholds the public link on a locked calendar', () => {
    const wrapper = mountPanel({ showPublicLink: false });
    const values = wrapper.findAll('input').map(i => (i.element as HTMLInputElement).value);

    expect(values).toEqual([LINKS.icsLink]);
    expect(wrapper.text()).not.toContain('Public link');
  });

  it('shows the links read-only, so a stray keystroke cannot corrupt one', () => {
    for (const input of mountPanel().findAll('input')) {
      expect(input.attributes('readonly')).toBeDefined();
    }
  });

  it('asks the page to copy rather than reaching for the clipboard itself', async () => {
    const wrapper = mountPanel();
    const buttons = wrapper.findAll('button');

    await buttons[0].trigger('click');
    await buttons[1].trigger('click');

    expect(wrapper.emitted('copy')).toEqual([[LINKS.publicLink], [LINKS.icsLink]]);
  });

  it('offers the settings link only when the reader may manage the calendar', () => {
    expect(mountPanel().find('a').exists()).toBe(false);

    const manager = mountPanel({ settingsPath: '/calendars/cal-1/settings' });
    expect(manager.find('a').exists()).toBe(true);
    expect(manager.text()).toContain('Edit calendar');
  });
});
