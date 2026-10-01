/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import {
  orderCalendars,
  orderedIds,
  type DashboardOrderingPrefs,
  type DashboardViewableCard,
} from './dashboardOrdering';

interface Card extends DashboardViewableCard {
  readonly id: string;
  readonly name: string;
}

const cards: Card[] = [
  { id: 'zeta', name: 'Zeta meeting' },
  { id: 'alpha', name: 'Alpha sync' },
  { id: 'mid', name: 'Middle school' },
  { id: 'beta', name: 'Beta launch' },
];

function prefs(overrides: Partial<DashboardOrderingPrefs> = {}): DashboardOrderingPrefs {
  return {
    sortMode: 'alphabetical',
    sortDirection: 'asc',
    pinnedIds: [],
    customOrder: [],
    ...overrides,
  };
}

const names = (result: Card[]) => result.map(c => c.id);

describe('orderCalendars', () => {
  it('sorts alphabetically ascending by default', () => {
    expect(names(orderCalendars(cards, prefs()))).toEqual(['alpha', 'beta', 'mid', 'zeta']);
  });

  it('sorts alphabetically descending', () => {
    expect(names(orderCalendars(cards, prefs({ sortDirection: 'desc' })))).toEqual([
      'zeta',
      'mid',
      'beta',
      'alpha',
    ]);
  });

  it('lifts pinned calendars to the top, sorted within the group', () => {
    const result = orderCalendars(cards, prefs({ pinnedIds: ['zeta', 'beta'] }));
    // Pinned come first (alphabetically), the rest follow alphabetically.
    expect(names(result)).toEqual(['beta', 'zeta', 'alpha', 'mid']);
  });

  it('honours pin order rather than reversing the pinned group on descending sort', () => {
    const result = orderCalendars(
      cards,
      prefs({ sortDirection: 'desc', pinnedIds: ['zeta', 'beta'] })
    );
    expect(names(result)).toEqual(['zeta', 'beta', 'mid', 'alpha']);
  });

  it('sorts by custom order within each group', () => {
    const result = orderCalendars(
      cards,
      prefs({ sortMode: 'custom', customOrder: ['alpha', 'beta', 'mid', 'zeta'] })
    );
    expect(names(result)).toEqual(['alpha', 'beta', 'mid', 'zeta']);
  });

  it('sends calendars the custom order does not know to the stable end', () => {
    const result = orderCalendars(
      cards,
      prefs({ sortMode: 'custom', customOrder: ['zeta', 'alpha'] })
    );
    // zeta and alpha first in that order; beta and mid are unknown, so they keep
    // their input relative order (mid before beta) at the end.
    expect(names(result)).toEqual(['zeta', 'alpha', 'mid', 'beta']);
  });

  it('ignores pinned ids that no longer exist', () => {
    const result = orderCalendars(cards, prefs({ pinnedIds: ['ghost', 'alpha'] }));
    expect(names(result)).toEqual(['alpha', 'beta', 'mid', 'zeta']);
  });

  it('ignores null entries', () => {
    const withNull = [...cards, null as unknown as Card];
    expect(names(orderCalendars(withNull, prefs()))).toEqual(['alpha', 'beta', 'mid', 'zeta']);
  });

  it('does not mutate the input array', () => {
    const input = [...cards];
    orderCalendars(input, prefs());
    expect(input.map(c => c.id)).toEqual(['zeta', 'alpha', 'mid', 'beta']);
  });
});

describe('orderedIds', () => {
  it('returns the display ids in order', () => {
    expect(orderedIds(cards, prefs())).toEqual(['alpha', 'beta', 'mid', 'zeta']);
  });
});
