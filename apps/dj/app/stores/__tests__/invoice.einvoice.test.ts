// The e-invoice check answers per format and never disables finance: a server
// without a generator (503) is "not available here", not "invoicing is off".
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { invoiceEInvoiceUrl, useInvoiceStore } from '../invoice'

const fetchMock = vi.fn()

describe('invoice store e-invoice', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
    // @ts-expect-error - mocking global $fetch
    global.$fetch = fetchMock
  })

  it('asks the server for the chosen format and unwraps the answer', async () => {
    fetchMock.mockResolvedValueOnce({ data: { format: 'xrechnung-cii', ready: false, problems: [{ field: 'buyer_reference', message: 'Needed' }] } })
    const chk = await useInvoiceStore().fetchEInvoiceCheck('inv 1', 'xrechnung-cii')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/invoices/inv%201/einvoice-check', { query: { format: 'xrechnung-cii' } })
    expect(chk).toEqual({ format: 'xrechnung-cii', ready: false, problems: [{ field: 'buyer_reference', message: 'Needed' }] })
  })

  it('tolerates a null problem list', async () => {
    fetchMock.mockResolvedValueOnce({ data: { ready: true, problems: null } })
    const chk = await useInvoiceStore().fetchEInvoiceCheck('i', 'facturx')
    expect(chk).toEqual({ format: 'facturx', ready: true, problems: [] })
  })

  it('resolves to null on 503 without disabling finance', async () => {
    fetchMock.mockRejectedValueOnce({ statusCode: 503, data: { error: 'service_unavailable', message: 'x' } })
    const store = useInvoiceStore()
    expect(await store.fetchEInvoiceCheck('i', 'facturx')).toBeNull()
    expect(store.disabled).toBe(false)
  })

  it('rethrows other failures', async () => {
    fetchMock.mockRejectedValueOnce({ statusCode: 409, data: { error: 'bad_state', message: 'draft' } })
    await expect(useInvoiceStore().fetchEInvoiceCheck('i', 'facturx')).rejects.toMatchObject({ code: 'bad_state' })
  })

  it('links the download to the GET attachment endpoint', () => {
    expect(invoiceEInvoiceUrl('a/b', 'xrechnung-ubl')).toBe('/api/v1/finance/invoices/a%2Fb/einvoice?format=xrechnung-ubl')
  })
})
