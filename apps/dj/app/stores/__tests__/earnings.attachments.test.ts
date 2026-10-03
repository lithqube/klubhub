// Receipts in the earnings store: upload / list / delete against the API
// contract (multipart `file` part, error codes), the row count they keep in
// step, and the `receipt` list filter.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { entryAttachmentUrl, useEarningsStore } from '../earnings'
import { FinanceApiError } from '../invoice'
import { deferred, httpError, makeAttachment, makeEntry, makeFile } from '../../components/finance/__tests__/fixtures'

const fetchMock = vi.fn()

beforeEach(() => {
  setActivePinia(createPinia())
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
})

describe('uploadAttachment', () => {
  it('POSTs multipart with a single `file` part and no hand-set Content-Type', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry()]
    const created = makeAttachment({ filename: 'cable.jpg', mime_type: 'image/jpeg' })
    fetchMock.mockResolvedValueOnce({ data: created })

    const result = await store.uploadAttachment('e-1', makeFile('cable.jpg', 'image/jpeg'))

    expect(result).toEqual(created)
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/finance/entries/e-1/attachments')
    expect(opts.method).toBe('POST')
    expect(opts.body).toBeInstanceOf(FormData)
    const parts = [...(opts.body as FormData).entries()]
    expect(parts.map(([name]) => name)).toEqual(['file'])
    expect((parts[0]![1] as File).name).toBe('cable.jpg')
    // The browser must add the multipart boundary itself.
    expect(JSON.stringify(opts.headers ?? {}).toLowerCase()).not.toContain('content-type')
  })

  it('raises the row count without touching the entry version token', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ updated_at: 'T1' })]
    fetchMock.mockResolvedValueOnce({ data: makeAttachment() })
    await store.uploadAttachment('e-1', makeFile('a.png', 'image/png'))
    expect(store.entries[0]).toMatchObject({ attachment_count: 1, updated_at: 'T1' })
    fetchMock.mockResolvedValueOnce({ data: makeAttachment({ id: 'a-2' }) })
    await store.uploadAttachment('e-1', makeFile('b.png', 'image/png'))
    expect(store.entries[0]!.attachment_count).toBe(2)
    expect(store.mutationRevision).toBe(0) // receipts do not move the ledger totals
  })

  it.each([
    [415, 'unsupported_media_type', 'unsupported attachment type: use a JPEG, PNG, WebP or PDF file'],
    [413, 'too_large', 'attachment too large: the limit is 15 MB per file'],
    [409, 'limit_reached', 'an entry can hold at most 10 files'],
    [409, 'inactive', 'voided finance entries cannot change their attachments'],
    [404, 'not_found', 'finance entry not found'],
    [400, 'validation_failed', 'invalid attachment: the file is empty'],
    [400, 'bad_request', 'missing "file" part'],
  ])('maps %i %s to a FinanceApiError with the server message', async (status, code, message) => {
    const store = useEarningsStore()
    store.entries = [makeEntry()]
    fetchMock.mockRejectedValueOnce(httpError(status, code, message))
    const err = await store.uploadAttachment('e-1', makeFile('a.png', 'image/png')).catch((e) => e)
    expect(err).toBeInstanceOf(FinanceApiError)
    expect(err).toMatchObject({ status, code, message })
    expect(store.entries[0]!.attachment_count).toBe(0) // nothing was stored
  })

  it('maps a bare 413 / 415 (a proxy in front of the API) to the same codes', async () => {
    const store = useEarningsStore()
    fetchMock.mockRejectedValueOnce(Object.assign(new Error('x'), { statusCode: 413 }))
    expect(await store.uploadAttachment('e-1', makeFile('a.png', 'image/png')).catch((e) => e)).toMatchObject({ code: 'too_large' })
    fetchMock.mockRejectedValueOnce(Object.assign(new Error('x'), { statusCode: 415 }))
    expect(await store.uploadAttachment('e-1', makeFile('a.png', 'image/png')).catch((e) => e)).toMatchObject({ code: 'unsupported_media_type' })
  })
})

describe('fetchAttachments / deleteAttachment', () => {
  it('lists oldest first, caches the list and syncs the row count', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ attachment_count: 0 })]
    const list = [makeAttachment({ id: 'a-1' }), makeAttachment({ id: 'a-2', filename: 'b.pdf', mime_type: 'application/pdf' })]
    fetchMock.mockResolvedValueOnce({ data: list })
    expect(await store.fetchAttachments('e-1')).toEqual(list)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/entries/e-1/attachments', {})
    expect(store.attachments['e-1']).toEqual(list)
    expect(store.entries[0]!.attachment_count).toBe(2)
  })

  it('treats a null list as empty', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ attachment_count: 3 })]
    fetchMock.mockResolvedValueOnce({ data: null })
    expect(await store.fetchAttachments('e-1')).toEqual([])
    expect(store.entries[0]!.attachment_count).toBe(0)
  })

  it('surfaces a failed list as a FinanceApiError', async () => {
    const store = useEarningsStore()
    fetchMock.mockRejectedValueOnce(httpError(404, 'not_found', 'finance entry not found'))
    await expect(store.fetchAttachments('gone')).rejects.toMatchObject({ code: 'not_found' })
  })

  it('DELETEs one receipt and lowers the count; a failed DELETE changes nothing', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ attachment_count: 2 })]
    store.attachments['e-1'] = [makeAttachment({ id: 'a-1' }), makeAttachment({ id: 'a-2' })]

    fetchMock.mockRejectedValueOnce(httpError(409, 'inactive', 'voided finance entries cannot change their attachments'))
    await expect(store.deleteAttachment('e-1', 'a-1')).rejects.toMatchObject({ code: 'inactive' })
    expect(store.entries[0]!.attachment_count).toBe(2)
    expect(store.attachments['e-1']).toHaveLength(2)

    fetchMock.mockResolvedValueOnce(undefined)
    await store.deleteAttachment('e-1', 'a-1')
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/entries/e-1/attachments/a-1', { method: 'DELETE' })
    expect(store.attachments['e-1']!.map((a) => a.id)).toEqual(['a-2'])
    expect(store.entries[0]!.attachment_count).toBe(1)
  })

  it('a list refresh that started before an upload does not overwrite the new count', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ attachment_count: 0 })]
    const refresh = deferred<{ data: unknown[] }>()
    fetchMock.mockReturnValueOnce(refresh.promise)
    const pending = store.fetchEntries()
    fetchMock.mockResolvedValueOnce({ data: makeAttachment() })
    await store.uploadAttachment('e-1', makeFile('a.png', 'image/png'))
    refresh.resolve({ data: [makeEntry({ attachment_count: 0 })] }) // stale: read before the upload
    await pending
    expect(store.entries[0]!.attachment_count).toBe(1)
  })
})

describe('entryAttachmentUrl', () => {
  it('builds the plain download URL and the inline thumbnail URL', () => {
    expect(entryAttachmentUrl('e-1', 'a-1')).toBe('/api/v1/finance/entries/e-1/attachments/a-1')
    expect(entryAttachmentUrl('e-1', 'a-1', { inline: true })).toBe('/api/v1/finance/entries/e-1/attachments/a-1?inline=1')
  })
})

describe('receipt filter', () => {
  it('sends receipt=missing on the list request and remembers it in the snapshot', async () => {
    const store = useEarningsStore()
    fetchMock.mockResolvedValue({ data: [] })
    store.setFilter('receipt', 'missing')
    await store.fetchEntries(store.filterAsEntryFilter())
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/entries', { params: { receipt: 'missing' } })
    expect(store.filterSnapshot).toEqual({ receipt: 'missing' })
  })

  it('combines with the other filters and clears with them', async () => {
    const store = useEarningsStore()
    fetchMock.mockResolvedValue({ data: [] })
    store.setFilter('kind', 'expense')
    store.setFilter('receipt', 'missing')
    expect(store.filterAsEntryFilter()).toEqual({ kind: 'expense', receipt: 'missing' })
    store.clearFilter()
    expect(store.filter.receipt).toBe('')
    expect(store.filterAsEntryFilter()).toEqual({})
    await store.fetchEntries(store.filterAsEntryFilter())
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/entries', { params: {} })
  })

  it('filters the loaded rows to match, and counts entries without a receipt', async () => {
    const store = useEarningsStore()
    const withReceipt = makeEntry({ id: 'with', attachment_count: 2 })
    const without = makeEntry({ id: 'without', attachment_count: 0 })
    fetchMock.mockResolvedValueOnce({ data: [withReceipt, without] })
    await store.fetchEntries({})
    expect(store.filterCounts.noReceipt).toBe(1)
    expect(store.filteredEntries.map((e) => e.id)).toEqual(['with', 'without'])

    fetchMock.mockResolvedValueOnce({ data: [without] })
    await store.fetchEntries({ receipt: 'missing' })
    expect(store.filteredEntries.map((e) => e.id)).toEqual(['without'])
  })

  // "No receipt" is about ACTIVE EXPENSES: income (gig payments, royalties) and
  // voided entries (which can no longer take files) are never counted or listed.
  it('counts and lists only active expenses without files', async () => {
    const store = useEarningsStore()
    const bare = makeEntry({ id: 'bare', kind: 'expense', attachment_count: 0 })
    const rows = [
      bare,
      makeEntry({ id: 'covered', kind: 'expense', attachment_count: 1 }),
      makeEntry({ id: 'income', kind: 'income', description: 'DJ set', notes: '', attachment_count: 0 }),
      makeEntry({ id: 'voided', kind: 'expense', status: 'voided', attachment_count: 0 }),
    ]
    fetchMock.mockResolvedValueOnce({ data: rows })
    await store.fetchEntries({ receipt: 'missing' })
    expect(store.filterCounts.noReceipt).toBe(1)
    expect(store.filteredEntries.map((e) => e.id)).toEqual(['bare'])
  })

  it('leaving the expense scope drops the receipt filter, entering it drops a kind or status that would empty it', () => {
    const store = useEarningsStore()
    store.setFilter('receipt', 'missing')
    store.setFilter('kind', 'income')
    expect(store.filter).toMatchObject({ kind: 'income', receipt: '' })

    store.setFilter('receipt', 'missing')
    expect(store.filter).toMatchObject({ kind: '', receipt: 'missing' }) // income cleared

    store.setFilter('status', 'voided')
    expect(store.filter).toMatchObject({ status: 'voided', receipt: '' })
    store.setFilter('receipt', 'missing')
    expect(store.filter).toMatchObject({ status: '', receipt: 'missing' }) // voided cleared

    // Compatible filters are left alone.
    store.setFilter('kind', 'expense')
    store.setFilter('currency', 'EUR')
    expect(store.filter).toMatchObject({ kind: 'expense', currency: 'EUR', receipt: 'missing' })
  })
})
