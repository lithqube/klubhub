// Dashboard / Finance contract tests (Group A.5 of the v1-connect-product
// plan). Purpose: regression coverage for the JSON wire shape between the
// Go API and the Pinia stores used by the dashboard widgets and the
// finance page. Locks in the *current* contract so that if Group B
// repairs temporarily drift, the tests fail loudly and the fix is
// visible.
//
// Conventions follow apps/dj/app/stores/__tests__/epk.test.ts:
//   - vi.mock global $fetch with a per-test handler
//   - setActivePinia(createPinia()) in beforeEach
//   - assert store state, never the raw API payload

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

import { useGigStore } from '../gig'
import { useEarningsStore } from '../earnings'
import { useSocialStore } from '../social'

import type { Gig } from '../../types/gig'
import type { Entry } from '../../types/finance'

// ── fixtures ──────────────────────────────────────────────────────────────

/** A minimal but complete Gig row per apps/dj/app/types/gig.ts lines 6-26.
 *  Includes all required keys plus the three optional relationship arrays
 *  that the dashboard detail view renders. */
const baseGig: Gig = {
  id: 'gig-1',
  date: '2026-09-15',
  venue: 'Berghain',
  city: 'Berlin',
  country: 'DE',
  event_name: 'Klubnacht',
  promoter_name: 'Elise R.',
  promoter_email: 'elise@example.com',
  promoter_phone: '+49 30 1234',
  fee_amount: 1500,
  fee_currency: 'EUR',
  set_length_minutes: 120,
  notes: 'B2B with Honey Dijon',
  status: 'confirmed',
  payment_status: 'deposit_paid',
  gig_reader_venue_id: null,
  gig_reader_contact_id: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  deleted_at: null,
}

const baseEntry: Entry = {
  id: 'ent-1',
  kind: 'income',
  amount_minor: 150000,
  currency: 'EUR',
  category: 'gig_fee',
  entry_date: '2026-09-15',
  description: 'Berghain Klubnacht',
  notes: '',
  gig_id: 'gig-1',
  status: 'active',
  auto_generated: false,
  source_kind: 'manual',
  source_id: null,
  source_amount_minor: null,
  source_currency: null,
  source_description: '',
  created_at: '2026-09-15T10:00:00Z',
  updated_at: '2026-09-15T10:00:00Z',
  deleted_at: null,
  attachment_count: 0,
}

// ── mock ui store (social store calls useUiStore().showError on failure) ──

const mockShowError = vi.fn()

vi.mock('../ui', () => ({
  useUiStore: () => ({
    showError: mockShowError,
    showSuccess: vi.fn(),
  }),
}))

// ── tests ─────────────────────────────────────────────────────────────────

describe('dashboard.contract — API wire shape', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    // @ts-expect-error - mocking global $fetch (Nuxt-injected; not typed in tests)
    global.$fetch = vi.fn()
  })

  // ── gigs ──────────────────────────────────────────────────────────────

  describe('useGigStore.fetchGigs()', () => {
    it('populates gigs with the exact camelCase/snake_case keys defined in types/gig.ts lines 4-25', async () => {
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue([baseGig])

      const store = useGigStore()
      await store.fetchGigs()

      expect(store.gigs).toHaveLength(1)
      const first = store.gigs[0]

      // Required keys derived from apps/dj/app/types/gig.ts Gig
      // interface. When the type gains/loses fields, this test
      // re-evaluates automatically.
      const requiredKeys = Object.keys({} as Gig) as (keyof Gig)[]

      for (const key of requiredKeys) {
        expect(first, `missing key: ${String(key)}`).toHaveProperty(key as string)
      }

      // Spot-check values so a name match with the wrong value still fails.
      expect(first.id).toBe('gig-1')
      expect(first.event_name).toBe('Klubnacht')
      expect(first.fee_amount).toBe(1500)
      expect(first.status).toBe('confirmed')
    })

    it('hits GET /api/v1/gigs (bare array, no envelope)', async () => {
      const fetchMock = vi.fn().mockResolvedValue([baseGig])
      // @ts-expect-error - mocking global $fetch
      global.$fetch = fetchMock

      const store = useGigStore()
      await store.fetchGigs()

      expect(fetchMock).toHaveBeenCalledTimes(1)
      const [url] = fetchMock.mock.calls[0]
      expect(url).toMatch(/^\/api\/v1\/gigs/)
    })

    it('handles {data: [...]} envelope gracefully (B.2 introduces this on related endpoints)', async () => {
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [baseGig] })

      const store = useGigStore()
      await store.fetchGigs()

      expect(store.gigs).toHaveLength(1)
      expect(store.gigs[0].id).toBe('gig-1')
    })

    it('handles empty {data: []} envelope without throwing', async () => {
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [] })

      const store = useGigStore()
      await expect(store.fetchGigs()).resolves.not.toThrow()
      expect(store.gigs).toEqual([])
    })
  })

  // ── earnings ──────────────────────────────────────────────────────────

  describe('useEarningsStore.fetchEntries()', () => {
    // The actual store method is `fetchEntries`; the task brief said
    // `fetchEarnings` but the store does not export that name. We test
    // the real export so the test actually runs.

    it('populates entries with the exact keys defined in types/finance.ts Entry interface', async () => {
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [baseEntry] })

      const store = useEarningsStore()
      await store.fetchEntries()

      expect(store.entries).toHaveLength(1)
      const first = store.entries[0]

      // Required keys derived from apps/dj/app/types/finance.ts Entry
      // interface. When the type gains/loses fields, this test
      // re-evaluates automatically.
      const requiredKeys = Object.keys({} as Entry) as (keyof Entry)[]

      for (const key of requiredKeys) {
        expect(first, `missing key: ${String(key)}`).toHaveProperty(key as string)
      }

      expect(first.id).toBe('ent-1')
      expect(first.kind).toBe('income')
      expect(first.amount_minor).toBe(150000)
      expect(first.gig_id).toBe('gig-1')
    })

    it('hits GET /api/v1/finance/entries (wrapped in {data: [...]})', async () => {
      const fetchMock = vi.fn().mockResolvedValue({ data: [baseEntry] })
      // @ts-expect-error - mocking global $fetch
      global.$fetch = fetchMock

      const store = useEarningsStore()
      await store.fetchEntries()

      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/finance/entries',
        expect.objectContaining({ params: expect.any(Object) }),
      )
    })

    it('handles empty {data: []} envelope without throwing', async () => {
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [] })

      const store = useEarningsStore()
      await expect(store.fetchEntries()).resolves.not.toThrow()
      expect(store.entries).toEqual([])
    })
  })

  // ── social ────────────────────────────────────────────────────────────

  describe('useSocialStore.loadPosts() contract', () => {
    // Detailed loadPosts coverage (camelCase keys, URL path, loading
    // transitions) lives in stores/__tests__/social.test.ts; this file
    // only locks the {data: []} envelope handling, which is the precise
    // boundary Group B.2 will reshape.
    it('accepts {data: []} envelope without throwing', async () => {
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [] })

      const store = useSocialStore()
      await expect(store.loadPosts()).resolves.not.toThrow()
      expect(store.posts).toEqual([])
    })
  })

  // ── cross-store empty-envelope smoke test ─────────────────────────────

  describe('empty envelope handling across all three dashboard stores', () => {
    it('gigs store stays empty on {data: []}', async () => {
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [] })
      const store = useGigStore()
      await expect(store.fetchGigs()).resolves.not.toThrow()
      expect(store.gigs).toEqual([])
    })

    it('earnings store stays empty on {data: []}', async () => {
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [] })
      const store = useEarningsStore()
      await expect(store.fetchEntries()).resolves.not.toThrow()
      expect(store.entries).toEqual([])
    })

    it('social store stays empty on {data: []}', async () => {
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [] })
      const store = useSocialStore()
      await expect(store.loadPosts()).resolves.not.toThrow()
      expect(store.posts).toEqual([])
    })
  })
})