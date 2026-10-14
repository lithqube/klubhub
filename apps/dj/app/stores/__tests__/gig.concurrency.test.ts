import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useGigStore } from '../gig'
import { GigConflictError, type Gig } from '../../types/gig'

const t0 = '2026-09-01T00:00:00.123456Z'
const t1 = '2026-09-01T00:00:01.234567Z'
const t2 = '2026-09-01T00:00:02.345678Z'
const fixture = (updated_at: string | null = t0): Gig => ({
  id: 'gig-1', date: '2026-10-01', venue: 'Legacy venue', city: 'Berlin', country: 'DE',
  event_name: 'Event', promoter_name: 'Legacy promoter', promoter_email: '', promoter_phone: '',
  fee_amount: 500, fee_currency: 'EUR', set_length_minutes: 90, notes: '', status: 'confirmed',
  payment_status: 'unpaid', gig_reader_venue_id: null, gig_reader_contact_id: null,
  created_at: t0, updated_at, deleted_at: null, linkedVenues: [], linkedContacts: [], linkedTracklists: [],
})
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

describe('gig optimistic concurrency', () => {
  beforeEach(() => { setActivePinia(createPinia()); vi.spyOn(console, 'error').mockImplementation(() => undefined) })
  afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

  it.each(['envelope', 'bare'] as const)('stores and propagates exact returned snapshot timestamps with %s responses', async (shape) => {
    const wrap = (g: Gig) => shape === 'envelope' ? { data: g } : g
    const fetch = vi.fn().mockResolvedValueOnce(wrap({ ...fixture(t1), notes: 'first' }))
      .mockResolvedValueOnce(wrap({ ...fixture(t2), notes: 'next' }))
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.gigs = [fixture()]
    const first = await store.updateGig('gig-1', { notes: 'first', updated_at: t0 })
    expect(fetch).toHaveBeenNthCalledWith(1, '/api/v1/gigs/gig-1', { method: 'PUT', body: { notes: 'first', updated_at: t0 } })
    expect(first).toMatchObject({ notes: 'first', updated_at: t1 })
    expect(store.gigs[0]?.updated_at).toBe(t1)
    await store.updateGig('gig-1', { ...first!, notes: 'next' })
    expect(fetch.mock.calls[1]?.[1].body).toMatchObject({ notes: 'next', updated_at: t1 })
    expect(store.gigs[0]?.updated_at).toBe(t2)
  })

  it.each(['envelope', 'bare'] as const)('refreshes after 409, surfaces conflict, and never transfers stale fields to the fresh token (%s)', async (shape) => {
    const fresh = { ...fixture(t1), notes: 'other writer', linkedVenues: [{ id: 'venue-1', name: 'Fresh linked venue' }] }
    const fetch = vi.fn().mockRejectedValueOnce({ response: { status: 409 } })
      .mockResolvedValueOnce(shape === 'envelope' ? { data: fresh } : fresh)
      .mockRejectedValueOnce({ status: 409 })
      .mockResolvedValueOnce(shape === 'envelope' ? { data: fresh } : fresh)
      .mockResolvedValueOnce({ data: { ...fixture(t2), notes: 'rebased edit' } })
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.gigs = [fixture()]
    await expect(store.updateGig('gig-1', { notes: 'my edit', updated_at: t0 })).rejects.toMatchObject({ name: 'GigConflictError', latest: fresh })
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch).toHaveBeenNthCalledWith(2, '/api/v1/gigs/gig-1')
    expect(store.gigs[0]).toEqual(fresh)
    // Even another explicit call with the same stale snapshot still sends t0.
    await expect(store.updateGig('gig-1', { notes: 'my edit', updated_at: t0 })).rejects.toBeInstanceOf(GigConflictError)
    expect(fetch).toHaveBeenNthCalledWith(3, '/api/v1/gigs/gig-1', { method: 'PUT', body: { notes: 'my edit', updated_at: t0 } })
    // Only the caller can intentionally reload/rebase its fields and version.
    await store.updateGig('gig-1', { ...fresh, notes: 'rebased edit' })
    expect(fetch.mock.calls[4]?.[1].body).toMatchObject({ notes: 'rebased edit', updated_at: t1 })
  })

  it.each([null, undefined])('sends caller version %s as null despite a newer cache and surfaces server 409', async (token) => {
    const fetch = vi.fn().mockRejectedValueOnce({ statusCode: 409 }).mockResolvedValueOnce({ data: fixture(t1) })
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.gigs = [fixture(t1)]
    const input = token === undefined ? { notes: 'edit' } : { notes: 'edit', updated_at: token }
    await expect(store.updateGig('gig-1', input)).rejects.toBeInstanceOf(GigConflictError)
    expect(fetch).toHaveBeenNthCalledWith(1, '/api/v1/gigs/gig-1', { method: 'PUT', body: { notes: 'edit', updated_at: null } })
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(store.gigs[0]?.updated_at).toBe(t1)
  })

  it('keeps unknown returned timestamps null rather than fabricating a version', async () => {
    const fetch = vi.fn().mockResolvedValue({ data: fixture(null) })
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.gigs = [fixture(null)]
    await store.updateGig('gig-1', { notes: 'edit' })
    expect(fetch).toHaveBeenCalledWith('/api/v1/gigs/gig-1', { method: 'PUT', body: { notes: 'edit', updated_at: null } })
    expect(store.gigs[0]?.updated_at).toBeNull()
  })

  it('retains C2 relations when PUT returns only the base gig', async () => {
    const fetch = vi.fn().mockResolvedValue({ data: { id: 'gig-1', notes: 'saved', updated_at: t1 } })
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.gigs = [fixture()]
    await store.updateGig('gig-1', { notes: 'saved', updated_at: t0 })
    expect(store.gigs[0]).toMatchObject({ linkedVenues: [], linkedContacts: [], linkedTracklists: [], notes: 'saved', updated_at: t1 })
  })

  it('retains the known cache version and distinct conflict when refresh fails', async () => {
    const fetch = vi.fn().mockRejectedValueOnce({ statusCode: 409 }).mockRejectedValueOnce(new Error('offline'))
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.gigs = [fixture()]
    await expect(store.updateGig('gig-1', { notes: 'edit', updated_at: t0 })).rejects.toMatchObject({ name: 'GigConflictError', latest: null })
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(store.gigs[0]?.updated_at).toBe(t0)
  })

  it('does not GET after non-conflict failures', async () => {
    const fetch = vi.fn().mockRejectedValue({ statusCode: 500 })
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.gigs = [fixture()]
    expect(await store.updateGig('gig-1', { notes: 'edit' })).toBeNull()
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('remembers a returned snapshot when the gig was not in the list without authorizing omitted caller versions', async () => {
    const fetch = vi.fn().mockResolvedValue({ data: fixture(t1) })
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    const saved = await store.updateGig('gig-1', { notes: 'edit', updated_at: t0 })
    expect(saved?.updated_at).toBe(t1)
    expect(store.gigs[0]?.updated_at).toBe(t1)
    await store.updateGig('gig-1', { notes: 'next' })
    expect(fetch).toHaveBeenLastCalledWith('/api/v1/gigs/gig-1', { method: 'PUT', body: { notes: 'next', updated_at: null } })
  })

  it.each([
    ['GET', 'before'], ['GET', 'during'], ['PUT', 'before'], ['PUT', 'during'],
    ['list', 'before'], ['list', 'during'],
  ] as const)('does not resurrect confirmed deletion from delayed %s started %s DELETE', async (kind, timing) => {
    const read = deferred<{ data: Gig | Gig[] }>()
    const deletion = deferred<unknown>()
    const unrelated = { ...fixture(), id: 'gig-2' }
    const unrelatedSaved = { ...unrelated, notes: 'unrelated saved', updated_at: t1 }
    vi.stubGlobal('$fetch', vi.fn((url: string, options?: { method?: string }) => {
      if (options?.method === 'DELETE') return deletion.promise
      if (url === '/api/v1/gigs/gig-2') return Promise.resolve({ data: unrelatedSaved })
      return read.promise
    }))
    const store = useGigStore()
    store.gigs = [fixture(), unrelated]
    const startRead = () => kind === 'list' ? store.fetchGigs()
      : kind === 'GET' ? store.fetchGig('gig-1') : store.updateGig('gig-1', { notes: 'old edit', updated_at: t0 })
    const pendingRead = timing === 'before' ? startRead() : undefined
    const pendingDelete = store.deleteGig('gig-1')
    const duringRead = timing === 'during' ? startRead() : undefined
    // A DELETE must not invalidate other gig operations or discard list rows.
    await store.updateGig('gig-2', { notes: unrelatedSaved.notes, updated_at: t0 })
    deletion.resolve(null)
    expect(await pendingDelete).toBe(true)
    expect(store.gigs.map(g => g.id)).toEqual(['gig-2'])
    read.resolve({ data: kind === 'list' ? [fixture(), unrelated] : fixture(t1) })
    await (pendingRead ?? duringRead)
    expect(store.gigs).toEqual([unrelatedSaved])
  })

  it.each(['list-first', 'delete-failure-first'] as const)('keeps a failed-delete record recoverable across an overlapping list omission (%s)', async (order) => {
    const deletion = deferred<unknown>()
    const list = deferred<{ data: Gig[] }>()
    const unrelated = { ...fixture(), id: 'gig-2' }
    const refreshed = { ...fixture(t2), notes: 'Authoritative recovery' }
    const fetch = vi.fn().mockReturnValueOnce(deletion.promise).mockReturnValueOnce(list.promise)
      .mockResolvedValueOnce({ data: refreshed })
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.gigs = [fixture(), unrelated]
    const pendingDelete = store.deleteGig('gig-1')
    const pendingList = store.fetchGigs()
    const completeList = async () => {
      list.resolve({ data: [{ ...unrelated, city: 'Paris' }] })
      await pendingList
    }
    const failDelete = async () => {
      deletion.reject({ status: 500 })
      expect(await pendingDelete).toBe(false)
    }
    if (order === 'list-first') { await completeList(); await failDelete() }
    else { await failDelete(); await completeList() }
    expect(store.gigs.find(g => g.id === 'gig-1')).toEqual(fixture())
    expect(store.gigs.find(g => g.id === 'gig-2')?.city).toBe('Paris')
    expect(await store.fetchGig('gig-1')).toEqual(refreshed)
    expect(store.gigs.find(g => g.id === 'gig-1')).toEqual(refreshed)
  })

  it.each(['GET', 'list'] as const)('permits an explicit authoritative %s started after deletion confirmation', async (kind) => {
    const recovered = { ...fixture(t2), notes: 'Later authoritative response' }
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ data: kind === 'GET' ? recovered : [recovered] }))
    const store = useGigStore()
    store.gigs = [fixture()]
    expect(await store.deleteGig('gig-1')).toBe(true)
    expect(store.gigs).toEqual([])
    if (kind === 'GET') await store.fetchGig('gig-1')
    else await store.fetchGigs()
    expect(store.gigs).toEqual([recovered])
  })

  it.each(['filtered-first', 'dashboard-first'] as const)('reconciles shared records without mixing list membership (%s)', async (order) => {
    const filtered = deferred<{ data: Gig[] }>()
    const dashboard = deferred<{ data: Gig[] }>()
    const fetch = vi.fn().mockReturnValueOnce(filtered.promise).mockReturnValueOnce(dashboard.promise)
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.setFilter('status', 'confirmed')
    const pendingFiltered = store.fetchGigs()
    const pendingDashboard = store.fetchDashboardGigs(() => true)
    const filteredG = { ...fixture(t1), venue: 'Filtered snapshot', notes: 'filtered fields' }
    const dashboardG = { ...fixture(t0), venue: 'Dashboard snapshot', notes: 'dashboard fields' }
    const cancelled = { ...fixture(), id: 'cancelled-only', status: 'cancelled' as const }
    const completeFiltered = async () => { filtered.resolve({ data: [filteredG] }); await pendingFiltered }
    const completeDashboard = async () => { dashboard.resolve({ data: [dashboardG, cancelled] }); await pendingDashboard }
    if (order === 'filtered-first') { await completeFiltered(); await completeDashboard() }
    else { await completeDashboard(); await completeFiltered() }
    // Generations, not timestamp comparison: the first accepted concurrent
    // installation owns this record until a later authoritative operation.
    const accepted = order === 'filtered-first' ? filteredG : dashboardG
    expect(store.gigs).toEqual([accepted])
    expect(store.dashboardGigs).toEqual([accepted, cancelled])
    expect(store.filters.status).toBe('confirmed')
    expect(fetch.mock.calls.map(([url]) => url)).toEqual(['/api/v1/gigs?status=confirmed', '/api/v1/gigs'])
  })

  it.each(['filtered-first', 'dashboard-first'] as const)('does not treat a cross-view list omission as deletion or preserve excluded membership (%s)', async (order) => {
    const filtered = deferred<{ data: Gig[] }>()
    const dashboard = deferred<{ data: Gig[] }>()
    vi.stubGlobal('$fetch', vi.fn().mockReturnValueOnce(filtered.promise).mockReturnValueOnce(dashboard.promise))
    const store = useGigStore()
    const excluded = { ...fixture(), status: 'cancelled' as const }
    store.gigs = [excluded]
    store.dashboardGigs = [excluded]
    store.setFilter('status', 'confirmed')
    const pendingFiltered = store.fetchGigs()
    const pendingDashboard = store.fetchDashboardGigs(() => true)
    const completeFiltered = async () => { filtered.resolve({ data: [] }); await pendingFiltered }
    const completeDashboard = async () => { dashboard.resolve({ data: [excluded] }); await pendingDashboard }
    if (order === 'filtered-first') { await completeFiltered(); await completeDashboard() }
    else { await completeDashboard(); await completeFiltered() }
    expect(store.gigs).toEqual([])
    expect(store.dashboardGigs).toEqual([excluded])
  })

  it.each([
    ['filtered-first', 'GET'], ['dashboard-first', 'GET'],
    ['filtered-first', 'PUT'], ['dashboard-first', 'PUT'],
    ['filtered-first', 'DELETE'], ['dashboard-first', 'DELETE'],
  ] as const)('keeps later %s cross-view completion behind authoritative %s', async (order, operation) => {
    const filtered = deferred<{ data: Gig[] }>()
    const dashboard = deferred<{ data: Gig[] }>()
    const authoritative = { ...fixture(t2), notes: 'Authoritative operation', venue: 'Newest venue' }
    const unrelated = { ...fixture(t1), id: 'unrelated', notes: 'Other list member' }
    vi.stubGlobal('$fetch', vi.fn().mockReturnValueOnce(filtered.promise).mockReturnValueOnce(dashboard.promise)
      .mockResolvedValueOnce(operation === 'DELETE' ? null : { data: authoritative }))
    const store = useGigStore()
    store.setFilter('status', 'confirmed')
    const pendingFiltered = store.fetchGigs()
    const pendingDashboard = store.fetchDashboardGigs(() => true)
    const completeFiltered = async () => { filtered.resolve({ data: [fixture()] }); await pendingFiltered }
    const completeDashboard = async () => { dashboard.resolve({ data: [fixture(t1), unrelated] }); await pendingDashboard }
    if (order === 'filtered-first') await completeFiltered()
    else await completeDashboard()
    if (operation === 'DELETE') expect(await store.deleteGig('gig-1')).toBe(true)
    else if (operation === 'GET') expect(await store.fetchGig('gig-1')).toEqual(authoritative)
    else expect(await store.updateGig('gig-1', { notes: authoritative.notes, updated_at: t0 })).toEqual(authoritative)
    if (order === 'filtered-first') await completeDashboard()
    else await completeFiltered()
    expect(store.gigs).toEqual(operation === 'DELETE' ? [] : [authoritative])
    expect(store.dashboardGigs).toEqual(operation === 'DELETE' ? [unrelated] : [authoritative, unrelated])
  })

  it('does not publish a disposed dashboard snapshot into the active filtered read', async () => {
    const filtered = deferred<{ data: Gig[] }>()
    const dashboard = deferred<{ data: Gig[] }>()
    vi.stubGlobal('$fetch', vi.fn().mockReturnValueOnce(filtered.promise).mockReturnValueOnce(dashboard.promise))
    const store = useGigStore()
    let active = true
    const pendingFiltered = store.fetchGigs()
    const pendingDashboard = store.fetchDashboardGigs(() => active)
    active = false
    dashboard.resolve({ data: [{ ...fixture(t2), notes: 'Disposed snapshot' }] })
    await pendingDashboard
    const accepted = { ...fixture(t1), notes: 'Active snapshot' }
    filtered.resolve({ data: [accepted] })
    await pendingFiltered
    expect(store.gigs).toEqual([accepted])
    expect(store.dashboardGigs).toEqual([])
  })

  it('preserves a PUT completed while an older list response was pending', async () => {
    const put = deferred<{ data: Gig }>()
    const list = deferred<{ data: Gig[] }>()
    const saved = { ...fixture(t1), notes: 'saved while loading' }
    vi.stubGlobal('$fetch', vi.fn().mockReturnValueOnce(put.promise).mockReturnValueOnce(list.promise))
    const store = useGigStore()
    store.gigs = [fixture()]
    const pendingPut = store.updateGig('gig-1', { notes: saved.notes, updated_at: t0 })
    const pendingList = store.fetchGigs()
    put.resolve({ data: saved })
    expect(await pendingPut).toEqual(saved)
    list.resolve({ data: [fixture()] })
    await pendingList
    expect(store.gigs[0]).toEqual(saved)
  })

  it('does not replace a newer conflict GET with an older pending list response', async () => {
    const list = deferred<{ data: Gig[] }>()
    const authoritative = { ...fixture(t2), notes: 'latest notes' }
    const unrelated = { ...fixture(), id: 'gig-2' }
    vi.stubGlobal('$fetch', vi.fn().mockReturnValueOnce(list.promise)
      .mockRejectedValueOnce({ status: 409 }).mockResolvedValueOnce({ data: authoritative }))
    const store = useGigStore()
    store.gigs = [fixture(), unrelated]
    const pendingList = store.fetchGigs()
    await expect(store.updateGig('gig-1', { notes: 'stale', updated_at: t0 })).rejects.toBeInstanceOf(GigConflictError)
    list.resolve({ data: [fixture(t1), { ...unrelated, city: 'Paris' }] })
    await pendingList
    expect(store.gigs.find(g => g.id === 'gig-1')).toEqual(authoritative)
    expect(store.gigs.find(g => g.id === 'gig-2')?.city).toBe('Paris')
  })

  it('does not install an earlier PUT after an authoritative list refresh', async () => {
    const pending = deferred<{ data: Gig }>()
    const authoritative = { ...fixture(t2), notes: 'list latest' }
    vi.stubGlobal('$fetch', vi.fn().mockReturnValueOnce(pending.promise)
      .mockResolvedValueOnce({ data: [authoritative] }))
    const store = useGigStore()
    store.gigs = [fixture()]
    const earlier = store.updateGig('gig-1', { notes: 'earlier', updated_at: t0 })
    await store.fetchGigs()
    pending.resolve({ data: { ...fixture(t1), notes: 'earlier' } })
    expect(await earlier).toBeNull()
    expect(store.gigs[0]).toEqual(authoritative)
  })

  it('keeps a pending unrelated gig PUT valid across another gig conflict GET', async () => {
    const pending = deferred<{ data: Gig }>()
    const unrelated = { ...fixture(), id: 'gig-2', notes: 'unrelated' }
    const saved = { ...unrelated, notes: 'unrelated saved', updated_at: t1 }
    vi.stubGlobal('$fetch', vi.fn().mockReturnValueOnce(pending.promise)
      .mockRejectedValueOnce({ status: 409 }).mockResolvedValueOnce({ data: fixture(t2) }))
    const store = useGigStore()
    store.gigs = [fixture(), unrelated]
    const unrelatedPut = store.updateGig('gig-2', { notes: saved.notes, updated_at: t0 })
    await expect(store.updateGig('gig-1', { notes: 'stale', updated_at: t0 })).rejects.toBeInstanceOf(GigConflictError)
    pending.resolve({ data: saved })
    expect(await unrelatedPut).toEqual(saved)
    expect(store.gigs.find(g => g.id === 'gig-2')).toEqual(saved)
    expect(store.gigs.find(g => g.id === 'gig-1')?.updated_at).toBe(t2)
  })

  it('does not install a delayed earlier PUT after a newer conflict GET or drop unrelated gigs', async () => {
    const pending = deferred<{ data: Gig }>()
    const authoritative = { ...fixture(t2), notes: 'newer writer', linkedContacts: [] }
    const unrelated = { ...fixture(t1), id: 'gig-2', notes: 'unrelated' }
    const fetch = vi.fn().mockReturnValueOnce(pending.promise)
      .mockRejectedValueOnce({ status: 409 }).mockResolvedValueOnce({ data: authoritative })
      .mockResolvedValueOnce({ data: { ...unrelated, notes: 'unrelated saved' } })
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    store.gigs = [fixture(), unrelated]
    const earlier = store.updateGig('gig-1', { notes: 'earlier', updated_at: t0 })
    await expect(store.updateGig('gig-1', { notes: 'stale', updated_at: t0 })).rejects.toBeInstanceOf(GigConflictError)
    await store.updateGig('gig-2', { notes: 'unrelated saved', updated_at: t1 })
    expect(store.gigs.find(g => g.id === 'gig-1')).toEqual(authoritative)
    pending.resolve({ data: { ...fixture(t1), notes: 'earlier' } })
    await earlier
    expect(store.gigs.find(g => g.id === 'gig-1')).toEqual(authoritative)
    expect(store.gigs.find(g => g.id === 'gig-2')?.notes).toBe('unrelated saved')
    expect(store.gigs).toHaveLength(2)
  })
})
