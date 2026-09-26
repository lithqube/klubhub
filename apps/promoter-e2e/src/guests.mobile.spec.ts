import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

test.describe('guest table (mobile)', () => {
  test('guests fit the phone width and pending names can be approved in reach', async ({ page }) => {
    await page.goto('/events/e-klubnacht/guests');
    await hydrated(page);
    await expect(page.getByRole('region', { name: 'List Artist guests' })).toBeVisible();
    await page.getByRole('navigation', { name: 'Guest status' }).getByRole('button', { name: /^PENDING/ }).click();
    const approve = page.getByRole('button', { name: /^Approve / }).first();
    await approve.scrollIntoViewIfNeeded();
    await expect(approve).toBeInViewport();
    await expect(approve).toHaveClass(/hit-44/); // 44 px tap target (libs/ui kinetic.css)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test('MORE reaches the guests overview', async ({ page }) => {
    await page.goto('/');
    await hydrated(page);
    await page.getByRole('button', { name: 'MORE' }).click();
    await page.getByRole('navigation', { name: 'More sections' }).getByRole('link', { name: /GUESTS/ }).click();
    await expect(page.locator('.page-title')).toHaveText('GUESTS');
    await expect(page.getByRole('heading', { name: 'STANDING LISTS' })).toBeVisible();
  });
});
