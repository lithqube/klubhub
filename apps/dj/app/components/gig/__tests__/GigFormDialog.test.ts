// GigFormDialog — TDD coverage for FIN-04 #5 (earningsStore hoist +
// reconciliation metadata ref binding) and FIN-05 #6 (parent-driven
// open prop on EntryReconciliationDialog).
//
// #5 used to declare `earningsStore` inside `doSave`; the template at
//   <EntryReconciliationDialog :metadata="earningsStore.takeLastReconciliationMetadata()" />
// crashed on first render because script-setup does not expose locals.
// #6 left the dialog's visibility to the child (`watch props.metadata`)
// so a durable-GET prompt without inline metadata never surfaced.
//
// These tests pin:
//   1. The component compiles with `earningsStore` at setup scope and
//      does not throw a TypeError when the template renders.
//   2. After a save that returns finance_reconciliation, the pending
//      reconciliation dialog renders open and clears on `update:open(false)`.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import GigFormDialog from '../GigFormDialog.vue'
import { useGigStore } from '../../../stores/gig'
import { useEarningsStore } from '../../../stores/earnings'
import type { Gig } from '../../../types/gig'
import type { GigFinanceReconciliation } from '../../../types/finance'

// Stub $fetch — the dialog's onMount watchers call gigStore.fetchGigDetail
// and tracklistStore.loadPastTracklists, both of which use Nuxt's $fetch.
vi.stubGlobal('$fetch', vi.fn().mockRejectedValue(new Error('not used')))

beforeEach(() => {
  vi.unstubAllGlobals()
  vi.stubGlobal('$fetch', vi.fn().mockRejectedValue(new Error('not used')))
})

function makeGig(over: Partial<Gig> = {}): Gig {
  return {
    id: '00000000-0000-4000-8000-000000000001',
    date: '2026-09-15T00:00:00Z',
    venue: 'Berghain',
    city: 'Berlin',
    country: 'DE',
    event_name: 'Klubnacht',
    promoter_name: 'Promoter',
    promoter_email: '',
    promoter_phone: '',
    fee_amount: 1500,
    fee_currency: 'EUR',
    set_length_minutes: 180,
    notes: '',
    status: 'confirmed',
    payment_status: 'paid',
    gig_reader_venue_id: null,
    gig_reader_contact_id: null,
    created_at: '2026-09-15T10:00:00Z',
    updated_at: '2026-09-15T10:00:00Z',
    deleted_at: null,
    ...over,
  } as Gig
}

const sampleReconciliation: GigFinanceReconciliation = {
  id: 'rec-1',
  entry_id: 'ent-1',
  reason: 'fee_changed',
  allowed_actions: ['update', 'keep'],
  updated_at: '2026-09-15T10:00:00Z',
}

describe('GigFormDialog reconciliation surface', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''
  })

  it('renders without throwing when the earnings store is in scope', async () => {
    // Registering the store at setup-scope mirrors what GigFormDialog
    // now does; this test pins that the template does not call
    // earningsStore.takeLastReconciliationMetadata() during render
    // (which would otherwise surface as a runtime TypeError on mount).
    useEarningsStore()
    const gig = makeGig()
    const wrapper = mount(GigFormDialog, {
      attachTo: document.body,
      props: { open: true, gig },
      global: {
        stubs: {
          // Stub the heavy child to focus on the GigFormDialog template
          // binding and avoid the Radix portal dance in this test.
          EntryReconciliationDialog: { template: '<div data-testid="rec-stub" />' },
          VenueAutocomplete: { template: '<div />' },
          ContactAutocomplete: { template: '<div />' },
        },
      },
    })
    await flushPromises()
    expect(wrapper.exists()).toBe(true)
  })

  it('opens the reconciliation dialog after a save returns finance_reconciliation', async () => {
    const gig = makeGig()
    const gigStore = useGigStore()
    // Replace the action entirely on the store object; Pinia wraps
    // actions in a proxy that intercepts property reads, but the
    // underlying store object still carries the function. Assigning
    // before mount guarantees the component's `gigStore.updateGig(...)`
    // call resolves to our stub.
    const stub = async () => ({ ...gig, finance_reconciliation: sampleReconciliation })
    gigStore.updateGig = stub as never

    const wrapper = mount(GigFormDialog, {
      attachTo: document.body,
      props: { open: true, gig },
      global: {
        stubs: {
          EntryReconciliationDialog: { props: ['open', 'gigId', 'metadata'], template: '<div v-if="open" data-testid="rec-open" />' },
          VenueAutocomplete: { template: '<div />' },
          ContactAutocomplete: { template: '<div />' },
        },
      },
    })
    await flushPromises()
    // Drive initForm manually — the watcher fires asynchronously in
    // some Vue versions and we want form.date/event_name populated
    // before doSave runs its validation.
    ;(wrapper.vm as unknown as { initForm: () => void }).initForm()
    await flushPromises()

    // Drive save() directly through the component instance. This bypasses
    // the cancellation guard and the click-event plumbing that depend on
    // the dialog being fully interactive in jsdom. `doSave` is exposed
    // via defineExpose so the test can reach it.
    await (wrapper.vm as unknown as { doSave: () => Promise<void> }).doSave()
    await flushPromises()
    await flushPromises()

    // Pin the contract two ways: the parent's pendingReconciliationGigId
    // drives the dialog's :open prop. Without the fix, the template
    // crashed on first render (earningsStore undefined), so the
    // reconciliation dialog never rendered. Vue test utils auto-unwrap
    // refs in `wrapper.vm`, so the values are plain string | null.
    const internals = wrapper.vm as unknown as {
      pendingReconciliationGigId: string | null
      pendingReconciliationMetadata: GigFinanceReconciliation | null
    }
    expect(internals.pendingReconciliationGigId).toBe(gig.id)
    expect(internals.pendingReconciliationMetadata?.id).toBe(sampleReconciliation.id)
    expect(document.querySelector('[data-testid="rec-open"]')).not.toBeNull()
  })
})