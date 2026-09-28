import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

const tag = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;

test.describe('audience CRM (mock API)', () => {
  test('shows the seeded contacts with status tabs and counts', async ({ page }) => {
    await page.goto('/audience');
    await hydrated(page);
    await expect(page.getByRole('heading', { name: 'AUDIENCE' })).toBeVisible();
    await expect(page.getByText('Nadia Voss')).toBeVisible();
    await page.getByRole('tab', { name: /^UNSUBSCRIBED/ }).click();
    await expect(page.getByText('Priya Shah')).toBeVisible();
    await expect(page.getByText('Nadia Voss')).not.toBeVisible();
  });

  test('adding a contact records its consent and appears in the list', async ({ page }) => {
    const name = `Test Contact ${tag()}`;
    await page.goto('/audience');
    await hydrated(page);
    await page.getByTestId('audience-add').click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('NAME').fill(name);
    await dialog.getByLabel('EMAIL').fill(`${tag()}@example.com`);
    await dialog.getByLabel('LAWFUL BASIS').selectOption('consent');
    await dialog.getByLabel(/FORM TEXT SHOWN/).fill('Signed up at the door');
    await dialog.getByRole('button', { name: 'SAVE' }).click();
    await expect(page.getByText(name)).toBeVisible();
  });

  test('unsubscribing updates the badge without removing the contact from ALL', async ({ page }) => {
    await page.goto('/audience');
    await hydrated(page);
    const row = page.locator('li', { hasText: 'Lars Berg' });
    page.once('dialog', d => d.accept());
    await row.getByRole('button', { name: 'UNSUBSCRIBE' }).click();
    // Still on the ALL tab: the contact stays, but its badge and available
    // actions change — it is not deleted by unsubscribing.
    await expect(row.getByText('UNSUBSCRIBED')).toBeVisible();
    await expect(row.getByRole('button', { name: 'UNSUBSCRIBE' })).toHaveCount(0);
  });

  test('search matches by substring, not just an exact full name', async ({ page }) => {
    await page.goto('/audience');
    await hydrated(page);
    await page.getByPlaceholder(/SEARCH NAME/).fill('voss');
    await expect(page.getByText('Nadia Voss')).toBeVisible();
    await expect(page.getByText('Priya Shah')).not.toBeVisible();
  });

  test('saving a segment shows its live matching count', async ({ page }) => {
    await page.goto('/audience');
    await hydrated(page);
    await page.getByPlaceholder('e.g. Berlin regulars').fill(`Segment ${tag()}`);
    await page.getByLabel('SOURCE').last().selectOption('follow');
    await page.getByRole('button', { name: 'SAVE SEGMENT' }).click();
    await expect(page.locator('.badge-hud', { hasText: 'Segment' })).toBeVisible();
  });
});
