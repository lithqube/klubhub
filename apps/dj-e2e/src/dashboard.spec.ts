import { test, expect } from '@playwright/test'
import { mockApi, makeGig, makePost, makeTracklist } from './api-mocks'

test.describe('dashboard', () => {
  test('renders hero, social queue, tracklists and finance from the API', async ({
    page,
  }) => {
    // One upcoming confirmed gig (hero + pending) and one played gig dated
    // today (earned-this-month + YTD), so every finance figure is
    // deterministic regardless of when the test runs.
    const upcoming = makeGig({
      id: '11111111-1111-4111-8111-111111111111',
      venue: 'Berghain',
      city: 'Berlin',
      country: 'DE',
      status: 'confirmed',
      fee_amount: 1500,
      set_length_minutes: 120,
    })
    const played = makeGig({
      id: '55555555-5555-4555-8555-555555555555',
      date: new Date().toISOString(),
      venue: 'Tresor',
      city: 'Berlin',
      country: 'DE',
      status: 'played',
      fee_amount: 800,
    })
    await mockApi(page, {
      gigs: [upcoming, played],
      posts: [makePost({ caption: 'Set times dropping soon' })],
      tracklists: [makeTracklist({ title: 'Klubnacht 2026' })],
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
    // Played gig (800) drives earned-this-month and YTD; the upcoming
    // confirmed gig (1500) drives pending.
    await expect(finance).toContainText('800,00')
    await expect(finance).toContainText('1.500,00')
  })

  test('shows empty states when the API has no data', async ({ page }) => {
    // The Go API encodes an empty gig list as JSON `null`; the store maps
    // that to []. Social posts and tracklists come back as empty envelopes.
    await mockApi(page, { gigs: null, posts: [], tracklists: [] })

    await page.goto('/')

    await expect(page.getByText('NO UPCOMING GIGS')).toBeVisible()
    await expect(page.getByText('QUEUE IS EMPTY')).toBeVisible()
    await expect(page.getByText('NO TRACKLISTS YET')).toBeVisible()
  })
})
