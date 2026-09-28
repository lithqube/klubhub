import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import EntryList from '../EntryList.vue'
import { useEarningsStore } from '../../../stores/earnings'
import type { Entry } from '../../../types/finance'

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
