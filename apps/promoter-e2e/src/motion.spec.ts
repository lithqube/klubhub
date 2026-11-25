import { expect, test, type Page } from '@playwright/test';

/** Server-rendered links are inert until Vue hydrates; wait for the app. */
async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

/** Whatever the animation left on the page sections; anything here can trap a fixed-position modal. */
const leftover = (page: Page) => page.locator('.page-body > *').evaluateAll(
  els => els.map((el) => { const s = (el as HTMLElement).style; return s.opacity + s.transform + s.translate; }).join(''),
);

for (const reducedMotion of ['no-preference', 'reduce'] as const) {
  test(`new page sections end without inline animation styles (${reducedMotion})`, async ({ page }) => {
    await page.emulateMedia({ reducedMotion });
    await page.goto('/');
    await hydrated(page);
    await page.getByRole('navigation', { name: 'Primary navigation' }).first().getByRole('link', { name: 'VENUES' }).click();
    await expect(page.locator('.page-title')).toHaveText('VENUES');
    await expect(page.locator('.page-body > [data-kh-revealed]').first()).toBeVisible();
    await expect.poll(() => leftover(page)).toBe('');
  });
}
