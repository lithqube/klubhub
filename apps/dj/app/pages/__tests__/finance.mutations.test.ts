import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import Finance from '../finance.vue'
import EarningsWorkspace from '../../components/finance/EarningsWorkspace.vue'
import EntryForm from '../../components/finance/EntryForm.vue'
import { useEarningsStore } from '../../stores/earnings'
import type { Entry } from '../../types/finance'

const entry: Entry = {
  id: 'entry-1', kind: 'income', amount_minor: 12000, currency: 'EUR', category: 'gig_fee',
  entry_date: '2026-10-01', description: 'Recorded income', notes: '', gig_id: null,
  status: 'active', auto_generated: false, source_kind: 'manual', source_id: null,
  source_amount_minor: null, source_currency: null, source_description: '',
  created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z', deleted_at: null, attachment_count: 0,
}
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const summary = (amount: number) => ({ EUR: { currency: 'EUR', income_minor: amount, expense_minor: 0 } })
const profitLoss = (amount: number) => ({ EUR: { ...summary(amount).EUR, profit_loss_minor: amount } })
let mutation: ReturnType<typeof deferred<unknown>>
let aggregateReads: { summary: ReturnType<typeof deferred<unknown>>; profitLoss: ReturnType<typeof deferred<unknown>> } | null
let aggregateError: boolean
const fetchMock = vi.fn((url: string, opts?: { method?: string; params?: unknown; body?: unknown }) => {
  if (opts?.method) return mutation.promise
  if (url.endsWith('/invoices')) return Promise.resolve({ data: [] })
  if (url.endsWith('/entries')) return Promise.resolve({ data: [entry] })
  if (url.endsWith('/summary') || url.endsWith('/profit-loss')) {
    if (aggregateError) return Promise.reject(new Error('Aggregate read failed'))
    if (aggregateReads) return url.endsWith('/summary') ? aggregateReads.summary.promise : aggregateReads.profitLoss.promise
    return Promise.resolve({ data: url.endsWith('/summary') ? summary(12000) : profitLoss(12000) })
  }
  throw new Error(`Unexpected request: ${url}`)
})
const wrappers: VueWrapper[] = []
async function render() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const wrapper = mount(Finance, { attachTo: document.body, global: { plugins: [pinia], stubs: {
    InvoiceWorkspace: true, EntryReconciliationDialog: true,
  } } })
  wrappers.push(wrapper)
  await flushPromises()
  expect(wrapper.findComponent(EarningsWorkspace).exists()).toBe(true)
  // The composer is inline and only exists while open: nothing mounted until a button asks for it.
  expect(wrapper.findComponent(EntryForm).exists()).toBe(false)
  expect(useEarningsStore().listLoaded).toBe(true)
  fetchMock.mockClear() // Only count requests after the initially loaded page has settled.
  return wrapper
}
async function click(wrapper: VueWrapper, label: string) {
  const button = wrapper.findAll('button').find(b => b.text().trim() === label)
  expect(button, label).toBeDefined()
  await button!.trigger('click')
  await flushPromises()
}
async function field(selector: string, value: string) {
  const input = document.querySelector(selector) as HTMLInputElement
  expect(input, selector).not.toBeNull()
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await flushPromises()
}
async function submit() {
  const form = document.querySelector('.ee-form')!
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await flushPromises()
}
function expectNoRefresh() {
  expect(fetchMock.mock.calls.filter(([url, opts]) => !opts?.method && /summary|profit-loss/.test(url))).toHaveLength(0)
  expect(useEarningsStore().summary).toEqual(summary(12000))
  expect(useEarningsStore().profitLossByScope?.data).toEqual(profitLoss(12000))
}
function expectOneRefreshPerScope() {
  const store = useEarningsStore()
  const requests = fetchMock.mock.calls.filter(([, opts]) => !opts?.method)
  expect(requests).toHaveLength(2)
  expect(requests.filter(([url]) => url.endsWith('/summary'))).toEqual([
    ['/api/v1/finance/summary', { params: Object.fromEntries(Object.entries(store.summaryScope).map(([key, value]) => [key, String(value)])) }],
  ])
  expect(requests.filter(([url]) => url.endsWith('/profit-loss'))).toEqual([
    ['/api/v1/finance/profit-loss', { params: Object.fromEntries(Object.entries(store.profitLossScope).map(([key, value]) => [key, String(value)])) }],
  ])
}
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-10-01T12:00:00Z'))
  mutation = deferred()
  aggregateReads = null
  aggregateError = false
  fetchMock.mockClear()
  vi.stubGlobal('$fetch', fetchMock)
  vi.stubGlobal('useHead', vi.fn())
  vi.stubGlobal('useRuntimeConfig', () => ({ public: {} }))
})
afterEach(() => {
  wrappers.splice(0).forEach(w => w.unmount())
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('confirmed finance mutations (real page, workspace, dialog and Pinia)', () => {
  it.each(['create', 'edit'] as const)('refreshes both selected aggregates only after %s confirms, once despite duplicate submit', async (action) => {
    const wrapper = await render()
    await click(wrapper, action === 'create' ? '+ NEW INCOME' : 'EDIT')
    await field('#ee-amount', '200')
    await field('#ee-description', 'Confirmed income')
    await submit()
    await submit()
    expect(fetchMock.mock.calls.filter(([, opts]) => opts?.method)).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledWith(action === 'create' ? '/api/v1/finance/entries' : '/api/v1/finance/entries/entry-1', {
      method: action === 'create' ? 'POST' : 'PUT',
      body: { kind: 'income', amount_minor: 20000, currency: 'EUR', category: 'gig_fee', entry_date: '2026-10-01', description: 'Confirmed income', notes: '', gig_id: null,
        ...(action === 'edit' ? { updated_at: entry.updated_at } : {}) },
    })
    expect(useEarningsStore().entries).toEqual([entry])
    expect(wrapper.findComponent(EarningsWorkspace).emitted('saved')).toBeUndefined()
    expectNoRefresh()
    aggregateReads = { summary: deferred(), profitLoss: deferred() }
    const confirmed = { ...entry, id: action === 'create' ? 'entry-2' : entry.id, amount_minor: 20000, description: 'Confirmed income', updated_at: '2026-10-01T12:00:00Z' }
    mutation.resolve({ data: confirmed })
    await flushPromises()
    expectOneRefreshPerScope()
    expect(wrapper.findComponent(EarningsWorkspace).emitted('saved')).toEqual([[confirmed]])
    expect(useEarningsStore().createOpen).toBe(false)
    expect(useEarningsStore().entries).toEqual(action === 'create' ? [confirmed, entry] : [confirmed])
    // The record is confirmed; aggregate caches still wait for their own transport.
    expect(useEarningsStore().summary).toEqual(summary(12000))
    expect(useEarningsStore().profitLossByScope?.data).toEqual(profitLoss(12000))
    aggregateReads.summary.resolve({ data: summary(32000) })
    aggregateReads.profitLoss.resolve({ data: profitLoss(32000) })
    await flushPromises()
    expect(useEarningsStore().summary).toEqual(summary(32000))
    expect(useEarningsStore().profitLossByScope?.data).toEqual(profitLoss(32000))
    expect(wrapper.get('.es-rows').text()).toContain('320')
    expect(wrapper.get('.pl-rows').text()).toContain('320')
    expectOneRefreshPerScope()
  })

  it.each(['create', 'edit', 'void', 'delete'] as const)('ignores late pre-%s aggregate responses after the confirmed refresh', async (action) => {
    const wrapper = await render()
    const store = useEarningsStore()
    const oldReads = { summary: deferred<unknown>(), profitLoss: deferred<unknown>() }
    aggregateReads = oldReads
    const oldRefresh = Promise.all([store.refreshSummary(), store.refreshProfitLoss()])
    if (action === 'create' || action === 'edit') {
      await click(wrapper, action === 'create' ? '+ NEW INCOME' : 'EDIT')
      await field('#ee-amount', '200')
      await field('#ee-description', 'Confirmed income')
      await submit()
    } else {
      await click(wrapper, action.toUpperCase())
      const confirm = Array.from(document.querySelectorAll('.finance-confirm button')).find(b => b.textContent?.trim() === (action === 'void' ? 'CONFIRM VOID' : 'DELETE')) as HTMLButtonElement
      confirm.click()
      await flushPromises()
    }
    const newReads = { summary: deferred<unknown>(), profitLoss: deferred<unknown>() }
    aggregateReads = newReads
    const confirmed = { ...entry, id: action === 'create' ? 'entry-2' : entry.id, amount_minor: 20000, status: action === 'void' ? 'voided' as const : 'active' as const }
    mutation.resolve({ data: action === 'delete' ? null : confirmed })
    await flushPromises()
    const amount = action === 'create' ? 32000 : action === 'edit' ? 20000 : 0
    newReads.summary.resolve({ data: summary(amount) })
    newReads.profitLoss.resolve({ data: profitLoss(amount) })
    await flushPromises()
    oldReads.summary.resolve({ data: summary(12000) })
    oldReads.profitLoss.resolve({ data: profitLoss(12000) })
    await oldRefresh
    await flushPromises()
    expect(store.summary).toEqual(summary(amount))
    expect(store.profitLossByScope?.data).toEqual(profitLoss(amount))
    expect(store.entries).toEqual(action === 'delete' ? [] : action === 'create' ? [confirmed, entry] : [confirmed])
    expect(store.summaryError).toBeNull()
    expect(store.profitLossError).toBeNull()
    expect(wrapper.get('.es-rows').text()).toContain(String(amount / 100))
    expect(wrapper.get('.pl-rows').text()).toContain(String(amount / 100))
  })

  it.each(['create', 'edit'] as const)('preserves records, form and aggregates when %s rejects', async (action) => {
    const wrapper = await render()
    await click(wrapper, action === 'create' ? '+ NEW INCOME' : 'EDIT')
    await field('#ee-amount', '200')
    await field('#ee-description', 'Unsaved income')
    await submit()
    expectNoRefresh()
    expect(useEarningsStore().entries).toEqual([entry])
    mutation.reject(new Error('Connection lost'))
    await flushPromises()
    expect(useEarningsStore().entries).toEqual([entry])
    expect(useEarningsStore().createOpen).toBe(true)
    expect(wrapper.findComponent(EarningsWorkspace).emitted('saved')).toBeUndefined()
    expect(wrapper.findComponent(EntryForm).emitted('saved')).toBeUndefined()
    expect(document.querySelector('.ee-form [role="alert"]')?.textContent).toContain('Could not reach the server.')
    expect((document.querySelector('#ee-description') as HTMLInputElement).value).toBe('Unsaved income')
    expectNoRefresh()
  })

  it.each(['void', 'delete'] as const)('waits for deferred %s confirmation before changing record or refreshing aggregates', async (action) => {
    const wrapper = await render()
    await click(wrapper, action.toUpperCase())
    const confirm = Array.from(document.querySelectorAll('.finance-confirm button')).find(b => b.textContent?.trim() === (action === 'void' ? 'CONFIRM VOID' : 'DELETE')) as HTMLButtonElement
    confirm.click()
    confirm.click()
    await flushPromises()
    expect(fetchMock.mock.calls.filter(([, opts]) => opts?.method)).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledWith(`/api/v1/finance/entries/entry-1${action === 'void' ? '/void' : ''}`, { method: action === 'void' ? 'POST' : 'DELETE', body: { updated_at: entry.updated_at } })
    expect(useEarningsStore().entries).toEqual([entry])
    expectNoRefresh()
    aggregateReads = { summary: deferred(), profitLoss: deferred() }
    const voided = { ...entry, status: 'voided' as const }
    mutation.resolve({ data: action === 'void' ? voided : null })
    await flushPromises()
    expect(useEarningsStore().entries).toEqual(action === 'void' ? [voided] : [])
    expectOneRefreshPerScope()
    expect(useEarningsStore().summary).toEqual(summary(12000))
    expect(useEarningsStore().profitLossByScope?.data).toEqual(profitLoss(12000))
    aggregateReads.summary.resolve({ data: summary(0) })
    aggregateReads.profitLoss.resolve({ data: profitLoss(0) })
    await flushPromises()
    expect(useEarningsStore().summary).toEqual(summary(0))
    expect(useEarningsStore().profitLossByScope?.data).toEqual(profitLoss(0))
    expectOneRefreshPerScope()
  })

  it.each(['void', 'delete'] as const)('preserves state and shows an accessible error after deferred %s rejection', async (action) => {
    const wrapper = await render()
    await click(wrapper, action.toUpperCase())
    const confirm = Array.from(document.querySelectorAll('.finance-confirm button')).find(b => b.textContent?.trim() === (action === 'void' ? 'CONFIRM VOID' : 'DELETE')) as HTMLButtonElement
    confirm.click()
    await flushPromises()
    expect(useEarningsStore().entries).toEqual([entry])
    expectNoRefresh()
    mutation.reject(new Error('Connection lost'))
    await flushPromises()
    expect(wrapper.get('.finance-notice[role="alert"]').text()).toBe('Could not reach the server.')
    expect(useEarningsStore().entries).toEqual([entry])
    expectNoRefresh()
  })

  it.each(['resolve', 'reject'] as const)('keeps newer reads busy when superseded aggregate reads %s', async (outcome) => {
    const wrapper = await render()
    const store = useEarningsStore()
    const oldReads = { summary: deferred<unknown>(), profitLoss: deferred<unknown>() }
    aggregateReads = oldReads
    const oldRefresh = Promise.all([store.refreshSummary(), store.refreshProfitLoss()])
    const newReads = { summary: deferred<unknown>(), profitLoss: deferred<unknown>() }
    aggregateReads = newReads
    const newRefresh = Promise.all([store.refreshSummary(), store.refreshProfitLoss()])
    if (outcome === 'resolve') {
      oldReads.summary.resolve({ data: summary(99900) })
      oldReads.profitLoss.resolve({ data: profitLoss(99900) })
    } else {
      const unavailable = Object.assign(new Error('Unavailable'), { statusCode: 503, data: { error: 'unavailable', message: 'Old failure' } })
      oldReads.summary.reject(unavailable)
      oldReads.profitLoss.reject(unavailable)
    }
    await oldRefresh
    await flushPromises()
    expect(store.summaryLoading).toBe(true)
    expect(store.profitLossLoading).toBe(true)
    expect(store.summaryError).toBeNull()
    expect(store.profitLossError).toBeNull()
    expect(store.disabled).toBe(false)
    expect(store.summary).toEqual(summary(12000))
    expect(store.profitLossByScope?.data).toEqual(profitLoss(12000))
    expect(wrapper.find('.es [role="status"]').exists()).toBe(true)
    expect(wrapper.find('.pl [role="status"]').exists()).toBe(true)
    newReads.summary.resolve({ data: summary(20000) })
    newReads.profitLoss.resolve({ data: profitLoss(20000) })
    await newRefresh
    await flushPromises()
    expect(store.summaryLoading).toBe(false)
    expect(store.profitLossLoading).toBe(false)
    expect(wrapper.get('.es-rows').text()).toContain('200')
    expect(wrapper.get('.pl-rows').text()).toContain('200')
  })

  it('ignores superseded rejections after the newer aggregate reads succeed', async () => {
    const wrapper = await render()
    const store = useEarningsStore()
    const oldReads = { summary: deferred<unknown>(), profitLoss: deferred<unknown>() }
    aggregateReads = oldReads
    const oldRefresh = Promise.all([store.refreshSummary(), store.refreshProfitLoss()])
    aggregateReads = null
    await Promise.all([store.refreshSummary(), store.refreshProfitLoss()])
    oldReads.summary.reject(new Error('Old summary failure'))
    oldReads.profitLoss.reject(new Error('Old P&L failure'))
    await oldRefresh
    await flushPromises()
    expect(store.summaryError).toBeNull()
    expect(store.profitLossError).toBeNull()
    expect(wrapper.find('.es [role="alert"]').exists()).toBe(false)
    expect(wrapper.find('.pl [role="alert"]').exists()).toBe(false)
    expect(wrapper.get('.es-rows').text()).toContain('120')
    expect(wrapper.get('.pl-rows').text()).toContain('120')
  })

  it('does not render late 2025 totals under the selected 2024 year', async () => {
    const wrapper = await render()
    const store = useEarningsStore()
    await click(wrapper.find('.es'), 'YEAR')
    await click(wrapper.find('.pl'), 'YEAR')
    const oldReads = { summary: deferred<unknown>(), profitLoss: deferred<unknown>() }
    aggregateReads = oldReads
    await wrapper.get('.es select[aria-label="Year"]').setValue('2025')
    await wrapper.get('.pl select[aria-label="Year"]').setValue('2025')
    await flushPromises()
    const newReads = { summary: deferred<unknown>(), profitLoss: deferred<unknown>() }
    aggregateReads = newReads
    await wrapper.get('.es select[aria-label="Year"]').setValue('2024')
    await wrapper.get('.pl select[aria-label="Year"]').setValue('2024')
    await flushPromises()
    newReads.summary.resolve({ data: summary(24000) })
    newReads.profitLoss.resolve({ data: profitLoss(24000) })
    await flushPromises()
    oldReads.summary.resolve({ data: summary(25000) })
    oldReads.profitLoss.resolve({ data: profitLoss(25000) })
    await flushPromises()
    expect(store.summaryScope).toEqual({ scope: 'year', year: 2024 })
    expect(store.profitLossScope).toEqual({ scope: 'year', year: 2024 })
    expect(store.summary).toEqual(summary(24000))
    expect(store.profitLossByScope).toEqual({ scope: '2024', data: profitLoss(24000) })
    expect(wrapper.get('.es-rows').text()).toContain('240')
    expect(wrapper.get('.pl-rows').text()).toContain('240')
    expect(wrapper.get('.pl-sub').text()).toBe('YEAR · 2024')
  })

  it('preserves non-default scopes when refreshing a confirmed edit', async () => {
    const wrapper = await render()
    await click(wrapper.find('.es'), 'YEAR')
    await wrapper.get('.es select[aria-label="Year"]').setValue('2025')
    await click(wrapper.find('.pl'), 'YEAR')
    await wrapper.get('.pl select[aria-label="Year"]').setValue('2024')
    await flushPromises()
    expect(useEarningsStore().summaryScope).toEqual({ scope: 'year', year: 2025 })
    expect(useEarningsStore().profitLossScope).toEqual({ scope: 'year', year: 2024 })
    fetchMock.mockClear()
    await click(wrapper, 'EDIT')
    await submit()
    expectNoRefresh()
    mutation.resolve({ data: { ...entry, updated_at: '2026-10-01T12:00:00Z' } })
    await flushPromises()
    expectOneRefreshPerScope()
    expect(useEarningsStore().summaryScope).toEqual({ scope: 'year', year: 2025 })
    expect(useEarningsStore().profitLossScope).toEqual({ scope: 'year', year: 2024 })
  })

  it('keeps a confirmed save successful while exposing both failed aggregate reads without an unhandled rejection', async () => {
    const wrapper = await render()
    await click(wrapper, 'EDIT')
    await field('#ee-amount', '200')
    await submit()
    aggregateError = true
    const confirmed = { ...entry, amount_minor: 20000 }
    mutation.resolve({ data: confirmed })
    await flushPromises()
    expect(useEarningsStore().entries).toEqual([confirmed])
    expect(useEarningsStore().createOpen).toBe(false)
    expectOneRefreshPerScope()
    expect(wrapper.get('.es [role="alert"]').text()).toContain('Could not reach the server.')
    expect(wrapper.get('.pl [role="alert"]').text()).toContain('Could not reach the server.')
    expect(useEarningsStore().summary).toEqual(summary(12000))
    expect(useEarningsStore().profitLossByScope?.data).toEqual(profitLoss(12000))
  })
})
