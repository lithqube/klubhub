import { expect, test } from '@playwright/test';

test.describe('promoter shell (desktop)', () => {
  test('dashboard loads with the organisation from the API', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveTitle(/Dashboard — KlubHub Promoter/);
    await expect(page.getByText('Nachtwerk Collective', { exact: false })).toBeVisible();
  });

  test('primary navigation reaches every section', async ({ page }) => {
    await page.goto('/');
    const nav = page.getByRole('navigation', { name: 'Primary navigation' }).first();
    for (const label of ['EVENTS', 'GUESTS', 'DOOR', 'AUDIENCE', 'CAMPAIGNS', 'BOOKINGS', 'SETTINGS']) {
      await nav.getByRole('link', { name: label }).click();
      await expect(page.locator('.page-title')).toHaveText(label);
    }
  });

  test('theme switcher persists the daytime theme', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Switch to DAYTIME mode' }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  });

  test('sends security headers', async ({ request }) => {
    const res = await request.get('/');
    const headers = res.headers();
    expect(headers['x-content-type-options']).toBe('nosniff');
    expect(headers['referrer-policy']).toBe('strict-origin-when-cross-origin');
    expect(headers['content-security-policy']).toContain('script-src');
  });
});
