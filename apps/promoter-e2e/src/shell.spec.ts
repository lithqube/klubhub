import { expect, test, type Page } from '@playwright/test';

/** Server-rendered buttons are inert until Vue hydrates; wait for the app. */
async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

test.describe('promoter shell (desktop)', () => {
  test('dashboard loads with the organisation from the API', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveTitle(/Dashboard — KlubHub Promoter/);
    await expect(page.getByText('Nachtwerk Collective', { exact: false })).toBeVisible();
  });

  test('primary navigation reaches every section', async ({ page }) => {
    await page.goto('/');
    const nav = page.getByRole('navigation', { name: 'Primary navigation' }).first();
    for (const label of ['EVENTS', 'VENUES', 'GUESTS', 'DOOR', 'AUDIENCE', 'CAMPAIGNS', 'BOOKINGS', 'SETTINGS']) {
      await nav.getByRole('link', { name: label }).click();
      await expect(page.locator('.page-title')).toHaveText(label);
    }
  });

  test('theme switcher persists the daytime theme', async ({ page }) => {
    await page.goto('/');
    await hydrated(page);
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

test.describe('sign-in and account security (mock API)', () => {
  test('login page renders without the app shell', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: 'SIGN IN' })).toBeVisible();
    await expect(page.getByRole('navigation', { name: 'Primary navigation' })).toHaveCount(0);
    await expect(page.getByLabel('EMAIL')).toHaveAttribute('autocomplete', 'username');
  });

  test('owner without TOTP is prompted and can start enrolment', async ({ page }) => {
    await page.goto('/');
    await hydrated(page);
    await page.getByRole('link', { name: /Add an authenticator app/ }).click();
    await expect(page.locator('.page-title')).toHaveText('ACCOUNT SECURITY');
    await page.getByRole('button', { name: 'SET UP AUTHENTICATOR' }).click();
    await expect(page.getByRole('img', { name: 'QR code for your authenticator app' }).locator('svg')).toBeVisible();
  });
});
