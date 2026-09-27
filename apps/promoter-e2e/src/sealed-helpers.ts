import { readFile } from 'node:fs/promises';
import { expect, type Page } from '@playwright/test';

export async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

export const tag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;
export const CSRF = { 'X-KlubHub-CSRF': '1' };
export const PASS = 'purple tiger river moon';

/**
 * Each test gets its own sealed state in the mock (kh_mock_sealed cookie:
 * specs run in parallel and "set up once" is org-wide) and signs in with
 * an authenticator code, which the mock counts as a second factor for this
 * browser (security.manage needs MFA and a sign-in in the last 15 min).
 */
export async function isolate(page: Page) {
  await page.context().addCookies([{ name: 'kh_mock_sealed', value: `e2e-${tag()}`, domain: 'localhost', path: '/' }]);
  const r = await page.request.post('/api/v1/auth/login', { data: { email: 'owner@example.org', password: 'x', totp: '123456' } });
  expect(r.ok()).toBe(true);
}

/**
 * Settings → ENCRYPTION & BAN LIST: create the member key, then the setup
 * wizard: read the kit off the page, download it, re-type the two groups
 * asked for. Returns the 8 groups.
 */
export async function setUpSealed(page: Page): Promise<string[]> {
  await page.goto('/settings#sealed');
  await hydrated(page);
  const section = page.getByTestId('sealed-section');
  await expect(section.getByTestId('sealed-status')).toContainText('NOT SET UP');
  await expect(section.getByTestId('sealed-status')).toContainText('NO KEY YET');
  await section.getByTestId('sealed-new-passphrase').fill(PASS);
  await section.getByTestId('sealed-new-passphrase-again').fill(PASS);
  await section.getByRole('button', { name: 'CREATE MY KEY' }).click();
  await expect(section.getByTestId('sealed-notice')).toContainText('Your key is ready', { timeout: 30_000 });

  await section.getByTestId('sealed-setup-start').click();
  const kit = section.getByTestId('sealed-kit');
  await expect(kit.getByRole('listitem')).toHaveCount(8);
  const groups: string[] = [];
  for (let i = 1; i <= 8; i++) groups.push(((await section.getByTestId(`kit-group-${i}`).locator('.g').textContent()) ?? '').trim());
  for (const g of groups) expect(g).toMatch(/^[A-Z2-7]{7}$/);

  const continueBtn = section.getByTestId('sealed-kit-continue');
  await expect(continueBtn).toBeDisabled();
  const [download] = await Promise.all([page.waitForEvent('download'), section.getByTestId('sealed-kit-download').click()]);
  expect(download.suggestedFilename()).toBe('klubhub-recovery-kit-nachtwerk.txt');
  const text = await readFile(String(await download.path()), 'utf8');
  expect(text).toContain(`${groups.slice(0, 4).join(' ')}`);
  expect(text).toContain(`${groups.slice(4).join(' ')}`);
  await continueBtn.click();

  // Re-type the two groups the page asks for (from the "stored" kit).
  for (const k of [0, 1]) {
    const input = section.getByTestId(`sealed-check-${k}`);
    const n = Number(await input.getAttribute('data-group'));
    await input.fill((groups[n - 1] ?? '').toLowerCase());
  }
  await section.getByTestId('sealed-setup-finish').click();
  await expect(section.getByTestId('sealed-notice')).toContainText('Sealed data is set up and unlocked', { timeout: 30_000 });
  await expect(section.getByTestId('sealed-status')).toContainText('READY · KEY VERSION 1');
  await expect(section.getByTestId('sealed-status')).toContainText('UNLOCKED');
  return groups;
}

/** On /ban-list (unlocked): add one entry with a 6-month expiry. */
export async function addBan(page: Page, name: string, reason: string) {
  await page.getByTestId('ban-add').click();
  const form = page.getByTestId('ban-form');
  await form.getByTestId('ban-name').fill(name);
  await form.getByTestId('ban-reason').fill(reason);
  await form.getByText('6 MONTHS').click();
  await form.getByTestId('ban-save').click();
  await expect(page.getByTestId('ban-notice')).toContainText(`${name} added`);
  await expect(page.getByTestId('ban-table')).toContainText(name);
}

export const overflow = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
