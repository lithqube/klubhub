import { expect, test, type Page } from '@playwright/test';

async function hydrated(page: Page) {
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as { __vue_app__?: unknown } | null)?.__vue_app__));
}

// e-klubnacht-02 is last week's mock night with seeded check-ins and door
// counters (one undone check-in and one undone walk-up that never count).
// No other spec writes to it.
async function openReport(page: Page) {
  await page.goto('/events/e-klubnacht-02/report');
  await hydrated(page);
  await expect(page.getByTestId('kpi-arrived')).toBeVisible();
}

test.describe('post-event report (mock API)', () => {
  test('the REPORT tab is reachable and shows the headline numbers', async ({ page }) => {
    await page.goto('/events/e-klubnacht-02/details');
    await hydrated(page);
    await page.getByRole('navigation', { name: 'Event sections' }).getByRole('link', { name: 'REPORT', exact: true }).click();
    await expect(page).toHaveURL(/\/events\/e-klubnacht-02\/report$/);
    await expect(page.getByTestId('kpi-arrived')).toContainText('6 / 8');
    await expect(page.getByTestId('kpi-no-show')).toContainText('25%');
    await expect(page.getByTestId('kpi-heads')).toContainText('11 / 16');
    await expect(page.getByTestId('kpi-heads')).toContainText('cap. 400');
    await expect(page.getByTestId('kpi-plus-ones')).toContainText('3 / 5');
    await expect(page.getByTestId('kpi-tickets')).toContainText('2 / 3');
    await expect(page.getByTestId('kpi-walkups')).toContainText('6');
    await expect(page.getByTestId('kpi-peak')).toContainText('17');
    // A past night is not live.
    await expect(page.getByTestId('report-live')).toHaveCount(0);
  });

  test('the check-in curve has an accessible data table and a keyboard readout', async ({ page }) => {
    await openReport(page);
    const chart = page.getByRole('img', { name: /Check-in curve in 15-minute steps/ });
    await expect(chart).toBeVisible();
    await expect(chart).toHaveAccessibleName(/peak of 17 inside/);
    const table = page.getByRole('table', { name: /Check-ins per 15 minutes/ });
    await expect(chart).toHaveAttribute('aria-describedby', /.+/);
    const describedBy = await chart.evaluate(el => el.getAttribute('aria-describedby'));
    await expect(page.locator(`[id="${describedBy}"]`).getByRole('table')).toHaveCount(1);
    // In the accessibility tree even before it is shown: header + 15 quarter hours.
    await expect(table.getByRole('row')).toHaveCount(16);

    await page.getByRole('button', { name: 'SHOW AS TABLE' }).click();
    await expect(page.getByRole('button', { name: 'HIDE TABLE' })).toHaveAttribute('aria-expanded', 'true');
    await expect(table.getByRole('row').last()).toBeVisible();
    await expect(table.getByRole('row').last()).toContainText('14'); // 14 inside after the last exits

    await chart.focus();
    await page.keyboard.press('Home');
    await expect(page.locator('[aria-live="polite"]').filter({ hasText: 'inside' })).toContainText('1 inside, 1 in, 0 walk-ups, 0 out');
  });

  test('allocations list artist submitters first and list back a CSV without contact details', async ({ page }) => {
    await openReport(page);
    const subs = page.getByTestId('report-submitters');
    const firstRows = subs.locator('tbody tr');
    await expect(firstRows.nth(0)).toContainText('Kaiser');
    await expect(firstRows.nth(1)).toContainText('Lena W');
    await expect(firstRows.nth(2)).toContainText('REVOKED');
    await expect(page.getByTestId('report-lists').locator('tbody tr').first()).toContainText('Artist guests');

    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.getByRole('button', { name: 'List back CSV for Kaiser' }).click(),
    ]);
    expect(download.suggestedFilename()).toBe('klubnacht-02-list-back-kaiser.csv');
    let csv = '';
    for await (const chunk of await download.createReadStream()) csv += chunk.toString();
    const [header, ...rows] = csv.replace(/^\uFEFF/, '').trim().split('\n');
    expect(header).toBe('name,plus_n,status,arrived,heads_admitted,first_in_local');
    expect(csv.toLowerCase()).not.toContain('email');
    expect(csv).not.toContain('@');
    expect(rows).toHaveLength(3);
    expect(rows.find(r => r.startsWith('Pia Lorenz'))).toMatch(/^Pia Lorenz,1,going,yes,2,\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/);
    expect(rows.find(r => r.startsWith('Yusuf Demir'))).toBe('Yusuf Demir,1,going,no,0,');
  });

  test('list-back refuses a missing id and another event\'s allocation', async ({ request }) => {
    expect((await request.get('/api/v1/events/e-klubnacht-02/report/list-back.csv')).status()).toBe(422);
    expect((await request.get('/api/v1/events/e-klubnacht-02/report/list-back.csv?allocation_id=al-ben')).status()).toBe(404);
  });

  test('tickets by type show scans of valid tickets', async ({ page }) => {
    await openReport(page);
    const tickets = page.getByTestId('report-tickets');
    // Scanned counts any status with a live check-in; valid only valid tickets.
    await expect(tickets.getByRole('row', { name: /Early bird/ }).getByRole('cell')).toHaveText(['Early bird', '1', '2']);
    await expect(tickets.getByRole('row', { name: /Regular/ }).getByRole('cell')).toHaveText(['Regular', '1', '1']);
  });

  test('an event without door activity shows the empty state', async ({ page }) => {
    await page.goto('/events/e-warehouse/report');
    await hydrated(page);
    await expect(page.getByText('NOTHING TO REPORT YET')).toBeVisible();
    await expect(page.getByTestId('kpi-arrived')).toHaveCount(0);
    await expect(page.getByTestId('report-live')).toHaveCount(0);
  });
});
