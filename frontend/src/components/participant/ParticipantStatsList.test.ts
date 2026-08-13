/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { describe, expect, it } from 'vitest';
import { mountWithI18n } from '@/test/harness';
import ParticipantStatsList from './ParticipantStatsList.vue';
import type { ParticipantStat } from './ParticipantStatsList.vue';

/**
 * The tally is how a participant finds the dates everyone shares: selecting names
 * highlights their common days on the grid. The selection lives in the parent, so the
 * only contract here is "render the counts, report the click, reflect what is selected".
 */

const STATS: ParticipantStat[] = [
  { name: 'Ada', count: 4 },
  { name: 'Grace', count: 1 },
  { name: 'Alan', count: 0 },
];

function mountList(selected: string[] = []) {
  return mountWithI18n(ParticipantStatsList, {
    props: { stats: STATS, selected: new Set(selected) },
  });
}

describe('ParticipantStatsList', () => {
  it('renders one row per participant', () => {
    const wrapper = mountList();
    const rows = wrapper.findAll('button');
    expect(rows).toHaveLength(3);
    expect(rows[0].text()).toContain('Ada');
    expect(rows[1].text()).toContain('Grace');
  });

  /*
   * "1 availabilities" is the sort of thing that ships, so the singular and plural keys
   * are separate. The rule is `count > 1`, which means zero also reads as singular —
   * "0 availability". Pinned here as the current behaviour rather than fixed: it is an
   * i18n rule, not a layout one, and English and French disagree about zero.
   */
  it('agrees the count with its noun', () => {
    const rows = mountList().findAll('button');
    expect(rows[0].text()).toContain('availabilities');
    expect(rows[1].text()).toContain('availability');
    expect(rows[1].text()).not.toContain('availabilities');
    expect(rows[2].text()).toContain('0 availability');
  });

  it('reports a click without changing anything itself', async () => {
    const wrapper = mountList();
    await wrapper.findAll('button')[1].trigger('click');

    expect(wrapper.emitted('toggle')).toEqual([['Grace']]);
    // The selection belongs to the parent: nothing here moved on its own.
    expect(wrapper.findAll('button')[1].classes()).not.toContain('border-purple-500');
  });

  it('marks the rows the parent says are selected', () => {
    const rows = mountList(['Ada']).findAll('button');
    expect(rows[0].classes()).toContain('border-purple-500');
    expect(rows[1].classes()).not.toContain('border-purple-500');
  });

  it('says so when the calendar has nobody on it', () => {
    const wrapper = mountWithI18n(ParticipantStatsList, {
      props: { stats: [], selected: new Set<string>() },
    });

    expect(wrapper.findAll('button')).toHaveLength(0);
    expect(wrapper.text()).toContain('No participant');
  });
});
