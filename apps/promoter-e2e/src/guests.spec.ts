import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

// The mock API keeps state for the life of the dev server, so every test
// that writes uses its own names.
const tag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;

async function openGuests(page: Page) {
  await page.goto('/events/e-klubnacht/guests');
  await hydrated(page);
  await expect(page.getByRole('region', { name: 'List Artist guests' })).toBeVisible();
}

test.describe('guest lists and guest table (mock API)', () => {
  test('the event has a GUESTS tab with lists, allocations and status tabs', async ({ page }) => {
    await page.goto('/events/e-klubnacht/details');
    await hydrated(page);
    await page.getByRole('link', { name: 'GUESTS', exact: true }).last().click();
    await expect(page).toHaveURL(/\/events\/e-klubnacht\/guests$/);
    const artist = page.getByRole('region', { name: 'List Artist guests' });
    await expect(artist.getByRole('meter', { name: /Ben Klock allocation/ })).toBeVisible();
    await expect(artist.getByText('APPROVAL')).toBeVisible();
    await expect(page.getByRole('navigation', { name: 'Guest status' }).getByRole('button', { name: /^PENDING/ })).toBeVisible();
  });

  test('search, status tabs and the list filter narrow the table', async ({ page }) => {
    await openGuests(page);
    const table = page.getByRole('table');
    await page.getByLabel('Search guests').fill('tomasz');
    await expect(table.getByRole('row')).toHaveCount(2); // header + Tomasz
    await expect(table).toContainText('Tomasz Nowak');
    await page.getByLabel('Search guests').fill('');
    await page.getByLabel('Filter by list').selectOption({ label: 'Industry' });
    await expect(table).toContainText('Aiko Tanaka');
    await expect(table).not.toContainText('Tomasz Nowak');
    await page.getByRole('navigation', { name: 'Guest status' }).getByRole('button', { name: /^INVITED/ }).click();
    await expect(table).toContainText('Rafael Ortiz');
    await expect(table).not.toContainText('Aiko Tanaka');
  });

  test('allocation quota and approval are enforced when pasting names', async ({ page }) => {
    const t = tag();
    await openGuests(page);
    const comp = page.getByRole('region', { name: 'List Comp' });
    await comp.getByRole('button', { name: '+ ALLOCATION' }).click();
    await comp.getByLabel('SUBMITTER').fill(`E2E ${t}`);
    await comp.getByLabel('QUOTA · HEADS').fill('2');
    await comp.getByLabel(/NEEDS APPROVAL/).check();
    await comp.getByRole('button', { name: 'ADD ALLOCATION' }).click();
    await expect(comp.getByRole('meter', { name: `E2E ${t} allocation: 0 of 2 heads` })).toBeVisible();

    await page.getByRole('button', { name: 'ADD GUESTS' }).click();
    const form = page.getByRole('form', { name: 'ADD GUESTS' });
    await form.getByLabel('LIST', { exact: true }).selectOption({ label: 'Comp' });
    await form.getByLabel('ALLOCATION', { exact: true }).selectOption({ label: `E2E ${t} · 0/2` });
    await form.getByLabel('NAMES · ONE PER LINE').fill(`Zed ${t} +1\nYara ${t}`);
    await expect(form.getByRole('status')).toHaveText('2 GUESTS · 3 HEADS');
    await form.getByRole('button', { name: 'ADD 2' }).click();
    await expect(form.getByRole('alert')).toContainText(`E2E ${t} is at 0 of 2 heads. These guests need 3`);

    await form.getByLabel('NAMES · ONE PER LINE').fill(`Zed ${t} +1`);
    await form.getByRole('button', { name: 'ADD 1' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Added 1 to Comp. 1 waiting for approval.' })).toBeVisible();

    await page.getByLabel('Search guests').fill(`zed ${t}`);
    await page.getByRole('button', { name: `Approve Zed ${t}` }).click();
    await expect(page.getByRole('row').filter({ hasText: `Zed ${t}` }).locator('.badge-hud')).toHaveText(/GOING/);
    await expect(comp.getByRole('meter', { name: `E2E ${t} allocation: 2 of 2 heads` })).toBeVisible();
  });

  test('name-only lists never ask for contact details', async ({ page }) => {
    await openGuests(page);
    await page.getByRole('button', { name: 'ADD GUESTS' }).click();
    const form = page.getByRole('form', { name: 'ADD GUESTS' });
    await form.getByRole('radio', { name: 'ONE GUEST' }).click();
    await form.getByLabel('LIST', { exact: true }).selectOption({ label: 'Comp' });
    await expect(form.getByLabel('EMAIL')).toHaveCount(0);
    await expect(form).toContainText('Comp is name-only: no email or phone is stored.');
    await form.getByLabel('LIST', { exact: true }).selectOption({ label: 'Industry' });
    await expect(form.getByLabel('EMAIL')).toBeVisible();
  });

  test('status by email updates matches and lists emails it could not find', async ({ page }) => {
    const t = tag();
    await openGuests(page);
    await page.getByRole('button', { name: 'STATUS BY EMAIL' }).click();
    const form = page.getByRole('form', { name: 'SET STATUS BY EMAIL' });
    await form.getByRole('textbox').fill(`Aiko <aiko@label.example>, ghost-${t}@example.org`);
    await form.getByLabel('NEW STATUS').selectOption('going');
    // Dry run over the loaded guests before anything changes.
    const preview = form.getByTestId('bulk-preview');
    await expect(preview).toContainText(/\d+ guests? → GOING/);
    await expect(preview).toContainText('1 email matches nobody here');
    await form.getByRole('button', { name: 'APPLY TO 2 EMAILS' }).click();
    const result = form.getByRole('status');
    await expect(result).toContainText('1 matched');
    await expect(result).toContainText(`ghost-${t}@example.org`);
  });

  test('a status change can be undone, DECLINED asks first, and tickets have no action chip', async ({ page }) => {
    const t = tag();
    await openGuests(page);
    await page.getByRole('button', { name: 'ADD GUESTS' }).click();
    const form = page.getByRole('form', { name: 'ADD GUESTS' });
    await form.getByLabel('LIST', { exact: true }).selectOption({ label: 'Comp' });
    await form.getByLabel('NAMES · ONE PER LINE').fill(`Una Status ${t}`);
    await form.getByRole('button', { name: 'ADD 1' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Added 1 to Comp.' })).toBeVisible();

    await page.getByLabel('Search guests').fill(`una status ${t}`);
    const select = page.getByLabel(`Status of Una Status ${t}`);
    await expect(select).toHaveValue('going');
    const box = await select.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);

    // DECLINED asks first; saying no keeps the old status.
    page.once('dialog', d => void d.dismiss());
    await select.selectOption('declined');
    await expect(select).toHaveValue('going');
    await expect(page.getByTestId('status-changed')).toHaveCount(0);

    await select.selectOption('waitlist');
    const changed = page.getByTestId('status-changed');
    await expect(changed).toContainText(`Status of Una Status ${t} → WAITLIST`);
    await changed.getByRole('button', { name: 'UNDO' }).click();
    await expect(select).toHaveValue('going');
    await expect(changed).toHaveCount(0);

    await page.getByLabel('Search guests').fill('theo brandt');
    await expect(page.getByRole('row').filter({ hasText: 'Theo Brandt' })).not.toContainText('TICKET ');
  });

  test('the CSV export downloads and neutralises formulas', async ({ page }) => {
    const t = tag();
    await openGuests(page);
    await page.getByRole('button', { name: 'ADD GUESTS' }).click();
    const form = page.getByRole('form', { name: 'ADD GUESTS' });
    await form.getByLabel('LIST', { exact: true }).selectOption({ label: 'Comp' });
    await form.getByLabel('NAMES · ONE PER LINE').fill(`=HYPERLINK("x") ${t}`);
    await form.getByRole('button', { name: 'ADD 1' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Added 1 to Comp.' })).toBeVisible();

    const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'EXPORT CSV' }).click()]);
    expect(download.suggestedFilename()).toBe('klubnacht-03-guests.csv');
    const stream = await download.createReadStream();
    let csv = '';
    for await (const chunk of stream) csv += chunk.toString();
    expect(csv).toContain('name,plus_n,status,list,allocation,email,phone,note');
    expect(csv).toContain(`"'=HYPERLINK(""x"") ${t}"`);
  });

  test('guests overview shows fill and approvals, and standing lists join new events', async ({ page, request }) => {
    const t = tag();
    await page.goto('/guests');
    await hydrated(page);
    await expect(page.getByRole('link', { name: /Klubnacht 03: \d+ going heads, \d+ to approve/ })).toBeVisible();
    await page.getByRole('button', { name: 'NEW STANDING LIST' }).click();
    await page.getByLabel('NAME', { exact: true }).fill(`Residents ${t}`);
    await page.getByLabel(/^CUTOFF/).fill('01:00');
    await page.getByRole('button', { name: 'CREATE STANDING LIST' }).click();
    await expect(page.getByText(`Residents ${t} will be added to every new event.`)).toBeVisible();

    const start = new Date(Date.now() + 30 * 86_400_000);
    const created = await request.post('/api/v1/events', { data: {
      title: `Standing ${t}`, starts_at: start.toISOString(), ends_at: new Date(start.getTime() + 6 * 3_600_000).toISOString(),
      doors_at: null, timezone: 'Europe/Berlin', venue_id: null, city: 'Berlin', location_mode: 'city_only', location_reveal_at: null,
      visibility: 'public', publish_at: null, min_age: null, genres: [], description_md: '', cost_text: '', external_ticket_url: null, capacity: null,
    } });
    const ev = await created.json();
    await page.goto(`/events/${ev.id}/guests`);
    await hydrated(page);
    const copied = page.getByRole('region', { name: `List Residents ${t}` });
    await expect(copied).toContainText('CUTOFF 01:00');
  });
  // e-klubnacht-02 is last week's mock night with seeded door check-ins
  // (one undone, one guest who left again); no test writes to it.
  test('CHECKED IN shows who arrived and each row shows its heads inside', async ({ page }) => {
    await page.goto('/events/e-klubnacht-02/guests');
    await hydrated(page);
    const table = page.getByRole('table');
    const tabs = page.getByRole('navigation', { name: 'Guest status' });
    // 5 guests with heads inside + 2 scanned tickets.
    await expect(tabs.getByRole('button', { name: /^CHECKED IN\s*7$/ })).toBeVisible();

    const carla = table.getByRole('row').filter({ hasText: 'Carla Mendes' });
    await expect(carla.getByTestId('checkin-tag')).toHaveText(/^IN 2\/3 · \d\d:\d\d$/);
    await expect(carla.getByTestId('checkin-tag')).toHaveAttribute('aria-label', /^Checked in: 2 of 3 heads inside, first in at \d\d:\d\d$/);
    // An undone check-in does not count; the live one does.
    await expect(table.getByRole('row').filter({ hasText: 'Otto Brandl' }).getByTestId('checkin-tag')).toHaveText(/^IN 1\/1 /);
    // Came and went: OUT, not in the CHECKED IN tab.
    await expect(table.getByRole('row').filter({ hasText: 'Mei Chen' }).getByTestId('checkin-tag')).toHaveText(/^OUT 0\/1 /);
    await expect(table.getByRole('row').filter({ hasText: 'Yusuf Demir' }).getByTestId('checkin-tag')).toHaveCount(0);
    await expect(table.getByRole('row').filter({ hasText: 'Nils Berg' }).getByTestId('checkin-tag')).toHaveText(/^IN · \d\d:\d\d$/);

    await tabs.getByRole('button', { name: /^CHECKED IN/ }).click();
    await expect(tabs.getByRole('button', { name: /^CHECKED IN/ })).toHaveAttribute('aria-pressed', 'true');
    await expect(table.getByRole('row')).toHaveCount(8); // header + 5 guests + 2 tickets
    await expect(table).toContainText('Carla Mendes');
    await expect(table).toContainText('Jonas Wolf');
    await expect(table).not.toContainText('Mei Chen');
    await expect(table).not.toContainText('Yusuf Demir');
    await expect(table).not.toContainText('Ada Berg');
    await page.getByLabel('Filter by list').selectOption({ label: 'Comp' });
    await expect(table.getByRole('row')).toHaveCount(3); // header + Greta Holm, Ravi Nair (tickets are on no list)
    await expect(table).toContainText('Greta Holm');
    await expect(tabs.getByRole('button', { name: /^CHECKED IN/ })).toHaveText(/CHECKED IN\s*2$/);
  });

  test('the guest API filters by status=checked_in; the CSV export refuses it', async ({ request }) => {
    const res = await request.get('/api/v1/events/e-klubnacht-02/guests?status=checked_in');
    expect(res.status()).toBe(200);
    const body = await res.json() as { guests: { name: string, heads_in: number, first_in_at: string | null }[], tickets: unknown[], counts: { checked_in: number, all: number } };
    expect(body.guests.map(g => g.name).sort()).toEqual(['Carla Mendes', 'Greta Holm', 'Otto Brandl', 'Pia Lorenz', 'Ravi Nair']);
    expect(body.guests.every(g => g.heads_in >= 1 && g.first_in_at)).toBe(true);
    expect(body.counts.checked_in).toBe(5);
    expect(body.tickets).toEqual([]);
    expect((await request.get('/api/v1/events/e-klubnacht-02/guests/export.csv?status=checked_in')).status()).toBe(422);
  });
});
