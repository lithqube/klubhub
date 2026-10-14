// Emailing an invoice never disables finance: a server without mail (503) is
// "not available here", and a refusal keeps the server's own explanation.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { FinanceApiError, useInvoiceStore } from '../invoice'

const fetchMock = vi.fn()

describe('invoice store email', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
    // @ts-expect-error - mocking global $fetch
    global.$fetch = fetchMock
  })

  it('posts the recipient and unwraps the stored message', async () => {
    const msg = { id: 'm1', to_email: 'a@b.example', subject: 's', status: 'sent', attachment_ids: ['p', 'x'] }
    fetchMock.mockResolvedValueOnce({ data: msg })
    const res = await useInvoiceStore().sendInvoiceEmail('inv 1', { to_email: 'a@b.example', include_einvoice: false })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/invoices/inv%201/email', {
      method: 'POST',
      body: { to_email: 'a@b.example', include_einvoice: false },
    })
    expect(res).toEqual(msg)
  })

  it('resolves to null on 503 without disabling finance', async () => {
    fetchMock.mockRejectedValueOnce({ statusCode: 503, data: { error: 'service_unavailable', message: 'x' } })
    const store = useInvoiceStore()
    expect(await store.sendInvoiceEmail('i', {})).toBeNull()
    expect(store.disabled).toBe(false)
  })

  it('keeps the problems of a refused e-invoice', async () => {
    fetchMock.mockRejectedValueOnce({
      statusCode: 422,
      data: { error: 'not_exportable', message: 'cannot', problems: [{ field: 'einvoice', message: 'No e-invoice to attach.' }] },
    })
    const err = await useInvoiceStore().sendInvoiceEmail('i', { include_einvoice: true }).catch((e) => e)
    expect(err).toBeInstanceOf(FinanceApiError)
    expect(err.code).toBe('not_exportable')
    expect(err.problems).toEqual([{ field: 'einvoice', message: 'No e-invoice to attach.' }])
  })

  it('rethrows validation failures with the server message', async () => {
    fetchMock.mockRejectedValueOnce({ statusCode: 400, data: { error: 'validation_failed', message: 'to_email: invalid' } })
    await expect(useInvoiceStore().sendInvoiceEmail('i', { to_email: 'x' })).rejects.toMatchObject({
      code: 'validation_failed',
      message: 'to_email: invalid',
    })
  })
})
