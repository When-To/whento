import { expect, test } from '@playwright/test';

test('the skip link is hidden until it takes focus', async ({ page }) => {
  await page.goto('/');
  const link = page.locator('.skip-link');

  // Present in the DOM and reachable, but occupying no visible space.
  await expect(link).toHaveCount(1);
  const before = await link.boundingBox();
  expect(before!.width).toBeLessThanOrEqual(2);
  expect(before!.height).toBeLessThanOrEqual(2);

  // Tab from the top of the document must land on it first.
  await page.keyboard.press('Tab');
  await expect(link).toBeFocused();

  const after = await link.boundingBox();
  expect(after!.width).toBeGreaterThan(40);
  expect(after!.height).toBeGreaterThan(20);
});
