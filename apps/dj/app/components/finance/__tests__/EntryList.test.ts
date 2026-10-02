import { describe, it, expect, beforeEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import EntryList from '../EntryList.vue'
import { useEarningsStore } from '../../../stores/earnings'
import type { Entry } from '../../../types/finance'
import { makeAttachment, makeEntry } from './fixtures'

function entry(over: Partial<Entry> = {}): Entry {
  return {
    id: 'e-1',
    kind: 'income',
    amount_minor: 100000,
    currency: 'EUR',
    category: 'gig_fee',
    entry_date: '2026-09-15',
    description: 'DJ set',
    notes: '',
    gig_id: null,
    status: 'active',
    auto_generated: false,
    source_kind: 'manual',
    source_id: null,
    source_amount_minor: null,
    source_currency: null,
    source_description: '',
    created_at: 't',
    updated_at: 't',
    deleted_at: null,
    attachment_count: 0,
    ...over,
  }
}

describe('EntryList', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''
  })

  it('renders rows with kind/category/source metadata', () => {
    const store = useEarningsStore()
    store.entries = [
      entry({ id: 'i', kind: 'income', description: 'DJ set', amount_minor: 150000, currency: 'EUR', category: 'gig_fee' }),
      entry({ id: 'e', kind: 'expense', notes: 'cable', amount_minor: 25000, currency: 'EUR', category: 'gear', description: '' }),
      entry({ id: 'a', kind: 'income', auto_generated: true, source_kind: 'gig_payment', amount_minor: 200000, currency: 'USD' }),
    ]
    store.listLoaded = true
    const wrapper = mount(EntryList, { attachTo: document.body, global: { plugins: [store.$pinia] } })
    const rows = wrapper.findAll('.el-row')
    expect(rows).toHaveLength(3)
    expect(wrapper.text()).toContain('FIN-10')
    expect(wrapper.text()).toContain('GIG PAYMENT')
  })

  it('emits edit / void / delete with the matching entry when buttons are clicked', async () => {
    const store = useEarningsStore()
    store.entries = [entry()]
    store.listLoaded = true
    const wrapper = mount(EntryList, { attachTo: document.body })
    await wrapper.findAll('button').find((b) => b.text().trim() === 'EDIT')!.trigger('click')
    await wrapper.findAll('button').find((b) => b.text().trim() === 'VOID')!.trigger('click')
    await wrapper.findAll('button').find((b) => b.text().trim() === 'DELETE')!.trigger('click')
    expect(wrapper.emitted('edit')?.length).toBe(1)
    expect(wrapper.emitted('void')?.length).toBe(1)
    expect(wrapper.emitted('delete')?.length).toBe(1)
    expect((wrapper.emitted('edit')![0]![0] as Entry).id).toBe('e-1')
  })

  it('keeps auto-generated income read-only (no edit / delete buttons)', () => {
    const store = useEarningsStore()
    store.entries = [entry({ id: 'auto', auto_generated: true, source_kind: 'gig_payment', category: 'gig_fee' })]
    store.listLoaded = true
    const wrapper = mount(EntryList, { attachTo: document.body })
    expect(wrapper.text()).not.toContain('EDIT')
    expect(wrapper.text()).not.toContain('DELETE')
    expect(wrapper.text()).toContain('VOID')
    expect(wrapper.find('.el-row-auto').exists()).toBe(true)
  })

  it('kind chips toggle the store filter', async () => {
    const store = useEarningsStore()
    store.entries = []
    store.listLoaded = true
    const wrapper = mount(EntryList, { attachTo: document.body })
    await wrapper.findAll('.el-chip').find((b) => b.text().includes('INCOME'))!.trigger('click')
    expect(store.filter.kind).toBe('income')
  })

  it('shows the empty-state when there are no entries yet', () => {
    const store = useEarningsStore()
    store.entries = []
    store.listLoaded = true
    const wrapper = mount(EntryList, { attachTo: document.body })
    expect(wrapper.text()).toContain('NO ENTRIES YET')
  })

  // FIN-04 #4 follow-up: clicking a kind chip and changing the status /
  // currency selects must refetch the entries list with the new filter —
  // otherwise the chips "light up" but the rows don't change. Cover all
  // four setters in one test to pin the same applyFilters() helper for
  // every code path (06cf84b introduced applyFilters; this PR exercises
  // each public setter).
  it('refetches entries whenever a kind / status / currency chip is clicked', async () => {
    const store = useEarningsStore()
    store.listLoaded = true
    const fetchSpy = vi.spyOn(store, 'fetchEntries').mockResolvedValue()
    const wrapper = mount(EntryList, { attachTo: document.body })

    await wrapper.findAll('.el-chip').find((b) => b.text().includes('INCOME'))!.trigger('click')
    expect(fetchSpy).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'income' }))

    // The chip-set helpers are the only public setters; the <select>
    // elements route through them via @change. Call them directly so
    // the test stays resilient to render-order changes in the toolbar.
    ;(wrapper.vm as unknown as { setStatus: (s: 'active' | 'voided' | '') => void }).setStatus('active')
    expect(fetchSpy).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'active' }))

    ;(wrapper.vm as unknown as { setCurrency: (c: string) => void }).setCurrency('USD')
    expect(fetchSpy).toHaveBeenLastCalledWith(expect.objectContaining({ currency: 'USD' }))

    ;(wrapper.vm as unknown as { clearFilters: () => void }).clearFilters()
    expect(fetchSpy.mock.calls.length).toBeGreaterThanOrEqual(4)
    expect(fetchSpy).toHaveBeenLastCalledWith({})
  })
})

describe('EntryList receipts', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''
  })

  const metaOf = (w: ReturnType<typeof mount>, id: string) =>
    w.get(`.el-row[data-entry-id="${id}"]`)

  it('shows a paperclip with the count on rows that have receipts, and none on rows without', () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ id: 'two', attachment_count: 2 }), makeEntry({ id: 'none', attachment_count: 0 })]
    store.listLoaded = true
    const wrapper = mount(EntryList, { attachTo: document.body })
    const clip = metaOf(wrapper, 'two').find('.el-clip')
    expect(clip.exists()).toBe(true)
    expect(clip.text()).toBe('2')
    expect(clip.attributes('aria-label')).toBe('2 receipts, show')
    expect(clip.attributes('aria-expanded')).toBe('false')
    expect(metaOf(wrapper, 'none').find('.el-clip').exists()).toBe(false)
  })

  it('opens the receipts under the row on demand, with links that open in a new tab', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ id: 'e-1', attachment_count: 2 })]
    store.listLoaded = true
    const fetchMock = vi.fn().mockResolvedValue({ data: [
      makeAttachment({ id: 'a-1', filename: 'hotel.png' }),
      makeAttachment({ id: 'a-2', filename: 'train.pdf', mime_type: 'application/pdf' }),
    ] })
    vi.stubGlobal('$fetch', fetchMock)
    const wrapper = mount(EntryList, { attachTo: document.body })
    expect(fetchMock).not.toHaveBeenCalled() // the list only carries a count

    await wrapper.get('.el-clip').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/entries/e-1/attachments', {})
    expect(wrapper.get('.el-clip').attributes('aria-expanded')).toBe('true')
    const detail = wrapper.get('.el-detail')
    expect(wrapper.get('.el-clip').attributes('aria-controls')).toBe(detail.attributes('id'))
    const links = detail.findAll('a')
    expect(links.map((a) => a.attributes('href'))).toEqual([
      '/api/v1/finance/entries/e-1/attachments/a-1',
      '/api/v1/finance/entries/e-1/attachments/a-2',
    ])
    for (const a of links) {
      expect(a.attributes('target')).toBe('_blank')
      expect(a.attributes('rel')).toBe('noopener')
    }
    expect(detail.find('img').attributes('src')).toBe('/api/v1/finance/entries/e-1/attachments/a-1?inline=1')
    expect(detail.findAll('img')).toHaveLength(1) // the PDF has a glyph instead

    await wrapper.get('.el-clip').trigger('click')
    expect(wrapper.find('.el-detail').exists()).toBe(false)
    vi.unstubAllGlobals()
  })

  it('says so when the receipts cannot be loaded, and retries', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ id: 'e-1', attachment_count: 1 })]
    store.listLoaded = true
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(Object.assign(new Error('x'), { statusCode: 404, data: { error: 'not_found', message: 'finance entry not found' } }))
      .mockResolvedValueOnce({ data: [makeAttachment()] })
    vi.stubGlobal('$fetch', fetchMock)
    const wrapper = mount(EntryList, { attachTo: document.body })
    await wrapper.get('.el-clip').trigger('click')
    await flushPromises()
    expect(wrapper.get('.el-detail [role="alert"]').text()).toContain('finance entry not found')
    await wrapper.get('.el-detail button').trigger('click')
    await flushPromises()
    expect(wrapper.get('.el-detail').text()).toContain('receipt.png')
    vi.unstubAllGlobals()
  })

  it('labels an active expense without receipts NO RECEIPT, quietly (muted token, no badge)', () => {
    const store = useEarningsStore()
    store.entries = [
      makeEntry({ id: 'bare', kind: 'expense', attachment_count: 0 }),
      makeEntry({ id: 'covered', kind: 'expense', attachment_count: 1 }),
      makeEntry({ id: 'income', kind: 'income', description: 'DJ set', notes: '', attachment_count: 0 }),
      makeEntry({ id: 'voided', kind: 'expense', status: 'voided', attachment_count: 0 }),
    ]
    store.listLoaded = true
    const wrapper = mount(EntryList, { attachTo: document.body })
    const label = (id: string) => metaOf(wrapper, id).find('.el-noreceipt')
    expect(label('bare').exists()).toBe(true)
    expect(label('bare').text()).toBe('NO RECEIPT')
    expect(label('bare').classes()).not.toContain('badge-hud')
    expect(label('covered').exists()).toBe(false)
    expect(label('income').exists()).toBe(false)
    expect(label('voided').exists()).toBe(false)
  })

  it('has a NO RECEIPT filter chip that asks the server for receipt=missing', async () => {
    const store = useEarningsStore()
    store.listLoaded = true
    const fetchSpy = vi.spyOn(store, 'fetchEntries').mockResolvedValue()
    const wrapper = mount(EntryList, { attachTo: document.body })
    const chip = wrapper.get('[data-testid="el-chip-no-receipt"]')
    expect(chip.text()).toContain('NO RECEIPT')
    expect(chip.attributes('aria-pressed')).toBe('false')

    await chip.trigger('click')
    expect(store.filter.receipt).toBe('missing')
    expect(fetchSpy).toHaveBeenLastCalledWith({ receipt: 'missing' })
    expect(wrapper.get('[data-testid="el-chip-no-receipt"]').attributes('aria-pressed')).toBe('true')

    // Pressing it again lifts the filter.
    await wrapper.get('[data-testid="el-chip-no-receipt"]').trigger('click')
    expect(store.filter.receipt).toBe('')
    expect(fetchSpy).toHaveBeenLastCalledWith({})
  })

  it('counts only the active expenses without receipts on the chip', () => {
    const store = useEarningsStore()
    store.entries = [
      makeEntry({ id: 'a' }),
      makeEntry({ id: 'b' }),
      makeEntry({ id: 'c', attachment_count: 3 }),
      makeEntry({ id: 'income', kind: 'income', description: 'DJ set', notes: '', attachment_count: 0 }),
      makeEntry({ id: 'voided', status: 'voided', attachment_count: 0 }),
    ]
    store.listLoaded = true
    const wrapper = mount(EntryList, { attachTo: document.body })
    expect(wrapper.get('[data-testid="el-chip-no-receipt"] .el-count').text()).toBe('2')
  })

  it('pressing INCOME while NO RECEIPT is on drops NO RECEIPT; pressing NO RECEIPT from INCOME leaves income', async () => {
    const store = useEarningsStore()
    store.listLoaded = true
    const fetchSpy = vi.spyOn(store, 'fetchEntries').mockResolvedValue()
    const wrapper = mount(EntryList, { attachTo: document.body })
    const chip = () => wrapper.get('[data-testid="el-chip-no-receipt"]')
    const kindChip = (label: string) => wrapper.findAll('.el-chips .el-chip').find((c) => c.text().startsWith(label))!

    await chip().trigger('click')
    await kindChip('INCOME').trigger('click')
    expect(store.filter).toMatchObject({ kind: 'income', receipt: '' })
    expect(fetchSpy).toHaveBeenLastCalledWith({ kind: 'income' })
    expect(chip().attributes('aria-pressed')).toBe('false')

    await chip().trigger('click')
    expect(store.filter).toMatchObject({ kind: '', receipt: 'missing' })
    expect(fetchSpy).toHaveBeenLastCalledWith({ receipt: 'missing' })
  })

  it('RESET clears the receipt filter along with the others', async () => {
    const store = useEarningsStore()
    store.listLoaded = true
    const fetchSpy = vi.spyOn(store, 'fetchEntries').mockResolvedValue()
    const wrapper = mount(EntryList, { attachTo: document.body })
    await wrapper.get('[data-testid="el-chip-no-receipt"]').trigger('click')
    const reset = wrapper.findAll('button').find((b) => b.text().trim() === 'RESET')!
    expect(reset).toBeDefined()
    await reset.trigger('click')
    expect(store.filter.receipt).toBe('')
    expect(fetchSpy).toHaveBeenLastCalledWith({})
    expect(wrapper.findAll('button').some((b) => b.text().trim() === 'RESET')).toBe(false)
  })

  it('keeps showing the rows while a refetch is in flight instead of swapping in the skeleton', () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ id: 'e-1' })]
    store.listLoaded = true
    store.listLoading = true
    const wrapper = mount(EntryList, { attachTo: document.body })
    expect(wrapper.find('.el-skel').exists()).toBe(false)
    expect(wrapper.findAll('.el-row')).toHaveLength(1)
  })
})
