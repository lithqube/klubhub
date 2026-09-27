import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { usePrivacyStore } from '../privacy'

const fetchMock = vi.fn()
vi.stubGlobal('$fetch', fetchMock)

const overview = (days: number) => ({ retention_days: days, upcoming: [], recent: [] })
const privacy = (purged: string | null) => ({ purge_after: '2026-11-10T05:00:00Z', purged_at: purged, retention_days: 30, personal_rows: purged ? 0 : 9 })

describe('usePrivacyStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('loads the retention overview and keeps a load error for a retry', async () => {
    const s = usePrivacyStore()
    fetchMock.mockRejectedValueOnce({ statusCode: 503, data: { error: 'unavailable' } })
    await s.fetchRetention()
    expect(s.retention).toBeNull()
    expect(s.retentionError).toMatchObject({ error: 'unavailable', status: 503 })
    fetchMock.mockResolvedValueOnce(overview(30))
    await s.fetchRetention()
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/org/retention', {})
    expect(s.retention?.retention_days).toBe(30)
    expect(s.retentionError).toBeNull()
    expect(s.retentionLoading).toBe(false)
  })

  it('saves the retention with CSRF, reloads the overview and drops stale event dates', async () => {
    const s = usePrivacyStore()
    s.events = { e1: privacy(null) }
    fetchMock.mockResolvedValueOnce(null).mockResolvedValueOnce(overview(90))
    await s.saveRetention(90)
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/org/retention', expect.objectContaining({
      method: 'PUT', body: { retention_days: 90 }, headers: expect.objectContaining({ 'X-KlubHub-CSRF': '1' }),
    }))
    expect(s.retention?.retention_days).toBe(90)
    expect(s.events).toEqual({})
  })

  it('previews which ended events a shorter period erases', async () => {
    const s = usePrivacyStore()
    fetchMock.mockResolvedValueOnce({ would_purge: [{ event_id: 'e1', title: 'A', ends_at: '2026-10-11T05:00:00Z' }], count: 1 })
    expect(await s.previewRetention(7)).toEqual({ would_purge: [{ event_id: 'e1', title: 'A', ends_at: '2026-10-11T05:00:00Z' }], count: 1 })
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/org/retention/preview', { query: { days: 7 } })
    fetchMock.mockRejectedValueOnce({ statusCode: 422, data: { error: 'invalid', field: 'days' } })
    await expect(s.previewRetention(0)).rejects.toMatchObject({ error: 'invalid', field: 'days' })
  })

  it('confirms a shortening with the count it erases', async () => {
    const s = usePrivacyStore()
    fetchMock.mockResolvedValueOnce(null).mockResolvedValueOnce(overview(7))
    await s.saveRetention(7, 3)
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/org/retention', expect.objectContaining({
      method: 'PUT', body: { retention_days: 7, confirm_purge: 3 },
    }))
    fetchMock.mockRejectedValueOnce({ statusCode: 409, data: { error: 'retention_would_purge', count: 4, would_purge: [] } })
    await expect(s.saveRetention(7, 3)).rejects.toMatchObject({ error: 'retention_would_purge', status: 409, detail: { count: 4 } })
  })

  it('throws the refusal of a retention save', async () => {
    fetchMock.mockRejectedValueOnce({ statusCode: 422, data: { error: 'invalid', field: 'retention_days', problem: '1 to 365 days' } })
    await expect(usePrivacyStore().saveRetention(400)).rejects.toMatchObject({ error: 'invalid', field: 'retention_days', status: 422 })
  })

  it('loads an event’s purge state and keeps errors per event', async () => {
    const s = usePrivacyStore()
    fetchMock.mockResolvedValueOnce(privacy(null))
    await s.fetchEvent('e1')
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/events/e1/privacy', {})
    expect(s.events.e1?.personal_rows).toBe(9)
    fetchMock.mockRejectedValueOnce({ statusCode: 404, data: { error: 'not_found' } })
    expect(await s.fetchEvent('e2')).toBeNull()
    expect(s.eventErrors.e2).toMatchObject({ error: 'not_found' })
    expect(s.events.e1).toBeDefined()
  })

  it('erases an event with the typed title, then reloads its purge state', async () => {
    const s = usePrivacyStore()
    fetchMock.mockResolvedValueOnce({ purged_at: '2026-10-20T10:00:00Z', counts: { guests: 9 } }).mockResolvedValueOnce(privacy('2026-10-20T10:00:00Z'))
    const r = await s.purgeEvent('e1', 'Klubnacht 02')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/events/e1/purge', expect.objectContaining({ method: 'POST', body: { confirm: 'Klubnacht 02' } }))
    expect(r).toEqual({ purged_at: '2026-10-20T10:00:00Z', counts: { guests: 9 } })
    expect(s.events.e1?.purged_at).toBe('2026-10-20T10:00:00Z')
  })

  it('marks the event erased even when the reload after an erase fails', async () => {
    const s = usePrivacyStore()
    s.events = { e1: privacy(null) }
    fetchMock.mockResolvedValueOnce(null).mockRejectedValueOnce(new Error('offline'))
    const r = await s.purgeEvent('e1', 'Klubnacht 02')
    expect(r.purged_at).toBeTruthy()
    expect(s.events.e1?.purged_at).toBe(r.purged_at)
    expect(s.events.e1?.personal_rows).toBe(0)
  })

  it('throws a step-up refusal without touching the purge state', async () => {
    const s = usePrivacyStore()
    s.events = { e1: privacy(null) }
    fetchMock.mockRejectedValueOnce({ statusCode: 403, data: { error: 'reauthentication_required' } })
    await expect(s.purgeEvent('e1', 'Klubnacht 02')).rejects.toMatchObject({ error: 'reauthentication_required', status: 403 })
    expect(s.events.e1?.purged_at).toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
