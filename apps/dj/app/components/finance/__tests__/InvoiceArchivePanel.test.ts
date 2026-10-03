import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import InvoiceArchivePanel from '../InvoiceArchivePanel.vue'
import { FinanceApiError, useInvoiceStore } from '../../../stores/invoice'
import type { ArchivedDocument } from '../../../types/finance'
import { makeInvoice } from './fixtures'

const SUM = 'abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789'

function doc(over: Partial<ArchivedDocument>): ArchivedDocument {
  return {
    id: 'd', kind: 'invoice_pdf', filename: 'invoice-INV-1.pdf', mime_type: 'application/pdf', size_bytes: 2048,
    checksum_sha256: SUM, version: 1, is_current: true, uploaded_by: 'system', created_at: '2026-10-03T10:00:00Z', ...over,
  }
}

function setup(answer: unknown, invOver = {}) {
  setActivePinia(createPinia())
  const store = useInvoiceStore()
  const spy = vi.spyOn(store, 'fetchInvoiceDocuments')
  if (answer instanceof Error) spy.mockRejectedValue(answer)
  else spy.mockResolvedValue(answer as never)
  const wrapper = mount(InvoiceArchivePanel, { props: { invoice: makeInvoice({ id: 'inv-9', status: 'issued', ...invOver }) } })
  return { wrapper, spy }
}

const open = async (w: ReturnType<typeof mount>) => { await w.get('.iar-toggle').trigger('click'); await flushPromises() }

describe('InvoiceArchivePanel', () => {
  beforeEach(() => { document.body.innerHTML = '' })

  it('loads nothing until opened', () => {
    const { wrapper, spy } = setup([])
    expect(spy).not.toHaveBeenCalled()
    expect(wrapper.find('.iar-body').exists()).toBe(false)
  })

  it('lists the files in a fixed order with size, checksum and download links', async () => {
    const { wrapper, spy } = setup([
      doc({ id: 'r', kind: 'validation_report', filename: 'r.json', mime_type: 'application/json' }),
      doc({ id: 'x', kind: 'einvoice_xml', filename: 'x.xml', mime_type: 'application/xml' }),
      doc({ id: 'p' }),
    ])
    await open(wrapper)
    expect(spy).toHaveBeenCalledWith('inv-9')
    const kinds = wrapper.findAll('.iar-kind').map((e) => e.text())
    expect(kinds).toEqual(['INVOICE PDF', 'E-INVOICE XML', 'VALIDATION REPORT'])
    const first = wrapper.findAll('.iar-item')[0]!
    expect(first.text()).toContain('2 KB')
    expect(first.text()).toContain('SHA-256 abcdef012345')
    expect(first.get('.iar-sum').attributes('title')).toContain(SUM)
    expect(first.get('a.iar-download').attributes('href')).toBe('/api/v1/finance/documents/p/download')
  })

  it('names a credit note PDF as one', async () => {
    const { wrapper } = setup([doc({})], { kind: 'credit_note' })
    await open(wrapper)
    expect(wrapper.get('.iar-kind').text()).toBe('CREDIT NOTE PDF')
  })

  it('says documents made later were rendered later, and system ones carry no such tag', async () => {
    const { wrapper } = setup([doc({ id: 'a', uploaded_by: 'backfill' }), doc({ id: 'b', kind: 'einvoice_xml' })])
    await open(wrapper)
    const items = wrapper.findAll('.iar-item')
    expect(items[0]!.text()).toContain('RENDERED LATER')
    expect(items[0]!.get('.iar-tag-later').attributes('title')).toContain('not the file produced when the invoice was issued')
    expect(items[1]!.text()).not.toContain('RENDERED LATER')
  })

  it('marks older versions as superseded', async () => {
    const { wrapper } = setup([doc({ id: 'new', version: 2 }), doc({ id: 'old', version: 1, is_current: false })])
    await open(wrapper)
    const items = wrapper.findAll('.iar-item')
    expect(items[0]!.text()).toContain('V2')
    expect(items[0]!.classes()).not.toContain('iar-old')
    expect(items[1]!.text()).toContain('SUPERSEDED')
    expect(items[1]!.classes()).toContain('iar-old')
  })

  it('says so when nothing is archived', async () => {
    const { wrapper } = setup([])
    await open(wrapper)
    expect(wrapper.text()).toContain('Nothing is archived')
    expect(wrapper.find('ul').exists()).toBe(false)
  })

  it('says so when the server keeps no archive', async () => {
    const { wrapper } = setup(null)
    await open(wrapper)
    expect(wrapper.text()).toContain("isn't enabled")
  })

  it('shows a failure and retries', async () => {
    const { wrapper, spy } = setup(new FinanceApiError(500, 'unknown', 'boom'))
    await open(wrapper)
    expect(wrapper.get('[role=alert]').text()).toContain('boom')
    spy.mockResolvedValue([doc({})])
    await wrapper.findAll('button').find((b) => b.text() === 'RETRY')!.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.iar-item')).toHaveLength(1)
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
  })
})
