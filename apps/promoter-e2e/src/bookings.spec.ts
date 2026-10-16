import { expect, test, type Page } from '@playwright/test';

/** Server-rendered buttons are inert until Vue hydrates; wait for the app. */
async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

const tag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;

test.describe('bookings: availability & outreach (mock API)', () => {
  test('slots → matches → compose → send lands in the sent log', async ({ page, request }) => {
    // A unique contact keeps this repeatable against the stateful mock:
    // seeded contacts may already be suppressed by an earlier run.
    const djName = `E2E DJ ${tag()}`;
    const djEmail = `${tag()}@e2e.example`;
    const created = await request.post('/api/v1/bookings/talent', {
      data: { name: djName, email: djEmail, city: 'Berlin', genres: ['techno'] },
    });
    expect(created.ok()).toBeTruthy();

    await page.goto('/bookings');
    await hydrated(page);
    await expect(page.locator('.page-title')).toHaveText('BOOKINGS');

    // Pick the seeded event; its two open slots show.
    await page.getByTestId('slot-event-picker').selectOption({ label: 'Klubnacht 03 · Berlin' });
    await expect(page.getByTestId('slot-row')).toHaveCount(2);
    await expect(page.getByText('Headliner')).toBeVisible();

    // Select the Headliner slot: the ranked matches load. Earlier runs may
    // have added Berlin contacts, so assert the seeded four are present
    // rather than an exact count.
    await page.getByTestId('slot-row').first().click();
    await expect(page.getByTestId('match-row').first()).toBeVisible();
    expect(await page.getByTestId('match-row').count()).toBeGreaterThanOrEqual(4);
    await expect(page.getByText('Nadia Kova')).toBeVisible();
    await expect(page.getByText('Omar Reyes')).toBeVisible();

    // The no-email contact is matchable but has no compose button (OFFER-10).
    const lena = page.getByTestId('match-row').filter({ hasText: 'Lena W' });
    await expect(lena.getByTestId('no-email-badge')).toBeVisible();
    await expect(lena.getByTestId('compose-btn')).toHaveCount(0);

    // Compose for the fresh contact: the default template pre-fills the
    // editor with the raw variables, and the preview renders them (this
    // event publishes its venue, so {{venue}} shows).
    await page.getByTestId('match-row').filter({ hasText: djName }).getByTestId('compose-btn').click();
    await expect(page.getByTestId('composer')).toBeVisible();
    await expect(page.getByTestId('composer-subject')).toHaveValue('Booking offer: {{event_name}} on {{date}}');
    await expect(page.getByTestId('composer-preview')).toContainText('Booking offer: Klubnacht 03 on');
    await expect(page.getByTestId('composer-preview')).toContainText('Venue: Tresor.West');
    await expect(page.getByTestId('venue-warning')).toHaveCount(0);

    await page.getByTestId('composer-send').click();
    await expect(page.getByTestId('composer-notice')).toContainText(`Offer sent to ${djName}.`);

    // The sent log shows the offer, newest first, with its delivery status.
    await page.getByTestId('tab-sent').click();
    const row = page.getByTestId('offer-row').first();
    await expect(row).toContainText(djEmail);
    await expect(row.getByTestId('offer-status')).toHaveText('SENT');
  });

  test('a withheld venue warns in the composer and renders empty (OFFER-09)', async ({ page }) => {
    await page.goto('/bookings');
    await hydrated(page);
    // Warehouse 10 is location_mode=secret before its reveal date.
    await page.getByTestId('slot-event-picker').selectOption({ label: 'Warehouse 10 · Berlin' });
    await page.getByTestId('slot-row').first().click();
    await page.getByTestId('match-row').filter({ hasText: 'Omar Reyes' }).getByTestId('compose-btn').click();
    // The default template mentions {{venue}}: the banner shows and the
    // preview renders the venue empty.
    await expect(page.getByTestId('venue-warning')).toBeVisible();
    await expect(page.getByTestId('composer-preview')).toContainText('Venue: \n');
    await expect(page.getByTestId('composer-preview')).not.toContainText('Tresor.West');
  });

  test('the talent book adds a DJ and one-click do-not-contact suppresses them', async ({ page }) => {
    const name = `E2E DJ ${tag()}`;
    await page.goto('/bookings');
    await hydrated(page);
    await page.getByTestId('tab-talent').click();

    await page.getByTestId('talent-add-open').click();
    await page.getByTestId('talent-name').fill(name);
    await page.getByTestId('talent-email').fill(`${tag()}@example.com`);
    await page.getByTestId('talent-city').fill('Berlin');
    await page.getByTestId('talent-genres').fill('techno');
    await page.getByTestId('talent-save').click();
    await expect(page.getByTestId('talent-row').filter({ hasText: name })).toBeVisible();

    // One-click opt-out: confirm, then the contact leaves the book and the
    // do-not-contact list (SENT tab) gains an entry.
    page.once('dialog', d => d.accept());
    await page.getByTestId('talent-row').filter({ hasText: name }).getByTestId('talent-suppress').click();
    await expect(page.getByTestId('talent-row').filter({ hasText: name })).toHaveCount(0);
    await page.getByTestId('tab-sent').click();
    await expect(page.getByTestId('suppression-row').first()).toBeVisible();
  });

  test('templates are created and edited with optimistic concurrency', async ({ page }) => {
    const name = `E2E template ${tag()}`;
    await page.goto('/bookings');
    await hydrated(page);
    await page.getByTestId('tab-templates').click();

    await page.getByTestId('template-add-open').click();
    await page.getByTestId('template-name').fill(name);
    await page.getByTestId('template-subject').fill('Hi {{dj_name}}');
    await page.getByTestId('template-body').fill('Play {{event_name}} on {{date}}.');
    await page.getByTestId('template-save').click();
    const row = page.getByTestId('template-row').filter({ hasText: name });
    await expect(row).toBeVisible();

    // Edit it: the version bumps (assert on this row — other rows from
    // earlier runs may also show v2).
    await row.getByRole('button', { name: 'Edit template' }).click();
    await page.getByTestId('template-subject').fill('Hi {{dj_name}}!');
    await page.getByTestId('template-save').click();
    await expect(row).toContainText('v2');
  });

  test('the sent log offers a one-click do-not-contact per row', async ({ page, request }) => {
    // A fresh contact + offer keep this repeatable: suppressing a seeded
    // row would 409 (duplicate) on the second run.
    const email = `${tag()}@e2e.example`;
    const contact = await request.post('/api/v1/bookings/talent', {
      data: { name: `E2E target ${tag()}`, email, city: 'Berlin', genres: ['techno'] },
    });
    expect(contact.ok()).toBeTruthy();
    const { id: contactId } = await contact.json() as { id: string };
    const offer = await request.post('/api/v1/bookings/offers', {
      data: { talent_contact_id: contactId, open_slot_id: 'os-klub-head' },
    });
    expect(offer.ok()).toBeTruthy();

    await page.goto('/bookings');
    await hydrated(page);
    await page.getByTestId('tab-sent').click();
    const before = await page.getByTestId('suppression-row').count();
    page.once('dialog', d => d.accept());
    await page.getByTestId('offer-row').filter({ hasText: email }).getByTestId('offer-suppress').click();
    await expect(page.getByTestId('suppression-row')).toHaveCount(before + 1);
  });
});
