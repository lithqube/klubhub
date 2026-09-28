// EntryReconciliationDialog + EntryReconciliationPanel — TDD coverage
// for the FIN-04 / FIN-05 open path. finance.vue used to pass
// `:auto-open="false"` and set `reconDialogOpen` in the parent, but
// nothing bound the dialog's internal `open` ref or called
// `openManually()`, so clicking a banner was a silent no-op.
//
// These tests pin two contracts:
//   1. The dialog renders open when its bound `open` prop is true
//      (v-model pattern — mirroring EarningsWorkspace/InvoiceWorkspace).
//   2. The panel emits `open` with the reconciliation row so the page
//      can flip the bound `open` and the dialog actually opens.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import EntryReconciliationDialog from '../EntryReconciliationDialog.vue'
import EntryReconciliationPanel from '../EntryReconciliationPanel.vue'
import { useEarningsStore } from '../../../stores/earnings'
import type { EntryReconciliation } from '../../../types/finance'

function makeReconciliation(over: Partial<EntryReconciliation> = {}): EntryReconciliation {
  return {
    id: 'rec-1',
    gig_id: 'gig-1',
    entry_id: 'ent-1',
    reason: 'fee_changed',
    allowed_actions: ['update', 'keep'],
    gig_amount_minor: 120000,
    gig_currency: 'EUR',
    gig_payment_status: 'paid',
    entry_updated_at: '2026-09-15T10:00:00Z',
    status: 'pending',
    resolution: null,
    created_at: '2026-09-15T10:00:00Z',
    updated_at: '2026-09-15T10:00:00Z',
    resolved_at: null,
    ...over,
  }
}

describe('EntryReconciliationDialog', () => {
  let mounted: ReturnType<typeof mount>[] = []
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''
    mounted = []
  })
  afterEach(() => {
    for (const w of mounted) w.unmount()
    document.body.innerHTML = ''
  })

  function tracked(mountOpts: Parameters<typeof mount>[1]) {
    const w = mount(EntryReconciliationDialog, { attachTo: document.body, ...mountOpts })
    mounted.push(w)
    return w
  }

  it('renders the dialog content when the bound open prop is true', async () => {
    const store = useEarningsStore()
    const rec = makeReconciliation()
    store.reconciliationByGig['gig-1'] = rec
    tracked({ props: { gigId: 'gig-1', open: true } })
    await flushPromises()
    // Radix portals DialogContent out to document.body. The reason copy
    // is in the open dialog body, and the bound open prop surfaces as
    // data-state="open" on the dialog content node.
    const dialogContent = document.querySelector('[role="dialog"]')
    expect(dialogContent?.getAttribute('data-state')).toBe('open')
    const text = dialogContent?.textContent ?? ''
    expect(text).toContain('FEE CHANGED')
    expect(text).toMatch(/1[,.]?200|€/)  // formatMinor output for 120000 EUR
  })

  it('does not render the dialog content while the bound open prop is false', async () => {
    const store = useEarningsStore()
    store.reconciliationByGig['gig-1'] = makeReconciliation()
    tracked({ props: { gigId: 'gig-1', open: false } })
    await flushPromises()
    // The dialog uses v-if on DialogContent, so when `open=false` the
    // portaled dialog node is not in the DOM at all. The page-level
    // panel test below pins the banner-click → dialog-open path.
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })
})

describe('EntryReconciliationPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''
  })

  it('emits open with the reconciliation row when a banner is clicked', async () => {
    const store = useEarningsStore()
    const rec = makeReconciliation()
    store.reconciliationByGig['gig-1'] = rec
    vi.spyOn(store, 'fetchReconciliationForGig').mockResolvedValue(rec)
    const wrapper = mount(EntryReconciliationPanel, { attachTo: document.body })
    await flushPromises()
    const banner = wrapper.find('.rec-banner')
    expect(banner.exists()).toBe(true)
    await banner.trigger('click')
    expect(wrapper.emitted('open')?.length).toBe(1)
    expect((wrapper.emitted('open')![0]![0] as EntryReconciliation).id).toBe('rec-1')
  })

  it('renders the gig amount using formatMinor (no raw 120000 integer)', async () => {
    const store = useEarningsStore()
    store.reconciliationByGig['gig-1'] = makeReconciliation({ gig_amount_minor: 120000, gig_currency: 'EUR' })
    const wrapper = mount(EntryReconciliationPanel, { attachTo: document.body })
    await flushPromises()
    const text = wrapper.text()
    expect(text).not.toMatch(/\b120000\s*EUR\b/)
    expect(text).toMatch(/1[,.]?200|€/)
  })
})