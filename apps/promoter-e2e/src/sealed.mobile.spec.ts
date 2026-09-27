import { expect, test, type Locator, type Page } from '@playwright/test';
import { addBan, CSRF, hydrated, isolate, overflow, PASS, setUpSealed, tag } from './sealed-helpers';

// Sealed tier and ban list at phone width (375 px): no horizontal scroll,
// 44 px targets (56 px at the door), every state of the flow on screen.

async function atLeast(el: Locator, px: number) {
  await el.scrollIntoViewIfNeeded();
  expect((await el.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(px);
}

async function noScroll(page: Page) {
  expect(await overflow(page)).toBeLessThanOrEqual(0);
}

test.describe('sealed tier and ban list (mobile)', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
  });

  test('setup wizard, kit, unlock and the ban list fit the phone', async ({ page }) => {
    const t = tag();
    await isolate(page);

    // Key creation and the wizard, checked at each step.
    await page.goto('/settings#sealed');
    await hydrated(page);
    const section = page.getByTestId('sealed-section');
    await expect(section.getByTestId('sealed-create')).toBeVisible();
    await atLeast(section.getByTestId('sealed-new-passphrase'), 44);
    await atLeast(section.getByRole('button', { name: 'Show passphrase' }), 44);
    await noScroll(page);

    const groups = await setUpSealed(page);
    expect(groups).toHaveLength(8);
    await noScroll(page);
    await atLeast(section.getByTestId('sealed-lock'), 44);

    // The ban list: add an entry, the table becomes cards, actions stay in reach.
    await section.getByRole('link', { name: 'OPEN THE BAN LIST →' }).click();
    await expect(page).toHaveURL(/\/ban-list$/);
    await page.getByTestId('ban-add').click();
    const form = page.getByTestId('ban-form');
    await atLeast(form.getByText('30 DAYS'), 44);
    await form.getByText('CUSTOM').click();
    await expect(form.getByTestId('ban-custom-date')).toBeFocused();
    await noScroll(page);
    await form.getByRole('button', { name: 'CANCEL' }).click();
    await addBan(page, `Tomasz Wiśniewski-Kowalczyk ${t}`, 'A long reason that must wrap on a phone instead of pushing the page sideways at the door');
    await noScroll(page);
    const row = page.getByTestId('ban-table').getByRole('row').filter({ hasText: `Tomasz Wiśniewski-Kowalczyk ${t}` });
    await atLeast(row.getByRole('button', { name: /^Edit / }), 44);
    await atLeast(row.getByRole('button', { name: /^Remove / }), 44);
    expect((await row.boundingBox())?.width ?? 999).toBeLessThanOrEqual(375);

    // Locked: the unlock form fits too.
    await page.getByTestId('ban-lock').click();
    await expect(page.getByTestId('sealed-unlock')).toBeVisible();
    await atLeast(page.getByTestId('sealed-passphrase'), 44);
    await noScroll(page);
    await page.getByTestId('sealed-passphrase').fill(PASS);
    await page.getByRole('button', { name: 'UNLOCK' }).click();
    await expect(page.getByTestId('ban-table')).toContainText(`Tomasz Wiśniewski-Kowalczyk ${t}`, { timeout: 30_000 });
  });

  test('the door card shows the ban warning and the manager PIN field at 56 px on a phone', async ({ page }) => {
    const t = tag();
    page.on('dialog', d => void d.accept());
    await isolate(page);
    await setUpSealed(page);
    await page.getByTestId('sealed-section').getByRole('link', { name: 'OPEN THE BAN LIST →' }).click();
    await addBan(page, `Ines Duarte ${t}`, 'Harassed guests');
    expect((await page.request.post('/api/v1/events/e-klubnacht/guests', {
      headers: CSRF, data: { list_id: 'gl-comp', allocation_id: null, source: 'manual', guests: [{ name: `Ines Duarte ${t}`, plus_n: 1, status: 'going' }] },
    })).status()).toBe(201);

    await page.goto('/events/e-klubnacht/door');
    await hydrated(page);
    await page.getByLabel('DEVICE NAME').fill(`Phone ${t}`);
    await page.getByRole('button', { name: 'USE THIS BROWSER AS A DOOR DEVICE' }).click();
    await expect(page.getByTestId(`door-sealed-Phone ${t}`)).toHaveText('READY TO PROVISION');
    const panel = page.getByTestId('door-ban-panel');
    await panel.getByTestId('sealed-passphrase').fill(PASS);
    await panel.getByRole('button', { name: 'UNLOCK' }).click();
    await expect(panel.getByTestId('door-ban-unlocked')).toBeVisible({ timeout: 30_000 });
    const provision = page.getByRole('button', { name: `Provision Phone ${t} with the ban list` });
    await atLeast(provision, 44);
    const confirm = page.getByTestId(`door-confirm-Phone ${t}`);
    await confirm.check();
    expect((await confirm.locator('xpath=..').boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await provision.click();
    await expect(page.getByTestId(`door-sealed-Phone ${t}`)).toHaveText('GETS THE BAN LIST');
    await noScroll(page);
    await page.getByRole('button', { name: /STAFF PIN$/ }).click();
    const staffPin = ((await page.getByTestId('door-staff-pin').textContent()) ?? '').trim();
    await page.getByRole('button', { name: /MANAGER PIN$/ }).click();

    await page.getByRole('link', { name: 'OPEN THE DOOR' }).click();
    await expect(page).toHaveURL(/\/door$/);
    const pad = page.getByRole('group', { name: 'DOOR PIN' });
    for (const d of staffPin) await pad.getByRole('button', { name: d, exact: true }).click();
    const search = page.getByLabel('Search guests and tickets');
    await search.fill(`ines ${t}`);
    await page.getByRole('list', { name: 'Search results' }).getByRole('button').first().click();
    const warning = page.getByTestId('door-ban');
    await expect(warning).toContainText('MANAGER CHECK');
    await atLeast(warning.getByTestId('door-ban-pin'), 56);
    await atLeast(warning.getByTestId('door-ban-reveal'), 56);
    await noScroll(page);
    await warning.getByTestId('door-ban-pin').fill('246810');
    await warning.getByTestId('door-ban-reveal').click();
    await expect(warning.getByTestId('door-ban-reasons')).toContainText('Harassed guests');
    await atLeast(warning.getByTestId('door-ban-turn-away'), 56);
    await atLeast(warning.getByTestId('door-ban-hide'), 56);
    await noScroll(page);
  });
});
