import { expect, test } from '@playwright/test';

test.describe('promoter shell (mobile)', () => {
  test('bottom bar navigates to the door', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: 'DOOR' }).last().click();
    await expect(page.locator('.page-title')).toHaveText('DOOR');
  });

  test('mobile header shows the product brand', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('banner', { name: 'KlubHub Promoter brand bar' })).toBeVisible();
  });
});
