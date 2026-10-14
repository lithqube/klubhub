import { test, expect } from '@playwright/test'
import {
  mockApi,
  makeGig,
  makePost,
  makeTracklist,
  makeEntry,
  makeInvoice,
} from './api-mocks'

test.describe('dashboard', () => {
  test('renders hero, social queue, tracklists and finance from the API', async ({
    page,
  }) => {
    // One upcoming confirmed gig drives the hero. Finance comes from the
    // real Phase 5 endpoints: an income entry dated today drives
    // earned-this-month and YTD, an issued invoice drives pending. Dating
    // the entry today keeps every figure deterministic whenever the test
    // runs.
    const upcoming = makeGig({
      id: '11111111-1111-4111-8111-111111111111',
      venue: 'Berghain',
      city: 'Berlin',
      country: 'DE',
      status: 'confirmed',
      fee_amount: 1500,
      set_length_minutes: 120,
    })
    await mockApi(page, {
      gigs: [upcoming],
      posts: [makePost({ caption: 'Set times dropping soon' })],
      tracklists: [makeTracklist({ title: 'Klubnacht 2026' })],
      entries: [makeEntry({ amount_minor: 80000 })],
      invoices: [makeInvoice({ outstanding_minor: 150000 })],
    })

    await page.goto('/')

    const hero = page.getByLabel('Upcoming gig')
    await expect(hero).toContainText('Berghain')
    await expect(hero).toContainText('Berlin, DE')
    await expect(hero).toContainText('CONFIRMED')
    await expect(hero).toContainText('1.500,00')
    await expect(hero).toContainText('120 MIN')

    const social = page.getByLabel('Social queue')
    await expect(social).toContainText('Set times dropping soon')
    await expect(social).toContainText('SCHEDULED')

    const tracklists = page.getByLabel('Recent tracklists')
    await expect(tracklists).toContainText('Klubnacht 2026')

    const finance = page.getByLabel('Financial overview')
    await expect(finance).toContainText('EARNED THIS MONTH')
    await expect(finance).toContainText('PENDING')
    await expect(finance).toContainText('YTD TOTAL')
    // Scope each amount to its metric card so a swapped total fails.
    const earned = finance.locator('.glass', { hasText: 'EARNED THIS MONTH' })
    const pending = finance.locator('.glass', { hasText: 'PENDING' })
    const ytd = finance.locator('.glass', { hasText: 'YTD TOTAL' })
    await expect(earned).toContainText('800,00')
    await expect(pending).toContainText('1.500,00')
    await expect(ytd).toContainText('800,00')
  })

  test('shows empty states when the API has no data', async ({ page }) => {
    // The Go API encodes an empty gig list as JSON `null`; the store maps
    // that to []. With no gigs at all the hero offers to create the first
    // one; social posts and tracklists come back as empty envelopes.
    await mockApi(page, { gigs: null, posts: [], tracklists: [] })

    await page.goto('/')

    await expect(page.getByText('NO GIGS YET')).toBeVisible()
    await expect(page.getByText('QUEUE IS EMPTY')).toBeVisible()
    await expect(page.getByText('NO TRACKLISTS YET')).toBeVisible()
  })

  test('distinguishes past gigs from a missing gig list', async ({ page }) => {
    // Gigs exist but none is upcoming: the app keeps today's gigs visible
    // all day, so the played gig is dated in the past.
    const played = makeGig({
      id: '55555555-5555-4555-8555-555555555555',
      date: '2020-01-01T20:00:00Z',
      venue: 'Tresor',
      city: 'Berlin',
      country: 'DE',
      status: 'played',
      fee_amount: 800,
    })
    await mockApi(page, { gigs: [played], posts: [], tracklists: [] })

    await page.goto('/')

    await expect(page.getByText('NO UPCOMING GIGS')).toBeVisible()
  })
})
