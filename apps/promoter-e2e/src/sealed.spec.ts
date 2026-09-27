import { expect, test, type Page } from '@playwright/test';
import { addBan, CSRF, fingerprintOf, hydrated, isolate, PASS, setUpSealed, tag } from './sealed-helpers';

// Sealed tier and ban list (P2.6) against the mock API. The mock keeps
// ciphertext only; every test has its own sealed namespace (see isolate()).

async function enterPin(page: Page, pin: string) {
  const pad = page.getByRole('group', { name: 'DOOR PIN' });
  for (const d of pin) await pad.getByRole('button', { name: d, exact: true }).click();
}

test.describe('sealed tier and ban list (mock API)', () => {
  test('owner sets up keys with the kit, locks and unlocks, bans a guest, provisions a door device and the door warns', async ({ page }) => {
    const t = tag();
    page.on('dialog', d => void d.accept());
    await isolate(page);
    await setUpSealed(page);
    const section = page.getByTestId('sealed-section');

    // Set up once: a second setup is refused.
    const again = await page.request.post('/api/v1/keys/org/setup', { headers: CSRF, data: { version: 1, recovery: {}, wraps: [] } });
    expect(again.status()).toBe(409);

    // The owner's own key fingerprint is on screen, as the server's copy of the public key says.
    const mine = await (await page.request.get('/api/v1/keys/me')).json() as { public_key: string };
    await expect(section.getByTestId('sealed-my-fingerprint')).toContainText('YOUR KEY FINGERPRINT');
    await expect(section.getByTestId('sealed-my-fingerprint-value')).toHaveText(fingerprintOf(mine.public_key));

    // Grant access to a member who already has a key: only after comparing her fingerprint.
    const access = section.getByTestId('sealed-access');
    const who = await (await page.request.get('/api/v1/keys/org/recipients')).json() as { members: { name: string, public_key: string | null }[] };
    const lena = who.members.filter(m => m.name === 'Lena Park').map(m => String(m.public_key));
    expect(lena).toHaveLength(1);
    const lenaKey = String(lena[0]);
    // What Lena sees in her Settings is fingerprintOf(her public key) — the same text the owner compares.
    await expect(access.getByTestId('sealed-fp-Lena Park')).toHaveText(fingerprintOf(lenaKey));
    const grantLena = access.getByRole('button', { name: 'Grant access to Lena Park' });
    await expect(grantLena).toBeDisabled();
    await access.getByTestId('sealed-confirm-Lena Park').check();
    await grantLena.click();
    await expect(access.getByTestId('sealed-access-notice')).toContainText('Lena Park can now open the sealed data');
    await expect(access.getByTestId('sealed-member-Lena Park')).toContainText('HAS ACCESS');
    await expect(access.getByTestId('sealed-member-Sam Ortiz')).toContainText('NO KEY YET');

    // Lock, a wrong passphrase, then unlock.
    await section.getByTestId('sealed-lock').click();
    await expect(section.getByTestId('sealed-status')).toContainText('LOCKED');
    await section.getByTestId('sealed-passphrase').fill('not the passphrase');
    await section.getByRole('button', { name: 'UNLOCK' }).click();
    await expect(section.getByTestId('sealed-unlock-error')).toContainText('does not unlock your key', { timeout: 30_000 });
    // The typed passphrase stays, selected, so a typo can be fixed.
    await expect(section.getByTestId('sealed-passphrase')).toBeFocused();
    await expect(section.getByTestId('sealed-passphrase')).toHaveValue('not the passphrase');
    await section.getByTestId('sealed-passphrase').fill(PASS);
    await section.getByRole('button', { name: 'UNLOCK' }).click();
    await expect(section.getByTestId('sealed-unlocked')).toBeVisible({ timeout: 30_000 });

    // The ban list, reached in the app (the keys stay in this tab's memory).
    await section.getByRole('link', { name: 'OPEN THE BAN LIST →' }).click();
    await expect(page).toHaveURL(/\/ban-list$/);
    await expect(page.getByText('NO ONE ON THE BAN LIST')).toBeVisible();
    // Name and reason are required.
    await page.getByTestId('ban-add').click();
    await page.getByTestId('ban-save').click();
    await expect(page.getByTestId('ban-form')).toContainText('Enter the person\'s name.');
    await expect(page.getByTestId('ban-name')).toBeFocused();
    await page.getByRole('button', { name: 'CANCEL' }).click();
    await addBan(page, `Viktor Brandt ${t}`, 'Fight at the bar in March');
    await addBan(page, `Mia Klein ${t}`, 'Stole a phone');
    // Search is accent-folded.
    await page.getByTestId('ban-search').fill(`víktor ${t}`);
    await expect(page.getByTestId('ban-table').getByRole('row')).toHaveCount(2); // header + 1
    // The server only ever saw ciphertext.
    const raw = await (await page.request.get('/api/v1/ban-list')).text();
    expect(raw).not.toContain('Viktor');
    expect(raw).not.toContain('Fight');

    // A guest whose name matches (with an accent) and one who doesn't.
    const res = await page.request.post('/api/v1/events/e-klubnacht/guests', {
      headers: CSRF, data: { list_id: 'gl-comp', allocation_id: null, source: 'manual', guests: [{ name: `Viktor Brändt ${t}`, plus_n: 0, status: 'going' }, { name: `Jonas Weber ${t}`, plus_n: 0, status: 'going' }] },
    });
    expect(res.status()).toBe(201);

    // Door tab: register this browser (its keypair is made here), unlock, PROVISION.
    await page.goto('/events/e-klubnacht/door');
    await hydrated(page);
    await page.getByLabel('DEVICE NAME').fill(`Door ${t}`);
    await page.getByRole('button', { name: 'USE THIS BROWSER AS A DOOR DEVICE' }).click();
    await expect(page.getByTestId('door-this-device')).toContainText(`Door ${t}`);
    await expect(page.getByTestId(`door-sealed-Door ${t}`)).toHaveText('READY TO PROVISION');
    const panel = page.getByTestId('door-ban-panel');
    await expect(panel.getByTestId('door-staff-briefing')).toContainText('MANAGER CHECK on the door means a possible ban list match.');
    await panel.getByTestId('sealed-passphrase').fill(PASS);
    await panel.getByRole('button', { name: 'UNLOCK' }).click();
    await expect(panel.getByTestId('door-ban-unlocked')).toBeVisible({ timeout: 30_000 });
    // The device's fingerprint as the owner sees it (from the server) is the one this browser holds.
    const ownerSeesFp = ((await page.getByTestId(`door-fp-Door ${t}`).textContent()) ?? '').trim();
    expect(ownerSeesFp).toMatch(/^[0-9A-F]{4}( [0-9A-F]{4}){3}$/);
    await expect(page.getByTestId('door-this-fp')).toHaveText(ownerSeesFp);
    const provisionBtn = page.getByRole('button', { name: `Provision Door ${t} with the ban list` });
    await expect(provisionBtn).toBeDisabled();
    await page.getByTestId(`door-confirm-Door ${t}`).check();
    await provisionBtn.click();
    await expect(page.getByTestId(`door-sealed-Door ${t}`)).toHaveText('GETS THE BAN LIST');
    await expect(page.getByTestId('door-provision-notice')).toContainText(`Door ${t} now gets the ban list`);

    await page.getByRole('button', { name: /STAFF PIN$/ }).click();
    const staffPin = ((await page.getByTestId('door-staff-pin').textContent()) ?? '').trim();
    await page.getByRole('button', { name: /MANAGER PIN$/ }).click();
    await expect(page.getByTestId('door-manager-pin')).toHaveText('246810');

    // The door: log in, find the guest, see the quiet warning; the reason needs the manager PIN.
    await page.getByRole('link', { name: 'OPEN THE DOOR' }).click();
    await expect(page).toHaveURL(/\/door$/);
    // The door device shows the same fingerprint on its own screen (login screen and menu).
    await expect(page.getByTestId('door-device-fp')).toContainText(ownerSeesFp);
    await enterPin(page, staffPin);
    const search = page.getByLabel('Search guests and tickets');
    await expect(search).toBeFocused();
    await expect(page.getByTestId('door-ban-note')).toHaveCount(0);
    await page.getByRole('button', { name: 'Door menu' }).click();
    await expect(page.getByTestId('door-ban-status')).toHaveText('BAN LIST CHECKED ON THIS DEVICE');
    await expect(page.getByTestId('door-menu-fp')).toContainText(ownerSeesFp);
    await page.getByRole('button', { name: 'Door menu' }).click();

    await search.fill(`jonas ${t}`);
    await page.getByRole('list', { name: 'Search results' }).getByRole('button').first().click();
    await expect(page.getByRole('article', { name: `Jonas Weber ${t}` })).toBeVisible();
    await expect(page.getByTestId('door-ban')).toHaveCount(0);
    await page.getByRole('button', { name: 'BACK TO SEARCH' }).click();

    await search.fill(`viktor brandt ${t}`);
    await page.getByRole('list', { name: 'Search results' }).getByRole('button').first().click();
    const card = page.getByRole('article', { name: `Viktor Brändt ${t}` });
    const warning = card.getByTestId('door-ban');
    // Discreet: the guest may see the screen, so the card never says "ban list" before the reveal.
    await expect(warning).toContainText('MANAGER CHECK');
    await expect(card).not.toContainText(/ban list/i);
    await expect(warning).not.toContainText('Fight at the bar');
    await expect(card.getByTestId('door-admit')).toBeDisabled();
    await expect(card.getByTestId('door-admit')).toHaveText('ASK A MANAGER');

    await warning.getByTestId('door-ban-pin').fill('000000');
    await warning.getByTestId('door-ban-reveal').click();
    await expect(warning.getByRole('alert')).toContainText('Wrong manager PIN.');
    await expect(warning.getByTestId('door-ban-pin')).toBeFocused();
    await warning.getByTestId('door-ban-pin').fill('246810');
    await warning.getByTestId('door-ban-reveal').click();
    await expect(warning.getByTestId('door-ban-reasons')).toContainText(`Viktor Brandt ${t} · Fight at the bar in March`);
    await expect(warning).not.toContainText('Stole a phone');

    // The manager decides deliberately: ADMIT is a second tap, TURN AWAY and HIDE REASON are there.
    // (ADMIT is not clicked: door.spec counts occupancy on this shared mock event in parallel.)
    await expect(card.getByTestId('door-admit')).toBeEnabled();
    await expect(card.getByTestId('door-admit')).toHaveText('MANAGER DECIDED? TAP TO ADMIT');
    await warning.getByTestId('door-ban-hide').click();
    await expect(warning.getByTestId('door-ban-reasons')).toHaveCount(0);
    await expect(warning.getByTestId('door-ban-hidden')).toBeVisible();
    await expect(card).not.toContainText('Fight at the bar');
    await warning.getByTestId('door-ban-turn-away').click();
    await expect(card).toHaveCount(0);
    await expect(search).toBeVisible();

    // Five wrong manager PINs pause SHOW REASON on this device, whichever card is open.
    await search.fill(`viktor brandt ${t}`);
    await page.getByRole('list', { name: 'Search results' }).getByRole('button').first().click();
    for (const left of ['4 tries', '3 tries', '2 tries', '1 try']) {
      await warning.getByTestId('door-ban-pin').fill('000000');
      await warning.getByTestId('door-ban-reveal').click();
      await expect(warning.getByRole('alert')).toContainText(`${left} left`);
    }
    await warning.getByTestId('door-ban-pin').fill('000000');
    await warning.getByTestId('door-ban-reveal').click();
    await expect(warning.getByTestId('door-ban-locked')).toContainText('Too many wrong PINs');
    await expect(warning.getByTestId('door-ban-reveal')).toBeDisabled();
    await expect(warning.getByTestId('door-ban-reveal')).toHaveText(/WAIT \d+ S/);
  });

  test('an owner who forgot the passphrase recovers with the kit, and the ban list still opens', async ({ page }) => {
    const t = tag();
    await isolate(page);
    const groups = await setUpSealed(page);
    const section = page.getByTestId('sealed-section');
    await section.getByRole('link', { name: 'OPEN THE BAN LIST →' }).click();
    await addBan(page, `Ole Sander ${t}`, 'Threatened staff');

    // A reload forgets the keys: locked.
    await page.goto('/settings#sealed');
    await hydrated(page);
    await expect(section.getByTestId('sealed-status')).toContainText('LOCKED');
    await section.getByTestId('sealed-show-recover').click();
    const recover = section.getByTestId('sealed-recover');
    await expect(recover).toBeVisible();

    // A typo is caught by the kit's checksum.
    const kit = groups.join('');
    const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
    const typo = `${kit.slice(0, 10)}${B32[(B32.indexOf(kit[10] ?? 'A') + 1) % 32]}${kit.slice(11)}`;
    await recover.getByTestId('sealed-recover-kit').fill(typo);
    await recover.getByTestId('sealed-recover-passphrase').fill('a brand new sealed passphrase');
    await recover.getByTestId('sealed-recover-passphrase-again').fill('a brand new sealed passphrase');
    await recover.getByTestId('sealed-recover-submit').click();
    await expect(recover.getByTestId('sealed-recover-error')).toContainText('typo');
    await expect(recover.getByTestId('sealed-recover-kit')).toBeFocused();

    // Lower case with dashes, as someone might type it from paper.
    await recover.getByTestId('sealed-recover-kit').fill(groups.join('-').toLowerCase());
    await expect(recover).toContainText('56 of 56 characters');
    await recover.getByTestId('sealed-recover-submit').click();
    await expect(section.getByTestId('sealed-notice')).toContainText('Recovered', { timeout: 30_000 });
    await expect(section.getByTestId('sealed-status')).toContainText('UNLOCKED');

    await section.getByRole('link', { name: 'OPEN THE BAN LIST →' }).click();
    await expect(page.getByTestId('ban-table')).toContainText(`Ole Sander ${t}`);
    await expect(page.getByTestId('ban-table')).toContainText('Threatened staff');
    // Loaded from the server: who added it.
    await expect(page.getByTestId('ban-added-by').first()).toHaveText('by Owner (you)');

    // The old passphrase no longer unlocks; the new one does.
    await page.getByTestId('ban-lock').click();
    await page.getByTestId('sealed-passphrase').fill(PASS);
    await page.getByRole('button', { name: 'UNLOCK' }).click();
    await expect(page.getByTestId('sealed-unlock-error')).toBeVisible({ timeout: 30_000 });
    await page.getByTestId('sealed-passphrase').fill('a brand new sealed passphrase');
    await page.getByRole('button', { name: 'UNLOCK' }).click();
    await expect(page.getByTestId('ban-table')).toContainText(`Ole Sander ${t}`, { timeout: 30_000 });
  });

  test('revoking a provisioned device asks for a rotation, which re-encrypts the ban list', async ({ page }) => {
    const t = tag();
    page.on('dialog', d => void d.accept());
    await isolate(page);
    await setUpSealed(page);
    const section = page.getByTestId('sealed-section');
    await section.getByRole('link', { name: 'OPEN THE BAN LIST →' }).click();
    await addBan(page, `Kai Berg ${t}`, 'Fight');
    const before = await (await page.request.get('/api/v1/ban-list')).json() as { entries: { entry_sealed: string }[] };

    // A device registered through the API with a public key, provisioned in settings, then revoked.
    const pub = Buffer.alloc(32, 9).toString('base64url');
    const dev = await (await page.request.post('/api/v1/door/devices', { headers: CSRF, data: { label: `Spare ${t}`, public_key: pub } })).json() as { id: string };
    await page.getByRole('link', { name: 'ENCRYPTION SETTINGS' }).click();
    const access = section.getByTestId('sealed-access');
    await expect(access.getByTestId(`sealed-fp-Spare ${t}`)).toHaveText(fingerprintOf(pub));
    await access.getByTestId(`sealed-confirm-Spare ${t}`).check();
    await access.getByRole('button', { name: `Provision Spare ${t}` }).click();
    await expect(access.getByTestId('sealed-access-notice')).toContainText(`Spare ${t} can now open`);
    expect((await page.request.delete(`/api/v1/door/devices/${dev.id}`, { headers: CSRF })).status()).toBe(204);

    await page.reload();
    await hydrated(page);
    await expect(section.getByTestId('sealed-status')).toContainText('ROTATION PENDING');
    await section.getByTestId('sealed-passphrase').fill(PASS);
    await section.getByRole('button', { name: 'UNLOCK' }).click();
    await section.getByTestId('sealed-rotate').click();
    await expect(section.getByTestId('sealed-access-notice')).toContainText('The collective key is now version 2', { timeout: 30_000 });
    await expect(section.getByTestId('sealed-status')).toContainText('READY · KEY VERSION 2');
    const after = await (await page.request.get('/api/v1/ban-list')).json() as { key_version: number, entries: { entry_sealed: string, key_version: number }[] };
    expect(after.key_version).toBe(2);
    expect(after.entries).toEqual([expect.objectContaining({ key_version: 2 })]);
    expect(after.entries[0]?.entry_sealed).not.toBe(before.entries[0]?.entry_sealed);
    await section.getByRole('link', { name: 'OPEN THE BAN LIST →' }).click();
    await expect(page.getByTestId('ban-table')).toContainText(`Kai Berg ${t}`);
  });

  test('without a second factor, owners can make their key but not set up sealed data', async ({ page }) => {
    await page.context().addCookies([{ name: 'kh_mock_sealed', value: `e2e-${tag()}`, domain: 'localhost', path: '/' }]);
    await page.goto('/settings#sealed');
    await hydrated(page);
    const section = page.getByTestId('sealed-section');
    await section.getByTestId('sealed-new-passphrase').fill('short');
    await section.getByRole('button', { name: 'CREATE MY KEY' }).click();
    await expect(section.getByTestId('sealed-create-error')).toHaveText('Use at least 12 characters.');
    await section.getByTestId('sealed-new-passphrase').fill(PASS);
    await section.getByTestId('sealed-new-passphrase-again').fill(PASS);
    await section.getByRole('button', { name: 'CREATE MY KEY' }).click();
    await expect(section.getByTestId('sealed-notice')).toContainText('Your key is ready', { timeout: 30_000 });
    await expect(section.getByTestId('sealed-mfa')).toContainText('two-factor');
    await expect(section.getByRole('link', { name: 'SET UP 2FA →' })).toHaveAttribute('href', /^\/account\/security\?next=/);
    await expect(section.getByTestId('sealed-setup-start')).toBeDisabled();
    // The ban list page explains instead of showing anything.
    await page.goto('/ban-list');
    await hydrated(page);
    await expect(page.getByTestId('ban-not-ready')).toContainText('Set up sealed data for the collective first');
  });

  test('the locked ban list offers owners RECOVER WITH KIT; a failed remove is an alert; the list says who added an entry', async ({ page }) => {
    const t = tag();
    page.on('dialog', d => void d.accept());
    await isolate(page);
    await setUpSealed(page);
    await page.getByTestId('sealed-section').getByRole('link', { name: 'OPEN THE BAN LIST →' }).click();
    await addBan(page, `Rafa Lindqvist ${t}`, 'Spiked a drink');
    await expect(page.getByTestId(`ban-table`).getByTestId('ban-added-by')).toHaveText('by You');

    // A remove that fails keeps the entry and says so in an alert box.
    await page.route('**/api/v1/ban-list/*', route => (route.request().method() === 'DELETE'
      ? route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'internal' }) })
      : route.continue()));
    await page.getByRole('button', { name: `Remove Rafa Lindqvist ${t}` }).click();
    const listError = page.getByTestId('ban-list-error');
    await expect(listError).toHaveText('Could not remove. Try again.');
    await expect(listError).toHaveAttribute('role', 'alert');
    await expect(listError).toHaveClass(/accent-bar-failed/);
    await expect(page.getByTestId('ban-table')).toContainText(`Rafa Lindqvist ${t}`);
    await page.unroute('**/api/v1/ban-list/*');

    // ADD waits while another row is being edited.
    await page.getByRole('button', { name: `Edit Rafa Lindqvist ${t}` }).click();
    await expect(page.getByTestId('ban-add')).toBeDisabled();
    await expect(page.getByTestId('ban-form')).toContainText('KEEP ·');
    await page.getByTestId('ban-form').getByRole('button', { name: 'CANCEL' }).click();
    await expect(page.getByTestId('ban-add')).toBeEnabled();

    // Locked (a reload forgets the keys): owners get a way back with the kit.
    await page.reload();
    await hydrated(page);
    await expect(page.getByTestId('sealed-unlock')).toBeVisible();
    await page.getByTestId('ban-recover-link').click();
    await expect(page).toHaveURL(/\/settings\?recover=1#sealed$/);
    await expect(page.getByTestId('sealed-recover')).toBeVisible();
  });

  test('the ban list form warns about one-word names and lists the GDPR hint', async ({ page }) => {
    await isolate(page);
    await setUpSealed(page);
    await page.getByTestId('sealed-section').getByRole('link', { name: 'OPEN THE BAN LIST →' }).click();
    await page.getByTestId('ban-add').click();
    const form = page.getByTestId('ban-form');
    await expect(form).toContainText('Use full name (first and last). One-word names match too many guests.');
    await expect(form.getByTestId('ban-gdpr-hint')).toHaveText('The person can ask what you hold about them. Write only what you\'d be comfortable showing them.');
    await form.getByTestId('ban-name').fill('Ole');
    await expect(form.getByTestId('ban-name-one-word')).toBeVisible();
    await form.getByTestId('ban-name').fill('Ole Petersen');
    await expect(form.getByTestId('ban-name-one-word')).toHaveCount(0);
    // Esc with something typed asks first; dismissing keeps the form.
    page.once('dialog', d => void d.dismiss());
    await form.getByTestId('ban-name').press('Escape');
    await expect(form).toBeVisible();
    page.once('dialog', d => void d.accept());
    await form.getByTestId('ban-name').press('Escape');
    await expect(form).toHaveCount(0);
  });

  test('a single sign-on account is told sealed data needs a local KlubHub account', async ({ page }) => {
    await isolate(page);
    await page.context().addCookies([{ name: 'kh_mock_sso', value: '1', domain: 'localhost', path: '/' }]);
    await page.goto('/settings#sealed');
    await hydrated(page);
    const section = page.getByTestId('sealed-section');
    await expect(section.getByTestId('sealed-local-required')).toContainText('Sealed data needs a local KlubHub account for now');
    await expect(section.getByTestId('sealed-create')).toHaveCount(0);
    await expect(section.getByRole('alert')).toHaveCount(0);
    await page.goto('/ban-list');
    await hydrated(page);
    await expect(page.getByTestId('ban-local-required')).toBeVisible();
  });
});
