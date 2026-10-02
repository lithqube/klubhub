import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import Finance from '../finance.vue'
import { useEarningsStore } from '../../stores/earnings'
import { useInvoiceStore } from '../../stores/invoice'
import type { Entry } from '../../types/finance'

const entry: Entry = {
  id: 'entry-1', kind: 'income', amount_minor: 12000, currency: 'EUR', category: 'gig_fee',
  entry_date: '2026-10-01', description: 'Recorded income', notes: '', gig_id: null,
  status: 'active', auto_generated: false, source_kind: 'manual', source_id: null,
  source_amount_minor: null, source_currency: null, source_description: '',
  created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z', deleted_at: null,
}
let rows: Entry[]
let demo = false
let readUnavailable = false
let writeError: Error | null
const fetchMock = vi.fn(async (url: string, opts?: { method?: string }) => {
  if (readUnavailable) throw new Error('Finance read unavailable')
  if (opts?.method === 'DELETE' || url.endsWith('/void')) {
    if (writeError) throw writeError
    if (opts?.method === 'DELETE') { rows = []; return { data: null } }
    rows = [{ ...entry, status: 'voided' }]
    return { data: rows[0] }
  }
  if (url === '/api/v1/finance/entries') return { data: rows }
  if (url === '/api/v1/finance/invoices') return { data: [] }
  if (url === '/api/v1/finance/summary') return { data: {} }
  if (url === '/api/v1/finance/profit-loss') return { data: {} }
  throw new Error(`Unexpected request: ${url}`)
})
const wrappers: VueWrapper[] = []
function render() {
  const wrapper = mount(Finance, { global: { stubs: {
    InvoiceWorkspace: true, EarningsWorkspace: true, EntryReconciliationDialog: true, teleport: true,
  } } })
  wrappers.push(wrapper)
  return wrapper
}
async function click(wrapper: VueWrapper, label: string) {
  const button = wrapper.findAll('button').find(b => b.text().trim() === label)
  expect(button, `button ${label}`).toBeDefined()
  await button!.trigger('click')
  await flushPromises()
}
beforeEach(() => {
  setActivePinia(createPinia())
  rows = [entry]
  demo = false
  readUnavailable = false
  writeError = null
  fetchMock.mockClear()
  vi.stubGlobal('$fetch', fetchMock)
  vi.stubGlobal('useHead', vi.fn())
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { demo, dashboardMockData: demo } }))
})
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.unstubAllGlobals() })

describe('honest finance page (mounted page and real Pinia)', () => {
  it.each([['VOID', 'CONFIRM VOID'], ['DELETE', 'DELETE']])('shows a failed %s request without removing the real entry', async (action, confirm) => {
    writeError = new Error('Connection lost while saving')
    const wrapper = render()
    await flushPromises()
    await click(wrapper, action)
    await wrapper.get('[role="dialog"]').findAll('button').find(b => b.text().trim() === confirm)!.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[role="alert"]').map(a => a.text()).join(' ')).toContain('Could not reach the server.')
    expect(useEarningsStore().entries).toEqual([entry])
    expect(wrapper.text()).toContain('Recorded income')
  })

  it('keeps the supported actions working in demo mode', async () => {
    demo = true
    const wrapper = render()
    await flushPromises()
    await click(wrapper, '+ NEW INVOICE')
    expect(useInvoiceStore().createOpen).toBe(true)
    await click(wrapper, '+ NEW INCOME')
    expect(useEarningsStore().createOpen).toBe(true)
    expect(useEarningsStore().createKind).toBe('income')
    await click(wrapper, '+ NEW EXPENSE')
    expect(useEarningsStore().createKind).toBe('expense')
    expect(wrapper.text()).not.toMatch(/NOT YET WIRED|COMING SOON/)
  })

  it('scopes the tax disclaimer to earnings, not the shipped invoice calculations', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).not.toContain('No tax or VAT is computed anywhere on this page')
    expect(wrapper.text()).not.toContain('no tax / VAT is computed anywhere on this page')
    expect(wrapper.text()).toContain('Earnings entries do not compute tax or VAT')
  })

  it('uses actual empty API results in normal mode without fallback fixtures', async () => {
    rows = []
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('NO ENTRIES YET')
    expect(wrapper.text()).toContain('NO INVOICES YET')
    expect(wrapper.text()).not.toMatch(/DEMO DATA|Recorded income|PHASE 5|NOT YET WIRED/)
  })

  it('renders failed reads as unavailable, never an empty ledger or mock fallback', async () => {
    readUnavailable = true
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('COULD NOT LOAD ENTRIES')
    expect(wrapper.text()).toContain('COULD NOT LOAD INVOICES')
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(4)
    expect(wrapper.text()).not.toMatch(/NO ENTRIES YET|NO INVOICES YET|DEMO DATA|Recorded income/)
  })

  it.each([['VOID', 'CONFIRM VOID', 'POST', '/api/v1/finance/entries/entry-1/void'], ['DELETE', 'DELETE', 'DELETE', '/api/v1/finance/entries/entry-1']])('keeps the working %s endpoint and confirms server state', async (action, confirm, method, url) => {
    const wrapper = render()
    await flushPromises()
    await click(wrapper, action)
    await wrapper.get('[role="dialog"]').findAll('button').find(b => b.text().trim() === confirm)!.trigger('click')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledWith(url, { method, body: { updated_at: entry.updated_at } })
    const writeIndex = fetchMock.mock.calls.findIndex(([requestUrl, opts]) => requestUrl === url && opts?.method === method)
    const requestsAfterWrite = fetchMock.mock.calls.slice(writeIndex + 1).map(([requestUrl]) => requestUrl)
    expect(requestsAfterWrite).toContain('/api/v1/finance/summary')
    expect(requestsAfterWrite).toContain('/api/v1/finance/profit-loss')
    expect(useEarningsStore().entries).toEqual(rows)
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(0)
  })
})
