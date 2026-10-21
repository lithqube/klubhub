import { test, expect, type Page, type Route } from '@playwright/test'
import { mockApi } from './api-mocks'

// Outreach page smoke (04.10-04): with a stateful in-memory API — open
// /outreach → add a slot → see the matched contact → compose → send → the
// request appears in the sent log with its delivery status. The handler
// mirrors the dev mocks (apps/dj/server/api/v1/outreach): the page's own
// writes are reflected by the next reads. A unique city per run keeps the
// match assertions deterministic against the stateful handler.

interface OutreachState {
  slots: Record<string, unknown>[]
  requests: Record<string, unknown>[]
}

const DEFAULT_TEMPLATE = {
  id: 'tpl-e2e',
  name: 'Default booking request',
  direction: 'dj_to_promoter',
  subject: 'Booking request: {{dj_name}} in {{city}} on {{date}}',
  body: 'Hi {{promoter_name}},\n\nI\'m {{dj_name}} — free in {{city}} on {{date}}.\n\n{{dj_name}}',
  is_default: true,
  version: 1,
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-01T10:00:00Z',
}

/** Intercepts the outreach + settings + EPK-export routes and answers from
 * a per-test state; every other /api/v1 call falls through to the next
 * registered handler (mockApi). */
async function mockOutreachApi(page: Page, state: OutreachState): Promise<void> {
  await page.route('**/api/v1/**', (route: Route) => {
    const url = new URL(route.request().url())
    const path = url.pathname
    const method = route.request().method()
    const json = (body: unknown, status = 200) => route.fulfill({ status, json: body })

    if (path === '/api/v1/settings' && method === 'GET') {
      return json({
        dj_name: 'SAMPLE_RATE',
        contact_info: JSON.stringify({ name: 'SAMPLE_RATE', email: 'samplerate@dj.example', phone: '+49 30 555 0100' }),
      })
    }
    if (path === '/api/v1/epk/exports' && method === 'GET') {
      return json({ data: [{ id: 'epk-e2e', garageObjectKey: 'epk/exports/epk-e2e.pdf', createdAt: '2026-10-01T10:00:00Z' }] })
    }
    if (path === '/api/v1/outreach/availability/slots' && method === 'GET') {
      return json(state.slots)
    }
    if (path === '/api/v1/outreach/availability/slots' && method === 'POST') {
      const body = route.request().postDataJSON() as Record<string, unknown>
      const slot = {
        id: `as-${String(body.city).toLowerCase().replace(/\s+/g, '-')}`,
        created_at: new Date().toISOString(),
        fee_min: null, fee_max: null, currency: null, notes: '', ...body,
      }
      state.slots.push(slot)
      return json(slot, 201)
    }
    if (path === '/api/v1/outreach/availability/matches' && method === 'GET') {
      const slotId = url.searchParams.get('slot_id')
      const slots = slotId ? state.slots.filter((s) => s.id === slotId) : state.slots
      // One promoter contact per slot city (the matcher's city rule).
      return json(slots.map((slot) => ({
        slot,
        contacts: [{
          id: `pc-${slot.id}`,
          name: 'E2E Promoter',
          company: 'E2E Bookings',
          email: 'e2e@promoter.example',
          has_email: true,
          city: slot.city,
          past_gig_in_city: false,
        }],
        venues: [{ id: `v-${slot.id}`, name: 'E2E Venue', city: slot.city }],
      })))
    }
    if (path === '/api/v1/outreach/templates' && method === 'GET') {
      return json([DEFAULT_TEMPLATE])
    }
    if (path === '/api/v1/outreach/requests' && method === 'GET') {
      return json(state.requests)
    }
    if (path === '/api/v1/outreach/requests' && method === 'POST') {
      const body = route.request().postDataJSON() as Record<string, unknown>
      const now = new Date().toISOString()
      const row = {
        id: `or-${state.requests.length + 1}`,
        email_message_id: 'plunk-e2e-1',
        sent_at: now,
        created_at: now,
        status: 'sent',
        email_status: 'delivered',
        availability_slot_id: null, template_id: null, epk_export_id: null,
        ...body,
        to_email: 'e2e@promoter.example',
      }
      state.requests.unshift(row)
      return json(row, 201)
    }
    if (path === '/api/v1/outreach/suppressions' && method === 'GET') {
      return json([])
    }
    return route.fallback()
  })
}

function seedState(): OutreachState {
  return {
    slots: [{
      id: 'as-berlin',
      date: '2027-01-16',
      city: 'Berlin',
      country: 'DE',
      genres: ['techno'],
      fee_min: 600,
      fee_max: 900,
      currency: 'EUR',
      notes: '',
      created_at: '2026-10-01T10:00:00Z',
    }],
    requests: [],
  }
}

test.describe('outreach page', () => {
  test('slot → match → compose → send → sent log', async ({ page }) => {
    const state = seedState()
    await mockOutreachApi(page, state)
    // Unique per run: the stateful handler keys the match on the slot city.
    const city = `E2E City ${Date.now()}`
    const date = '2027-06-12'

    await page.goto('/outreach')
    await expect(page).toHaveTitle('Outreach — KlubHub DJ')
    await expect(page.locator('.page-title')).toHaveText('OUTREACH')

    // AVAILABILITY: the seeded slot renders; add a new one.
    await expect(page.locator('[data-testid="slot-row"]')).toHaveCount(1)
    await page.locator('[data-testid="slot-add-open"]').click()
    await page.locator('[data-testid="slot-date"]').fill(date)
    await page.locator('[data-testid="slot-city"]').fill(city)
    await page.locator('[data-testid="slot-form"]').locator('[data-testid="slot-save"]').click()
    await expect(page.locator('[data-testid="slot-row"]')).toHaveCount(2)
    await expect(page.locator('[data-testid="slot-row"]').last()).toContainText(city)

    // MATCHES: pick the new slot — its city-matched contact appears.
    await page.locator('[data-testid="tab-matches"]').click()
    await page.selectOption('[data-testid="match-slot-picker"]', { label: `${date} · ${city}` })
    const row = page.locator('[data-testid="match-row"]')
    await expect(row).toHaveCount(1)
    await expect(row).toContainText('E2E Promoter')
    await expect(page.locator('[data-testid="venue-chip"]')).toContainText('E2E Venue')

    // COMPOSE: the composer pre-fills from the default template.
    await row.locator('[data-testid="compose-btn"]').click()
    await expect(page.locator('[data-testid="composer"]')).toBeVisible()
    await expect(page.locator('[data-testid="composer-contact"]')).toContainText('E2E Promoter')
    await expect(page.locator('[data-testid="composer-preview"]')).toContainText(`Booking request: SAMPLE_RATE in ${city} on ${date}`)

    // SEND: the request is posted and the composer confirms.
    await page.locator('[data-testid="composer-send"]').click()
    await expect(page.locator('[data-testid="composer-notice"]')).toContainText('Request sent to E2E Promoter.')

    // SENT: the request lands in the log with its delivery status.
    await page.locator('[data-testid="tab-sent"]').click()
    const sent = page.locator('[data-testid="request-row"]')
    await expect(sent).toHaveCount(1)
    await expect(sent).toContainText('e2e@promoter.example')
    await expect(sent.locator('[data-testid="request-status"]')).toHaveText('DELIVERED')
  })

  test('navigates from the dashboard to outreach via the primary nav', async ({ page }) => {
    await mockApi(page, { gigs: [], posts: [], tracklists: [] })
    await mockOutreachApi(page, seedState())

    await page.goto('/')
    await page.getByRole('link', { name: 'OUTREACH' }).click()

    await expect(page).toHaveURL(/\/outreach$/)
    await expect(page.locator('.page-title')).toHaveText('OUTREACH')
    await expect(page.locator('[data-testid="outreach-tabs"]')).toBeVisible()
  })
})
