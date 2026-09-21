/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { describe, expect, it } from 'vitest';
import { mountWithI18n } from '@/test/harness';
import ActivityLogSection from './ActivityLogSection.vue';
import type { DateActivityEntry } from '@/types';

/**
 * The owner-facing journal.
 *
 * Both slots are independently absent — a date can have a join and no withdrawal, a
 * withdrawal and no join, or neither while still having people available — so what is
 * worth testing is that each absence renders as an absence rather than as `undefined`,
 * `null` or a blank line.
 */

const ENTRY: DateActivityEntry = {
  date: '2026-03-05',
  threshold: 2,
  count: 2,
  available: [
    { participant_id: 'a1', name: 'Alice' },
    { participant_id: 'b2', name: 'Bob' },
  ],
  last_joined: { participant_id: 'b2', name: 'Bob', at: '2026-03-04T10:00:00Z' },
  last_withdrawn: null,
};

function mountSection(props: Partial<Record<string, unknown>> = {}) {
  return mountWithI18n(ActivityLogSection, {
    props: { entries: [], loading: false, error: false, ...props },
  });
}

describe('ActivityLogSection', () => {
  it('renders one row per date, naming the last joiner and everyone available', () => {
    const wrapper = mountSection({ entries: [ENTRY] });
    const text = wrapper.text();

    expect(wrapper.findAll('li')).toHaveLength(1);
    expect(text).toContain('Bob');
    expect(text).toContain('Alice');
    // The count is rendered against the threshold, not on its own.
    expect(text).toContain('2/2');
  });

  it('highlights a withdrawal when there is one', () => {
    const withdrawn: DateActivityEntry = {
      ...ENTRY,
      last_withdrawn: { participant_id: 'c3', name: 'Carol', at: '2026-03-04T18:00:00Z' },
    };

    const wrapper = mountSection({ entries: [withdrawn] });

    expect(wrapper.text()).toContain('Carol');
    // The withdrawal is the line the owner opened the section for, so it is the only
    // one carrying colour.
    expect(wrapper.html()).toContain('text-danger-600');
  });

  it('says so when a date has no journal entry at all', () => {
    const bare: DateActivityEntry = {
      ...ENTRY,
      available: [],
      count: 0,
      last_joined: null,
      last_withdrawn: null,
    };

    const wrapper = mountSection({ entries: [bare] });
    const text = wrapper.text();

    // A null slot must read as an absence, never as "undefined" or an empty gap.
    expect(text).toContain('no entry');
    expect(text).toContain('nobody');
    expect(text).not.toContain('undefined');
    expect(text).not.toContain('null');
  });

  it('renders an empty state rather than an empty list', () => {
    const wrapper = mountSection({ entries: [] });

    expect(wrapper.findAll('li')).toHaveLength(0);
    expect(wrapper.text()).toContain('No activity');
  });

  it('shows the error state instead of an empty state when the load failed', () => {
    // These are different things, and telling the owner "no activity" when the request
    // actually failed is the worse of the two lies.
    const wrapper = mountSection({ entries: [], error: true });

    expect(wrapper.text()).toContain('could not be loaded');
    expect(wrapper.text()).not.toContain('No activity');
  });

  it('shows the loading state while the range is in flight', () => {
    const wrapper = mountSection({ entries: [], loading: true });

    expect(wrapper.text()).toContain('Loading');
    expect(wrapper.findAll('li')).toHaveLength(0);
  });
});
