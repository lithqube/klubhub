import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

test.describe('events, export and venues (mock API)', () => {
  test('dashboard lists what needs attention and links to the next night', async ({ page }) => {
    await page.goto('/');
    const queue = page.getByRole('region', { name: /NEEDS YOUR ATTENTION/ });
    await expect(queue.getByRole('link', { name: /KLUBNACHT 03 · 1 act has no set time/i })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'KLUBNACHT 03' })).toBeVisible();
  });

  test('events list opens an event on its lineup & timetable tab', async ({ page }) => {
    await page.goto('/events');
    await hydrated(page);
    await page.getByRole('link', { name: /Klubnacht 03/i }).first().click();
    await page.getByRole('link', { name: 'LINEUP & TIMETABLE' }).click();
    await expect(page).toHaveURL(/\/events\/e-klubnacht\/lineup$/);
    await expect(page.getByRole('region', { name: /Main Room/ }).first()).toBeVisible();
  });

  test('export leads with what is withheld and marks the embargo', async ({ page }) => {
    await page.goto('/events/e-warehouse/export');
    const withheld = page.getByRole('region', { name: 'WITHHELD FROM THIS PACK' });
    await expect(withheld).toContainText('SECRET until');
    await expect(withheld).toContainText('EMBARGO');
    await expect(page.getByText('WITHHELD', { exact: true }).first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'DOWNLOAD ZIP' })).toBeEnabled();
  });

  test('the export ZIP downloads with the embargo suffix', async ({ page }) => {
    await page.goto('/events/e-warehouse/export');
    await hydrated(page);
    const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'DOWNLOAD ZIP' }).click()]);
    expect(download.suggestedFilename()).toBe('warehouse-10-embargoed-export.zip');
  });

  test('protected venue fields need a second factor to show', async ({ page }) => {
    await page.goto('/venues/v-tresor');
    await hydrated(page);
    await expect(page.getByLabel('ADDRESS: hidden')).toBeVisible();
    await page.getByRole('button', { name: 'SHOW TO EDIT' }).click();
    await expect(page.getByRole('alert')).toContainText('two-factor');
  });

  test('a venue used by upcoming events cannot be archived', async ({ page }) => {
    await page.goto('/venues/v-tresor');
    await hydrated(page);
    page.once('dialog', d => d.accept());
    await page.getByRole('button', { name: 'ARCHIVE' }).click();
    await expect(page.getByRole('alert')).toContainText('Upcoming events still use this venue');
  });
});
