/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { expect, test, type Page } from '@playwright/test';
import { fetchAvailabilities, fetchRangeSummary, seedCalendar } from './fixtures/seed';

/**
 * The write paths, driven through the real ParticipantView against a real server.
 *
 * None of this was reachable before: the existing browser suite runs on
 * `dev/preview.html`, where the components emit into the void with no store, no router
 * and no HTTP client, so a click produced no request for any test to observe. Every
 * assertion here that reads the API back is checking what actually persisted, not what
 * the component believes.
 */

/** Wait for the view to be interactive rather than merely mounted. */
async function openCalendar(page: Page, url: string) {
  await page.goto(url, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('#displayMode', { timeout: 30_000 });
  // The grid renders from the range summary, so wait for a real cell.
  await page.waitForSelector('.cal-cell[data-date]');
}

function cell(page: Page, date: string) {
  return page.locator(`.cal-cell[data-date="${date}"]`);
}

/**
 * Assert that a cell carries a marker attribute, by presence rather than by value.
 *
 * Vue serialises `:data-own="true"` as `data-own="true"`, not as the empty string, and
 * the exact rendering is an implementation detail of the binding. Selecting on
 * `[data-own]` asks the question the CSS asks.
 */
async function expectMarked(page: Page, date: string, marker: string) {
  await expect(page.locator(`.cal-cell[data-date="${date}"][${marker}]`)).toHaveCount(1);
}

async function expectNotMarked(page: Page, date: string, marker: string) {
  await expect(page.locator(`.cal-cell[data-date="${date}"][${marker}]`)).toHaveCount(0);
}

test.describe('adding and removing availability', () => {
  test('a click adds an availability and it survives a reload', async ({ page, baseURL }) => {
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    // Tuesday of the following week: allowed, empty, and in the future.
    const target = seed.day(8);
    await expect(cell(page, target)).toHaveAttribute('data-status', 'free');

    await cell(page, target).click();
    await expectMarked(page, target, 'data-own');

    // What the server stored, not what the component thinks it stored.
    const stored = await fetchAvailabilities(
      seed.publicToken,
      seed.participants.Ada.id,
      seed.day(0),
      seed.day(20)
    );
    expect(stored.map(a => a.date)).toContain(target);

    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForSelector('.cal-cell[data-date]');
    await expectMarked(page, target, 'data-own');
  });

  test('a second click removes it again', async ({ page, baseURL }) => {
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    const target = seed.day(8);

    await cell(page, target).click();
    await expectMarked(page, target, 'data-own');

    await cell(page, target).click();
    await expectNotMarked(page, target, 'data-own');

    const stored = await fetchAvailabilities(
      seed.publicToken,
      seed.participants.Ada.id,
      seed.day(0),
      seed.day(20)
    );
    expect(stored.map(a => a.date)).not.toContain(target);
  });

  test('a closed day cannot be answered', async ({ page, baseURL }) => {
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    // Wednesday is not in allowed_weekdays.
    const wednesday = seed.day(2);
    await expect(cell(page, wednesday)).toHaveAttribute('data-status', 'disabled');

    await cell(page, wednesday).click({ force: true });
    await page.waitForTimeout(500);

    const stored = await fetchAvailabilities(
      seed.publicToken,
      seed.participants.Ada.id,
      seed.day(0),
      seed.day(20)
    );
    expect(stored.map(a => a.date)).not.toContain(wednesday);
  });
});

test.describe('dragging across days', () => {
  test('selects the whole span and skips the closed day inside it', async ({ page, baseURL }) => {
    // The regression this suite exists to catch end to end: a drag that crosses a
    // disabled cell must still cover everything on both sides of it.
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    const from = seed.day(8); // Tuesday, open
    const across = seed.day(9); // Wednesday, closed
    const to = seed.day(10); // Thursday, open

    // boundingBox() is in page coordinates while the mouse works in viewport ones, so
    // a row below the fold has to be scrolled to first or the pointer never lands on it.
    await cell(page, from).scrollIntoViewIfNeeded();
    const start = await cell(page, from).boundingBox();
    const end = await cell(page, to).boundingBox();
    expect(start && end).toBeTruthy();

    await page.mouse.move(start!.x + start!.width / 2, start!.y + start!.height / 2);
    await page.mouse.down();
    // A small first move so the gesture is recognised as a drag before it travels.
    await page.mouse.move(start!.x + start!.width / 2 + 6, start!.y + start!.height / 2);
    await page.mouse.move(end!.x + end!.width / 2, end!.y + end!.height / 2, { steps: 15 });
    await page.mouse.up();

    await expectMarked(page, from, 'data-own');
    await expectMarked(page, to, 'data-own');

    const stored = await fetchAvailabilities(
      seed.publicToken,
      seed.participants.Ada.id,
      seed.day(0),
      seed.day(20)
    );
    const dates = stored.map(a => a.date);

    expect(dates).toContain(from);
    expect(dates).toContain(to);
    // The closed day in the middle is skipped, not refused as a whole.
    expect(dates).not.toContain(across);
  });
});

test.describe('recurrences', () => {
  test('an occurrence is shown without an explicit availability behind it', async ({
    page,
    baseURL,
  }) => {
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    const friday = seed.day(4);
    await expectMarked(page, friday, 'data-recurring');

    // This is the distinction the whole ownAvailabilities fetch exists for: the range
    // summary counts the occurrence, while the participant has no stored answer there.
    const stored = await fetchAvailabilities(
      seed.publicToken,
      seed.participants.Ada.id,
      seed.day(0),
      seed.day(6)
    );
    expect(stored.map(a => a.date)).not.toContain(friday);

    const summary = await fetchRangeSummary(seed.publicToken, seed.day(0), seed.day(6));
    expect(summary.map(d => d.date)).toContain(friday);
  });

  test('an excepted occurrence is absent', async ({ page, baseURL }) => {
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    const excepted = seed.day(11);
    await expectNotMarked(page, excepted, 'data-recurring');

    const summary = await fetchRangeSummary(seed.publicToken, seed.day(7), seed.day(13));
    expect(summary.map(d => d.date)).not.toContain(excepted);
  });

  test('clicking an occurrence excepts that date rather than adding an availability', async ({
    page,
    baseURL,
  }) => {
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    const friday = seed.day(4);
    await expectMarked(page, friday, 'data-recurring');

    await cell(page, friday).click();
    await expectNotMarked(page, friday, 'data-recurring');

    // An exception, not a one-off that masks the rule.
    const stored = await fetchAvailabilities(
      seed.publicToken,
      seed.participants.Ada.id,
      seed.day(0),
      seed.day(6)
    );
    expect(stored.map(a => a.date)).not.toContain(friday);

    const summary = await fetchRangeSummary(seed.publicToken, seed.day(0), seed.day(6));
    expect(summary.map(d => d.date)).not.toContain(friday);
  });
});

test.describe('what the grid reports', () => {
  test('shows the seeded counts and marks the day that meets the threshold', async ({
    page,
    baseURL,
  }) => {
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    // Monday has two participants overlapping, against a threshold of two.
    await expectMarked(page, seed.monday, 'data-threshold');
    // Tuesday has one, so it does not.
    await expectNotMarked(page, seed.day(1), 'data-threshold');
  });

  test('an untimed answer adopts the calendar opening hours', async ({ page, baseURL }) => {
    // Worth pinning because it is not what "all day" sounds like. Posting an
    // availability with no times does not store null; the server runs it through
    // adjustTimesByAllowedHours, which fills in the window configured for that
    // weekday. On this calendar that is 08:00-20:00, so "all day" means "all of the
    // hours this calendar is open", not "midnight to midnight".
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Grace'));

    const untimed = seed.day(3);
    await expectMarked(page, untimed, 'data-own');

    const stored = await fetchAvailabilities(
      seed.publicToken,
      seed.participants.Grace.id,
      seed.day(0),
      seed.day(6)
    );
    const entry = stored.find(a => a.date === untimed);

    expect(entry).toBeTruthy();
    expect(entry?.start_time).toBe('08:00');
    expect(entry?.end_time).toBe('20:00');
  });

  test('an untimed answer spans the whole day when the calendar sets no hours', async ({
    baseURL,
  }) => {
    // The other half of the same rule, and the reason null times are hard to obtain
    // through the API at all: BuildAllowedHoursJSON defaults an empty weekday map to
    // 00:00-23:59 for all seven days, so there is always a window to clamp to. An
    // untimed answer therefore lands as 00:00-23:59 — which isFullDay reads back as
    // "all day", so the UI is right either way.
    const seed = await seedCalendar({ weekdayTimes: false });

    const stored = await fetchAvailabilities(
      seed.publicToken,
      seed.participants.Grace.id,
      seed.day(0),
      seed.day(6)
    );
    const entry = stored.find(a => a.date === seed.day(3));

    expect(entry).toBeTruthy();
    expect(entry?.start_time).toBe('00:00');
    expect(entry?.end_time).toBe('23:59');
    expect(baseURL).toBeTruthy();
  });
});

test.describe('the server is the authority', () => {
  test('a duplicate answer for the same date does not create a second row', async ({
    page,
    baseURL,
  }) => {
    // UNIQUE(participant_id, date) lives in the database. A mocked backend would never
    // reject anything, so this constraint has no other way of being exercised.
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    const target = seed.day(8);
    await cell(page, target).click();
    await expectMarked(page, target, 'data-own');

    const response = await page.request.post(
      `/api/v1/availabilities/calendar/${seed.publicToken}` +
        `/participant/${seed.participants.Ada.id}`,
      { data: { date: target, start_time: '09:00', end_time: '10:00' } }
    );

    expect(response.status()).toBe(409);

    const stored = await fetchAvailabilities(
      seed.publicToken,
      seed.participants.Ada.id,
      seed.day(0),
      seed.day(20)
    );
    expect(stored.filter(a => a.date === target)).toHaveLength(1);
  });

  test('an answer outside the calendar date range is refused', async ({ baseURL, request }) => {
    const seed = await seedCalendar();

    const response = await request.post(
      `${baseURL}/api/v1/availabilities/calendar/${seed.publicToken}` +
        `/participant/${seed.participants.Ada.id}`,
      { data: { date: seed.day(400) } }
    );

    expect(response.ok()).toBeFalsy();
  });
});

test.describe('display settings persist', () => {
  test('the chosen view survives a reload', async ({ page, baseURL }) => {
    const seed = await seedCalendar();
    await openCalendar(page, seed.participantUrl(baseURL!, 'Ada'));

    await page.selectOption('#displayMode', 'week');
    await expect(page.locator('.cal-week')).toBeVisible();

    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForSelector('#displayMode');

    await expect(page.locator('#displayMode')).toHaveValue('week');
    await expect(page.locator('.cal-week')).toBeVisible();
  });
});
