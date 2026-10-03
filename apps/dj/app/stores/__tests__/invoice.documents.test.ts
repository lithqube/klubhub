// The archive list never disables finance: a server with no object storage
// (503) is "no archive here", and downloads are plain links to the server.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { documentDownloadUrl, useInvoiceStore } from '../invoice'

const fetchMock = vi.fn()

describe('invoice store archive', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
    // @ts-expect-error - mocking global $fetch
    global.$fetch = fetchMock
  })

  it('asks for the invoice\'s own documents and unwraps them', async () => {
    const docs = [{ id: 'd1', kind: 'invoice_pdf' }]
    fetchMock.mockResolvedValueOnce({ data: docs })
    const res = await useInvoiceStore().fetchInvoiceDocuments('inv-1')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/documents', { query: { owner_type: 'invoice', owner_id: 'inv-1' } })
    expect(res).toEqual(docs)
  })

  it('treats a null list as empty', async () => {
    fetchMock.mockResolvedValueOnce({ data: null })
    expect(await useInvoiceStore().fetchInvoiceDocuments('i')).toEqual([])
  })

  it('resolves to null on 503 without disabling finance', async () => {
    fetchMock.mockRejectedValueOnce({ statusCode: 503, data: { error: 'service_unavailable', message: 'x' } })
    const store = useInvoiceStore()
    expect(await store.fetchInvoiceDocuments('i')).toBeNull()
    expect(store.disabled).toBe(false)
  })

  it('rethrows other failures', async () => {
    fetchMock.mockRejectedValueOnce({ statusCode: 400, data: { error: 'validation_failed', message: 'bad owner' } })
    await expect(useInvoiceStore().fetchInvoiceDocuments('i')).rejects.toMatchObject({ code: 'validation_failed' })
  })

  it('links downloads to the attachment endpoint', () => {
    expect(documentDownloadUrl('a/b')).toBe('/api/v1/finance/documents/a%2Fb/download')
  })
})
