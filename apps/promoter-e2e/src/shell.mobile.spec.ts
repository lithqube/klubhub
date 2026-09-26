import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

test.describe('promoter shell (mobile)', () => {
  test('bottom bar reaches venues, and MORE reaches the door', async ({ page }) => {
    await page.goto('/');
    await hydrated(page);
    await page.getByRole('link', { name: 'VENUES' }).last().click();
    await expect(page.locator('.page-title')).toHaveText('VENUES');
    await page.getByRole('button', { name: 'MORE' }).click();
    await page.getByRole('navigation', { name: 'More sections' }).getByRole('link', { name: /DOOR/ }).click();
    await expect(page.locator('.page-title')).toHaveText('DOOR');
  });

  test('mobile header shows the product brand', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('banner', { name: 'KlubHub Promoter brand bar' })).toBeVisible();
  });

  test('NOW opens the timetable of the next night, without sideways scrolling', async ({ page }) => {
    await page.goto('/now');
    await expect(page).toHaveURL(/\/events\/[^/]+\/lineup$/);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
