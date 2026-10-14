import type { Page } from '@playwright/test'

// Shared API fixtures + router for the dj-e2e specs.
//
// The static build calls the Go API with same-origin relative URLs
// (/api/v1/...), so a page.route() glob over /api/v1/ intercepts every
// call before it reaches the static file server. Fixture shapes mirror
// the real API responses (see api/internal/...): social posts, tracklists
// and finance rows are { data: [...] } envelopes; gigs are served as a bare
// array (the store's unwrap accepts both that and the API's { data: [...] }
// envelope; JSON null when empty); social posts use snake_case keys because
// the store camelCases them.

export interface GigFixture {
  id: string
  date: string
  venue: string
  city: string
  country: string
  event_name: string
  promoter_name: string
  promoter_email: string
  promoter_phone: string
  fee_amount: number
  fee_currency: string
  set_length_minutes: number
  notes: string
  status: string
  payment_status: string
  gig_reader_venue_id: string | null
  gig_reader_contact_id: string | null
  created_at: string
  updated_at: string
  deleted_at: string | null
}

export function makeGig(overrides: Partial<GigFixture> = {}): GigFixture {
  const now = new Date()
  const future = new Date(now.getTime() + 30 * 24 * 3600 * 1000)
  return {
    id: '11111111-1111-4111-8111-111111111111',
    date: future.toISOString(),
    venue: 'Berghain',
    city: 'Berlin',
    country: 'DE',
    event_name: 'Klubnacht',
    promoter_name: 'Test Promoter',
    promoter_email: 'promoter@example.org',
    promoter_phone: '+49 30 000000',
    fee_amount: 1500,
    fee_currency: 'EUR',
    set_length_minutes: 120,
    notes: '',
    status: 'confirmed',
    payment_status: 'unpaid',
    gig_reader_venue_id: null,
    gig_reader_contact_id: null,
    created_at: now.toISOString(),
    updated_at: now.toISOString(),
    deleted_at: null,
    ...overrides,
  }
}

export interface PostFixture {
  id: string
  account_id: string
  status: string
  post_type: string
  caption: string
  image_minio_path: string
  scheduled_at_utc: string
  timezone_name: string
  retry_count: number
  next_retry_at: string | null
  last_error: string | null
  created_at: string
  updated_at: string
}

export function makePost(overrides: Partial<PostFixture> = {}): PostFixture {
  const now = new Date()
  const scheduled = new Date(now.getTime() + 7 * 24 * 3600 * 1000)
  return {
    id: '22222222-2222-4222-8222-222222222222',
    account_id: '33333333-3333-4333-8333-333333333333',
    status: 'scheduled',
    post_type: 'feed',
    caption: 'Set times dropping soon',
    image_minio_path: '',
    scheduled_at_utc: scheduled.toISOString(),
    timezone_name: 'Europe/Berlin',
    retry_count: 0,
    next_retry_at: null,
    last_error: null,
    created_at: now.toISOString(),
    updated_at: now.toISOString(),
    ...overrides,
  }
}

export interface TracklistFixture {
  id: string
  title: string
  sourceFormat: string
  rawFilePath: string
  preset: string
  bgMode: string
  bgValue: string
  maxTracks: number
  trackRangeStart: number
  trackRangeEnd: number
  createdAt: string
  updatedAt: string
}

export function makeTracklist(
  overrides: Partial<TracklistFixture> = {}
): TracklistFixture {
  const now = new Date().toISOString()
  return {
    id: '44444444-4444-4444-8444-444444444444',
    title: 'Klubnacht 2026',
    sourceFormat: 'rekordbox',
    rawFilePath:
      'raw-uploads/44444444-4444-4444-8444-444444444444/rekordbox_sample.txt',
    preset: 'default',
    bgMode: 'solid',
    bgValue: '#0a0a0a',
    maxTracks: 50,
    trackRangeStart: 1,
    trackRangeEnd: 50,
    createdAt: now,
    updatedAt: now,
    ...overrides,
  }
}

export interface EntryFixture {
  id: string
  kind: string
  amount_minor: number
  currency: string
  category: string
  entry_date: string
  description: string
  notes: string
  gig_id: string | null
  status: string
  auto_generated: boolean
  source_kind: string
  source_id: string | null
  source_amount_minor: number | null
  source_currency: string | null
  source_description: string
  created_at: string
  updated_at: string
  deleted_at: string | null
  attachment_count: number
}

export function makeEntry(overrides: Partial<EntryFixture> = {}): EntryFixture {
  const now = new Date()
  return {
    id: '66666666-6666-4666-8666-666666666666',
    kind: 'income',
    amount_minor: 80000,
    currency: 'EUR',
    category: 'gig_fee',
    // Dated today (UTC) so the dashboard's month/year filters always match,
    // regardless of when the test runs.
    entry_date: now.toISOString().slice(0, 10),
    description: 'Test gig fee',
    notes: '',
    gig_id: null,
    status: 'active',
    auto_generated: false,
    source_kind: 'manual',
    source_id: null,
    source_amount_minor: null,
    source_currency: null,
    source_description: '',
    created_at: now.toISOString(),
    updated_at: now.toISOString(),
    deleted_at: null,
    attachment_count: 0,
    ...overrides,
  }
}

// Only the fields the dashboard reads; the real Invoice has more
// (see apps/dj/app/types/finance.ts).
export interface InvoiceFixture {
  id: string
  kind: string
  gig_id: string
  replaced_by_invoice_id: string | null
  invoice_number: string | null
  currency: string
  status: string
  outstanding_minor: number
  created_at: string
  updated_at: string
}

export function makeInvoice(overrides: Partial<InvoiceFixture> = {}): InvoiceFixture {
  const now = new Date().toISOString()
  return {
    id: '77777777-7777-4777-8777-777777777777',
    kind: 'invoice',
    gig_id: '11111111-1111-4111-8111-111111111111',
    replaced_by_invoice_id: null,
    invoice_number: 'RE-2026-0001',
    currency: 'EUR',
    status: 'issued',
    outstanding_minor: 150000,
    created_at: now,
    updated_at: now,
    ...overrides,
  }
}

export interface MockApiOptions {
  /** Bare gig array, JSON `null` (the API's empty-list encoding), or a
   * function of the request URL for query-dependent responses. */
  gigs?: GigFixture[] | null | ((url: URL) => GigFixture[] | null)
  posts?: PostFixture[]
  tracklists?: TracklistFixture[]
  /** Earnings ledger rows (GET /api/v1/finance/entries). */
  entries?: EntryFixture[]
  /** Invoices (GET /api/v1/finance/invoices). */
  invoices?: InvoiceFixture[]
}

/** Intercepts every `/api/v1/**` call and answers from the given fixtures. */
export async function mockApi(page: Page, opts: MockApiOptions): Promise<void> {
  await page.route('**/api/v1/**', (route) => {
    const url = new URL(route.request().url())
    const path = url.pathname
    if (path === '/api/v1/gigs' && route.request().method() === 'GET') {
      // Default only an absent option to []; an explicit null is the API's
      // empty-list encoding and must reach the app unchanged.
      const body =
        typeof opts.gigs === 'function'
          ? opts.gigs(url)
          : opts.gigs === undefined
            ? []
            : opts.gigs
      return route.fulfill({ json: body })
    }
    if (path === '/api/v1/social/posts' && route.request().method() === 'GET') {
      return route.fulfill({ json: { data: opts.posts ?? [] } })
    }
    if (path === '/api/v1/tracklists' && route.request().method() === 'GET') {
      return route.fulfill({ json: { data: opts.tracklists ?? [] } })
    }
    if (path === '/api/v1/finance/entries' && route.request().method() === 'GET') {
      return route.fulfill({ json: { data: opts.entries ?? [] } })
    }
    if (path === '/api/v1/finance/invoices' && route.request().method() === 'GET') {
      return route.fulfill({ json: { data: opts.invoices ?? [] } })
    }
    return route.fulfill({ status: 404, json: { error: 'not mocked' } })
  })
}
