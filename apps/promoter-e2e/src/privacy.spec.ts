import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

// The mock API keeps state for the life of the dev server and specs run in
// parallel: e-klubnacht-01 starts erased (read only here), e-klubnacht-02
// is ended but must never be erased (report and guest specs read it), and
// ERASE NOW only runs on an event each test creates for itself.
const tag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;

/** A fresh event that ended three days ago, with one list, one allocation and two guests. */
async function endedEvent(request: APIRequestContext, title: string) {
  const start = new Date(Date.now() - 3 * 86_400_000);
  const ev = await (await request.post('/api/v1/events', { data: {
    title, starts_at: start.toISOString(), ends_at: new Date(start.getTime() + 6 * 3_600_000).toISOString(),
    doors_at: null, timezone: 'Europe/Berlin', venue_id: null, city: 'Berlin', location_mode: 'city_only', location_reveal_at: null,
    visibility: 'public', publish_at: null, min_age: null, genres: [], description_md: '', cost_text: '', external_ticket_url: null, capacity: null,
  } })).json() as { id: string };
  const list = await (await request.post(`/api/v1/events/${ev.id}/lists`, { data: {
    name: 'Artist guests', type: 'artist', collect_contact: false, entry_terms: { price_mode: 'free', reduced_price_text: '', cutoff_at: null, perks: [] },
  } })).json() as { id: string };
  const alloc = await (await request.post(`/api/v1/events/${ev.id}/lists/${list.id}/allocations`, { data: {
    label: 'Kaiser', submitter_contact: null, quota: 6, plus_n_max: 1, deadline: null, requires_approval: false,
  } })).json() as { id: string };
  const added = await request.post(`/api/v1/events/${ev.id}/guests`, { data: {
    list_id: list.id, allocation_id: alloc.id, source: 'paste', guests: [{ name: 'Mara Weiss', plus_n: 1 }, { name: 'Tomasz Nowak', plus_n: 0 }],
  } });
  expect(added.status()).toBe(201);
  return ev.id;
}

test.describe('privacy and retention (mock API)', () => {
  test('settings: retention presets, custom days with validation, save feedback and the purge lists', async ({ page }) => {
    await page.goto('/settings');
    await hydrated(page);
    const section = page.locator('#retention');
    await expect(section.getByRole('heading', { name: 'DATA RETENTION' })).toBeVisible();
    const group = section.getByRole('group', { name: 'KEEP GUEST NAMES AND CONTACTS FOR' });
    await expect(group.getByRole('radio')).toHaveCount(5);
    await expect(section).toContainText('ERASED');
    await expect(section).toContainText('Guest names, emails, phone numbers and notes');
    await expect(section).toContainText('The report: check-in curve, walk-ups, numbers by list and submitter');
    await expect(section.getByTestId('retention-upcoming')).toContainText('Klubnacht 02');
    const recent = section.getByTestId('retention-recent');
    await expect(recent).toContainText('Klubnacht 01');
    await expect(recent.getByRole('listitem').filter({ hasText: 'Klubnacht 01' })).toContainText('AUTOMATIC');
    await expect(recent.getByRole('listitem').filter({ hasText: 'Klubnacht 01' })).toContainText(/\d+ guests · 2 tickets/);

    const save = section.getByRole('button', { name: 'SAVE RETENTION' });
    await group.getByText('CUSTOM').click();
    const days = section.getByTestId('retention-custom');
    await expect(days).toBeFocused();
    await days.fill('0');
    await days.blur();
    await expect(section.getByTestId('retention-error')).toHaveText('Enter a whole number of days from 1 to 365.');
    await expect(days).toHaveAttribute('aria-invalid', 'true');
    await expect(save).toBeDisabled();
    await days.fill('400');
    await expect(section.getByTestId('retention-error')).toBeVisible();
    await days.fill('45');
    await expect(section.getByTestId('retention-error')).toHaveCount(0);
    await save.click();
    await expect(section.getByTestId('retention-saved')).toContainText('KEPT 45 DAYS AFTER EACH EVENT');
    await expect(section.getByTestId('retention-upcoming').getByRole('listitem').filter({ hasText: 'Klubnacht 02' })).toContainText('45 days after it ended');
    await expect(save).toBeDisabled();

    // Back to the default so parallel specs see the usual 30 days.
    await group.getByText('30 DAYS · DEFAULT').click();
    await save.click();
    await expect(section.getByTestId('retention-saved')).toContainText('KEPT 30 DAYS');
    await page.reload();
    await hydrated(page);
    await expect(page.locator('#retention').getByRole('radio', { name: /30 DAYS/ })).toBeChecked();
  });

  test('an ended event shows when guest data is erased; an upcoming one shows nothing', async ({ page }) => {
    await page.goto('/events/e-klubnacht-02/guests');
    await hydrated(page);
    const banner = page.getByTestId('privacy-banner');
    await expect(banner).toContainText(/Guest names and contacts for this event are erased on \d{1,2} [A-Z][a-z]{2,3}( \d{4})? \(\d+ days? after it ended\)\./);
    await expect(banner.getByRole('link', { name: 'RETENTION SETTINGS →' })).toHaveAttribute('href', '/settings#retention');
    await expect(banner.getByRole('button', { name: 'ERASE NOW' })).toBeVisible();

    // The dialog explains and only erases with the exact title (cancelled here: other specs read this event).
    await banner.getByRole('button', { name: 'ERASE NOW' }).click();
    const dialog = page.getByRole('dialog', { name: 'ERASE GUEST DATA NOW?' });
    await expect(dialog).toContainText("Klubnacht 02: this can't be undone.");
    await expect(dialog).toContainText('ERASED FOR GOOD');
    await expect(dialog).toContainText('KEPT');
    const input = dialog.getByLabel('Type the event title Klubnacht 02 to confirm');
    await expect(input).toBeFocused();
    const erase = dialog.getByRole('button', { name: 'ERASE NOW' });
    await expect(erase).toBeDisabled();
    await input.fill('klubnacht 02');
    await expect(erase).toBeDisabled();
    await page.keyboard.press('Escape');
    await expect(dialog).toHaveCount(0);
    await expect(page.getByTestId('privacy-banner-text')).toContainText('are erased on');

    await page.getByRole('navigation', { name: 'Event sections' }).getByRole('link', { name: 'REPORT', exact: true }).click();
    await expect(page.getByTestId('privacy-banner')).toContainText(/are erased on .* after it ended/);

    await page.goto('/events/e-klubnacht/guests');
    await hydrated(page);
    await expect(page.getByRole('region', { name: 'List Artist guests' })).toBeVisible();
    await expect(page.getByTestId('privacy-banner')).toHaveCount(0);
  });

  test('ERASE NOW with the typed title leaves erased rows and turns personal-data actions off', async ({ page, request }) => {
    const t = tag();
    const title = `Erase ${t}`;
    const id = await endedEvent(request, title);
    await request.post('/api/v1/auth/login', { data: { email: 'owner@example.org', password: 'x', totp: '' } }); // fresh sign-in (step-up)

    await page.goto(`/events/${id}/guests`);
    await hydrated(page);
    const table = page.getByRole('table');
    await expect(table).toContainText('Mara Weiss');
    await page.getByTestId('erase-now').click();
    const dialog = page.getByRole('dialog', { name: 'ERASE GUEST DATA NOW?' });
    await expect(dialog).toContainText('It erases the names and contacts of 2 guests and ticket holders.');
    await dialog.getByLabel(`Type the event title ${title} to confirm`).fill(title);
    await dialog.getByRole('button', { name: 'ERASE NOW' }).click();
    await expect(dialog).toHaveCount(0);

    await expect(page.getByTestId('privacy-erased')).toHaveText('Erased: 2 guests. Counts, check-ins and the report stay.');
    await expect(page.getByTestId('privacy-erased')).toBeFocused();
    await expect(page.getByTestId('privacy-banner-text')).toHaveText(/^Guest names and contacts were erased on \d{1,2} [A-Z][a-z]{2,3}\.$/);
    await expect(page.getByTestId('erase-now')).toHaveCount(0);
    await expect(table).not.toContainText('Mara Weiss');
    await expect(table.getByTestId('erased-name')).toHaveText(['Erased guest', 'Erased guest']);
    const row = table.getByRole('row').filter({ hasText: 'Erased guest' }).first();
    await expect(row).toContainText('Artist guests');
    await expect(row).toContainText('GOING');
    await expect(row).toContainText('ERASED · NO CHANGES');
    await expect(page.getByTestId('guest-export')).toBeDisabled();
    await expect(page.getByRole('button', { name: 'ADD GUESTS' })).toBeDisabled();
    await expect(page.getByRole('button', { name: 'IMPORT ATTENDEES' })).toBeDisabled();
    await expect(page.getByTestId('purged-reason')).toContainText('adding guests, importing attendees, status by email and the CSV export are off');
    expect((await request.get(`/api/v1/events/${id}/guests/export.csv`)).status()).toBe(409);

    // The report keeps its numbers; list-backs are off with the reason.
    await page.getByRole('navigation', { name: 'Event sections' }).getByRole('link', { name: 'REPORT', exact: true }).click();
    await expect(page.getByTestId('privacy-banner-text')).toContainText('were erased on');
    await expect(page.getByTestId('report-heading')).toHaveText('POST-EVENT REPORT');
  });

  test('a stale sign-in asks to sign in again before erasing', async ({ page, request }) => {
    const t = tag();
    const title = `Step-up ${t}`;
    const id = await endedEvent(request, title);
    await page.route(`**/api/v1/events/${id}/purge`, route => route.fulfill({
      status: 403, contentType: 'application/json', body: JSON.stringify({ error: 'reauthentication_required' }),
    }));
    await page.goto(`/events/${id}/guests`);
    await hydrated(page);
    await page.getByTestId('erase-now').click();
    const dialog = page.getByRole('dialog', { name: 'ERASE GUEST DATA NOW?' });
    await dialog.getByLabel(`Type the event title ${title} to confirm`).fill(title);
    await dialog.getByRole('button', { name: 'ERASE NOW' }).click();
    await expect(dialog.getByTestId('erase-error')).toContainText('erasing needs a recent sign-in');
    const again = dialog.getByRole('link', { name: 'SIGN IN AGAIN →' });
    await expect(again).toHaveAttribute('href', `/login?next=/events/${id}/guests`);
    await again.click();
    await expect(page).toHaveURL(/\/login\?next=/);
    await page.getByLabel('EMAIL').fill('owner@example.org');
    await page.getByLabel('PASSWORD').fill('x');
    await page.getByRole('button', { name: 'SIGN IN' }).click();
    await expect(page).toHaveURL(new RegExp(`/events/${id}/guests$`));
    await expect(page.getByTestId('erase-now')).toBeVisible();
  });

  test('an event erased by the schedule: erased rows with check-ins, no exports, list-backs or PINs', async ({ page, request }) => {
    await page.goto('/events/e-klubnacht-01/guests');
    await hydrated(page);
    await expect(page.getByTestId('privacy-banner-text')).toHaveText(/^Guest names and contacts were erased on \d{1,2} [A-Z][a-z]{2,3}\.$/);
    await expect(page.getByTestId('erase-now')).toHaveCount(0);
    const table = page.getByRole('table');
    await expect(table.getByTestId('erased-name')).toHaveCount(6); // 4 guests + 2 tickets
    await expect(table).toContainText('Erased ticket holder');
    await expect(table.getByRole('row').filter({ hasText: 'Erased guest' }).first().getByTestId('checkin-tag')).toHaveText(/^IN 2\/2 · \d\d:\d\d$/);
    await expect(page.getByTestId('guest-export')).toBeDisabled();
    await expect(page.getByRole('button', { name: 'STATUS BY EMAIL' })).toBeDisabled();
    await expect(page.getByTestId('purged-reason')).toBeVisible();
    await expect(page.getByLabel('Search guests')).toHaveAttribute('placeholder', 'Names were erased');

    // The collect-contact toggle says what turning it on means.
    await page.getByRole('button', { name: 'NEW LIST' }).click();
    await expect(page.getByText('Off: names only (default). On: also email and phone, erased with the rest after the retention period')).toBeVisible();

    expect((await request.get('/api/v1/events/e-klubnacht-01/guests/export.csv')).status()).toBe(409);
    const add = await request.post('/api/v1/events/e-klubnacht-01/guests', { data: { list_id: 'gl-01-artist', allocation_id: null, source: 'manual', guests: [{ name: 'New Name', plus_n: 0 }] } });
    expect(add.status()).toBe(409);
    expect(await add.text()).toContain('event_purged');
    expect((await request.get('/api/v1/events/e-klubnacht-01/report/list-back.csv?allocation_id=al-01-kaiser')).status()).toBe(409);

    await page.goto('/events/e-klubnacht-01/report');
    await hydrated(page);
    await expect(page.getByTestId('kpi-arrived')).toBeVisible();
    await expect(page.getByTestId('list-back-purged')).toContainText('nothing to list back');
    const listBack = page.getByRole('button', { name: 'List back for Kaiser unavailable: guest names were erased' });
    await expect(listBack).toBeDisabled();
    await expect(listBack).toHaveText('NAMES ERASED');
    await expect(page.getByTestId('report-list-backs-link')).toHaveCount(0);

    await page.goto('/events/e-klubnacht-01/door');
    await hydrated(page);
    await expect(page.getByTestId('door-purged')).toContainText("new PINs can't be generated");
    await expect(page.getByRole('button', { name: 'GENERATE STAFF PIN' })).toBeDisabled();
    await expect(page.getByRole('button', { name: 'GENERATE MANAGER PIN' })).toBeDisabled();
  });
});
