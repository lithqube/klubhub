import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

const overflow = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);

// Read-only on shared mock events: e-klubnacht-01 starts erased; e-klubnacht-02 is only looked at (the dialog is cancelled).
test.describe('privacy and retention (mobile)', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
  });

  test('settings retention fits the phone width with 44 px choices', async ({ page }) => {
    await page.goto('/settings#retention');
    await hydrated(page);
    const section = page.locator('#retention');
    await expect(section.getByTestId('retention-recent')).toContainText('Klubnacht 01');
    const chip = section.locator('label', { hasText: 'CUSTOM' });
    await chip.scrollIntoViewIfNeeded();
    expect((await chip.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(await overflow(page)).toBeLessThanOrEqual(0);
  });

  test('erased guest table and banner fit the phone width', async ({ page }) => {
    await page.goto('/events/e-klubnacht-01/guests');
    await hydrated(page);
    await expect(page.getByTestId('privacy-banner-text')).toContainText('were erased on');
    await expect(page.getByTestId('erased-name').first()).toBeVisible();
    const link = page.getByRole('link', { name: 'RETENTION SETTINGS →' });
    expect((await link.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(await overflow(page)).toBeLessThanOrEqual(0);
  });

  test('the erase dialog fits the phone and its buttons are in reach', async ({ page }) => {
    await page.goto('/events/e-klubnacht-02/guests');
    await hydrated(page);
    await page.getByTestId('erase-now').click();
    const dialog = page.getByRole('dialog', { name: 'ERASE GUEST DATA NOW?' });
    const cancel = dialog.getByRole('button', { name: 'CANCEL' });
    await cancel.scrollIntoViewIfNeeded();
    await expect(cancel).toBeInViewport();
    const box = await dialog.boundingBox();
    expect(box?.width ?? 999).toBeLessThanOrEqual(375);
    expect((await cancel.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect((await dialog.getByRole('button', { name: 'Close' }).boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(await overflow(page)).toBeLessThanOrEqual(0);
    await cancel.click();
    await expect(dialog).toHaveCount(0);
  });
});
