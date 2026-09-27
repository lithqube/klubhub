import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

// The mock API keeps state for the life of the dev server, so every test
// that writes uses its own order numbers, names and barcodes.
const tag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;

async function openImport(page: Page) {
  await page.goto('/events/e-klubnacht/guests');
  await hydrated(page);
  await expect(page.getByRole('region', { name: 'List Artist guests' })).toBeVisible();
  await page.getByRole('button', { name: 'IMPORT ATTENDEES' }).click();
  return page.getByRole('form', { name: 'IMPORT ATTENDEES' });
}

const upload = (name: string, csv: string) => ({ name, mimeType: 'text/csv', buffer: Buffer.from(csv, 'utf8') });

test.describe('attendee import (mock API)', () => {
  test('imported ticket holders sit in the guest table under TICKETS', async ({ page }) => {
    await page.goto('/events/e-klubnacht/guests');
    await hydrated(page);
    await expect(page.getByLabel('Guest summary')).toContainText(/TICKETS \d+/);
    await page.getByRole('navigation', { name: 'Guest status' }).getByRole('button', { name: /^TICKETS/ }).click();
    const row = page.getByRole('row').filter({ hasText: 'Theo Brandt' });
    await expect(row).toContainText('DICE · order D-7002');
    await expect(row).toContainText('REFUNDED');
    await expect(page.getByRole('table')).not.toContainText('Tomasz Nowak');
    // A list filter leaves tickets out: they are on no list.
    await page.getByRole('navigation', { name: 'Guest status' }).getByRole('button', { name: /^ALL/ }).click();
    await page.getByLabel('Filter by list').selectOption({ label: 'Comp' });
    await expect(page.getByRole('table')).not.toContainText('Theo Brandt');
  });

  test('RA export: check, review, import, then a re-import updates instead of duplicating', async ({ page }) => {
    const t = tag();
    const csv = (status: string) => 'Order ID;Ticket ID;First name;Last name;Email;Ticket type;Barcode;Status\n'
      + `R${t}1;T${t}1;Quinn;Vale${t};quinn-${t}@example.org;Tier 1;B${t}1;Valid\n`
      + `R${t}1;T${t}2;Rhea;Vale${t};quinn-${t}@example.org;Tier 1;B${t}2;${status}\n`
      + `R${t}2;T${t}3;;;;Tier 2;B${t}3;Valid\n`;
    const form = await openImport(page);
    await form.getByRole('radio', { name: 'RA' }).click();
    await form.getByLabel('2 · EXPORT FILE (CSV)').setInputFiles(upload('ra.csv', csv('Valid')));
    await expect(form.getByTestId('import-file-summary')).toHaveText('3 ROWS · 8 COLUMNS');
    await expect(form.getByRole('list', { name: 'Columns found' })).toContainText('BARCODE / QR Barcode');

    await form.getByRole('button', { name: 'CHECK FILE' }).click();
    await expect(form.getByRole('status')).toContainText('3 rows: 2 new tickets in 1 new orders, 0 updated, 0 unchanged, 1 skipped.');
    const preview = form.getByRole('table', { name: 'Preview, names shortened' });
    await expect(preview).toContainText('Qu… V…');
    await expect(preview).not.toContainText(`Vale${t}`);
    await expect(form.getByRole('list', { name: 'Skipped rows' })).toHaveText('Line 4: no name or email');
    await form.getByRole('button', { name: 'IMPORT 2 TICKETS' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Imported 2 tickets from RA: 2 new, 0 updated, 1 row skipped.' })).toBeVisible();

    await page.getByLabel('Search guests').fill(`vale${t}`);
    await expect(page.getByRole('table').getByRole('row')).toHaveCount(3); // header + Quinn + Rhea
    await expect(page.getByRole('row').filter({ hasText: `Rhea Vale${t}` })).toContainText('RA · order R');

    // The newer export: Rhea was refunded.
    const again = await openImport(page);
    await again.getByRole('radio', { name: 'RA' }).click();
    await again.getByLabel('2 · EXPORT FILE (CSV)').setInputFiles(upload('ra.csv', csv('Refunded')));
    await again.getByRole('button', { name: 'CHECK FILE' }).click();
    await expect(again.getByRole('status')).toContainText('0 new tickets in 0 new orders, 1 updated, 1 unchanged');
    await again.getByRole('button', { name: 'IMPORT 1 TICKET' }).click();
    await page.getByLabel('Search guests').fill(`rhea vale${t}`);
    await expect(page.getByRole('table').getByRole('row')).toHaveCount(2);
    await expect(page.getByRole('row').filter({ hasText: `Rhea Vale${t}` })).toContainText('REFUNDED');
  });

  test('a file that does not match the platform offers picking the columns', async ({ page }) => {
    const t = tag();
    const form = await openImport(page);
    await form.getByRole('radio', { name: 'DICE' }).click();
    await form.getByLabel('2 · EXPORT FILE (CSV)').setInputFiles(upload('list.csv', `Guest,Mail,Ref\nSol ${t},sol-${t}@example.org,X${t}\n`));
    await expect(form.getByRole('alert')).toContainText('This does not look like a DICE export: no column for name or email and order number, ticket id or barcode.');
    await expect(form.getByRole('button', { name: 'CHECK FILE' })).toBeDisabled();
    await form.getByRole('button', { name: 'PICK COLUMNS MYSELF' }).click();
    await expect(form.getByRole('radio', { name: 'OTHER CSV' })).toHaveAttribute('aria-checked', 'true');
    await form.getByLabel('FULL NAME').selectOption('Guest');
    await form.getByLabel('EMAIL', { exact: true }).selectOption('Mail');
    await form.getByLabel('ORDER NUMBER').selectOption('Ref');
    await form.getByRole('button', { name: 'CHECK FILE' }).click();
    await expect(form.getByRole('list', { name: 'Ticket types' })).toContainText('General admission');
    await form.getByRole('button', { name: 'IMPORT 1 TICKET' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Imported 1 ticket from OTHER CSV: 1 new, 0 updated.' })).toBeVisible();
  });
});
