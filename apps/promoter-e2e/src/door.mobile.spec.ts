import { expect, test, type Locator, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

const tag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;
const overflow = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);

async function tall(l: Locator) {
  const box = await l.boundingBox();
  expect(box?.height ?? 0, 'element is rendered at least 56 px tall').toBeGreaterThanOrEqual(56);
}

test.use({ viewport: { width: 375, height: 812 } });

test.describe('door (phone)', () => {
  test('fits 375 px with 56 px targets from PIN pad to check-in', async ({ page }) => {
    page.on('dialog', d => void d.accept());
    const t = tag();
    // Prepare the device the way the DOOR tab does: register, keep the token in localStorage.
    await page.goto('/door');
    await hydrated(page);
    await expect(page.getByText('THIS BROWSER IS NOT A DOOR DEVICE YET')).toBeVisible();
    const d = await (await page.request.post('/api/v1/door/devices', { headers: { 'X-KlubHub-CSRF': '1' }, data: { label: `Phone ${t}` } })).json();
    await page.evaluate(rec => localStorage.setItem('klubhub-door-device', JSON.stringify(rec)), {
      ...d, event: { id: 'e-klubnacht', title: 'Klubnacht 03', starts_at: new Date().toISOString() },
    });
    await page.reload();
    await hydrated(page);

    await expect(page.getByRole('heading', { name: 'Klubnacht 03', level: 1 })).toBeVisible();
    for (const k of ['1', '5', '0']) await tall(page.getByRole('button', { name: k, exact: true }));
    expect(await overflow(page)).toBeLessThanOrEqual(0);

    // The mock refuses 000000 like a wrong PIN.
    for (let i = 0; i < 6; i++) await page.getByRole('button', { name: '0', exact: true }).click();
    await expect(page.getByRole('alert')).toContainText('Wrong PIN');
    for (const k of '135790') await page.getByRole('button', { name: k, exact: true }).click();

    const search = page.getByLabel('Search guests and tickets');
    await expect(search).toBeVisible();
    await tall(search);
    for (const name of ['Scan a QR code', 'WALK-UP +1', 'OUT −1', 'ADD GUEST', 'LOG OUT']) {
      await tall(page.getByRole('button', { name }));
    }
    for (const name of ['STANDARD', 'EXPRESS']) await tall(page.getByRole('radio', { name }));
    await tall(page.getByTestId('door-sync'));
    expect(await overflow(page)).toBeLessThanOrEqual(0);

    await search.fill('mara');
    const hit = page.getByRole('list', { name: 'Search results' }).getByRole('button').first();
    await tall(hit);
    await hit.click();
    const card = page.getByRole('article', { name: 'Mara Weiss' });
    await expect(card).toBeVisible();
    for (const l of [card.getByTestId('door-admit'), card.getByRole('button', { name: 'One more' }), card.getByRole('button', { name: 'BACK TO SEARCH' })]) await tall(l);
    expect(await overflow(page)).toBeLessThanOrEqual(0);

    // The ADD sheet fits too.
    await card.getByRole('button', { name: 'BACK TO SEARCH' }).click();
    await page.getByRole('button', { name: 'ADD GUEST' }).click();
    const sheet = page.getByRole('dialog', { name: 'ADD AT THE DOOR' });
    await tall(sheet.getByLabel('NAME'));
    await tall(sheet.getByRole('button', { name: /ADD & ADMIT/ }));
    expect(await overflow(page)).toBeLessThanOrEqual(0);
    await sheet.getByRole('button', { name: 'Close' }).click();

    await page.getByRole('button', { name: 'LOG OUT' }).click();
    await expect(page.getByRole('group', { name: 'DOOR PIN' })).toBeVisible();
  });
});
