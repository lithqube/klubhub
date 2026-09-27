import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useReportStore } from '../report'

const fetchMock = vi.fn()
vi.stubGlobal('$fetch', fetchMock)

const report = (id: string) => ({ event: { id }, live: false, totals: {}, by_list: [], by_submitter: [], tickets_by_type: [], curve: [] })

describe('useReportStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('loads the report of an event', async () => {
    fetchMock.mockResolvedValue(report('e1'))
    const s = useReportStore()
    await s.load('e1')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/events/e1/report', {})
    expect(s.report?.event.id).toBe('e1')
    expect(s.loading).toBe(false)
    expect(s.error).toBeNull()
  })

  it('keeps the error for a retry and never shows another event’s numbers', async () => {
    fetchMock.mockResolvedValueOnce(report('e1'))
    const s = useReportStore()
    await s.load('e1')
    fetchMock.mockRejectedValueOnce({ statusCode: 503, data: { error: 'unavailable' } })
    await s.load('e2')
    expect(s.report).toBeNull()
    expect(s.error).toMatchObject({ error: 'unavailable', status: 503 })
    fetchMock.mockResolvedValueOnce(report('e2'))
    await s.load('e2')
    expect(s.error).toBeNull()
    expect(s.report?.event.id).toBe('e2')
  })

  it('keeps the last report while reloading the same event', async () => {
    fetchMock.mockResolvedValueOnce(report('e1'))
    const s = useReportStore()
    await s.load('e1')
    fetchMock.mockRejectedValueOnce({ statusCode: 500 })
    await s.load('e1')
    expect(s.report?.event.id).toBe('e1')
    expect(s.error?.error).toBe('network_error')
  })

  it('fetches one allocation’s list-back CSV as text', async () => {
    fetchMock.mockResolvedValue('name\n')
    const s = useReportStore()
    expect(await s.listBackCsv('e1', 'al-1')).toBe('name\n')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/events/e1/report/list-back.csv', { query: { allocation_id: 'al-1' }, responseType: 'text' })
    fetchMock.mockRejectedValue({ statusCode: 404, data: { error: 'not_found' } })
    await expect(s.listBackCsv('e1', 'nope')).rejects.toMatchObject({ error: 'not_found', status: 404 })
  })

  it('tracks every list-back in flight in a busy set', async () => {
    const s = useReportStore()
    let resolveA!: (v: string) => void
    let rejectB!: (e: unknown) => void
    fetchMock
      .mockReturnValueOnce(new Promise<string>((r) => { resolveA = r }))
      .mockReturnValueOnce(new Promise<string>((_, j) => { rejectB = j }))
    const a = s.listBackCsv('e1', 'al-a')
    const b = s.listBackCsv('e1', 'al-b')
    expect([...s.busy]).toEqual(['al-a', 'al-b'])
    resolveA('name\n')
    await a
    expect(s.busy.has('al-a')).toBe(false)
    expect(s.busy.has('al-b')).toBe(true)
    rejectB({ statusCode: 500 })
    await expect(b).rejects.toMatchObject({ error: 'network_error' })
    expect(s.busy.size).toBe(0)
  })
})
