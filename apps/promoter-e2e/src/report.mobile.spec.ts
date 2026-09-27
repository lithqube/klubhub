import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

const overflow = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);

test.describe('post-event report (mobile)', () => {
  test('KPIs, curve and tables fit the phone width; list back is in reach', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/events/e-klubnacht-02/report');
    await hydrated(page);
    await expect(page.getByTestId('kpi-arrived')).toContainText('6 / 8');
    await expect(page.getByTestId('checkin-curve')).toBeVisible();
    await expect(page.getByTestId('kpi-door')).toBeVisible();
    expect(await overflow(page)).toBeLessThanOrEqual(0);
    await page.getByText('HOW THESE ARE COUNTED').click();
    expect(await overflow(page)).toBeLessThanOrEqual(0);

    await page.getByRole('button', { name: 'SHOW AS TABLE' }).click();
    await expect(page.getByTestId('curve-table')).toBeVisible();
    const listBack = page.getByRole('button', { name: 'List back CSV for Kaiser' });
    await listBack.scrollIntoViewIfNeeded();
    await expect(listBack).toBeInViewport();
    const box = await listBack.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(await overflow(page)).toBeLessThanOrEqual(0);
  });
});
