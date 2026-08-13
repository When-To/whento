import { expect, test } from '@playwright/test';

const PREVIEW = '/dev/preview.html';

// The per-participant button inside each cell used to be focusable. Tab therefore
// walked button-to-button, Enter opened the details popup, and toggling an
// availability — the grid's primary action — could not be reached by keyboard at
// all. An ARIA grid owns its own navigation, so nothing inside a cell may be a
// tab stop.
test.describe('month grid keyboard', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(PREVIEW);
    await page.waitForSelector('[role="grid"]');
  });

  test('no control inside a cell is a tab stop', async ({ page }) => {
    const counts = page.locator('[role="gridcell"] .cal-count');
    expect(await counts.count()).toBeGreaterThan(0);
    for (const handle of await counts.elementHandles()) {
      expect(await handle.getAttribute('tabindex')).toBe('-1');
    }
  });

  test('Tab reaches a cell and then leaves the grid', async ({ page }) => {
    await page.locator('[role="gridcell"][tabindex="0"]').first().focus();
    expect(await page.evaluate(() => document.activeElement?.getAttribute('role'))).toBe(
      'gridcell'
    );

    // One Tab must exit the grid entirely rather than descend into the cells.
    await page.keyboard.press('Tab');
    const stillInside = await page.evaluate(
      () => !!document.activeElement?.closest('[role="gridcell"]')
    );
    expect(stillInside).toBe(false);
  });
});
