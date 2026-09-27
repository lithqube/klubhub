import { test, expect } from '@playwright/test';

test('dashboard loads', async ({ page }) => {
  await page.goto('/');

  // The scaffold placeholder ("Welcome" h1) never matched this app: the
  // dashboard has no <h1>. useHead() in apps/dj/app/pages/index.vue sets
  // the real, stable title.
  await expect(page).toHaveTitle('Dashboard — KlubHub DJ');
  await expect(page.getByRole('navigation', { name: 'Primary navigation' })).toBeVisible();
});
