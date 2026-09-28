// Phase 5 earnings store (FIN-01…FIN-09). Mirrors `stores/invoice.ts`
// conventions: setup-function Pinia store, $fetch inside actions only,
// storeToRefs in templates, FinanceApiError surfaced from every server
// reply (reuses toFinanceError from invoice.ts).
//
// Endpoints:
//   GET    /api/v1/finance/entries?kind&status&currency&category&gig_id&from&to
//   POST   /api/v1/finance/entries
//   GET    /api/v1/finance/entries/:id
//   PUT    /api/v1/finance/entries/:id
//   DELETE /api/v1/finance/entries/:id
//   POST   /api/v1/finance/entries/:id/void
//   GET    /api/v1/finance/summary?scope=month&year&month  (or ?from&to)
//   GET    /api/v1/finance/profit-loss?scope=gig&gig_id
//                                   ?scope=month&year&month
//                                   ?scope=year&year
//   GET    /api/v1/finance/reconciliations?gig_id=
//   POST   /api/v1/finance/reconciliations/:id/resolve

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type {
  Entry,
  EntryCreateInput,
  EntryFilter,
  EntryKind,
  EntryTotals,
  GigFinanceReconciliation,
  ProfitLossTotals,
  ResolveReconciliationInput,
} from '../types/finance'
import { toFinanceError, type FinanceApiError } from './invoice'

const BASE = '/api/v1/finance'

export type SummaryScopeArg =
  | { scope: 'month'; year: number; month: number }
  | { scope: 'year'; year: number }
  | { from: string; to: string }

export type ProfitLossArg =
  | { scope: 'gig'; gig_id: string }
  | { scope: 'month'; year: number; month: number }
  | { scope: 'year'; year: number }

function unwrap<T>(res: unknown): T {
  if (res && typeof res === 'object' && 'data' in (res as object)) return (res as { data: T }).data
  return res as T
}

function currentYear(): number {
  return new Date().getFullYear()
}

function currentMonth(): number {
  return new Date().getUTCMonth() + 1
}

export const useEarningsStore = defineStore('earnings', () => {
  // ── Entries list ──
  const entries = ref<Entry[]>([])
  const listLoading = ref(false)
  const listLoaded = ref(false)
  const listError = ref<FinanceApiError | null>(null)
  const disabled = ref(false)

  const filter = ref<{ kind: EntryKind | ''; currency: string; category: string; status: string; gig_id: string; from: string; to: string }>({
    kind: '', currency: '', category: '', status: '', gig_id: '', from: '', to: '',
  })
  const filterSnapshot = ref<EntryFilter>({})

  // ── Summary / P&L caches ──
  const summary = ref<Record<string, EntryTotals>>({})
  const profitLossByScope = ref<{ scope: string; data: Record<string, ProfitLossTotals> } | null>(null)
  const summaryLoading = ref(false)
  const summaryError = ref<FinanceApiError | null>(null)
  const profitLossLoading = ref(false)
  const profitLossError = ref<FinanceApiError | null>(null)

  /** Scope used when the page calls refreshSummary without an explicit arg. */
  const summaryScope = ref<SummaryScopeArg>({ scope: 'year', year: currentYear() })
  const profitLossScope = ref<ProfitLossArg>({ scope: 'month', year: currentYear(), month: currentMonth() })

  // ── Reconciliations (FIN-04 / FIN-05) ──
  const reconciliationByGig = ref<Record<string, unknown>>({})
  const reconciliationLoading = ref<Record<string, boolean>>({})
  const reconciliationErrors = ref<Record<string, FinanceApiError>>({})
  // Used by GigFormDialog to show the prompt after a save. Maps gigId → metadata.
  const pendingReconciliationsByGig = ref<Record<string, GigFinanceReconciliation>>({})

  // ── Workspace UI shared with finance.vue ──
  const createOpen = ref(false)
  const createKind = ref<EntryKind>('income')
  const createPresetGigId = ref<string | null>(null)
  const editId = ref<string | null>(null)

  const currencies = computed(() => {
    const set = new Set<string>()
    for (const e of entries.value) set.add(e.currency)
    for (const c of Object.keys(summary.value)) set.add(c)
    return [...set].sort()
  })

  const filteredEntries = computed(() => {
    // The list endpoint does the server-side filter, but we keep a
    // client-side equivalent so the chip counts and inline filters match.
    const f = filterSnapshot.value
    return entries.value.filter((e) => {
      if (f.kind && e.kind !== f.kind) return false
      if (f.status && e.status !== f.status) return false
      if (f.currency && e.currency !== f.currency) return false
      if (f.category && e.category !== f.category) return false
      if (f.gig_id && e.gig_id !== f.gig_id) return false
      if (f.from && e.entry_date < f.from) return false
      if (f.to && e.entry_date >= f.to) return false
      return true
    })
  })

  const filterCounts = computed(() => {
    const all = entries.value
    return {
      all: all.length,
      income: all.filter((e) => e.kind === 'income' && !e.deleted_at && e.status === 'active').length,
      expense: all.filter((e) => e.kind === 'expense' && !e.deleted_at && e.status === 'active').length,
      voided: all.filter((e) => e.status === 'voided').length,
    }
  })

  const gigById = computed(() => {
    const map = new Map<string, Entry>()
    for (const e of entries.value) {
      if (e.gig_id && e.source_kind === 'gig_payment' && !e.deleted_at && e.status === 'active') {
        if (!map.has(e.gig_id)) map.set(e.gig_id, e)
      }
    }
    return map
  })

  async function request<T>(url: string, opts: Record<string, unknown> = {}): Promise<T> {
    try {
      const res = await $fetch<unknown>(url, opts as never)
      return res as T
    } catch (e) {
      const err = toFinanceError(e)
      if (err.code === 'unavailable') disabled.value = true
      throw err
    }
  }

  function buildEntryQuery(f: EntryFilter): Record<string, string> {
    const out: Record<string, string> = {}
    if (f.kind) out.kind = f.kind
    if (f.status) out.status = f.status
    if (f.currency) out.currency = f.currency
    if (f.category) out.category = f.category
    if (f.gig_id) out.gig_id = f.gig_id
    if (f.from) out.from = f.from
    if (f.to) out.to = f.to
    return out
  }

  function tokenFor(id: string): string {
    const e = entries.value.find((x) => x.id === id)
    return e?.updated_at ?? ''
  }

  async function fetchEntries(f: EntryFilter = {}): Promise<void> {
    listLoading.value = true
    listError.value = null
    try {
      const params = buildEntryQuery(f)
      const res = await request<unknown>(`${BASE}/entries`, { params })
      entries.value = unwrap<Entry[] | null>(res) ?? []
      filterSnapshot.value = { ...f }
      listLoaded.value = true
      disabled.value = false
    } catch (e) {
      listError.value = toFinanceError(e)
    } finally {
      listLoading.value = false
    }
  }

  async function fetchEntry(id: string): Promise<Entry | null> {
    try {
      const res = await request<unknown>(`${BASE}/entries/${id}`)
      const e = unwrap<Entry>(res)
      upsertEntry(e)
      return e
    } catch {
      return null
    }
  }

  async function refreshSummary(arg: SummaryScopeArg = summaryScope.value): Promise<void> {
    summaryLoading.value = true
    summaryError.value = null
    summaryScope.value = arg
    try {
      const params: Record<string, string> = {}
      if ('scope' in arg) {
        params.scope = arg.scope
        if (arg.scope === 'month') {
          params.year = String(arg.year)
          params.month = String(arg.month)
        } else {
          params.year = String(arg.year)
        }
      } else {
        params.from = arg.from
        params.to = arg.to
      }
      const res = await request<unknown>(`${BASE}/summary`, { params })
      summary.value = unwrap<Record<string, EntryTotals> | null>(res) ?? {}
    } catch (e) {
      summaryError.value = toFinanceError(e)
    } finally {
      summaryLoading.value = false
    }
  }

  async function refreshProfitLoss(arg: ProfitLossArg = profitLossScope.value): Promise<void> {
    profitLossLoading.value = true
    profitLossError.value = null
    profitLossScope.value = arg
    try {
      const params: Record<string, string> = { scope: arg.scope }
      if (arg.scope === 'gig') params.gig_id = arg.gig_id
      if (arg.scope === 'month') {
        params.year = String(arg.year)
        params.month = String(arg.month)
      }
      if (arg.scope === 'year') params.year = String(arg.year)
      const res = await request<unknown>(`${BASE}/profit-loss`, { params })
      const data = unwrap<Record<string, ProfitLossTotals> | null>(res) ?? {}
      profitLossByScope.value = { scope: labelFor(arg), data }
    } catch (e) {
      profitLossError.value = toFinanceError(e)
    } finally {
      profitLossLoading.value = false
    }
  }

  function labelFor(arg: ProfitLossArg): string {
    if (arg.scope === 'gig') return `gig:${arg.gig_id}`
    if (arg.scope === 'month') return `${arg.year}-${String(arg.month).padStart(2, '0')}`
    return String(arg.year)
  }

  async function fetchReconciliationForGig(gigId: string): Promise<unknown> {
    reconciliationLoading.value[gigId] = true
    Reflect.deleteProperty(reconciliationErrors.value, gigId)
    try {
      const res = await request<unknown>(`${BASE}/reconciliations`, { params: { gig_id: gigId } })
      const data = unwrap<unknown>(res)
      reconciliationByGig.value[gigId] = data
      Reflect.deleteProperty(pendingReconciliationsByGig.value, gigId)
      return data
    } catch (e) {
      const err = toFinanceError(e)
      if (err.code === 'not_found') {
        reconciliationByGig.value[gigId] = null
        Reflect.deleteProperty(pendingReconciliationsByGig.value, gigId)
      } else {
        reconciliationErrors.value[gigId] = err
      }
      return null
    } finally {
      reconciliationLoading.value[gigId] = false
    }
  }

  function rememberReconciliationMetadata(metadata: GigFinanceReconciliation | null | undefined): void {
    if (!metadata || !metadata.id) return
    pendingReconciliationsByGig.value[metadata.id] = metadata  // canonical store by reconciliation id
    // Also index by gig_id for the page-level surface.
    // The Go response carries the gig_id only through `id`/`entry_id` —
    // the caller resolves it (GigFormDialog knows the gig).
    pendingReconciliationsByGig.value['_last'] = metadata
  }

  /** Pull the metadata attached to the last gig update response, if any. */
  function takeLastReconciliationMetadata(): GigFinanceReconciliation | null {
    const last = pendingReconciliationsByGig.value['_last']
    Reflect.deleteProperty(pendingReconciliationsByGig.value, "_last")
    return last ?? null
  }

  async function resolveReconciliation(
    id: string,
    input: ResolveReconciliationInput,
  ): Promise<unknown> {
    try {
      const res = await request<unknown>(`${BASE}/reconciliations/${id}/resolve`, {
        method: 'POST',
        body: input,
      })
      const data = unwrap<unknown>(res)
      // Refresh both the in-memory reconciliation cache and entries list so
      // every consumer reflects the resolution immediately.
      Reflect.deleteProperty(pendingReconciliationsByGig.value, id)
      await fetchEntries(filterSnapshot.value)
      return data
    } catch (e) {
      throw toFinanceError(e)
    }
  }

  function upsertEntry(e: Entry): void {
    const idx = entries.value.findIndex((x) => x.id === e.id)
    if (idx === -1) entries.value.unshift(e)
    else entries.value.splice(idx, 1, e)
  }

  async function createEntry(input: EntryCreateInput): Promise<Entry> {
    try {
      const e = unwrap<Entry>(await request(`${BASE}/entries`, { method: 'POST', body: input }))
      upsertEntry(e)
      return e
    } catch (err) {
      throw toFinanceError(err)
    }
  }

  async function updateEntry(id: string, input: EntryCreateInput): Promise<Entry> {
    const token = tokenFor(id)
    try {
      const e = unwrap<Entry>(await request(`${BASE}/entries/${id}`, {
        method: 'PUT',
        body: { ...input, updated_at: token },
      }))
      upsertEntry(e)
      return e
    } catch (err) {
      const ferr = toFinanceError(err)
      if (ferr.code === 'conflict') await fetchEntries(filterSnapshot.value)
      throw ferr
    }
  }

  async function deleteEntry(id: string): Promise<void> {
    const token = tokenFor(id)
    try {
      await request(`${BASE}/entries/${id}`, { method: 'DELETE', body: { updated_at: token } })
      entries.value = entries.value.filter((e) => e.id !== id)
    } catch (err) {
      const ferr = toFinanceError(err)
      if (ferr.code === 'conflict') await fetchEntries(filterSnapshot.value)
      throw ferr
    }
  }

  async function voidEntry(id: string): Promise<Entry> {
    const token = tokenFor(id)
    try {
      const e = unwrap<Entry>(await request(`${BASE}/entries/${id}/void`, {
        method: 'POST',
        body: { updated_at: token },
      }))
      upsertEntry(e)
      return e
    } catch (err) {
      const ferr = toFinanceError(err)
      if (ferr.code === 'conflict') await fetchEntries(filterSnapshot.value)
      throw ferr
    }
  }

  function setFilter<K extends keyof typeof filter.value>(key: K, val: typeof filter.value[K]): void {
    filter.value[key] = val
  }

  function clearFilter(): void {
    filter.value = { kind: '', currency: '', category: '', status: '', gig_id: '', from: '', to: '' }
  }

  function filterAsEntryFilter(): EntryFilter {
    const f: EntryFilter = {}
    if (filter.value.kind) f.kind = filter.value.kind as EntryKind
    if (filter.value.currency) f.currency = filter.value.currency
    if (filter.value.category) f.category = filter.value.category
    if (filter.value.status) f.status = filter.value.status as Entry['status']
    if (filter.value.gig_id) f.gig_id = filter.value.gig_id
    if (filter.value.from) f.from = filter.value.from
    if (filter.value.to) f.to = filter.value.to
    return f
  }

  function openCreate(kind: EntryKind = 'income', presetGigId: string | null = null): void {
    createKind.value = kind
    createPresetGigId.value = presetGigId
    editId.value = null
    createOpen.value = true
  }

  function openEdit(id: string): void {
    editId.value = id
    createOpen.value = true
  }

  function setCreateOpen(open: boolean): void {
    createOpen.value = open
    if (!open) {
      createPresetGigId.value = null
      editId.value = null
    }
  }

  /** Returns reconciliation metadata stored after the last gig update response. */
  function getPendingReconciliationForGig(gigId: string): unknown {
    const last = pendingReconciliationsByGig.value['_last']
    if (last && last.id) return last
    // Fall back to the cached GET (after page refresh).
    return reconciliationByGig.value[gigId] ?? null
  }

  function clearReconciliationForGig(gigId: string): void {
    Reflect.deleteProperty(reconciliationByGig.value, gigId)
    Reflect.deleteProperty(reconciliationLoading.value, gigId)
    Reflect.deleteProperty(reconciliationErrors.value, gigId)
  }

  return {
    // state
    entries, listLoading, listLoaded, listError, disabled, filter, summary, profitLossByScope,
    summaryLoading, summaryError, profitLossLoading, profitLossError,
    summaryScope, profitLossScope,
    reconciliationByGig, reconciliationLoading, reconciliationErrors, pendingReconciliationsByGig,
    createOpen, createKind, createPresetGigId, editId,
    // getters
    currencies, filteredEntries, filterCounts, gigById, filterSnapshot,
    // actions
    fetchEntries, fetchEntry, refreshSummary, refreshProfitLoss,
    fetchReconciliationForGig, resolveReconciliation, rememberReconciliationMetadata,
    takeLastReconciliationMetadata, getPendingReconciliationForGig, clearReconciliationForGig,
    createEntry, updateEntry, deleteEntry, voidEntry,
    setFilter, clearFilter, filterAsEntryFilter,
    openCreate, openEdit, setCreateOpen,
  }
})
