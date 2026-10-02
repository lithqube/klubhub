import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type {
  FinanceErrorBody,
  FinanceErrorCode,
  Invoice,
  InvoiceCreateInput,
  InvoiceGigOption,
  InvoiceLine,
  InvoiceListFilter,
  InvoiceListQuery,
  InvoiceSummary,
  InvoiceUpdateInput,
  IssueCheck,
  IssueProblem,
  Party,
  Payment,
  PaymentCreateInput,
  TaxNotes,
  TaxNotesMap,
  TaxSuggestion,
} from '../types/finance'
import { isActiveInvoice, matchesFilter, todayIso, LIST_FILTERS } from '../utils/invoiceDisplay'

const BASE = '/api/v1/finance'

export const CONFLICT_NOTICE = 'This invoice changed elsewhere. Showing latest; review and retry.'
export const BAD_STATE_NOTICE = 'This invoice is no longer in a state that allows that action. Showing latest.'
export const DISABLED_MESSAGE = "Invoicing isn't enabled on this server."

/** Typed error for every finance request; built from `{error, message, problems?}`. */
export class FinanceApiError extends Error {
  readonly status: number
  readonly code: FinanceErrorCode
  readonly problems: IssueProblem[]
  /** Field named by a 400 validation message, when it can be parsed. */
  readonly field: string | null

  constructor(status: number, code: FinanceErrorCode, message: string, problems: IssueProblem[] = []) {
    super(message)
    this.name = 'FinanceApiError'
    this.status = status
    this.code = code
    this.problems = problems
    const m = /(?:validation failed:\s*)?([a-z_]+(?:\[\d+\])?(?:\.[a-z_0-9]+(?:\[\d+\])?)*):\s/.exec(message)
    this.field = status === 400 && m ? (m[1] ?? null) : null
  }
}

const KNOWN_CODES: FinanceErrorCode[] = [
  'validation_failed', 'bad_request', 'not_found', 'conflict', 'bad_state',
  'not_issuable', 'invoice_not_payable', 'exceeds_balance',
  'unsupported_media_type', 'too_large', 'limit_reached', 'inactive',
]

function readBody(data: unknown): Partial<FinanceErrorBody> {
  if (!data || typeof data !== 'object') return {}
  const d = data as Record<string, unknown>
  // Nitro/h3 wraps custom bodies as `{ data: {...} }`; the Go API does not.
  if (typeof d.error !== 'string' && d.data && typeof d.data === 'object') {
    return readBody(d.data)
  }
  return d as Partial<FinanceErrorBody>
}

/** Maps anything thrown by $fetch into a FinanceApiError. */
export function toFinanceError(e: unknown): FinanceApiError {
  if (e instanceof FinanceApiError) return e
  const err = (e ?? {}) as { statusCode?: number; status?: number; response?: { status?: number }; data?: unknown; message?: string }
  const status = err.statusCode ?? err.status ?? err.response?.status ?? 0
  const body = readBody(err.data)
  const raw = typeof body.error === 'string' ? body.error : ''
  let code: FinanceErrorCode
  if (status === 503) code = 'unavailable'
  else if ((KNOWN_CODES as string[]).includes(raw)) code = raw as FinanceErrorCode
  else if (status === 0) code = 'network'
  else if (status === 413) code = 'too_large'
  else if (status === 415) code = 'unsupported_media_type'
  else if (status === 400) code = 'validation_failed'
  else if (status === 404) code = 'not_found'
  else if (status === 409) code = 'conflict'
  else code = 'unknown'
  const message =
    code === 'unavailable'
      ? DISABLED_MESSAGE
      : body.message || (status === 0 ? 'Could not reach the server.' : err.message || 'Request failed.')
  return new FinanceApiError(status, code, message, Array.isArray(body.problems) ? body.problems : [])
}

/** GET endpoint returning the invoice PDF as an attachment. Link to it; never fetch it into memory. */
export function invoicePdfUrl(id: string): string {
  return `${BASE}/invoices/${encodeURIComponent(id)}/pdf`
}

function unwrap<T>(res: unknown): T {
  if (res && typeof res === 'object' && 'data' in (res as object)) return (res as { data: T }).data
  return res as T
}

function nowIso(): string {
  return new Date().toISOString()
}

export const useInvoiceStore = defineStore('invoice', () => {
  // ── List ──
  const invoices = ref<Invoice[]>([])
  const listLoading = ref(false)
  const listLoaded = ref(false)
  const listError = ref<FinanceApiError | null>(null)
  /** True once the server answered 503: finance is not configured. */
  const disabled = ref(false)
  const filter = ref<InvoiceListFilter>('all')
  const summaries = ref<Record<string, InvoiceSummary>>({})

  // ── Current invoice (detail sheet) ──
  const current = ref<Invoice | null>(null)
  const lines = ref<InvoiceLine[]>([])
  const payments = ref<Payment[]>([])
  const currentLoading = ref(false)
  const currentError = ref<FinanceApiError | null>(null)
  const issueCheck = ref<IssueCheck | null>(null)
  const issueCheckLoading = ref(false)
  /** Banner after a 409: the invoice changed underneath the user. */
  const notice = ref<string | null>(null)

  // ── Gigs (picker + list fallback labels) ──
  const gigOptions = ref<InvoiceGigOption[]>([])
  const gigsLoading = ref(false)
  const gigsLoaded = ref(false)
  const gigsError = ref<string | null>(null)
  const gigInvoices = ref<Record<string, Invoice[]>>({})

  // ── Workspace UI (create dialog + detail sheet), shared by every entry
  // point: the finance list, the gig form strip and the gig actions. ──
  const createOpen = ref(false)
  const createPresetGigId = ref<string | null>(null)
  const detailOpen = ref(false)
  const detailId = ref<string | null>(null)

  const filteredInvoices = computed(() => {
    const today = todayIso()
    return invoices.value.filter((inv) => matchesFilter(inv, filter.value, today))
  })

  const filterCounts = computed(() => {
    const today = todayIso()
    const counts = {} as Record<InvoiceListFilter, number>
    for (const f of LIST_FILTERS) {
      counts[f.value] = invoices.value.filter((inv) => matchesFilter(inv, f.value, today)).length
    }
    return counts
  })

  const activeInvoiceByGig = computed(() => {
    const map: Record<string, Invoice> = {}
    for (const inv of invoices.value) {
      if (isActiveInvoice(inv) && !map[inv.gig_id]) map[inv.gig_id] = inv
    }
    return map
  })

  const gigById = computed(() => {
    const map: Record<string, InvoiceGigOption> = {}
    for (const g of gigOptions.value) map[g.id] = g
    return map
  })

  let detailGeneration = 0
  let issueCheckGeneration = 0
  let paymentsGeneration = 0
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

  /** `touchCurrent: false` records a confirmed write in the list caches
   * without letting it replace the detail the user has since moved to. */
  function upsert(inv: Invoice, touchCurrent = true): void {
    const idx = invoices.value.findIndex((i) => i.id === inv.id)
    if (idx === -1) invoices.value.unshift(inv)
    else invoices.value.splice(idx, 1, inv)
    if (touchCurrent && current.value?.id === inv.id) current.value = inv
    const cached = gigInvoices.value[inv.gig_id]
    if (cached) {
      const ci = cached.findIndex((i) => i.id === inv.id)
      gigInvoices.value[inv.gig_id] = ci === -1 ? [inv, ...cached] : cached.map((i) => (i.id === inv.id ? inv : i))
    }
  }

  function tokenFor(id: string): string {
    if (current.value?.id === id) return current.value.updated_at
    return invoices.value.find((i) => i.id === id)?.updated_at ?? ''
  }

  // ── Reads ──

  async function fetchInvoices(query: InvoiceListQuery = {}): Promise<void> {
    listLoading.value = true
    listError.value = null
    try {
      const params: Record<string, string> = {}
      for (const [k, v] of Object.entries(query)) if (v) params[k] = String(v)
      const res = await request<unknown>(`${BASE}/invoices`, { params })
      invoices.value = unwrap<Invoice[] | null>(res) ?? []
      listLoaded.value = true
      disabled.value = false
    } catch (e) {
      listError.value = toFinanceError(e)
    } finally {
      listLoading.value = false
    }
  }

  async function fetchSummaries(): Promise<void> {
    try {
      summaries.value = unwrap<Record<string, InvoiceSummary> | null>(await request(`${BASE}/invoices/summaries`)) ?? {}
    } catch {
      summaries.value = {}
    }
  }

  /** Invoices for one gig, cached separately so the main list is untouched. */
  async function fetchInvoicesForGig(gigId: string): Promise<Invoice[]> {
    const res = await request<unknown>(`${BASE}/invoices`, { params: { gig_id: gigId } })
    const list = unwrap<Invoice[] | null>(res) ?? []
    gigInvoices.value[gigId] = list
    return list
  }

  async function fetchPayments(id: string): Promise<void> {
    const lifetime = detailGeneration
    const generation = ++paymentsGeneration
    const isCurrent = () => lifetime === detailGeneration && generation === paymentsGeneration && current.value?.id === id
    const res = await request<unknown>(`${BASE}/invoices/${id}/payments`, {}, isCurrent)
    if (isCurrent()) payments.value = unwrap<Payment[] | null>(res) ?? []
  }

  async function fetchIssueCheck(id: string, callerCurrent: () => boolean = () => true): Promise<IssueCheck | null> {
    const lifetime = detailGeneration
    const generation = ++issueCheckGeneration
    const isCurrent = () => callerCurrent() && lifetime === detailGeneration && generation === issueCheckGeneration && current.value?.id === id
    issueCheckLoading.value = true
    try {
      const check = unwrap<IssueCheck>(await request(`${BASE}/invoices/${id}/issue-check`, {}, isCurrent))
      const normalized = { ready: !!check?.ready, problems: check?.problems ?? [] }
      // Only apply the result while the user is still on the same editor
      // session; otherwise a stale fetch could overwrite a newer selection.
      if (isCurrent() && current.value?.id === id) issueCheck.value = normalized
      return normalized
    } finally {
      if (isCurrent()) issueCheckLoading.value = false
    }
  }

  function acceptsPayments(inv: Invoice): boolean {
    return inv.kind === 'invoice' && inv.status !== 'draft' && inv.status !== 'cancelled'
  }

  /** Loads an invoice with its lines, then payments or the issue check. */
  async function fetchInvoice(id: string, opts: { keepNotice?: boolean } = {}): Promise<Invoice | null> {
    const generation = ++detailGeneration
    issueCheckGeneration++
    paymentsGeneration++
    issueCheckLoading.value = false
    const isCurrent = () => generation === detailGeneration
    if (current.value?.id !== id) {
      current.value = null
      lines.value = []
      payments.value = []
      issueCheck.value = null
    }
    if (!opts.keepNotice) notice.value = null
    currentLoading.value = true
    currentError.value = null
    try {
      const res = await request<{ data: Invoice; lines?: InvoiceLine[] | null }>(`${BASE}/invoices/${id}`, {}, isCurrent)
      if (!isCurrent()) return null
      const inv = res.data
      current.value = inv
      lines.value = res.lines ?? []
      upsert(inv)
      if (acceptsPayments(inv)) await fetchPayments(id)
      else payments.value = []
      if (!isCurrent()) return null
      if (inv.status === 'draft') await fetchIssueCheck(id, isCurrent).catch(() => undefined)
      else issueCheck.value = null
      return inv
    } catch (e) {
      if (isCurrent()) currentError.value = toFinanceError(e)
      return null
    } finally {
      if (isCurrent()) currentLoading.value = false
    }
  }

  function closeCurrent(): void {
    detailGeneration++
    issueCheckGeneration++
    paymentsGeneration++
    currentLoading.value = false
    issueCheckLoading.value = false
    current.value = null
    lines.value = []
    payments.value = []
    issueCheck.value = null
    notice.value = null
    currentError.value = null
  }

  async function fetchGigOptions(force = false): Promise<void> {
    if (gigsLoaded.value && !force) return
    gigsLoading.value = true
    gigsError.value = null
    try {
      const res = await $fetch<unknown>('/api/v1/gigs')
      const raw = (Array.isArray(res) ? res : unwrap<unknown[] | null>(res) ?? []) as Record<string, unknown>[]
      gigOptions.value = raw.map((g) => ({
        id: String(g.id),
        date: String(g.date ?? ''),
        label: String(g.event_name || g.venue || 'Untitled gig'),
        status: String(g.status ?? ''),
        fee_amount: Number(g.fee_amount) || 0,
        fee_currency: String(g.fee_currency || 'EUR'),
      }))
      gigsLoaded.value = true
    } catch {
      gigsError.value = 'Could not load gigs.'
    } finally {
      gigsLoading.value = false
    }
  }

  async function suggestTax(customer: Pick<Party, 'country' | 'vat_id' | 'is_business'>): Promise<TaxSuggestion> {
    const res = await request<unknown>(`${BASE}/invoices/tax-suggestion`, {
      params: {
        customer_country: customer.country,
        customer_vat_id: customer.vat_id,
        customer_is_business: String(customer.is_business),
      },
    })
    return unwrap<TaxSuggestion>(res)
  }

  // ── Legal-note wording for the supplier's country ──
  // Loaded once and shared; the draft editor swaps the note itself when the
  // user changes the treatment. A failure is silent: the built-in wording in
  // utils/vatTreatment.ts applies, and the next editor tries again.
  const taxNotes = ref<TaxNotesMap | null>(null)
  let taxNotesRequest: Promise<void> | null = null

  function fetchTaxNotes(): Promise<void> {
    if (taxNotes.value) return Promise.resolve()
    taxNotesRequest ??= request<unknown>(`${BASE}/invoices/tax-notes`)
      .then((res) => { taxNotes.value = unwrap<TaxNotes>(res).notes })
      .catch(() => { /* keep the built-in defaults */ })
      .finally(() => { taxNotesRequest = null })
    return taxNotesRequest
  }

  // ── Writes (no optimistic updates: state changes only from responses) ──

  /** Shared 409/422 handling: refetch and surface the reason, then rethrow. */
  async function handleWriteError(e: unknown, id: string | null, isCurrent: () => boolean = () => true): Promise<never> {
    const err = toFinanceError(e)
    if (!isCurrent()) throw err
    if (id && (err.code === 'conflict' || err.code === 'bad_state')) {
      notice.value = err.code === 'conflict' ? CONFLICT_NOTICE : BAD_STATE_NOTICE
      // Only refresh the store when the editor session is still live;
      // otherwise a backgrounded write could clobber the user's newer
      // selection in the parent component.
      if (isCurrent()) await fetchInvoice(id, { keepNotice: true })
    } else if (id && err.code === 'not_issuable') {
      if (isCurrent() && current.value?.id === id) issueCheck.value = { ready: false, problems: err.problems }
    } else if (id && (err.code === 'exceeds_balance' || err.code === 'invoice_not_payable')) {
      if (isCurrent()) await fetchInvoice(id, { keepNotice: true })
    }
    throw err
  }

  async function createInvoice(input: InvoiceCreateInput): Promise<Invoice> {
    try {
      const inv = unwrap<Invoice>(await request(`${BASE}/invoices`, { method: 'POST', body: input }))
      upsert(inv)
      return inv
    } catch (e) {
      return handleWriteError(e, null)
    }
  }

  async function updateInvoice(id: string, input: InvoiceUpdateInput, callerCurrent: () => boolean = () => true): Promise<Invoice> {
    const generation = detailGeneration
    const isCurrent = () => callerCurrent() && generation === detailGeneration
    // FE-2 hardening: the caller owns the field/version snapshot. We must
    // never substitute a newer Pinia cache token for the dialog's original
    // T1 fields + token; the body is exactly what the caller passed.
    try {
      const inv = unwrap<Invoice>(
        await request(`${BASE}/invoices/${id}`, { method: 'PUT', body: { ...input } }, isCurrent),
      )
      if (!isCurrent()) { upsert(inv, false); return inv }
      upsert(inv)
      notice.value = null
      // The PUT answers with the invoice only; a replaced line list has to
      // be read back before the editor reseeds from it.
      if (input.lines) await refreshLines(id, isCurrent)
      if (inv.status === 'draft') await fetchIssueCheck(id, isCurrent).catch(() => undefined)
      return inv
    } catch (e) {
      return handleWriteError(e, id, isCurrent)
    }
  }

  /** Re-reads just the lines of the shown invoice (after a PUT that replaced them). */
  async function refreshLines(id: string, isCurrent: () => boolean): Promise<void> {
    try {
      const res = await request<{ data: Invoice; lines?: InvoiceLine[] | null }>(`${BASE}/invoices/${id}`, {}, isCurrent)
      if (isCurrent() && current.value?.id === id) lines.value = res.lines ?? []
    } catch {
      if (isCurrent()) notice.value = 'Saved, but the line items could not be reloaded. Reopen the invoice to see them.'
    }
  }

  async function transition(
    id: string,
    action: string,
    extra: Record<string, unknown> = {},
  ): Promise<Invoice> {
    // FE-2 hardening: caller may pass `updated_at` to lock in their original
    // snapshot. If absent we fall back to the cache token (no dialog owns
    // the call site for unparameterised historical transitions).
    let generation = detailGeneration
    const isCurrent = () => generation === detailGeneration
    const updatedAt = typeof extra.updated_at === 'string' ? extra.updated_at : tokenFor(id)
    const { updated_at: _ignored, ...rest } = extra
    void _ignored
    try {
      const inv = unwrap<Invoice>(
        await request(`${BASE}/invoices/${id}/${action}`, {
          method: 'POST',
          body: { ...rest, updated_at: updatedAt },
        }, isCurrent),
      )
      if (!isCurrent()) { upsert(inv, false); return inv }
      upsert(inv)
      notice.value = null
      const refresh = fetchInvoice(id)
      generation = detailGeneration
      await refresh
      return inv
    } catch (e) {
      return handleWriteError(e, id, isCurrent)
    }
  }

  const issueInvoice = (id: string, updatedAt?: string) =>
    transition(id, 'issue', updatedAt ? { updated_at: updatedAt } : {})
  const cancelInvoice = (id: string, updatedAt?: string) =>
    transition(id, 'cancel', updatedAt ? { updated_at: updatedAt } : {})
  const markPaid = (id: string, input: { paid_at?: string; payment_ref?: string; updated_at?: string } = {}) =>
    transition(id, 'pay', {
      paid_at: input.paid_at ?? nowIso(),
      payment_ref: input.payment_ref ?? '',
      updated_at: input.updated_at,
    })

  async function issueCreditNote(
    id: string,
    reason: string,
    updatedAt?: string,
  ): Promise<{ credit_note: Invoice; original: Invoice }> {
    let generation = detailGeneration
    const isCurrent = () => generation === detailGeneration
    const token = updatedAt ?? tokenFor(id)
    try {
      const res = unwrap<{ credit_note: Invoice; original: Invoice }>(
        await request(`${BASE}/invoices/${id}/credit-note`, {
          method: 'POST',
          body: { reason, updated_at: token },
        }, isCurrent),
      )
      if (!isCurrent()) { upsert(res.original, false); upsert(res.credit_note, false); return res }
      upsert(res.original)
      upsert(res.credit_note)
      const refresh = fetchInvoice(id)
      generation = detailGeneration
      await refresh
      return res
    } catch (e) {
      return handleWriteError(e, id, isCurrent)
    }
  }

  /** Credit note + replacement draft; the sheet switches to the replacement. */
  async function correctInvoice(
    id: string,
    reason: string,
    updatedAt?: string,
  ): Promise<{ credit_note: Invoice; original: Invoice; replacement: Invoice }> {
    let generation = detailGeneration
    const isCurrent = () => generation === detailGeneration
    const token = updatedAt ?? tokenFor(id)
    try {
      const res = unwrap<{ credit_note: Invoice; original: Invoice; replacement: Invoice }>(
        await request(`${BASE}/invoices/${id}/correct`, {
          method: 'POST',
          body: { reason, updated_at: token },
        }, isCurrent),
      )
      if (!isCurrent()) {
        upsert(res.original, false); upsert(res.credit_note, false); upsert(res.replacement, false)
        return res
      }
      upsert(res.original)
      upsert(res.credit_note)
      upsert(res.replacement)
      if (detailId.value === id) detailId.value = res.replacement.id
      const refresh = fetchInvoice(res.replacement.id)
      generation = detailGeneration
      await refresh
      return res
    } catch (e) {
      return handleWriteError(e, id, isCurrent)
    }
  }

  async function createPayment(invoiceId: string, input: PaymentCreateInput): Promise<Payment> {
    const generation = detailGeneration
    const isCurrent = () => generation === detailGeneration
    const currency = current.value?.id === invoiceId
      ? current.value.currency
      : invoices.value.find((i) => i.id === invoiceId)?.currency ?? ''
    let created: Payment
    try {
      created = unwrap<Payment>(
        await request(`${BASE}/invoices/${invoiceId}/payments`, {
          method: 'POST',
          body: {
            currency,
            amount_minor: input.amount_minor,
            kind: input.kind,
            method: input.method ?? '',
            reference: input.reference ?? '',
            received_at: input.received_at ?? null,
          },
        }, isCurrent),
      )
    } catch (e) {
      return handleWriteError(e, invoiceId, isCurrent)
    }
    // The API creates payments as pending; "already received" completes it.
    if (input.received) {
      try {
        created = await putPaymentStatus(created, 'completed', input.received_at ?? nowIso(), isCurrent)
      } catch (e) {
        if (isCurrent()) await fetchInvoice(invoiceId, { keepNotice: true })
        const err = toFinanceError(e)
        throw new FinanceApiError(
          err.status,
          err.code,
          `Payment saved as pending, but marking it received failed: ${err.message} Use MARK RECEIVED in the ledger.`,
          err.problems,
        )
      }
    }
    if (isCurrent()) await fetchInvoice(invoiceId, { keepNotice: true })
    return created
  }

  async function putPaymentStatus(p: Payment, status: Payment['status'], receivedAt: string | null, isCurrent: () => boolean = () => true): Promise<Payment> {
    return unwrap<Payment>(
      await request(`${BASE}/payments/${p.id}`, {
        method: 'PUT',
        body: {
          status,
          method: p.method,
          reference: p.reference,
          received_at: receivedAt,
          updated_at: p.updated_at,
        },
      }, isCurrent),
    )
  }

  async function markPaymentReceived(p: Payment): Promise<Payment> {
    const generation = detailGeneration
    const isCurrent = () => generation === detailGeneration
    try {
      const updated = await putPaymentStatus(p, 'completed', p.received_at ?? nowIso(), isCurrent)
      if (isCurrent()) await fetchInvoice(p.invoice_id, { keepNotice: true })
      return updated
    } catch (e) {
      return handleWriteError(e, p.invoice_id, isCurrent)
    }
  }

  function setFilter(f: InvoiceListFilter): void {
    filter.value = f
  }

  function clearNotice(): void {
    notice.value = null
  }

  function openCreate(gigId: string | null = null): void {
    createPresetGigId.value = gigId
    createOpen.value = true
  }

  function setCreateOpen(open: boolean): void {
    createOpen.value = open
    if (!open) createPresetGigId.value = null
  }

  /** Opens the detail sheet and loads the invoice (the sheet never fetches itself). */
  function openDetail(id: string): Promise<Invoice | null> {
    detailId.value = id
    detailOpen.value = true
    return fetchInvoice(id)
  }

  function setDetailOpen(open: boolean): void {
    detailOpen.value = open
    if (!open) {
      detailId.value = null
      closeCurrent()
    }
  }

  return {
    // state
    invoices, listLoading, listLoaded, listError, disabled, filter, summaries,
    current, lines, payments, currentLoading, currentError, issueCheck, issueCheckLoading, notice,
    gigOptions, gigsLoading, gigsLoaded, gigsError, gigInvoices,
    createOpen, createPresetGigId, detailOpen, detailId,
    // getters
    filteredInvoices, filterCounts, activeInvoiceByGig, gigById,
    // actions
    fetchInvoices, fetchSummaries, fetchInvoicesForGig, fetchInvoice, fetchPayments, fetchIssueCheck,
    fetchGigOptions, suggestTax, taxNotes, fetchTaxNotes, closeCurrent,
    createInvoice, updateInvoice, issueInvoice, cancelInvoice, markPaid,
    issueCreditNote, correctInvoice, createPayment, markPaymentReceived,
    setFilter, clearNotice, openCreate, setCreateOpen, openDetail, setDetailOpen,
  }
})
