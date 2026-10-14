import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { computed, onMounted, ref } from 'vue'
import Dashboard from '../index.vue'
import { useGigStore } from '../../stores/gig'
import { useSocialStore } from '../../stores/social'
import { useTracklistStore } from '../../stores/tracklist'
import type { Gig } from '../../types/gig'

function gig(overrides: Partial<Gig> = {}): Gig {
  return { id: 'gig', date: '2026-10-02', venue: 'Real venue', city: 'Berlin', country: 'DE', event_name: '', promoter_name: '', promoter_email: '', promoter_phone: '', fee_amount: 400, fee_currency: 'EUR', set_length_minutes: 90, notes: '', status: 'confirmed', payment_status: 'unpaid', gig_reader_venue_id: null, gig_reader_contact_id: null, created_at: '', updated_at: null, deleted_at: null, ...overrides }
}

const responses = new Map<string, unknown>()
const fetchMock = vi.fn(async (url: string) => {
  const value = responses.get(url.split('?')[0]!)
  if (value instanceof Error) throw value
  return value ?? { data: [] }
})
let demo = false
function render() {
  return mount(Dashboard, { global: { stubs: { NuxtLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } } })
}

beforeEach(() => {
  setActivePinia(createPinia())
  responses.clear()
  fetchMock.mockClear()
  demo = false
  vi.stubGlobal('$fetch', fetchMock)
  vi.stubGlobal('useHead', vi.fn())
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { dashboardMockData: demo, demo } }))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-01T12:00:00Z'))
})
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })

describe('honest dashboard (mounted page with real Pinia stores)', () => {
  it('shows a fresh database without fixtures and unavailable earnings rather than invented zero', async () => {
    responses.set('/api/v1/finance/entries', new Error('finance disabled'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('NO GIGS YET')
    expect(wrapper.text()).toContain('QUEUE IS EMPTY')
    expect(wrapper.text()).toContain('NO TRACKLISTS YET')
    expect(wrapper.get('[aria-label="Financial overview"]').text()).toContain('EARNINGS UNAVAILABLE')
    expect(wrapper.text()).not.toMatch(/CYBER VOID|DEMO DATA|Invalid Date|NaN/)
    expect(wrapper.get('[aria-label="Financial overview"]').text()).not.toContain('0,00')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toEqual(expect.arrayContaining(['/gigs', '/social', '/tracklist', '/finance']))
    wrapper.unmount()
  })

  it('selects the earliest booked calendar day, not an inquiry or a UTC-midnight cutoff', async () => {
    responses.set('/api/v1/gigs', { data: [
      gig({ id: 'inquiry', status: 'inquiry', date: '2026-10-01', venue: 'Not booked' }),
      gig({ id: 'later', date: '2026-10-04', venue: 'Later venue' }),
      gig({ id: 'cancelled', status: 'cancelled', date: '2026-10-01' }),
      gig({ id: 'deleted', deleted_at: '2026-09-30', date: '2026-10-01' }),
      gig({ id: 'today', status: 'advanced', date: '2026-10-01', venue: 'Tonight venue', fee_amount: '750.50' as unknown as number, fee_currency: 'USD' }),
    ] })
    const wrapper = render()
    await flushPromises()
    const hero = wrapper.get('[aria-label="Upcoming gig"]')
    expect(hero.text()).toContain('Tonight venue')
    expect(hero.text()).toContain('01 OCT')
    expect(hero.text()).toContain('750,50')
    expect(hero.text()).not.toContain('00:00')
    expect(hero.findAll('a').map(a => a.attributes('href'))).toEqual(['/tracklist', '/social'])
    wrapper.unmount()
  })

  it('reacts to a real store create after the empty database load', async () => {
    const wrapper = render()
    await flushPromises()
    const created = gig({ venue: 'Created venue', date: '2026-10-05', fee_amount: 1234 })
    responses.set('/api/v1/gigs', { data: created })
    await useGigStore().createGig(created)
    await flushPromises()
    expect(wrapper.get('[aria-label="Upcoming gig"]').text()).toContain('Created venue')
    expect(wrapper.text()).toContain('05 OCT')
    expect(wrapper.text()).toContain('1.234,00')
    wrapper.unmount()
  })

  it('orders the newest three real tracklists and never invents unavailable track metadata', async () => {
    responses.set('/api/v1/tracklists', { data: [
      { id: 'old', title: 'Old upload', createdAt: '2026-01-01T00:00:00Z' },
      { id: 'new', title: 'Newest upload', createdAt: '2026-10-01T00:00:00Z' },
      { id: 'middle', title: 'Middle upload', createdAt: '2026-09-01T00:00:00Z' },
      { id: 'third', title: 'Third upload', createdAt: '2026-08-01T00:00:00Z' },
    ] })
    const wrapper = render()
    await flushPromises()
    const section = wrapper.get('[aria-label="Recent tracklists"]')
    expect(section.findAll('a').slice(1, 4).map(a => a.text())).toEqual([
      expect.stringContaining('Newest upload'), expect.stringContaining('Middle upload'), expect.stringContaining('Third upload'),
    ])
    expect(section.text()).not.toMatch(/Old upload|TRK|BPM|READY/)
    wrapper.unmount()
  })

  it.each([
    ['scheduled', 'badge-ready', 'accent-bar-ready'],
    ['publishing', 'badge-ready', 'accent-bar-ready'],
    ['published', 'badge-archived', 'accent-bar-archived'],
    ['draft', 'badge-draft', 'accent-bar-draft'],
    ['failed', 'badge-failed', 'accent-bar-failed'],
    ['permanently_failed', 'badge-failed', 'accent-bar-failed'],
  ])('renders actual social status %s with the correct styles', async (status, badge, accent) => {
    responses.set('/api/v1/social/posts', { data: [{ id: status, caption: 'Actual caption', status, postType: 'story', scheduledAtUtc: status === 'draft' ? '' : '2026-10-02T19:00:00Z', lastError: 'Real publish error' }] })
    const wrapper = render()
    await flushPromises()
    const post = wrapper.get('.spost')
    expect(post.classes()).toContain(accent)
    expect(post.get('.badge-hud').classes()).toContain(badge)
    expect(post.text()).toContain(status.toUpperCase())
    expect(post.text()).toContain('INSTAGRAM STORY')
    if (status.includes('failed')) expect(post.text()).toContain('Real publish error')
    if (status === 'draft') expect(post.text()).toContain('NOT SCHEDULED')
    expect(post.text()).not.toMatch(/Invalid Date|undefined/)
    wrapper.unmount()
  })

  it('shows explicit loading and endpoint failures, not empty states or stale records', async () => {
    for (const url of ['/api/v1/gigs', '/api/v1/social/posts', '/api/v1/tracklists']) responses.set(url, new Error('offline'))
    const wrapper = render()
    expect(wrapper.text()).toContain('LOADING GIGS')
    expect(wrapper.text()).not.toContain('NO GIGS YET')
    await flushPromises()
    expect(wrapper.text()).toContain('GIGS UNAVAILABLE')
    expect(wrapper.text()).toContain('SOCIAL QUEUE UNAVAILABLE')
    expect(wrapper.text()).toContain('TRACKLISTS UNAVAILABLE')
    expect(wrapper.text()).not.toMatch(/NO GIGS YET|QUEUE IS EMPTY|NO TRACKLISTS YET/)
    wrapper.unmount()
  })

  it('labels each mock-backed section and never labels a normal production dashboard as demo', async () => {
    demo = true
    responses.set('/api/v1/gigs', { data: [gig()] })
    const wrapper = render()
    await flushPromises()
    for (const label of ['Upcoming gig', 'Social queue', 'Recent tracklists', 'Financial overview']) {
      expect(wrapper.get(`[aria-label="${label}"]`).text()).toContain('DEMO DATA')
    }
    expect(wrapper.findAll('[data-demo-label]')).toHaveLength(4)
    wrapper.unmount()
  })

  it('uses active ledger income and issued outstanding invoice balances, separately by currency', async () => {
    responses.set('/api/v1/gigs', { data: [gig({ status: 'played', fee_amount: 999999 })] })
    const entry = { id: 'entry-0', kind: 'income', status: 'active', deleted_at: null, currency: 'EUR', amount_minor: 12345, entry_date: '2026-10-01' }
    responses.set('/api/v1/finance/entries', { data: [
      entry,
      { ...entry, id: 'entry-1', currency: 'USD', amount_minor: 5000 },
      { ...entry, id: 'entry-2', amount_minor: 10000, entry_date: '2026-09-01' },
      { ...entry, id: 'entry-3', status: 'voided', amount_minor: 999999 },
      { ...entry, id: 'entry-4', deleted_at: '2026-10-01', amount_minor: 999999 },
      { ...entry, id: 'entry-5', kind: 'expense', amount_minor: 999999 },
      { ...entry, id: 'entry-6', entry_date: '2025-01-01', amount_minor: 999999 },
      { ...entry, id: 'entry-7', entry_date: '2026-10-02', amount_minor: 999999 },
    ] })
    const invoice = { id: 'inv', kind: 'invoice', status: 'issued', replaced_by_invoice_id: null, currency: 'EUR', outstanding_minor: 2500 }
    responses.set('/api/v1/finance/invoices', { data: [
      invoice,
      { ...invoice, id: 'usd', currency: 'USD', outstanding_minor: 7000 },
      { ...invoice, id: 'draft', status: 'draft', outstanding_minor: 999999 },
      { ...invoice, id: 'cancelled', status: 'cancelled', outstanding_minor: 999999 },
      { ...invoice, id: 'credit', kind: 'credit_note', outstanding_minor: 999999 },
      { ...invoice, id: 'replaced', replaced_by_invoice_id: 'new', outstanding_minor: 999999 },
    ] })
    const wrapper = render()
    await flushPromises()
    const cards = wrapper.get('[aria-label="Financial overview"]').findAll('.glass')
    expect(cards[0]!.text()).toContain('123,45')
    expect(cards[0]!.text()).toContain('50,00')
    expect(cards[1]!.text()).toContain('25,00')
    expect(cards[1]!.text()).toContain('70,00')
    expect(cards[1]!.text()).toContain('Outstanding issued invoices')
    expect(cards[2]!.text()).toContain('223,45')
    expect(cards[2]!.text()).toContain('50,00')
    expect(cards.map(c => c.text()).join(' ')).not.toContain('999')
    wrapper.unmount()
  })

  it('distinguishes a successful empty ledger from unavailable pending balances', async () => {
    responses.set('/api/v1/finance/invoices', new Error('offline'))
    const wrapper = render()
    await flushPromises()
    const finance = wrapper.get('[aria-label="Financial overview"]')
    expect(finance.text()).toContain('0 — NO RECORDED INCOME')
    expect(finance.text()).toContain('PENDING UNAVAILABLE')
    expect(finance.text()).not.toContain('EARNINGS UNAVAILABLE')
    wrapper.unmount()
  })

  it('caps social posts at three without sorting or mutating the shared API result', async () => {
    responses.set('/api/v1/social/posts', { data: [1, 2, 3, 4].map(n => ({ id: String(n), caption: `Caption ${n}`, status: 'draft', postType: 'feed', scheduledAtUtc: '' })) })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.findAll('.spost').map(p => p.text())).toEqual([expect.stringContaining('Caption 1'), expect.stringContaining('Caption 2'), expect.stringContaining('Caption 3')])
    expect(wrapper.text()).not.toContain('Caption 4')
    wrapper.unmount()
  })

  it('does not apply date-only gig formatting to a scheduled social midnight instant', async () => {
    responses.set('/api/v1/social/posts', { data: [{ id: 'midnight', caption: 'Midnight post', status: 'scheduled', postType: 'feed', scheduledAtUtc: '2026-10-02T00:00:00Z' }] })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('.spost').text()).toMatch(/\d{2}:\d{2}/)
    wrapper.unmount()
  })

  it('renders a missing gig fee as unavailable rather than zero or NaN', async () => {
    responses.set('/api/v1/gigs', { data: [gig({ fee_amount: null as unknown as number })] })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[aria-label="Upcoming gig"]').text()).toContain('FEE UNAVAILABLE')
    expect(wrapper.text()).not.toContain('NaN')
    wrapper.unmount()
  })

  it('ignores a retained gigs-page filter without changing that filter', async () => {
    const store = useGigStore()
    store.filters.status = 'cancelled'
    responses.set('/api/v1/gigs', { data: [gig()] })
    const wrapper = render()
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/gigs')
    expect(store.filters.status).toBe('cancelled')
    expect(wrapper.get('[aria-label="Upcoming gig"]').text()).toContain('Real venue')
    wrapper.unmount()
  })

  it.each([true, false])('does not resurrect a deleted gig from an ABA dashboard read (unmounted=%s)', async (unmounted) => {
    let resolveRead!: (value: { data: Gig[] }) => void
    const read = new Promise<{ data: Gig[] }>(resolve => { resolveRead = resolve })
    fetchMock.mockImplementationOnce(() => read)
    const store = useGigStore()
    expect(store.gigs).toEqual([])
    const wrapper = render()
    if (unmounted) wrapper.unmount()
    const authoritative = gig({ venue: 'Authoritative GET' })
    responses.set('/api/v1/gigs/gig', { data: authoritative })
    expect(await store.fetchGig('gig')).toEqual(authoritative)
    expect(store.gigs).toEqual([authoritative])
    expect(await store.deleteGig('gig')).toBe(true)
    expect(store.gigs).toEqual([]) // back to the exact initial value: ABA
    resolveRead({ data: [gig({ venue: 'Obsolete dashboard' })] })
    await flushPromises()
    expect(store.gigs).toEqual([])
    expect(store.dashboardGigs).toEqual([])
    if (!unmounted) {
      expect(wrapper.text()).toContain('NO GIGS YET')
      expect(wrapper.text()).not.toContain('Obsolete dashboard')
      wrapper.unmount()
    }
  })

  it('invalidates only the unmounted dashboard read and ignores its late errors and list writes', async () => {
    let rejectRead!: (reason: unknown) => void
    let resolveSocial!: (value: unknown) => void
    const read = new Promise((_, reject) => { rejectRead = reject })
    const social = new Promise(resolve => { resolveSocial = resolve })
    fetchMock.mockImplementationOnce(() => read).mockImplementationOnce(() => social)
    const wrapper = render()
    const state = (wrapper.vm as any).$.setupState
    expect(state.loading).toBe(true)
    wrapper.unmount()
    const store = useGigStore()
    let resolveList!: (value: { data: Gig[] }) => void
    const list = new Promise<{ data: Gig[] }>(resolve => { resolveList = resolve })
    fetchMock.mockImplementationOnce(() => list)
    const unrelatedList = store.fetchGigs()
    expect(store.loading).toBe(true)
    rejectRead(new Error('old dashboard offline'))
    resolveSocial({ data: [{ id: 'obsolete', caption: 'Old page' }] })
    await flushPromises()
    expect(state.gigError).toBe(false)
    expect(state.loading).toBe(false)
    expect(store.loading).toBe(true)
    expect(useSocialStore().posts).toEqual([])
    resolveList({ data: [gig({ id: 'unrelated' })] })
    await unrelatedList
    expect(store.gigs.map(g => g.id)).toEqual(['unrelated'])
    expect(store.loading).toBe(false)
  })

  it('ignores a late tracklist success and social failure after dashboard teardown', async () => {
    let rejectSocial!: (reason: unknown) => void
    let resolveTracklists!: (value: unknown) => void
    const social = new Promise((_, reject) => { rejectSocial = reject })
    const tracklists = new Promise(resolve => { resolveTracklists = resolve })
    responses.set('/api/v1/social/posts', social)
    responses.set('/api/v1/tracklists', tracklists)
    const wrapper = render()
    const state = (wrapper.vm as any).$.setupState
    expect(state.loading).toBe(true)
    wrapper.unmount()
    rejectSocial(new Error('Disposed social failure'))
    resolveTracklists({ data: [{ id: 'obsolete-tracklist', title: 'Disposed upload' }] })
    await flushPromises()
    expect(useTracklistStore().pastTracklists).toEqual([])
    expect(useSocialStore().posts).toEqual([])
    expect(state.socialError).toBe(false)
    expect(state.tracklistError).toBe(false)
    expect(state.loading).toBe(false)
  })

  it('drops a successful post-unmount read without invalidating an unrelated pending PUT', async () => {
    let resolveRead!: (value: { data: Gig[] }) => void
    const read = new Promise<{ data: Gig[] }>(resolve => { resolveRead = resolve })
    fetchMock.mockImplementationOnce(() => read)
    const wrapper = render()
    const store = useGigStore()
    const existing = gig({ id: 'other', venue: 'Other gig' })
    responses.set('/api/v1/gigs/other', { data: existing })
    await store.fetchGig('other')
    let resolvePut!: (value: { data: Gig }) => void
    const put = new Promise<{ data: Gig }>(resolve => { resolvePut = resolve })
    fetchMock.mockImplementationOnce(() => put)
    const saving = store.updateGig('other', { notes: 'Saved independently', updated_at: null })
    wrapper.unmount()
    // Late success must not install new rows or interfere with the other save.
    resolveRead({ data: [gig({ venue: 'Obsolete page' }), existing] })
    await flushPromises()
    expect(store.dashboardGigs).toEqual([existing])
    expect(store.gigs).toEqual([existing])
    const saved = { ...existing, notes: 'Saved independently' }
    resolvePut({ data: saved })
    expect(await saving).toEqual(saved)
    expect(store.gigs).toEqual([saved])
    expect(store.dashboardGigs).toEqual([saved])
  })

  it('preserves a newer authoritative GET while installing unrelated dashboard rows', async () => {
    let resolveRead!: (value: { data: Gig[] }) => void
    const read = new Promise<{ data: Gig[] }>(resolve => { resolveRead = resolve })
    fetchMock.mockImplementationOnce(() => read)
    const wrapper = render()
    const store = useGigStore()
    const authoritative = gig({ venue: 'New authoritative venue', updated_at: '2026-10-01T12:00:00Z' })
    responses.set('/api/v1/gigs/gig', { data: authoritative })
    await store.fetchGig('gig')
    const unrelated = gig({ id: 'other', date: '2026-10-03' })
    resolveRead({ data: [gig({ venue: 'Stale venue' }), unrelated] })
    await flushPromises()
    expect(store.dashboardGigs).toEqual([authoritative, unrelated])
    expect(store.gigs).toEqual([authoritative])
    expect(wrapper.get('[aria-label="Upcoming gig"]').text()).toContain('New authoritative venue')
    wrapper.unmount()
  })

  it('keeps an unfiltered dashboard read separate from the gigs-page cache', async () => {
    const store = useGigStore()
    store.filters.status = 'cancelled'
    const filtered = gig({ id: 'cancelled', status: 'cancelled' })
    store.gigs = [filtered]
    responses.set('/api/v1/gigs', { data: [gig({ venue: 'Unfiltered upcoming' })] })
    const wrapper = render()
    await flushPromises()
    expect(store.filters.status).toBe('cancelled')
    expect(store.gigs).toEqual([filtered])
    expect(wrapper.get('[aria-label="Upcoming gig"]').text()).toContain('Unfiltered upcoming')
    wrapper.unmount()
  })

  it('does not overwrite a store create completing during the dashboard list read', async () => {
    let resolveRead!: (value: { data: Gig[] }) => void
    const read = new Promise<{ data: Gig[] }>(resolve => { resolveRead = resolve })
    fetchMock.mockImplementationOnce(() => read)
    const wrapper = render()
    const created = gig({ venue: 'Concurrent create' })
    responses.set('/api/v1/gigs', { data: created })
    await useGigStore().createGig(created)
    resolveRead({ data: [] })
    await flushPromises()
    expect(wrapper.get('[aria-label="Upcoming gig"]').text()).toContain('Concurrent create')
    wrapper.unmount()
  })
})
