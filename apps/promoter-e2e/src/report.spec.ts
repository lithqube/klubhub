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
    await expect(page.getByTestId('kpi-heads')).toContainText('LIST & TICKET HEADS');
    await expect(page.getByTestId('kpi-heads')).toContainText('11 / 16');
    await expect(page.getByTestId('kpi-heads')).toContainText('checked in of 16 expected');
    await expect(page.getByTestId('kpi-heads')).not.toContainText('400');
    await expect(page.getByTestId('kpi-plus-ones')).toContainText('3 / 5');
    await expect(page.getByTestId('kpi-tickets')).toContainText('2 / 3');
    await expect(page.getByTestId('kpi-walkups')).toContainText('6');
    await expect(page.getByTestId('kpi-peak')).toContainText('17');
    // The peak is a 15-minute span, not an instant.
    await expect(page.getByTestId('kpi-peak')).toContainText(/busiest 15 min: \d{2}:\d{2}–\d{2}:\d{2}/);
    // A past night is not live.
    await expect(page.getByTestId('report-live')).toHaveCount(0);
    await expect(page.getByTestId('report-heading')).toHaveText('POST-EVENT REPORT');
  });

  test('THROUGH THE DOOR leads and is never below the peak', async ({ page }) => {
    await openReport(page);
    const door = page.getByTestId('kpi-door');
    await expect(page.getByTestId('checkin-curve')).toBeVisible();
    await expect(door).toContainText('THROUGH THE DOOR');
    await expect(door).toContainText('of 400 capacity · peak 17 inside');
    const total = Number((await door.locator('dd').first().innerText()).replace(/\D/g, ''));
    // Walk-ups and manual ins are in it, so it covers everyone who was ever inside.
    expect(total).toBeGreaterThanOrEqual(17);
    // First tile in the grid.
    await expect(page.locator('[data-testid^="kpi-"]').first()).toHaveAttribute('data-testid', 'kpi-door');
    await page.getByText('HOW THESE ARE COUNTED').click();
    await expect(page.getByTestId('kpi-definitions')).toContainText('busiest 15 minutes');
  });

  test('the check-in curve has an accessible data table and a keyboard readout', async ({ page }) => {
    await openReport(page);
    const chart = page.getByRole('img', { name: /Check-in curve in 15-minute steps/ });
    await expect(chart).toBeVisible();
    await expect(chart).toHaveAccessibleName(/peak of 17 inside at \d{2}:\d{2}–\d{2}:\d{2}/);
    const table = page.getByRole('table', { name: /Check-ins per 15 minutes/ });
    // The whole table is not read out as the chart's description.
    await expect(chart).not.toHaveAttribute('aria-describedby', /.*/);
    // In the accessibility tree even before it is shown: header + 15 quarter hours.
    await expect(table.getByRole('row')).toHaveCount(16);

    await page.getByRole('button', { name: 'SHOW AS TABLE' }).click();
    await expect(page.getByRole('button', { name: 'HIDE TABLE' })).toHaveAttribute('aria-expanded', 'true');
    await expect(table.getByRole('row').last()).toBeVisible();
    await expect(table.getByRole('row').last()).toContainText('14'); // 14 inside after the last exits

    await expect(table.getByRole('columnheader')).toHaveText(['TIME', 'CHECKED IN', 'WALK-UPS', 'LEFT', 'INSIDE']);

    // Pointer moves show the readout but announce nothing.
    const live = page.getByTestId('curve-announce');
    const box = await chart.boundingBox();
    expect(box).not.toBeNull();
    await page.mouse.move((box?.x ?? 0) + (box?.width ?? 0) / 2, (box?.y ?? 0) + 60);
    await expect(page.getByTestId('curve-crosshair')).toHaveCount(1); // a 0-px-wide line never counts as visible
    await expect(live).toHaveText('');
    // The crosshair line and the dot share one x.
    const [lineX, dotX] = await Promise.all([
      page.getByTestId('curve-crosshair').getAttribute('x1'),
      page.getByTestId('curve-crosshair-dot').getAttribute('cx'),
    ]);
    expect(lineX).toBe(dotX);

    await chart.focus();
    await page.keyboard.press('Home');
    await expect(live).toContainText('1 inside, 1 checked in, 0 walk-ups, 0 left');
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
    const notice = page.getByTestId('report-notice');
    await expect(notice).toHaveAttribute('role', 'status');
    await expect(notice).toHaveText(/List back for Kaiser downloaded: 3 guests, \d+ arrived\. Send it to them\./);
    // A reload clears the stale notice.
    await page.getByRole('button', { name: 'REFRESH' }).click();
    await expect(notice).toHaveCount(0);
  });

  test('the header links to the list backs; an allocation without going guests cannot be listed back', async ({ page }) => {
    // Loaded client-side (tab navigation), so the report response can be shaped here.
    await page.route('**/api/v1/events/e-klubnacht-02/report', async (route) => {
      const res = await route.fetch();
      const body = await res.json();
      body.by_submitter = body.by_submitter.map((r: { allocation_id: string }) => (r.allocation_id === 'al-02-lena' ? { ...r, going: 0, arrived: 0 } : r));
      await route.fulfill({ response: res, json: body });
    });
    await page.goto('/events/e-klubnacht-02/details');
    await hydrated(page);
    await page.getByRole('navigation', { name: 'Event sections' }).getByRole('link', { name: 'REPORT', exact: true }).click();
    const lena = page.getByTestId('list-back-al-02-lena');
    await expect(lena).toHaveText(/NO GUESTS/);
    await expect(lena).toBeDisabled();
    await page.getByTestId('report-list-backs-link').click();
    await expect(page).toHaveURL(/#report-submitters$/);
    await expect(page.getByTestId('report-submitters')).toBeInViewport();
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
