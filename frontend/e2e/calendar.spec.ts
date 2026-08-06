/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { expect, test, type Page } from '@playwright/test';

const PREVIEW = '/dev/preview.html';

/** Fail a test on any console error, rather than letting it pass quietly. */
async function gotoPreview(page: Page, query = ''): Promise<string[]> {
  const errors: string[] = [];
  page.on('console', message => {
    if (message.type() === 'error') errors.push(message.text());
  });
  page.on('pageerror', error => errors.push(error.message));
  await page.goto(`${PREVIEW}${query}`);
  await page.waitForSelector('[role="grid"]');
  return errors;
}

function cell(page: Page, date: string) {
  return page.locator(`[role="gridcell"][data-date="${date}"]`);
}

test.describe('month grid', () => {
  test('renders without console errors', async ({ page }) => {
    const errors = await gotoPreview(page);
    // Scoped to the month grid: the week grid's slots are gridcells too.
    await expect(page.locator('.cal-grid > [role="gridcell"]')).toHaveCount(35);
    expect(errors).toEqual([]);
  });

  test('marks past, weekend and open days differently', async ({ page }) => {
    await gotoPreview(page);
    // Today is 2026-04-06 and the calendar allows Monday to Friday.
    await expect(cell(page, '2026-04-02')).toHaveAttribute('data-status', 'disabled');
    await expect(cell(page, '2026-04-11')).toHaveAttribute('data-status', 'disabled');
    await expect(cell(page, '2026-04-06')).toHaveAttribute('data-today', 'true');
    await expect(cell(page, '2026-04-07')).not.toHaveAttribute('data-status', 'disabled');
  });

  test('fills only the days this participant answered', async ({ page }) => {
    await gotoPreview(page);
    // The fill answers one question: did I mark this day? It says nothing about how
    // many others did — that is the gauge and the count below it.
    await expect(cell(page, '2026-04-08')).toHaveAttribute('data-own', 'true');
    await expect(cell(page, '2026-04-15')).toHaveAttribute('data-own', 'true');
    // Busy for everyone else, but not answered by me.
    await expect(cell(page, '2026-04-13')).not.toHaveAttribute('data-own', /.*/);
    await expect(cell(page, '2026-04-13')).toHaveAttribute('data-threshold', 'true');
  });

  test('outlines a day covered only by a recurrence', async ({ page }) => {
    await gotoPreview(page);
    // 2 April onwards is a Thursday recurrence; 9 April also has an explicit answer.
    await expect(cell(page, '2026-04-30')).toHaveAttribute('data-recurring', 'true');
    await expect(cell(page, '2026-04-30')).not.toHaveAttribute('data-own', /.*/);
    await expect(cell(page, '2026-04-09')).toHaveAttribute('data-own', 'true');
  });

  test('shows a gauge and a count on every open day', async ({ page }) => {
    await gotoPreview(page);
    const target = cell(page, '2026-04-09');
    await expect(target.locator('.cal-gauge-fill')).toBeVisible();
    await expect(target.locator('.cal-count')).toContainText('3/5');
  });

  test('shows a one-off availability and a recurrence on the same day', async ({ page }) => {
    await gotoPreview(page);
    // 16 April carries two availabilities and a Thursday recurrence. The previous grid
    // collapsed these into one status and the recurrence vanished.
    const tags = cell(page, '2026-04-16').locator('.cal-tag');
    await expect(tags).toHaveCount(3);
    await expect(tags.nth(0)).toContainText('09:00-12:00');
    await expect(tags.nth(1)).toContainText('14:00-18:00');
    await expect(tags.nth(2)).toContainText('10:00-11:00');
  });

  test('omits the recurrence on a date that has an exception', async ({ page }) => {
    await gotoPreview(page);
    // 23 April is a Thursday but is excluded from the recurrence.
    const tags = cell(page, '2026-04-23').locator('.cal-tag');
    await expect(tags).toHaveCount(1);
    await expect(tags.first()).toContainText('08:00-10:00');
  });

  test('marks the public holiday and its eve', async ({ page }) => {
    await gotoPreview(page);
    // 1 May is a French public holiday, so 30 April is its eve.
    await expect(cell(page, '2026-04-30')).toHaveAttribute('data-holiday-eve', 'true');
    await expect(cell(page, '2026-04-06')).toHaveAttribute('data-holiday', 'true'); // Easter Monday
  });
});

test.describe('locale', () => {
  test('starts the week on the locale first day', async ({ page }) => {
    await gotoPreview(page, '?lang=en');
    await expect(page.locator('.cal-weekday').first()).toHaveText('Sun');

    await gotoPreview(page, '?lang=fr');
    await expect(page.locator('.cal-weekday').first()).toHaveText(/lun/i);
  });

  test('keeps the same days selectable across locales', async ({ page }) => {
    await gotoPreview(page, '?lang=en');
    const enabledEn = await page
      .locator('[role="gridcell"]:not([data-status="disabled"]):not([data-status="outside"])')
      .evaluateAll(nodes => nodes.map(n => n.getAttribute('data-date')).sort());

    await gotoPreview(page, '?lang=fr');
    const enabledFr = await page
      .locator('[role="gridcell"]:not([data-status="disabled"]):not([data-status="outside"])')
      .evaluateAll(nodes => nodes.map(n => n.getAttribute('data-date')).sort());

    expect(enabledFr).toEqual(enabledEn);
  });
});

test.describe('layout', () => {
  test('transposes below the compact breakpoint with no JavaScript', async ({ page }) => {
    await gotoPreview(page);

    await page.setViewportSize({ width: 1200, height: 900 });
    const wide = await page.locator('.cal-grid').evaluate(el => getComputedStyle(el).gridAutoFlow);
    expect(wide).toContain('row');

    await page.setViewportSize({ width: 400, height: 900 });
    const narrow = await page
      .locator('.cal-grid')
      .evaluate(el => getComputedStyle(el).gridAutoFlow);
    expect(narrow).toContain('column');
  });
});

test.describe('theme', () => {
  test('repaints immediately when the theme is toggled', async ({ page }) => {
    await gotoPreview(page, '?theme=light');
    const target = cell(page, '2026-04-13');
    const light = await target.evaluate(el => getComputedStyle(el).backgroundColor);

    await page.getByRole('button', { name: 'light' }).click();
    await expect(page.locator('html')).toHaveClass(/dark/);

    // The old week grid picked colours in JavaScript at render time, so a toggle left
    // every fill stale until something unrelated re-rendered.
    await expect
      .poll(() => target.evaluate(el => getComputedStyle(el).backgroundColor))
      .not.toBe(light);
  });
});

test.describe('keyboard', () => {
  test('exposes exactly one tab stop and moves with the arrows', async ({ page }) => {
    await gotoPreview(page);

    const tabStops = page.locator('[role="gridcell"][tabindex="0"]');
    await expect(tabStops).toHaveCount(1);

    await tabStops.first().focus();
    const first = await page.evaluate(() => document.activeElement?.getAttribute('data-date'));

    await page.keyboard.press('ArrowRight');
    const second = await page.evaluate(() => document.activeElement?.getAttribute('data-date'));
    expect(second).not.toBe(first);

    await page.keyboard.press('ArrowDown');
    const third = await page.evaluate(() => document.activeElement?.getAttribute('data-date'));
    expect(third).not.toBe(second);

    // Still one tab stop after moving: the roving tabindex followed focus.
    await expect(page.locator('[role="gridcell"][tabindex="0"]')).toHaveCount(1);
  });

  test('never focuses a disabled day', async ({ page }) => {
    await gotoPreview(page);
    await page.locator('[role="gridcell"][tabindex="0"]').first().focus();

    for (let i = 0; i < 12; i++) {
      await page.keyboard.press('ArrowRight');
      const status = await page.evaluate(() => document.activeElement?.getAttribute('data-status'));
      if (status) expect(['free', 'available', 'recurring', 'threshold']).toContain(status);
    }
  });

  test('extends a selection with shift and the arrows', async ({ page }) => {
    await gotoPreview(page);
    await page.locator('[role="gridcell"][tabindex="0"]').first().focus();

    await page.keyboard.down('Shift');
    await page.keyboard.press('ArrowRight');
    await page.keyboard.press('ArrowRight');
    await page.keyboard.up('Shift');

    await expect(page.locator('[role="gridcell"][data-drag]')).not.toHaveCount(0);
  });
});

test.describe('week grid', () => {
  test('leaves coverage spans unpainted, so only what matters is coloured', async ({ page }) => {
    await gotoPreview(page);
    const fill = await page
      .locator('.cal-band[data-kind="coverage"]')
      .first()
      .evaluate(el => getComputedStyle(el).backgroundColor);
    expect(fill).toBe('rgba(0, 0, 0, 0)');
  });

  test('outlines the range where the threshold is met', async ({ page }) => {
    await gotoPreview(page);
    // 16 April has all five participants overlapping between 11:00 and 12:00.
    await expect(page.locator('.cal-band[data-kind="threshold"]')).not.toHaveCount(0);
  });

  test('makes a met threshold visible even over the blue fill', async ({ page }) => {
    await gotoPreview(page);
    const band = page.locator('.cal-band[data-kind="threshold"]').first();
    const [width, colour, background] = await band.evaluate(el => {
      const style = getComputedStyle(el);
      return [parseFloat(style.borderTopWidth), style.borderTopColor, style.backgroundColor];
    });
    // A hairline ring disappeared against the fill underneath it.
    expect(width).toBeGreaterThanOrEqual(3);
    const [r, g, b] = colour.match(/[\d.]+/g)!.map(Number);
    expect(g).toBeGreaterThan(r + 40);
    expect(g).toBeGreaterThan(b + 40);
    expect(background).not.toBe('rgba(0, 0, 0, 0)');
  });

  test('shows how many of the threshold each span reaches', async ({ page }) => {
    await gotoPreview(page);
    const counts = page.locator('.cal-band-count');
    await expect(counts.first()).toContainText('/');
    await expect(page.locator('.cal-band-gauge-fill').first()).toBeVisible();
  });

  test('labels the closing time of the last row', async ({ page }) => {
    await gotoPreview(page);
    // Without it the end of the day has to be counted out from the last hour mark.
    await expect(page.locator('.cal-week-time--last')).toHaveText('20:00');
  });

  test('offers a details control on every day header', async ({ page }) => {
    await gotoPreview(page);
    // Right-clicking the header was the only way in, which is undiscoverable and
    // unusable on touch. The control also has to opt out of the header drag surface.
    const controls = page.locator('.cal-week-day-wrap .cal-count');
    await expect(controls).toHaveCount(7);
    await expect(controls.first()).toHaveAttribute('data-no-drag', '');
  });

  test('fills the slots this participant answered', async ({ page }) => {
    await gotoPreview(page);
    await expect(page.locator('.cal-slot[data-own]')).not.toHaveCount(0);
  });
});
