/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { i18n } from '@/i18n';
import en from '@/locales/en.json';
import type { CalendarWithParticipants, Participant, PublicCalendar } from '@/types';

const calendarsApi = {
  getAll: vi.fn(),
  getById: vi.fn(),
  getPublic: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  delete: vi.fn(),
  addParticipant: vi.fn(),
  updateParticipant: vi.fn(),
  deleteParticipant: vi.fn(),
  addAnonymousParticipant: vi.fn(),
  regeneratePublicToken: vi.fn(),
  regenerateICSToken: vi.fn(),
};

vi.mock('@/api/calendars', () => ({ calendarsApi }));

const { useCalendarStore } = await import('./calendar');

const PARTICIPANT = { id: 'p-1', name: 'Ada' } as Participant;

function publicCalendar(): PublicCalendar {
  return { id: 'c-1', participants: [] } as unknown as PublicCalendar;
}

function calendar(overrides: Partial<CalendarWithParticipants> = {}): CalendarWithParticipants {
  return {
    id: 'c-1',
    name: 'Rehearsals',
    participants: [],
    ...overrides,
  } as CalendarWithParticipants;
}

/** A promise with its resolvers exposed, so a test can control when it settles. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(res => {
    resolve = res;
  });
  return { promise, resolve };
}

type CalendarStore = ReturnType<typeof useCalendarStore>;
type CalendarAction = (store: CalendarStore) => Promise<unknown>;

function freshStore() {
  setActivePinia(createPinia());
  return useCalendarStore();
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('calendar store', () => {
  describe('loading', () => {
    it('stays true until the last concurrent action settles', async () => {
      // The defect: twelve actions shared one flag, each clearing it in its own
      // `finally`. Fetching the list while a participant write is in flight — the
      // ordinary case on the settings screen — turned the spinner off early and
      // re-enabled the save button mid-write.
      const list = deferred<CalendarWithParticipants[]>();
      const write = deferred<Participant>();
      calendarsApi.getAll.mockReturnValue(list.promise);
      calendarsApi.addParticipant.mockReturnValue(write.promise);
      const store = freshStore();

      const fetching = store.fetchCalendars();
      const adding = store.addParticipant('c-1', { name: 'Ada' });
      expect(store.loading).toBe(true);

      list.resolve([]);
      await fetching;
      expect(store.loading).toBe(true);

      write.resolve(PARTICIPANT);
      await adding;
      expect(store.loading).toBe(false);
    });

    it('is not left stuck when one of two actions fails', async () => {
      const list = deferred<CalendarWithParticipants[]>();
      calendarsApi.getAll.mockReturnValue(list.promise);
      calendarsApi.delete.mockRejectedValue({ code: 'FORBIDDEN' });
      const store = freshStore();

      const fetching = store.fetchCalendars();
      await expect(store.deleteCalendar('c-1')).rejects.toBeDefined();
      expect(store.loading).toBe(true);

      list.resolve([]);
      await fetching;
      expect(store.loading).toBe(false);
    });
  });

  describe('error', () => {
    it('translates the API code instead of showing the backend message', async () => {
      calendarsApi.getAll.mockRejectedValue({
        code: 'FORBIDDEN',
        message: 'user does not own calendar',
      });
      const store = freshStore();

      await expect(store.fetchCalendars()).rejects.toBeDefined();

      expect(store.error).toBe(i18n.global.t('errors.forbidden'));
      expect(store.error).not.toContain('does not own');
    });

    it.each([
      ['fetchCalendars', s => s.fetchCalendars(), 'getAll', 'calendar.fetchError'],
      ['fetchCalendar', s => s.fetchCalendar('c-1'), 'getById', 'calendar.fetchError'],
      [
        'fetchPublicCalendar',
        s => s.fetchPublicCalendar('tok'),
        'getPublic',
        'calendar.fetchError',
      ],
      ['createCalendar', s => s.createCalendar({ name: 'x' }), 'create', 'calendar.createError'],
      [
        'updateCalendar',
        s => s.updateCalendar('c-1', { name: 'x' }),
        'update',
        'calendar.updateError',
      ],
      ['deleteCalendar', s => s.deleteCalendar('c-1'), 'delete', 'calendar.deleteError'],
      [
        'addParticipant',
        s => s.addParticipant('c-1', { name: 'x' }),
        'addParticipant',
        'calendar.addParticipantError',
      ],
      [
        'updateParticipant',
        s => s.updateParticipant('c-1', 'p-1', { name: 'x' }),
        'updateParticipant',
        'calendar.updateParticipantError',
      ],
      [
        'deleteParticipant',
        s => s.deleteParticipant('c-1', 'p-1'),
        'deleteParticipant',
        'calendar.deleteParticipantError',
      ],
      [
        'addAnonymousParticipant',
        s => s.addAnonymousParticipant('tok', { name: 'x' }),
        'addAnonymousParticipant',
        'calendar.addParticipantError',
      ],
      [
        'regeneratePublicToken',
        s => s.regeneratePublicToken('c-1'),
        'regeneratePublicToken',
        'calendar.regenerateError',
      ],
      [
        'regenerateICSToken',
        s => s.regenerateICSToken('c-1'),
        'regenerateICSToken',
        'calendar.regenerateError',
      ],
    ] as [string, CalendarAction, keyof typeof calendarsApi, string][])(
      '%s reports its failure through i18n',
      async (_name, call, method, key) => {
        // Each action used to write its own hard-coded English literal
        // ('Failed to fetch calendars', ...) straight into `error`, whatever the
        // reader's language.
        calendarsApi[method].mockRejectedValue(new Error('offline'));
        const store = freshStore();

        await expect(call(store)).rejects.toThrow();

        expect(store.error).toBe(i18n.global.t(key));
        expect(store.loading).toBe(false);
      }
    );

    it('speaks the reader language', async () => {
      // The proof that the message is a lookup and not a literal.
      calendarsApi.getAll.mockRejectedValue(new Error('offline'));
      const store = freshStore();

      const previous = i18n.global.locale.value;
      try {
        i18n.global.locale.value = 'fr';
        await expect(store.fetchCalendars()).rejects.toThrow();

        expect(store.error).toBe(i18n.global.t('calendar.fetchError'));
        expect(store.error).not.toBe(en.calendar.fetchError);
      } finally {
        i18n.global.locale.value = previous;
      }
    });

    it('is cleared by clearError', async () => {
      calendarsApi.getAll.mockRejectedValue(new Error('offline'));
      const store = freshStore();
      await expect(store.fetchCalendars()).rejects.toThrow();

      store.clearError();

      expect(store.error).toBeNull();
    });
  });

  describe('state updates', () => {
    it('keeps the list an array when the backend answers with something else', async () => {
      calendarsApi.getAll.mockResolvedValue(null);
      const store = freshStore();

      await store.fetchCalendars();

      expect(store.calendars).toEqual([]);
    });

    it('empties the list on a failed fetch', async () => {
      calendarsApi.getAll.mockResolvedValue([calendar()]);
      const store = freshStore();
      await store.fetchCalendars();

      calendarsApi.getAll.mockRejectedValue(new Error('offline'));
      await expect(store.fetchCalendars()).rejects.toThrow();

      expect(store.calendars).toEqual([]);
    });

    it('appends a created calendar with an empty participant list', async () => {
      calendarsApi.create.mockResolvedValue({ id: 'c-2', name: 'New' });
      const store = freshStore();

      await store.createCalendar({ name: 'New' });

      expect(store.calendars).toHaveLength(1);
      expect(store.calendars[0].participants).toEqual([]);
    });

    it('preserves participants across an update, which the API does not return', async () => {
      calendarsApi.getAll.mockResolvedValue([calendar({ participants: [PARTICIPANT] })]);
      calendarsApi.update.mockResolvedValue({ id: 'c-1', name: 'Renamed' });
      const store = freshStore();
      await store.fetchCalendars();

      await store.updateCalendar('c-1', { name: 'Renamed' });

      expect(store.calendars[0].name).toBe('Renamed');
      expect(store.calendars[0].participants).toEqual([PARTICIPANT]);
    });

    it('drops the deleted calendar, and clears it if it was current', async () => {
      calendarsApi.getById.mockResolvedValue(calendar());
      calendarsApi.getAll.mockResolvedValue([calendar()]);
      calendarsApi.delete.mockResolvedValue(undefined);
      const store = freshStore();
      await store.fetchCalendars();
      await store.fetchCalendar('c-1');

      await store.deleteCalendar('c-1');

      expect(store.calendars).toEqual([]);
      expect(store.currentCalendar).toBeNull();
    });

    it('keeps the public calendar in its own slot', async () => {
      // One slot for both shapes made owner-only fields look present on public views.
      calendarsApi.getPublic.mockResolvedValue(publicCalendar());
      const store = freshStore();

      await store.fetchPublicCalendar('tok');

      expect(store.currentPublicCalendar).not.toBeNull();
      expect(store.currentCalendar).toBeNull();
    });

    it('adds, updates and removes a participant on the current calendar', async () => {
      calendarsApi.getById.mockResolvedValue(calendar());
      calendarsApi.addParticipant.mockResolvedValue(PARTICIPANT);
      calendarsApi.updateParticipant.mockResolvedValue({ ...PARTICIPANT, name: 'Ada L.' });
      calendarsApi.deleteParticipant.mockResolvedValue(undefined);
      const store = freshStore();
      await store.fetchCalendar('c-1');

      await store.addParticipant('c-1', { name: 'Ada' });
      expect(store.currentCalendar?.participants).toHaveLength(1);

      await store.updateParticipant('c-1', 'p-1', { name: 'Ada L.' });
      expect(store.currentCalendar?.participants[0].name).toBe('Ada L.');

      await store.deleteParticipant('c-1', 'p-1');
      expect(store.currentCalendar?.participants).toHaveLength(0);
    });

    it('writes a regenerated token to both the list and the current calendar', async () => {
      calendarsApi.getAll.mockResolvedValue([calendar()]);
      calendarsApi.getById.mockResolvedValue(calendar());
      calendarsApi.regeneratePublicToken.mockResolvedValue({ public_token: 'new-public' });
      calendarsApi.regenerateICSToken.mockResolvedValue({ ics_token: 'new-ics' });
      const store = freshStore();
      await store.fetchCalendars();
      await store.fetchCalendar('c-1');

      await store.regeneratePublicToken('c-1');
      await store.regenerateICSToken('c-1');

      expect(store.currentCalendar?.public_token).toBe('new-public');
      expect(store.currentCalendar?.ics_token).toBe('new-ics');
      expect(store.calendars[0].public_token).toBe('new-public');
      expect(store.calendars[0].ics_token).toBe('new-ics');
    });

    it('appends an anonymous participant to the public calendar', async () => {
      calendarsApi.getPublic.mockResolvedValue(publicCalendar());
      calendarsApi.addAnonymousParticipant.mockResolvedValue(PARTICIPANT);
      const store = freshStore();
      await store.fetchPublicCalendar('tok');

      await store.addAnonymousParticipant('tok', { name: 'Ada' });

      expect(store.currentPublicCalendar?.participants).toHaveLength(1);
    });

    it('clears both current slots', async () => {
      calendarsApi.getById.mockResolvedValue(calendar());
      calendarsApi.getPublic.mockResolvedValue(publicCalendar());
      const store = freshStore();
      await store.fetchCalendar('c-1');
      await store.fetchPublicCalendar('tok');

      store.clearCurrentCalendar();

      expect(store.currentCalendar).toBeNull();
      expect(store.currentPublicCalendar).toBeNull();
    });
  });
});
