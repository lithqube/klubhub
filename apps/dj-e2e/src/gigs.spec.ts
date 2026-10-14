import { test, expect } from '@playwright/test'
import { mockApi, makeGig } from './api-mocks'

test.describe('gigs page', () => {
  test('lists the gigs returned by the API', async ({ page }) => {
    await mockApi(page, {
      gigs: [
        makeGig({
          id: '11111111-1111-4111-8111-111111111111',
          venue: 'Berghain',
          city: 'Berlin',
          country: 'DE',
          status: 'confirmed',
          fee_amount: 1500,
        }),
        makeGig({
          id: '55555555-5555-4555-8555-555555555555',
          venue: 'Tresor',
          city: 'Leipzig',
          country: 'DE',
          status: 'inquiry',
          fee_amount: 800,
        }),
      ],
    })

    await page.goto('/gigs')

    await expect(page).toHaveTitle('Gigs — KlubHub DJ')
    const rows = page.locator('.gig-list-row')
    await expect(rows).toHaveCount(2)
    await expect(rows.nth(0)).toContainText('Berghain')
    await expect(rows.nth(0)).toContainText('Berlin, DE')
    // GigListRow formats fees en-US with no fraction digits (€1,500),
    // unlike the dashboard's de-DE formatCurrency (1.500,00 €).
    await expect(rows.nth(0)).toContainText('€1,500')
    await expect(rows.nth(0)).toContainText('CONFIRMED')
    await expect(rows.nth(1)).toContainText('Tresor')
    await expect(rows.nth(1)).toContainText('Leipzig, DE')
    await expect(rows.nth(1)).toContainText('INQUIRY')
  })

  test('refetches with the status filter query and updates the list', async ({
    page,
  }) => {
    const confirmed = makeGig({
      id: '11111111-1111-4111-8111-111111111111',
      venue: 'Berghain',
      status: 'confirmed',
    })
    const inquiry = makeGig({
      id: '55555555-5555-4555-8555-555555555555',
      venue: 'Tresor',
      status: 'inquiry',
    })
    const requested: string[] = []
    await page.route('**/api/v1/gigs**', (route) => {
      const url = new URL(route.request().url())
      requested.push(url.pathname + url.search)
      const status = url.searchParams.get('status')
      const all = [confirmed, inquiry]
      return route.fulfill({
        json: status ? all.filter((g) => g.status === status) : all,
      })
    })

    await page.goto('/gigs')
    await expect(page.locator('.gig-list-row')).toHaveCount(2)

    await page.selectOption('select.hud-input', 'confirmed')

    await expect(page.locator('.gig-list-row')).toHaveCount(1)
    await expect(page.locator('.gig-list-row').first()).toContainText('Berghain')
    expect(requested).toContain('/api/v1/gigs')
    expect(requested).toContain('/api/v1/gigs?status=confirmed')
  })

  test('shows the empty state when the API returns no gigs', async ({ page }) => {
    await mockApi(page, { gigs: null })

    await page.goto('/gigs')

    await expect(page.getByText('NO GIGS FOUND')).toBeVisible()
  })

  test('navigates from the dashboard to gigs via the primary nav', async ({
    page,
  }) => {
    await mockApi(page, { gigs: [], posts: [], tracklists: [] })

    await page.goto('/')
    await page.getByRole('link', { name: 'GIGS' }).click()

    await expect(page).toHaveURL(/\/gigs$/)
    await expect(page.locator('.page-title')).toHaveText('GIGS')
  })
})
