import { computed, onMounted, onScopeDispose, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useGigStore } from '../stores/gig'
import { useSocialStore } from '../stores/social'
import { useTracklistStore } from '../stores/tracklist'
import { useEarningsStore } from '../stores/finance'
import { useInvoiceStore } from '../stores/invoice'
import { formatMinor } from '../utils/money'
import type { ScheduledPost } from '../types/social'
import type { Tracklist } from '../types/tracklist'

export function useDashboard() {
  const gigStore = useGigStore()
  const socialStore = useSocialStore()
  const tracklistStore = useTracklistStore()
  const earningsStore = useEarningsStore()
  const invoiceStore = useInvoiceStore()
  const { dashboardGigs: gigs } = storeToRefs(gigStore)
  const { posts } = storeToRefs(socialStore)
  const { pastTracklists } = storeToRefs(tracklistStore)
  const { entries, listError, listLoaded, disabled } = storeToRefs(earningsStore)
  const loading = ref(true)
  const gigError = ref(false)
  const socialError = ref(false)
  const tracklistError = ref(false)
  const config = useRuntimeConfig()
  const demoData = config.public.demo === true || config.public.dashboardMockData === true

  // Invalidate only this component's completions, not shared store operations.
  let active = true
  onScopeDispose(() => {
    active = false
    loading.value = false
  })

  // Social/tracklist legacy actions swallow failures. Keep local unavailable
  // state and guard their installation against teardown as well.
  async function loadList<T>(url: string, target: { value: T[] }, error: { value: boolean }) {
    const before = JSON.stringify(target.value)
    try {
      const response = await $fetch<T[] | { data: T[] | null }>(url)
      const rows = Array.isArray(response) ? response : response.data ?? []
      if (!Array.isArray(rows)) throw new Error('Invalid list response')
      // Do not overwrite a create/update that completed during this read.
      if (active && JSON.stringify(target.value) === before) target.value = rows
    } catch {
      if (active) error.value = true
    }
  }
  async function loadGigs() {
    try {
      await gigStore.fetchDashboardGigs(() => active)
    } catch {
      if (active) gigError.value = true
    }
  }
  onMounted(async () => {
    await Promise.all([
      loadGigs(),
      loadList<ScheduledPost>('/api/v1/social/posts', posts, socialError),
      loadList<Tracklist>('/api/v1/tracklists', pastTracklists, tracklistError),
      earningsStore.fetchEntries(),
      invoiceStore.fetchInvoices(),
    ])
    if (active) loading.value = false
  })

  const financeUnavailable = computed(() => loading.value || !listLoaded.value || !!listError.value || disabled.value)
  const pendingUnavailable = computed(() => loading.value || !invoiceStore.listLoaded || !!invoiceStore.listError || invoiceStore.disabled)
  function currencyTotals(rows: { currency: string; minor: number }[], empty: string): string {
    const sums = new Map<string, number>()
    for (const row of rows) sums.set(row.currency, (sums.get(row.currency) ?? 0) + row.minor)
    return [...sums].sort(([a], [b]) => a.localeCompare(b))
      .map(([currency, minor]) => formatMinor(minor, currency, { locale: 'de-DE' })).join(' · ') || empty
  }
  const finance = computed(() => {
    const now = new Date()
    // Match Phase 5 finance's UTC reporting periods, not gig day display.
    const year = String(now.getUTCFullYear())
    const month = `${year}-${String(now.getUTCMonth() + 1).padStart(2, '0')}`
    const today = `${month}-${String(now.getUTCDate()).padStart(2, '0')}`
    const income = entries.value.filter(e => e.kind === 'income' && e.status === 'active' && !e.deleted_at && e.entry_date.slice(0, 10) <= today)
    const total = (rows: typeof income) => currencyTotals(rows.map(e => ({ currency: e.currency, minor: e.amount_minor })), '0 — NO RECORDED INCOME')
    return {
      mtd: total(income.filter(e => e.entry_date.startsWith(month))),
      ytd: total(income.filter(e => e.entry_date.startsWith(year))),
      pending: currencyTotals(invoiceStore.invoices.filter(i => i.kind === 'invoice' && i.status === 'issued' && !i.replaced_by_invoice_id && i.outstanding_minor > 0)
        .map(i => ({ currency: i.currency, minor: i.outstanding_minor })), '0 — NO OUTSTANDING INVOICES'),
    }
  })
  return { gigs, posts, pastTracklists, loading, gigError, socialError, tracklistError, demoData, financeUnavailable, pendingUnavailable, finance }
}
