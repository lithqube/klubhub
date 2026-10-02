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
  EntryUpdateInput,
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
  // Confirmed persistence is independent of the dialog that initiated it.
  const mutationRevision = ref(0)

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

  let summaryGeneration = 0
  let profitLossGeneration = 0
  let listGeneration = 0
  let recordClock = 0
  const recordGeneration = new Map<string, number>()
  const writeGeneration = new Map<string, number>()
  const readGeneration = new Map<string, number>()
  function beginWrite(id: string): number { const generation = advance(id); writeGeneration.set(id, generation); return generation }
  const pendingDeletes = new Map<string, Entry | undefined>()
  function advance(id: string): number { const g = ++recordClock; recordGeneration.set(id, g); return g }

  async function request<T>(url: string, opts: Record<string, unknown> = {}, isCurrent: () => boolean = () => true): Promise<T> {
    try {
      const res = await $fetch<unknown>(url, opts as never)
      return res as T
    } catch (e) {
      const err = toFinanceError(e)
      if (isCurrent() && err.code === 'unavailable') disabled.value = true
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
    f = { ...f }
    const generation = ++listGeneration
    const started = recordClock
    const isCurrent = () => generation === listGeneration
    listLoading.value = true
    listError.value = null
    try {
      const params = buildEntryQuery(f)
      const res = await request<unknown>(`${BASE}/entries`, { params }, isCurrent)
      if (!isCurrent()) return
      const rows = unwrap<Entry[] | null>(res) ?? []
      const merged = new Map<string, Entry>()
      for (const row of rows) {
        if (pendingDeletes.has(row.id) || (recordGeneration.get(row.id) ?? 0) > started) {
          const local = entries.value.find(e => e.id === row.id)
          if (local) merged.set(row.id, local)
        } else merged.set(row.id, row)
      }
      for (const local of entries.value) {
        if (pendingDeletes.has(local.id) || (recordGeneration.get(local.id) ?? 0) > started) merged.set(local.id, local)
      }
      entries.value = [...merged.values()]
      filterSnapshot.value = { ...f }
      listLoaded.value = true
      disabled.value = false
    } catch (e) {
      if (isCurrent()) listError.value = toFinanceError(e)
    } finally {
      if (isCurrent()) listLoading.value = false
    }
  }

  /** Authoritative read that distinguishes a superseded read (newer read or
   * write for the id took over; benign) from a failed one. */
  async function readEntry(id: string): Promise<{ entry: Entry | null; superseded: boolean }> {
    const recordAtStart = recordGeneration.get(id) ?? 0
    const generation = (readGeneration.get(id) ?? 0) + 1
    readGeneration.set(id, generation)
    const isCurrent = () => readGeneration.get(id) === generation && (recordGeneration.get(id) ?? 0) === recordAtStart && !pendingDeletes.has(id)
    try {
      const res = await request<unknown>(`${BASE}/entries/${id}`, {}, isCurrent)
      const e = unwrap<Entry>(res)
      if (!isCurrent()) return { entry: null, superseded: true }
      upsertEntry(e)
      return { entry: e, superseded: false }
    } catch {
      return { entry: null, superseded: !isCurrent() }
    }
  }

  async function fetchEntry(id: string): Promise<Entry | null> {
    return (await readEntry(id)).entry
  }

  async function refreshSummary(arg: SummaryScopeArg = summaryScope.value): Promise<void> {
    arg = { ...arg }
    const generation = ++summaryGeneration
    const isCurrent = () => generation === summaryGeneration
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
      const res = await request<unknown>(`${BASE}/summary`, { params }, isCurrent)
      if (!isCurrent()) return
      summary.value = unwrap<Record<string, EntryTotals> | null>(res) ?? {}
    } catch (e) {
      if (isCurrent()) summaryError.value = toFinanceError(e)
    } finally {
      if (isCurrent()) summaryLoading.value = false
    }
  }

  async function refreshProfitLoss(arg: ProfitLossArg = profitLossScope.value): Promise<void> {
    arg = { ...arg }
    const generation = ++profitLossGeneration
    const isCurrent = () => generation === profitLossGeneration
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
      const res = await request<unknown>(`${BASE}/profit-loss`, { params }, isCurrent)
      if (!isCurrent()) return
      const data = unwrap<Record<string, ProfitLossTotals> | null>(res) ?? {}
      profitLossByScope.value = { scope: labelFor(arg), data }
    } catch (e) {
      if (isCurrent()) profitLossError.value = toFinanceError(e)
    } finally {
      if (isCurrent()) profitLossLoading.value = false
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

  /** Records metadata from a gig update's response, keyed by gigId (the
   * caller knows which gig it just saved — the Go response itself only
   * carries the reconciliation/entry ids, not the gig id). */
  function rememberReconciliationMetadata(gigId: string, metadata: GigFinanceReconciliation | null | undefined): void {
    if (!metadata || !metadata.id) return
    pendingReconciliationsByGig.value[gigId] = metadata
  }

  /** Pulls the metadata remembered for gigId, if any, and forgets it —
   * meant to be read once per prompt render, not on every re-render. */
  function takeReconciliationMetadata(gigId: string): GigFinanceReconciliation | null {
    const pending = pendingReconciliationsByGig.value[gigId]
    Reflect.deleteProperty(pendingReconciliationsByGig.value, gigId)
    return pending ?? null
  }

  async function resolveReconciliation(
    id: string,
    input: ResolveReconciliationInput,
    gigId?: string,
  ): Promise<unknown> {
    try {
      const res = await request<unknown>(`${BASE}/reconciliations/${id}/resolve`, {
        method: 'POST',
        body: input,
      })
      const data = unwrap<unknown>(res)
      // Refresh both the in-memory reconciliation cache and entries list so
      // every consumer reflects the resolution immediately. The cache is
      // keyed by gigId, not the reconciliation's own id.
      if (gigId) Reflect.deleteProperty(pendingReconciliationsByGig.value, gigId)
      await fetchEntries(filterSnapshot.value)
      return data
    } catch (e) {
      throw toFinanceError(e)
    }
  }

  function upsertEntry(e: Entry): void {
    advance(e.id)
    const idx = entries.value.findIndex((x) => x.id === e.id)
    if (idx === -1) entries.value.unshift(e)
    else entries.value.splice(idx, 1, e)
  }

  async function createEntry(input: EntryCreateInput): Promise<Entry> {
    try {
      const e = unwrap<Entry>(await request(`${BASE}/entries`, { method: 'POST', body: input }))
      upsertEntry(e)
      mutationRevision.value++
      return e
    } catch (err) {
      throw toFinanceError(err)
    }
  }

  async function updateEntry(id: string, input: EntryUpdateInput): Promise<Entry> {
    // FE-2 hardening: the caller owns the field/version snapshot. We must
    // never substitute a newer Pinia cache token for the dialog's original
    // T1 fields + token; the body is exactly what the caller passed.
    const generation = beginWrite(id)
    const isCurrent = () => writeGeneration.get(id) === generation && !pendingDeletes.has(id)
    try {
      const e = unwrap<Entry>(await request(`${BASE}/entries/${id}`, {
        method: 'PUT',
        body: { ...input },
      }, isCurrent))
      if (isCurrent()) upsertEntry(e)
      mutationRevision.value++
      return e
    } catch (err) {
      const ferr = toFinanceError(err)
      // On 409 we refetch just that id so the dialog can decide whether to
      // adopt the new fields + version together; we never reach into the
      // broader list (which would clobber other in-flight work and bypass
      // the conflict banner).
      if (isCurrent() && ferr.code === 'conflict') await fetchEntry(id)
      throw ferr
    }
  }

  async function deleteEntry(id: string, input?: { updated_at?: string }): Promise<void> {
    // Confirmations pass their captured version; retain the cache fallback
    // for legacy non-editor consumers.
    const token = input?.updated_at ?? tokenFor(id)
    beginWrite(id)
    const retained = entries.value.find(e => e.id === id)
    pendingDeletes.set(id, retained)
    try {
      await request(`${BASE}/entries/${id}`, { method: 'DELETE', body: { updated_at: token } })
      advance(id) // Confirmation invalidates reads started during DELETE too.
      entries.value = entries.value.filter((e) => e.id !== id)
      mutationRevision.value++
    } catch (err) {
      advance(id)
      if (retained && !entries.value.some(e => e.id === id)) upsertEntry(retained)
      throw toFinanceError(err)
    } finally {
      pendingDeletes.delete(id)
    }
  }

  async function voidEntry(id: string, input?: { updated_at?: string }): Promise<Entry> {
    const token = input?.updated_at ?? tokenFor(id)
    const generation = beginWrite(id)
    const isCurrent = () => writeGeneration.get(id) === generation && !pendingDeletes.has(id)
    try {
      const e = unwrap<Entry>(await request(`${BASE}/entries/${id}/void`, {
        method: 'POST',
        body: { updated_at: token },
      }, isCurrent))
      if (isCurrent()) upsertEntry(e)
      mutationRevision.value++
      return e
    } catch (err) {
      const ferr = toFinanceError(err)
      if (isCurrent() && ferr.code === 'conflict') await fetchEntry(id)
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

  /** Returns reconciliation metadata for gigId: from the last gig update
   * response if one is cached, else the durable GET (after a refresh). */
  function getPendingReconciliationForGig(gigId: string): unknown {
    const pending = pendingReconciliationsByGig.value[gigId]
    if (pending && pending.id) return pending
    return reconciliationByGig.value[gigId] ?? null
  }

  function clearReconciliationForGig(gigId: string): void {
    Reflect.deleteProperty(reconciliationByGig.value, gigId)
    Reflect.deleteProperty(reconciliationLoading.value, gigId)
    Reflect.deleteProperty(reconciliationErrors.value, gigId)
  }

  return {
    // state
    entries, listLoading, listLoaded, listError, disabled, mutationRevision, filter, summary, profitLossByScope,
    summaryLoading, summaryError, profitLossLoading, profitLossError,
    summaryScope, profitLossScope,
    reconciliationByGig, reconciliationLoading, reconciliationErrors, pendingReconciliationsByGig,
    createOpen, createKind, createPresetGigId, editId,
    // getters
    currencies, filteredEntries, filterCounts, gigById, filterSnapshot,
    // actions
    fetchEntries, fetchEntry, readEntry, refreshSummary, refreshProfitLoss,
    fetchReconciliationForGig, resolveReconciliation, rememberReconciliationMetadata,
    takeReconciliationMetadata, getPendingReconciliationForGig, clearReconciliationForGig,
    createEntry, updateEntry, deleteEntry, voidEntry,
    setFilter, clearFilter, filterAsEntryFilter,
    openCreate, openEdit, setCreateOpen,
  }
})
