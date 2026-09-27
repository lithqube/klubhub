import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

// The mock API keeps state for the life of the dev server and specs run in
// parallel: e-klubnacht-01 starts erased (read only here), e-klubnacht-02
// is ended but must never be erased (report and guest specs read it), and
// ERASE NOW only runs on an event each test creates for itself.
const tag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;

/** A fresh sign-in for this browser (the mock's step-up clock is a per-browser cookie). */
async function freshSignIn(page: Page) {
  const r = await page.request.post('/api/v1/auth/login', { data: { email: 'owner@example.org', password: 'x', totp: '' } });
  expect(r.ok()).toBe(true);
}

/** A fresh event that ended `daysAgo` days ago, with one list, one allocation and two guests. */
async function endedEvent(request: APIRequestContext, title: string, daysAgo = 3) {
  const start = new Date(Date.now() - daysAgo * 86_400_000);
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

// Both change the org retention period, so they never run at the same time.
test.describe.serial('retention settings (mock API)', () => {
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
    await expect(section.getByTestId('retention-saved')).toContainText('SAVED');
    await expect(section.getByTestId('retention-saved')).toContainText('Guest data is now kept 45 days after each event.');
    await expect(section.getByTestId('retention-saved')).toBeFocused();
    await expect(section.getByTestId('retention-upcoming').getByRole('listitem').filter({ hasText: 'Klubnacht 02' })).toContainText('45 days after it ended');
    await expect(save).toBeDisabled();

    // Back to the default so parallel specs see the usual 30 days.
    await group.getByText('30 DAYS · DEFAULT').click();
    await save.click();
    await expect(section.getByTestId('retention-saved')).toContainText('kept 30 days');
    await page.reload();
    await hydrated(page);
    await expect(page.locator('#retention').getByRole('radio', { name: /30 DAYS/ })).toBeChecked();
  });

  test('shortening retention warns which ended events it erases and needs an acknowledged SAVE AND ERASE', async ({ page }) => {
    // A dedicated event that ended 20 days ago: with 15 days it becomes due at once, with 30 it does not.
    const title = `Shorten ${tag()}`;
    await endedEvent(page.request, title, 20);
    await freshSignIn(page);

    // The API refuses the save without the count (409) and with a wrong one.
    const refused = await page.request.put('/api/v1/org/retention', { data: { retention_days: 15 } });
    expect(refused.status()).toBe(409);
    expect(await refused.json()).toMatchObject({ data: { error: 'retention_would_purge', count: 1 } });
    expect((await page.request.put('/api/v1/org/retention', { data: { retention_days: 15, confirm_purge: 2 } })).status()).toBe(409);
    const preview = await (await page.request.get('/api/v1/org/retention/preview?days=15')).json() as { count: number, would_purge: { title: string }[] };
    expect(preview.count).toBe(1);
    expect(preview.would_purge[0]?.title).toBe(title);

    await page.goto('/settings#retention');
    await hydrated(page);
    const section = page.locator('#retention');
    const group = section.getByRole('group', { name: 'KEEP GUEST NAMES AND CONTACTS FOR' });
    await group.getByText('CUSTOM').click();
    await section.getByTestId('retention-custom').fill('15');
    const warning = section.getByRole('alert').filter({ hasText: 'erased within the hour' });
    await expect(warning).toHaveText(new RegExp(`With 15 days, guest names and contacts of 1 ended event \\(${title}\\) are erased within the hour\\. This can't be undone\\.`));
    const save = section.getByTestId('retention-save');
    await expect(save).toHaveText('SAVE AND ERASE 1');
    await expect(save).toBeDisabled();
    await warning.getByLabel('I understand').check();
    await expect(save).toBeEnabled();

    // Lengthening instead clears the warning and the danger button.
    await group.getByText('90 DAYS').click();
    await expect(section.getByTestId('retention-warning')).toHaveCount(0);
    await expect(save).toHaveText('SAVE RETENTION');

    await group.getByText('CUSTOM').click();
    await section.getByTestId('retention-custom').fill('15');
    await expect(save).toHaveText('SAVE AND ERASE 1');
    await expect(save).toBeDisabled();
    await section.getByTestId('retention-ack').check();
    await save.click();
    await expect(section.getByTestId('retention-saved')).toContainText('Guest data is now kept 15 days after each event. 1 ended event is erased within the hour.');
    await expect(section.getByTestId('retention-warning')).toHaveCount(0);

    // Back to 30 days: lengthening needs no confirmation.
    await group.getByText('30 DAYS · DEFAULT').click();
    await expect(save).toHaveText('SAVE RETENTION');
    await save.click();
    await expect(section.getByTestId('retention-saved')).toContainText('kept 30 days');
  });
});

test.describe('privacy and retention (mock API)', () => {

  test('an ended event shows when guest data is erased; an upcoming one shows nothing', async ({ page }) => {
    await freshSignIn(page);
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
    await expect(dialog.getByTestId('erase-when')).toHaveText(/^This happens automatically on \d{1,2} [A-Z][a-z]{2,3}( \d{4})?\. Erasing now only brings it forward\.$/);
    await expect(dialog.getByTestId('erase-door-warning')).toHaveCount(0);
    await expect(dialog).toContainText('ERASED FOR GOOD');
    await expect(dialog).toContainText('KEPT');
    const input = dialog.getByLabel('Type the event title Klubnacht 02 to confirm');
    await expect(input).toBeFocused();
    const erase = dialog.getByRole('button', { name: 'ERASE NOW' });
    await expect(erase).toBeDisabled();
    await expect(input).not.toHaveAttribute('aria-invalid', 'true');
    await input.fill('klubnacht 2');
    await expect(erase).toBeDisabled();
    await expect(dialog.getByTestId('erase-match')).toHaveText('Not matching yet — check spelling and spaces.');
    await expect(input).toHaveAttribute('aria-invalid', 'true');
    // Case and spacing do not matter (normalised like the server).
    await input.fill('  KLUBNACHT   02 ');
    await expect(dialog.getByTestId('erase-match')).toHaveText('Title matches.');
    await expect(input).toHaveAttribute('aria-invalid', 'false');
    await expect(erase).toBeEnabled();
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
    await freshSignIn(page);

    await page.goto(`/events/${id}/guests`);
    await hydrated(page);
    const table = page.getByRole('table');
    await expect(table).toContainText('Mara Weiss');
    await page.getByTestId('erase-now').click();
    const dialog = page.getByRole('dialog', { name: 'ERASE GUEST DATA NOW?' });
    await expect(dialog).toContainText('It erases the names and contacts of 2 guests and ticket holders.');
    // Typed in lower case: normalised on both sides.
    await dialog.getByLabel(`Type the event title ${title} to confirm`).fill(title.toLowerCase());
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
    // No per-row noise: an empty actions cell and one hint above the table.
    await expect(table).not.toContainText('ERASED · NO CHANGES');
    await expect(row.getByRole('button')).toHaveCount(0);
    await expect(page.getByTestId('erased-hint')).toHaveText("Erased rows keep their list, +N, status and check-ins. They can't be changed.");
    await expect(page.getByTestId('guest-export')).toBeDisabled();
    await expect(page.getByTestId('guest-export')).toHaveAttribute('aria-describedby', 'purged-consequence');
    await expect(page.getByRole('button', { name: 'ADD GUESTS' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'IMPORT ATTENDEES' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'STATUS BY EMAIL' })).toHaveCount(0);
    await expect(page.locator('#purged-consequence')).toHaveText('Adding guests, importing attendees, status by email and the CSV export are off.');
    expect((await request.get(`/api/v1/events/${id}/guests/export.csv`)).status()).toBe(409);

    // The report keeps its numbers; list-backs are off with the reason.
    await page.getByRole('navigation', { name: 'Event sections' }).getByRole('link', { name: 'REPORT', exact: true }).click();
    await expect(page.getByTestId('privacy-banner-text')).toContainText('were erased on');
    await expect(page.getByTestId('report-heading')).toHaveText('POST-EVENT REPORT');
  });

  test('a stale sign-in asks to sign in again first, then comes back with the dialog open', async ({ page, context, baseURL }) => {
    const title = `Step-up ${tag()}`;
    const id = await endedEvent(page.request, title);
    // This browser signed in 20 minutes ago (the mock reads its step-up clock from this cookie).
    await context.addCookies([{ name: 'kh_mock_auth_at', value: String(Date.now() - 20 * 60_000), url: baseURL ?? 'http://localhost:4400' }]);

    await page.goto(`/events/${id}/guests`);
    await hydrated(page);
    await page.getByTestId('erase-now').click();
    const dialog = page.getByRole('dialog', { name: 'ERASE GUEST DATA NOW?' });
    await expect(dialog.getByTestId('erase-sign-in-first')).toHaveText("For safety, erasing needs a recent sign-in. Sign in again and you'll come straight back here.");
    await expect(dialog.getByTestId('erase-confirm-input')).toHaveCount(0);
    await dialog.getByRole('link', { name: 'SIGN IN AGAIN' }).click();

    await expect(page).toHaveURL(/\/login\?/);
    const url = new URL(page.url());
    expect(url.searchParams.get('next')).toBe(`/events/${id}/guests?erase=1`);
    expect(url.searchParams.get('why')).toBe('erase');
    await expect(page.getByTestId('login-why')).toHaveText("Confirm it's you to erase guest data.");
    await page.getByLabel('EMAIL').fill('owner@example.org');
    await page.getByLabel('PASSWORD').fill('x');
    await page.getByRole('button', { name: 'SIGN IN' }).click();

    // Straight back, the query dropped, the dialog open on the typed-title step.
    await expect(page).toHaveURL(new RegExp(`/events/${id}/guests$`));
    await expect(dialog).toBeVisible();
    const input = dialog.getByLabel(`Type the event title ${title} to confirm`);
    await expect(input).toBeFocused();

    // Should the server still want a fresh sign-in, the error links there too.
    await page.route(`**/api/v1/events/${id}/purge`, route => route.fulfill({
      status: 403, contentType: 'application/json', body: JSON.stringify({ error: 'reauthentication_required' }),
    }));
    await input.fill(title);
    await dialog.getByRole('button', { name: 'ERASE NOW' }).click();
    await expect(dialog.getByTestId('erase-error')).toContainText('erasing needs a recent sign-in');
    await expect(dialog.getByTestId('erase-error').getByRole('link', { name: 'SIGN IN AGAIN →' })).toHaveAttribute('href', /why=erase/);
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
    await expect(table).not.toContainText('ERASED · NO CHANGES');
    await expect(page.getByTestId('guest-export')).toBeDisabled();
    await expect(page.getByRole('button', { name: 'STATUS BY EMAIL' })).toHaveCount(0);
    await expect(page.getByTestId('privacy-consequence')).toBeVisible();
    await expect(page.getByLabel('Search guests')).toHaveAttribute('placeholder', 'Names were erased');
    await expect(page.getByLabel('Search guests')).toBeDisabled();

    // The collect-contact toggle says what turning it on means.
    await page.getByRole('button', { name: 'NEW LIST' }).click();
    await expect(page.getByText(/Off: names only \(default\)\. On: also email and phone, erased with the rest \d+ days? after the event ends/)).toBeVisible();

    expect((await request.get('/api/v1/events/e-klubnacht-01/guests/export.csv')).status()).toBe(409);
    const add = await request.post('/api/v1/events/e-klubnacht-01/guests', { data: { list_id: 'gl-01-artist', allocation_id: null, source: 'manual', guests: [{ name: 'New Name', plus_n: 0 }] } });
    expect(add.status()).toBe(409);
    expect(await add.text()).toContain('event_purged');
    expect((await request.get('/api/v1/events/e-klubnacht-01/report/list-back.csv?allocation_id=al-01-kaiser')).status()).toBe(409);

    await page.goto('/events/e-klubnacht-01/report');
    await hydrated(page);
    await expect(page.getByTestId('kpi-arrived')).toBeVisible();
    await expect(page.getByTestId('list-back-purged')).toHaveText('List-backs are off because guest names were erased.');
    await expect(page.getByTestId('report-submitters')).toContainText('Kaiser');
    await expect(page.getByRole('button', { name: /List back/ })).toHaveCount(0);
    await expect(page.getByTestId('report-list-backs-link')).toHaveCount(0);

    await page.goto('/events/e-klubnacht-01/door');
    await hydrated(page);
    await expect(page.getByTestId('door-purged')).toContainText("new PINs can't be generated");
    await expect(page.getByTestId('door-device-purged')).toHaveText("This event's guest list was erased, so it can't be used at the door.");
    await expect(page.getByRole('button', { name: 'USE THIS BROWSER AS A DOOR DEVICE' })).toHaveCount(0);
    await expect(page.locator('fieldset').filter({ hasText: 'VALID UNTIL' }).locator('input[type="date"]')).toBeDisabled();
    await expect(page.getByRole('button', { name: 'GENERATE STAFF PIN' })).toBeDisabled();
    await expect(page.getByRole('button', { name: 'GENERATE MANAGER PIN' })).toBeDisabled();
  });
});
