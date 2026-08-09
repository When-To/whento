/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useCalendarHistoryStore } from './calendarHistory';

const STORAGE_KEY = 'whento_visited_calendars';

/** Read back what the store persisted. */
function stored(): unknown {
  const raw = localStorage.getItem(STORAGE_KEY);
  return raw === null ? null : JSON.parse(raw);
}

function seed(value: unknown) {
  localStorage.setItem(STORAGE_KEY, typeof value === 'string' ? value : JSON.stringify(value));
}

/**
 * A fresh Pinia per test.
 *
 * The store calls init() at creation, so localStorage has to be arranged *before* the
 * store is instantiated — which is exactly the sequence that makes a corrupt value a
 * page-load failure rather than a recoverable one.
 */
function freshStore() {
  setActivePinia(createPinia());
  return useCalendarHistoryStore();
}

describe('calendarHistory', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('reading a corrupt store', () => {
    it('survives a stored number', () => {
      // The crash path: JSON.parse('42') succeeds, and the store then spreads it.
      seed('42');

      const store = freshStore();

      expect(store.calendars).toEqual([]);
    });

    it('survives a stored object', () => {
      seed({ token: 'abc' });

      expect(freshStore().calendars).toEqual([]);
    });

    it('survives a stored string', () => {
      seed('"just a string"');

      expect(freshStore().calendars).toEqual([]);
    });

    it('survives null', () => {
      seed('null');

      expect(freshStore().calendars).toEqual([]);
    });

    it('survives malformed JSON', () => {
      seed('{not json');

      expect(freshStore().calendars).toEqual([]);
    });

    it('drops entries that are not calendars but keeps the ones that are', () => {
      seed([
        { token: 'good', name: 'Kept', lastVisited: 2 },
        null,
        42,
        'nonsense',
        { name: 'no token', lastVisited: 3 },
        { token: 'also-good', name: 'Also kept', lastVisited: 1 },
      ]);

      const store = freshStore();

      expect(store.calendars.map(c => c.token)).toEqual(['good', 'also-good']);
    });

    it('starts empty when nothing is stored', () => {
      expect(freshStore().calendars).toEqual([]);
    });
  });

  describe('addCalendar', () => {
    it('stores a calendar and persists it', () => {
      const store = freshStore();

      store.addCalendar('tok-1', 'First', 'p1');

      expect(store.calendars).toHaveLength(1);
      expect(store.calendars[0]).toMatchObject({
        token: 'tok-1',
        name: 'First',
        participantId: 'p1',
      });
      expect(stored()).toHaveLength(1);
    });

    it('orders by most recently visited', () => {
      vi.useFakeTimers();
      const store = freshStore();

      vi.setSystemTime(new Date('2026-01-01T10:00:00Z'));
      store.addCalendar('older', 'Older');
      vi.setSystemTime(new Date('2026-01-02T10:00:00Z'));
      store.addCalendar('newer', 'Newer');

      expect(store.calendars.map(c => c.token)).toEqual(['newer', 'older']);
    });

    it('keeps at most ten entries, dropping the oldest', () => {
      vi.useFakeTimers();
      const store = freshStore();

      for (let i = 0; i < 13; i++) {
        vi.setSystemTime(new Date(Date.UTC(2026, 0, i + 1)));
        store.addCalendar(`tok-${i}`, `Calendar ${i}`);
      }

      expect(store.calendars).toHaveLength(10);
      // The three oldest are gone, the most recent is first.
      expect(store.calendars[0].token).toBe('tok-12');
      expect(store.calendars.map(c => c.token)).not.toContain('tok-0');
      expect(store.calendars.map(c => c.token)).not.toContain('tok-2');
      expect(store.calendars.map(c => c.token)).toContain('tok-3');
    });

    it('does not duplicate a revisited calendar', () => {
      const store = freshStore();

      store.addCalendar('tok-1', 'First');
      store.addCalendar('tok-1', 'First, renamed');

      expect(store.calendars).toHaveLength(1);
      expect(store.calendars[0].name).toBe('First, renamed');
    });

    it('preserves display settings when a calendar is revisited', () => {
      // The reason addCalendar spreads an existing entry: a revisit must not silently
      // reset the hours and view the participant chose.
      const store = freshStore();

      store.addCalendar('tok-1', 'First', 'p1');
      store.updateDisplaySettings('tok-1', {
        displayMode: 'week',
        periodCount: 2,
        startHour: 9,
        endHour: 18,
        slotDuration: 15,
        viewStyle: 'list',
      });

      store.addCalendar('tok-1', 'First');

      expect(store.getDisplaySettings('tok-1')).toMatchObject({
        displayMode: 'week',
        periodCount: 2,
        startHour: 9,
        endHour: 18,
        slotDuration: 15,
        viewStyle: 'list',
      });
    });

    it('preserves the participant when the revisit does not name one', () => {
      const store = freshStore();

      store.addCalendar('tok-1', 'First', 'p1');
      store.addCalendar('tok-1', 'First');

      expect(store.getParticipantId('tok-1')).toBe('p1');
    });

    it('replaces the participant when the revisit names a different one', () => {
      const store = freshStore();

      store.addCalendar('tok-1', 'First', 'p1');
      store.addCalendar('tok-1', 'First', 'p2');

      expect(store.getParticipantId('tok-1')).toBe('p2');
    });
  });

  describe('removal', () => {
    it('removes one calendar and leaves the rest', () => {
      const store = freshStore();

      store.addCalendar('tok-1', 'First');
      store.addCalendar('tok-2', 'Second');
      store.removeCalendar('tok-1');

      expect(store.calendars.map(c => c.token)).toEqual(['tok-2']);
      expect(stored()).toHaveLength(1);
    });

    it('ignores an unknown token', () => {
      const store = freshStore();

      store.addCalendar('tok-1', 'First');
      store.removeCalendar('nope');

      expect(store.calendars).toHaveLength(1);
    });

    it('clears everything, including the stored key', () => {
      const store = freshStore();

      store.addCalendar('tok-1', 'First');
      store.clearHistory();

      expect(store.calendars).toEqual([]);
      expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
    });
  });

  describe('display settings', () => {
    it('returns undefined for an unknown calendar', () => {
      expect(freshStore().getDisplaySettings('nope')).toBeUndefined();
    });

    it('ignores an update for an unknown calendar', () => {
      const store = freshStore();

      store.updateDisplaySettings('nope', { displayMode: 'week' });

      expect(store.calendars).toEqual([]);
    });

    it('applies only the settings named', () => {
      const store = freshStore();

      store.addCalendar('tok-1', 'First');
      store.updateDisplaySettings('tok-1', { startHour: 9, endHour: 18 });
      store.updateDisplaySettings('tok-1', { startHour: 7 });

      expect(store.getDisplaySettings('tok-1')).toMatchObject({ startHour: 7, endHour: 18 });
    });

    it('mirrors periodCount into the legacy monthCount', () => {
      const store = freshStore();

      store.addCalendar('tok-1', 'First');
      store.updateDisplaySettings('tok-1', { periodCount: 3 });

      expect(store.getMonthCount('tok-1')).toBe(3);
    });

    it('falls back to the legacy monthCount when periodCount is absent', () => {
      // Entries written before periodCount existed only carry monthCount.
      seed([{ token: 'legacy', name: 'Legacy', lastVisited: 1, monthCount: 4 }]);

      expect(freshStore().getDisplaySettings('legacy')?.periodCount).toBe(4);
    });

    it('prefers periodCount over monthCount when both are present', () => {
      seed([{ token: 'both', name: 'Both', lastVisited: 1, monthCount: 4, periodCount: 2 }]);

      expect(freshStore().getDisplaySettings('both')?.periodCount).toBe(2);
    });

    /**
     * The store persists whatever view style it is given, including the retired
     * `classic` and `compact` names, and getDisplaySettings hands them back unchanged.
     * Only useCalendarViewState's normalizeViewStyle maps them onto `grid`.
     *
     * Pinned as documentation, not endorsement: a consumer reading getDisplaySettings
     * directly receives the un-migrated value and has to normalise it itself.
     */
    it('returns a legacy view style un-migrated', () => {
      seed([{ token: 'legacy', name: 'Legacy', lastVisited: 1, viewStyle: 'compact' }]);

      expect(freshStore().getDisplaySettings('legacy')?.viewStyle).toBe('compact');
    });
  });

  describe('panel state', () => {
    it('toggles and closes', () => {
      const store = freshStore();

      expect(store.isOpen).toBe(false);
      store.toggle();
      expect(store.isOpen).toBe(true);
      store.toggle();
      expect(store.isOpen).toBe(false);

      store.toggle();
      store.close();
      expect(store.isOpen).toBe(false);
    });
  });

  describe('persistence failures', () => {
    it('does not throw when localStorage refuses to write', () => {
      // Private browsing and quota exhaustion both surface this way. Losing the
      // history is acceptable; taking the page down with it is not.
      const store = freshStore();
      const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw new DOMException('QuotaExceededError');
      });
      const error = vi.spyOn(console, 'error').mockImplementation(() => {});

      expect(() => store.addCalendar('tok-1', 'First')).not.toThrow();
      // The in-memory list still updated, so the session keeps working.
      expect(store.calendars).toHaveLength(1);
      expect(error).toHaveBeenCalled();

      setItem.mockRestore();
      error.mockRestore();
    });
  });
});
