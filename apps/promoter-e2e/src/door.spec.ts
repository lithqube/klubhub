import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

// The mock API keeps state for the life of the dev server, so every test
// uses its own device label and guest names.
const tag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;
const CSRF = { 'X-KlubHub-CSRF': '1' };

/** Prepare this browser from the event DOOR tab and generate both PINs. */
async function prepare(page: Page, t: string) {
  page.on('dialog', d => void d.accept());
  await page.goto('/events/e-klubnacht/door');
  await hydrated(page);
  await page.getByLabel('DEVICE NAME').fill(`Door ${t}`);
  await page.getByRole('button', { name: 'USE THIS BROWSER AS A DOOR DEVICE' }).click();
  await expect(page.getByTestId('door-this-device')).toContainText(`Door ${t}`);
  await expect(page.getByRole('list', { name: 'Door devices' })).toContainText(`Door ${t}`);

  await page.getByRole('button', { name: /STAFF PIN$/ }).click();
  const staffPin = (await page.getByTestId('door-staff-pin').textContent() ?? '').trim();
  expect(staffPin).toMatch(/^\d{6}$/);
  await expect(page.getByTestId('pin-status-staff')).toContainText('VALID UNTIL');
  await page.getByRole('button', { name: /MANAGER PIN$/ }).click();
  const managerPin = (await page.getByTestId('door-manager-pin').textContent() ?? '').trim();
  expect(managerPin).toBe('246810'); // the mock's fixed manager PIN
  return { staffPin, managerPin };
}

async function enterPin(page: Page, pin: string) {
  const pad = page.getByRole('group', { name: 'DOOR PIN' });
  for (const [i, d] of [...pin].entries()) {
    await pad.getByRole('button', { name: d, exact: true }).click();
    if (i < 5) await expect(pad.getByRole('status')).toHaveAttribute('aria-label', `${i + 1} of 6 digits entered`);
  }
}

async function openDoor(page: Page, pin: string) {
  await page.getByRole('link', { name: 'OPEN THE DOOR' }).click();
  await expect(page).toHaveURL(/\/door$/);
  await expect(page.getByRole('group', { name: 'DOOR PIN' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Klubnacht 03', level: 1 })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect(page.getByRole('navigation', { name: 'Primary navigation' })).toHaveCount(0);
  await enterPin(page, pin);
  await expect(page.getByLabel('Search guests and tickets')).toBeFocused();
}

const inside = async (page: Page) => Number(await page.getByTestId('door-inside').textContent());

test.describe('offline door (mock API)', () => {
  test('search with accents and a typo, partial arrival, undo, and an offline check-in that syncs later', async ({ page, context }) => {
    const t = tag();
    const { staffPin } = await prepare(page, t);
    // A guest with an accent and two plus-ones, added before the door downloads the list.
    const res = await page.request.post('/api/v1/events/e-klubnacht/guests', {
      headers: CSRF, data: { list_id: 'gl-comp', allocation_id: null, source: 'manual', guests: [{ name: `Zoë Lindqvist ${t}`, plus_n: 2, status: 'going' }] },
    });
    expect(res.status()).toBe(201);

    await openDoor(page, staffPin);
    const search = page.getByLabel('Search guests and tickets');
    const before = await inside(page);

    // No accent, one swapped letter.
    await search.fill(`zoe lindqvsit ${t}`);
    const results = page.getByRole('list', { name: 'Search results' });
    await expect(results.getByRole('button')).toHaveCount(1);
    await results.getByRole('button').click();
    const card = page.getByRole('article', { name: `Zoë Lindqvist ${t}` });
    await expect(card).toContainText('GUEST · Comp · +2');
    await expect(card.getByTestId('door-heads')).toHaveText('IN 0 OF 3 · 3 REMAINING');

    // Two of three arrive.
    await card.getByRole('button', { name: 'One fewer' }).click();
    await card.getByTestId('door-admit').click();
    await expect(page.getByTestId('door-toast')).toContainText(`Zoë Lindqvist ${t} · 2 IN`);
    await expect(page.getByTestId('door-announce')).toHaveText(`Zoë Lindqvist ${t} · 2 IN`);
    await expect.poll(() => inside(page)).toBe(before + 2);

    // Undo within the toast window.
    await page.getByTestId('door-toast').getByRole('button', { name: 'UNDO' }).click();
    await expect(page.getByTestId('door-toast')).toContainText('UNDONE');
    await expect.poll(() => inside(page)).toBe(before);
    await expect(page.getByTestId('door-sync')).toContainText('SYNCED', { timeout: 20_000 });

    // Offline: the check-in is queued on the device.
    await context.setOffline(true);
    await search.fill(`lindq ${t}`);
    await results.getByRole('button').first().click();
    await expect(card.getByTestId('door-heads')).toHaveText('IN 0 OF 3 · 3 REMAINING');
    await card.getByRole('button', { name: 'One fewer' }).click();
    await card.getByRole('button', { name: 'One fewer' }).click();
    await card.getByTestId('door-admit').click();
    await expect(page.getByTestId('door-sync')).toContainText('OFFLINE · 1 QUEUED');
    await expect.poll(() => inside(page)).toBe(before + 1);

    // Back online: it syncs.
    await context.setOffline(false);
    await expect(page.getByTestId('door-sync')).toContainText('SYNCED', { timeout: 20_000 });
    await search.fill(`zoe ${t}`);
    await expect(results.getByRole('button')).toContainText('1/3 IN');

    // RECENT offers the same undo as the toast (only while the search is empty).
    await search.fill('');
    const recent = page.getByRole('list', { name: 'RECENT' });
    await expect(recent.getByRole('listitem').first()).toContainText(`Zoë Lindqvist ${t}`);
    await expect(recent.getByRole('listitem').first()).toContainText('1 IN');

    // Another door device sees the check-in in its bundle.
    const devices = await (await page.request.get('/api/v1/door/devices')).json() as { label: string }[];
    expect(devices.some(d => d.label === `Door ${t}`)).toBe(true);
  });

  test('a refunded ticket is blocked, and a manager PIN adds a guest at the door', async ({ page }) => {
    const t = tag();
    const { staffPin, managerPin } = await prepare(page, t);
    await openDoor(page, staffPin);
    const search = page.getByLabel('Search guests and tickets');

    // Ticket order numbers match exactly.
    await search.fill('D-7002');
    await page.getByRole('list', { name: 'Search results' }).getByRole('button').click();
    const ticket = page.getByRole('article', { name: 'Theo Brandt' });
    await expect(ticket.getByRole('alert')).toContainText('DO NOT ADMIT · TICKET REFUNDED');
    await expect(ticket.getByTestId('door-admit')).toBeDisabled();
    await ticket.getByRole('button', { name: 'BACK TO SEARCH' }).click();

    // Typed QR code in STANDARD mode opens the card.
    await page.getByRole('button', { name: 'Scan a QR code' }).click();
    const scanner = page.getByRole('dialog');
    await scanner.getByLabel('OR TYPE THE CODE').fill('DICE-0003');
    await scanner.getByRole('button', { name: 'CHECK' }).click();
    await expect(page.getByRole('article', { name: 'Theo Brandt' })).toBeVisible();
    await page.getByRole('button', { name: 'BACK TO SEARCH' }).click();

    // An unknown code sends staff to the name search.
    await page.getByRole('button', { name: 'Scan a QR code' }).click();
    await page.getByRole('dialog').getByLabel('OR TYPE THE CODE').fill('NOPE-0000');
    await page.getByRole('dialog').getByRole('button', { name: 'CHECK' }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByText('Code not found — search the name.')).toBeVisible();
    await expect(search).toBeFocused();

    // EXPRESS keeps the scanner open between codes and shows each result over the camera.
    await page.getByRole('button', { name: 'Scan a QR code' }).click();
    const scan = page.getByRole('dialog');
    await scan.getByRole('radio', { name: 'STANDARD' }).focus();
    await page.keyboard.press('ArrowRight');
    await expect(scan.getByRole('radio', { name: 'EXPRESS' })).toHaveAttribute('aria-checked', 'true');
    await expect(scan.getByRole('radio', { name: 'EXPRESS' })).toBeFocused();
    await scan.getByLabel('OR TYPE THE CODE').fill('NOPE-0001');
    await scan.getByRole('button', { name: 'CHECK' }).click();
    await expect(scan.getByTestId('scan-result')).toContainText('CODE NOT FOUND');
    await expect(scan).toBeVisible();
    // A refunded ticket must be looked at: the card opens and the scanner closes.
    await scan.getByLabel('OR TYPE THE CODE').fill('DICE-0003');
    await scan.getByRole('button', { name: 'CHECK' }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('article', { name: 'Theo Brandt' })).toBeVisible();
    await page.getByRole('button', { name: 'BACK TO SEARCH' }).click();
    // Back to STANDARD for the rest of the run (the mode is remembered on the device).
    await page.getByRole('button', { name: 'Scan a QR code' }).click();
    await page.getByRole('dialog').getByRole('radio', { name: 'STANDARD' }).click();
    await page.getByRole('button', { name: 'Close scanner' }).click();

    // On-the-spot add.
    await page.getByRole('button', { name: 'ADD GUEST' }).click();
    const sheet = page.getByRole('dialog', { name: 'ADD AT THE DOOR' });
    await expect(sheet.getByLabel('NAME')).toBeFocused();
    await expect(sheet.getByLabel('LIST')).toHaveValue(''); // no default list
    await sheet.getByLabel('NAME').fill(`Ola Door ${t}`);
    await sheet.getByRole('button', { name: 'One more plus' }).click();
    await sheet.getByLabel('LIST').selectOption({ label: 'Comp' });
    await sheet.getByLabel('MANAGER PIN').fill('111111');
    await sheet.getByRole('button', { name: 'ADD & ADMIT 2 NOW' }).click();
    await expect(sheet.getByRole('alert')).toHaveText('Wrong manager PIN. Ask the manager on shift.');
    await sheet.getByLabel('MANAGER PIN').fill(managerPin);
    await sheet.getByRole('button', { name: 'ADD & ADMIT 2 NOW' }).click();
    await expect(sheet).toHaveCount(0);
    await expect(page.getByTestId('door-toast')).toContainText(`Ola Door ${t} ADDED · 2 IN`);
    await expect(page.getByTestId('door-sync')).toContainText('SYNCED', { timeout: 20_000 });

    await search.fill(`ola door ${t}`);
    await expect(page.getByRole('list', { name: 'Search results' }).getByRole('button')).toContainText('ALL IN');
    const page2 = await (await page.request.get('/api/v1/events/e-klubnacht/guests')).json() as { guests: { name: string, source: string }[] };
    expect(page2.guests.find(g => g.name === `Ola Door ${t}`)?.source).toBe('door');

    // Logging out (from the header menu, after a confirm) wipes the device; the PIN pad comes back.
    await page.getByRole('button', { name: 'Door menu' }).click();
    await page.getByRole('button', { name: 'LOG OUT' }).click();
    await expect(page.getByText('Logged out. Nothing from the guest list is left on this device.')).toBeVisible();
    await expect(page.getByRole('group', { name: 'DOOR PIN' })).toBeVisible();
  });

  test('a second card hides the undo toast, and an armed ADMIT ignores a double tap', async ({ page }) => {
    const t = tag();
    const { staffPin } = await prepare(page, t);
    const res = await page.request.post('/api/v1/events/e-klubnacht/guests', {
      headers: CSRF, data: { list_id: 'gl-comp', allocation_id: null, source: 'manual', guests: [{ name: `Ada Toast ${t}`, plus_n: 0, status: 'going' }, { name: `Bo Toast ${t}`, plus_n: 0, status: 'going' }] },
    });
    expect(res.status()).toBe(201);
    await openDoor(page, staffPin);
    const search = page.getByLabel('Search guests and tickets');
    const results = page.getByRole('list', { name: 'Search results' });

    await search.fill(`ada toast ${t}`);
    await results.getByRole('button').first().click();
    await page.getByRole('article', { name: `Ada Toast ${t}` }).getByTestId('door-admit').click();
    await expect(page.getByTestId('door-toast')).toContainText(`Ada Toast ${t} · 1 IN`);

    await search.fill(`bo toast ${t}`);
    await results.getByRole('button').first().click();
    const card = page.getByRole('article', { name: `Bo Toast ${t}` });
    await expect(card.getByRole('heading', { name: `Bo Toast ${t}` })).toBeFocused();
    await expect(page.getByTestId('door-toast')).toHaveCount(0);
    await card.getByRole('button', { name: 'BACK TO SEARCH' }).click();

    // Ada again: already in. The warning names when; a double tap only arms.
    await search.fill(`ada toast ${t}`);
    await results.getByRole('button').first().click();
    const again = page.getByRole('article', { name: `Ada Toast ${t}` });
    await expect(again.getByTestId('door-warning')).toContainText(/ALREADY IN · \d{2}:\d{2} · THIS DOOR/);
    const admit = again.getByTestId('door-admit');
    await expect(admit).toHaveText('CHECK: ALREADY IN');
    await admit.dblclick();
    await expect(admit).toContainText('TAP AGAIN · ADMIT 1');
    await expect(again).toBeVisible();
    // A deliberate second tap (held past the 700 ms guard) admits.
    await admit.click({ delay: 750 });
    await expect(page.getByTestId('door-toast')).toContainText(`Ada Toast ${t} · 1 IN`);
  });

  test('LOG OUT asks first, and a locked or expired PIN says so', async ({ page }) => {
    const t = tag();
    const { staffPin } = await prepare(page, t);
    page.removeAllListeners('dialog');
    const asked: string[] = [];
    let accept = false;
    page.on('dialog', (d) => {
      asked.push(d.message());
      void (accept ? d.accept() : d.dismiss());
    });
    await openDoor(page, staffPin);

    await page.getByRole('button', { name: 'Door menu' }).click();
    await page.getByRole('button', { name: 'LOG OUT' }).click();
    expect(asked.at(-1)).toContain('Log out? You need the door PIN and a connection to log in again.');
    await expect(page.getByLabel('Search guests and tickets')).toBeVisible(); // dismissed: still open

    accept = true;
    await page.getByRole('button', { name: 'LOG OUT' }).click();
    await expect(page.getByRole('group', { name: 'DOOR PIN' })).toBeVisible();

    // The mock answers '999999' like an expired PIN and '111111' like a locked one.
    await enterPin(page, '999999');
    await expect(page.getByRole('alert')).toContainText('This door PIN has expired');
    await enterPin(page, '111111');
    await expect(page.getByTestId('door-pin-locked')).toContainText(/PIN LOCKED · TRY AGAIN AT \d{2}:\d{2}/);
    await expect(page.getByTestId('door-pin-locked')).toContainText('A manager can unlock it now by generating a new PIN in the event’s DOOR tab.');
    await expect(page.getByRole('group', { name: 'DOOR PIN' }).getByRole('button', { name: '1', exact: true })).toBeDisabled();
  });
});
