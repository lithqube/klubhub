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
    const m = /(?:validation failed:\s*)?([a-z_]+(?:\.[a-z_0-9]+)*):\s/.exec(message)
    this.field = status === 400 && m ? (m[1] ?? null) : null
  }
}

const KNOWN_CODES: FinanceErrorCode[] = [
  'validation_failed', 'bad_request', 'not_found', 'conflict', 'bad_state',
  'not_issuable', 'invoice_not_payable', 'exceeds_balance',
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

  function upsert(inv: Invoice): void {
    const idx = invoices.value.findIndex((i) => i.id === inv.id)
    if (idx === -1) invoices.value.unshift(inv)
    else invoices.value.splice(idx, 1, inv)
    if (current.value?.id === inv.id) current.value = inv
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
    const res = await request<unknown>(`${BASE}/invoices/${id}/payments`)
    if (current.value?.id === id) payments.value = unwrap<Payment[] | null>(res) ?? []
  }

  async function fetchIssueCheck(id: string): Promise<IssueCheck | null> {
    issueCheckLoading.value = true
    try {
      const check = unwrap<IssueCheck>(await request(`${BASE}/invoices/${id}/issue-check`))
      const normalized = { ready: !!check?.ready, problems: check?.problems ?? [] }
      if (current.value?.id === id) issueCheck.value = normalized
      return normalized
    } finally {
      issueCheckLoading.value = false
    }
  }

  function acceptsPayments(inv: Invoice): boolean {
    return inv.kind === 'invoice' && inv.status !== 'draft' && inv.status !== 'cancelled'
  }

  /** Loads an invoice with its lines, then payments or the issue check. */
  async function fetchInvoice(id: string, opts: { keepNotice?: boolean } = {}): Promise<Invoice | null> {
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
      const res = await request<{ data: Invoice; lines?: InvoiceLine[] | null }>(`${BASE}/invoices/${id}`)
      const inv = res.data
      current.value = inv
      lines.value = res.lines ?? []
      upsert(inv)
      if (acceptsPayments(inv)) await fetchPayments(id)
      else payments.value = []
      if (inv.status === 'draft') await fetchIssueCheck(id).catch(() => undefined)
      else issueCheck.value = null
      return inv
    } catch (e) {
      currentError.value = toFinanceError(e)
      return null
    } finally {
      currentLoading.value = false
    }
  }

  function closeCurrent(): void {
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

  // ── Writes (no optimistic updates: state changes only from responses) ──

  /** Shared 409/422 handling: refetch and surface the reason, then rethrow. */
  async function handleWriteError(e: unknown, id: string | null): Promise<never> {
    const err = toFinanceError(e)
    if (id && (err.code === 'conflict' || err.code === 'bad_state')) {
      notice.value = err.code === 'conflict' ? CONFLICT_NOTICE : BAD_STATE_NOTICE
      await fetchInvoice(id, { keepNotice: true })
    } else if (id && err.code === 'not_issuable') {
      if (current.value?.id === id) issueCheck.value = { ready: false, problems: err.problems }
    } else if (id && (err.code === 'exceeds_balance' || err.code === 'invoice_not_payable')) {
      await fetchInvoice(id, { keepNotice: true })
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

  async function updateInvoice(id: string, input: InvoiceUpdateInput): Promise<Invoice> {
    try {
      const inv = unwrap<Invoice>(
        await request(`${BASE}/invoices/${id}`, { method: 'PUT', body: { ...input, updated_at: tokenFor(id) } }),
      )
      upsert(inv)
      notice.value = null
      if (inv.status === 'draft') await fetchIssueCheck(id).catch(() => undefined)
      return inv
    } catch (e) {
      return handleWriteError(e, id)
    }
  }

  async function transition(id: string, action: string, extra: Record<string, unknown> = {}): Promise<Invoice> {
    try {
      const inv = unwrap<Invoice>(
        await request(`${BASE}/invoices/${id}/${action}`, {
          method: 'POST',
          body: { ...extra, updated_at: tokenFor(id) },
        }),
      )
      upsert(inv)
      notice.value = null
      await fetchInvoice(id)
      return inv
    } catch (e) {
      return handleWriteError(e, id)
    }
  }

  const issueInvoice = (id: string) => transition(id, 'issue')
  const cancelInvoice = (id: string) => transition(id, 'cancel')
  const markPaid = (id: string, input: { paid_at?: string; payment_ref?: string } = {}) =>
    transition(id, 'pay', { paid_at: input.paid_at ?? nowIso(), payment_ref: input.payment_ref ?? '' })

  async function issueCreditNote(id: string, reason: string): Promise<{ credit_note: Invoice; original: Invoice }> {
    try {
      const res = unwrap<{ credit_note: Invoice; original: Invoice }>(
        await request(`${BASE}/invoices/${id}/credit-note`, {
          method: 'POST',
          body: { reason, updated_at: tokenFor(id) },
        }),
      )
      upsert(res.original)
      upsert(res.credit_note)
      await fetchInvoice(id)
      return res
    } catch (e) {
      return handleWriteError(e, id)
    }
  }

  /** Credit note + replacement draft; the sheet switches to the replacement. */
  async function correctInvoice(
    id: string,
    reason: string,
  ): Promise<{ credit_note: Invoice; original: Invoice; replacement: Invoice }> {
    try {
      const res = unwrap<{ credit_note: Invoice; original: Invoice; replacement: Invoice }>(
        await request(`${BASE}/invoices/${id}/correct`, {
          method: 'POST',
          body: { reason, updated_at: tokenFor(id) },
        }),
      )
      upsert(res.original)
      upsert(res.credit_note)
      upsert(res.replacement)
      if (detailId.value === id) detailId.value = res.replacement.id
      await fetchInvoice(res.replacement.id)
      return res
    } catch (e) {
      return handleWriteError(e, id)
    }
  }

  async function createPayment(invoiceId: string, input: PaymentCreateInput): Promise<Payment> {
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
        }),
      )
    } catch (e) {
      return handleWriteError(e, invoiceId)
    }
    // The API creates payments as pending; "already received" completes it.
    if (input.received) {
      try {
        created = await putPaymentStatus(created, 'completed', input.received_at ?? nowIso())
      } catch (e) {
        await fetchInvoice(invoiceId, { keepNotice: true })
        const err = toFinanceError(e)
        throw new FinanceApiError(
          err.status,
          err.code,
          `Payment saved as pending, but marking it received failed: ${err.message} Use MARK RECEIVED in the ledger.`,
          err.problems,
        )
      }
    }
    await fetchInvoice(invoiceId, { keepNotice: true })
    return created
  }

  async function putPaymentStatus(p: Payment, status: Payment['status'], receivedAt: string | null): Promise<Payment> {
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
      }),
    )
  }

  async function markPaymentReceived(p: Payment): Promise<Payment> {
    try {
      const updated = await putPaymentStatus(p, 'completed', p.received_at ?? nowIso())
      await fetchInvoice(p.invoice_id, { keepNotice: true })
      return updated
    } catch (e) {
      return handleWriteError(e, p.invoice_id)
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
    fetchGigOptions, suggestTax, closeCurrent,
    createInvoice, updateInvoice, issueInvoice, cancelInvoice, markPaid,
    issueCreditNote, correctInvoice, createPayment, markPaymentReceived,
    setFilter, clearNotice, openCreate, setCreateOpen, openDetail, setDetailOpen,
  }
})
