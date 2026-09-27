import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

test.describe('attendee import (mobile)', () => {
  test('the import check and preview fit the phone width', async ({ page }) => {
    const t = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;
    await page.goto('/events/e-klubnacht/guests');
    await hydrated(page);
    await expect(page.getByRole('region', { name: 'List Artist guests' })).toBeVisible();
    await page.getByRole('button', { name: 'IMPORT ATTENDEES' }).click();
    const form = page.getByRole('form', { name: 'IMPORT ATTENDEES' });
    await form.getByRole('radio', { name: 'PRETIX' }).click();
    const csv = 'Order code,Position ID,Status,Email,Product,Attendee name,Attendee email,Secret\n'
      + `M${t},1,Paid,m-${t}@example.org,Early bird,Maren Holt${t},,S${t}1\n`
      + `M${t},2,Canceled,m-${t}@example.org,Regular,Ivo Holt${t},,S${t}2\n`;
    await form.getByLabel('2 · EXPORT FILE (CSV)').setInputFiles({ name: 'pretix.csv', mimeType: 'text/csv', buffer: Buffer.from(csv) });
    await form.getByRole('button', { name: 'CHECK FILE' }).click();
    await expect(form.getByRole('status')).toContainText('2 new tickets in 1 new orders');
    const confirm = form.getByRole('button', { name: 'IMPORT 2 TICKETS' });
    await confirm.scrollIntoViewIfNeeded();
    await expect(confirm).toBeInViewport();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(0);
    await confirm.click();
    await expect(page.getByRole('status').filter({ hasText: 'Imported 2 tickets from PRETIX' })).toBeVisible();
    await page.getByLabel('Search guests').fill(`holt${t}`);
    await expect(page.getByRole('row').filter({ hasText: `Ivo Holt${t}` })).toContainText('CANCELLED');
  });
});
