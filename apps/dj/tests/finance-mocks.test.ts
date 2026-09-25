// @vitest-environment node
// Contract rules enforced by the in-memory finance mocks
// (apps/dj/server/api/v1/finance/**, docs/INVOICING.md §3–§4.1). The Nitro
// auto-imports are stubbed so the route handlers run as plain functions.
import { beforeAll, describe, expect, it, vi } from 'vitest'
import type { Invoice } from '../app/types/finance'

interface FakeEvent {
  params: Record<string, string>
  body?: unknown
  query?: Record<string, string>
  status: number
}

vi.stubGlobal('defineEventHandler', (h: unknown) => h)
vi.stubGlobal('setResponseStatus', (e: FakeEvent, s: number) => { e.status = s })
vi.stubGlobal('getRouterParam', (e: FakeEvent, name: string) => e.params[name])
vi.stubGlobal('readBody', async (e: FakeEvent) => e.body)
vi.stubGlobal('getQuery', (e: FakeEvent) => e.query ?? {})

type Handler = (e: FakeEvent) => unknown
const base = '../server/api/v1/finance'
let db: typeof import('../server/api/v1/finance/-mockDb')
const h: Record<string, Handler> = {}

beforeAll(async () => {
  db = await import(`${base}/-mockDb`)
  const load = async (key: string, path: string) => { h[key] = (await import(`${base}/${path}`)).default as Handler }
  await load('list', 'invoices/index.get')
  await load('create', 'invoices/index.post')
  await load('get', 'invoices/[id].get')
  await load('put', 'invoices/[id].put')
  await load('check', 'invoices/[id]/issue-check.get')
  await load('issue', 'invoices/[id]/issue.post')
  await load('pay', 'invoices/[id]/pay.post')
  await load('cancel', 'invoices/[id]/cancel.post')
  await load('credit', 'invoices/[id]/credit-note.post')
  await load('correct', 'invoices/[id]/correct.post')
  await load('payments', 'invoices/[id]/payments.post')
  await load('summaries', 'invoices/summaries.get')
  await load('suggest', 'invoices/tax-suggestion.get')
})

async function call(name: string, opts: Partial<FakeEvent> = {}) {
  const e: FakeEvent = { params: {}, status: 200, ...opts }
  const res = (await h[name]!(e)) as Record<string, unknown>
  return { status: e.status, res }
}

async function createDraft(gigId: string) {
  const { status, res } = await call('create', { body: { gig_id: gigId } })
  expect(status).toBe(201)
  return (res.data as Invoice)
}

async function put(inv: Invoice, patch: Partial<Invoice> = {}) {
  const body = {
    customer: inv.customer, vat_treatment: inv.vat_treatment, tax_rate_bps: inv.tax_rate_bps, tax_note: inv.tax_note,
    withholding_rate_bps: inv.withholding_rate_bps, supply_date: inv.supply_date, due_at: inv.due_at,
    number_prefix: inv.number_prefix, internal_notes: inv.internal_notes, updated_at: inv.updated_at, ...patch,
  }
  return call('put', { params: { id: inv.id }, body })
}

const freshGig = (n: number) => `00000000-0000-4000-8000-${String(900 + n).padStart(12, '0')}`

describe('finance mocks', () => {
  it('only one active invoice per gig (409 bad_state)', async () => {
    const gig = freshGig(1)
    await createDraft(gig)
    const { status, res } = await call('create', { body: { gig_id: gig } })
    expect(status).toBe(409)
    expect(res.error).toBe('bad_state')
  })

  it('drafts have no number; issue allocates PREFIX-0001-CUR and outstanding appears', async () => {
    const inv = await createDraft(db.GIG_IDS.unbilled)
    expect(inv.invoice_number).toBeNull()
    expect(inv.outstanding_minor).toBe(0)
    // The seeded DE customer is complete, so domestic 19% issues directly.
    const res = await call('issue', { params: { id: inv.id }, body: { updated_at: inv.updated_at } })
    expect(res.status).toBe(200)
    const issued = res.res.data as Invoice
    expect(issued.invoice_number).toMatch(/^INV-\d{4}-EUR$/)
    expect(issued.outstanding_minor).toBe(issued.net_payable_minor)
  })

  it('422 not_issuable lists problems; issue-check agrees', async () => {
    const inv = (await call('list', { query: { gig_id: db.GIG_IDS.incomplete } })).res.data as Invoice[]
    const draft = inv.find((i) => i.status === 'draft')!
    const check = await call('check', { params: { id: draft.id } })
    const problems = (check.res.data as { ready: boolean; problems: { field: string }[] })
    expect(problems.ready).toBe(false)
    expect(problems.problems.map((p) => p.field)).toContain('customer.city')
    const res = await call('issue', { params: { id: draft.id }, body: { updated_at: draft.updated_at } })
    expect(res.status).toBe(422)
    expect(res.res.error).toBe('not_issuable')
  })

  it('stale updated_at is a 409 conflict; missing token is a 400', async () => {
    const inv = await createDraft(freshGig(2))
    expect((await put(inv, { updated_at: '2000-01-01T00:00:00Z' })).res.error).toBe('conflict')
    expect((await call('cancel', { params: { id: inv.id }, body: {} })).status).toBe(400)
  })

  it('PUT validates prefix, supply_date and withholding; normalises VAT ids', async () => {
    const inv = await createDraft(freshGig(3))
    const cn = await put(inv, { number_prefix: 'CN' })
    expect(cn.status).toBe(400)
    expect(String(cn.res.message)).toContain('number_prefix')
    expect((await put(inv, { supply_date: '2026-09-20T00:00:00Z' })).status).toBe(400)
    expect((await put(inv, { withholding_rate_bps: 10001 })).status).toBe(400)
    const ok = await put(inv, {
      withholding_rate_bps: 6000, // drafts accept up to 100%…
      customer: { ...inv.customer, country: 'fr', vat_id: 'fr 00.000 000 000' },
    })
    expect(ok.status).toBe(200)
    const saved = ok.res.data as Invoice
    expect(saved.customer.country).toBe('FR')
    expect(saved.customer.vat_id).toBe('FR00000000000')
    const check = await call('check', { params: { id: saved.id } })
    // …but issuing needs ≤ 50%
    expect((check.res.data as { problems: { field: string }[] }).problems.map((p) => p.field)).toContain('withholding_rate_bps')
  })

  it('cancel is draft-only; non-drafts report a status problem in issue-check', async () => {
    const [paid] = (await call('list', { query: { gig_id: db.GIG_IDS.domestic } })).res.data as Invoice[]
    const res = await call('cancel', { params: { id: paid!.id }, body: { updated_at: paid!.updated_at } })
    expect(res.status).toBe(409)
    expect(res.res.error).toBe('bad_state')
    const check = await call('check', { params: { id: paid!.id } })
    expect(check.res.data).toEqual({ ready: false, problems: [expect.objectContaining({ field: 'status' })] })
  })

  it('only issued invoices owe money; payments cannot exceed the balance', async () => {
    const all = (await call('list')).res.data as Invoice[]
    for (const i of all) if (i.status !== 'issued') expect(i.outstanding_minor).toBe(0)
    const rc = all.find((i) => i.gig_id === db.GIG_IDS.reverseCharge)!
    const over = await call('payments', {
      params: { id: rc.id },
      body: { currency: 'EUR', kind: 'payment', amount_minor: rc.outstanding_minor + 1 },
    })
    expect(over.status).toBe(422)
    expect(over.res.error).toBe('exceeds_balance')
    const refund = await call('payments', {
      params: { id: rc.id },
      body: { currency: 'EUR', kind: 'refund', amount_minor: rc.received_minor + 1 },
    })
    expect(refund.res.error).toBe('exceeds_balance')
  })

  it('recording a payment never changes the invoice status; /pay does', async () => {
    const inv = (await call('list', { query: { gig_id: db.GIG_IDS.us } })).res.data as Invoice[]
    const us = inv[0]!
    await call('payments', { params: { id: us.id }, body: { currency: 'USD', kind: 'payment', amount_minor: us.outstanding_minor } })
    const after = (await call('get', { params: { id: us.id } })).res.data as Invoice
    expect(after.status).toBe('issued')
    expect(after.outstanding_minor).toBe(0)
    const paid = await call('pay', { params: { id: us.id }, body: { updated_at: after.updated_at } })
    expect((paid.res.data as Invoice).status).toBe('paid')
  })

  it('credit notes use CN numbers, carry no due date and exclude from summaries', async () => {
    const inv = await createDraft(freshGig(4))
    // synthesized gigs have no customer; fill it so it can be issued
    const filled = (await put(inv, { customer: { ...inv.customer, legal_name: 'Club Alpha', address_line1: '1 Example Street', city: 'Sample City', country: 'DE' } })).res.data as Invoice
    const issued = (await call('issue', { params: { id: filled.id }, body: { updated_at: filled.updated_at } })).res.data as Invoice
    const before = (await call('summaries')).res.data as Record<string, { issued_count: number }>
    const res = await call('credit', { params: { id: issued.id }, body: { reason: 'Slot cancelled', updated_at: issued.updated_at } })
    expect(res.status).toBe(201)
    const { credit_note, original } = res.res.data as { credit_note: Invoice; original: Invoice }
    expect(credit_note.invoice_number).toMatch(/^CN-\d{4}-EUR$/)
    expect(credit_note.due_at).toBeNull()
    expect(credit_note.internal_notes).toBe('Slot cancelled')
    expect(original.status).toBe('credited')
    const after = (await call('summaries')).res.data as Record<string, { issued_count: number }>
    expect(after.EUR!.issued_count).toBe(before.EUR!.issued_count - 1)
  })

  it('correct creates a replacement draft that "Replaces <number>"', async () => {
    const [paid] = (await call('list', { query: { gig_id: db.GIG_IDS.domestic } })).res.data as Invoice[]
    const res = await call('correct', { params: { id: paid!.id }, body: { reason: 'Wrong address', updated_at: paid!.updated_at } })
    const { original, replacement } = res.res.data as { original: Invoice; replacement: Invoice }
    expect(original.status).toBe('corrected')
    expect(original.replaced_by_invoice_id).toBe(replacement.id)
    expect(replacement.status).toBe('draft')
    expect(replacement.invoice_number).toBeNull()
    expect(replacement.internal_notes).toBe(`Replaces ${paid!.invoice_number}`)
  })

  it('suggests per §3, including the confirm rows', async () => {
    const s = async (q: Record<string, string>) => (await call('suggest', { query: q })).res.data as { vat_treatment: string; reason: string }
    expect((await s({ customer_country: 'FR', customer_vat_id: 'FR00000000000', customer_is_business: 'true' })).vat_treatment).toBe('reverse_charge')
    expect((await s({ customer_country: 'FR', customer_vat_id: 'FR00000000000', customer_is_business: 'false' })).reason).toBe('customer not marked as a business — confirm')
    expect((await s({ customer_country: 'FR', customer_vat_id: '', customer_is_business: 'true' })).reason).toBe('no customer VAT ID — confirm')
    expect((await s({ customer_country: '', customer_vat_id: '', customer_is_business: 'true' })).reason).toBe('customer country unknown — confirm')
    expect((await s({ customer_country: 'US', customer_vat_id: '', customer_is_business: 'true' })).vat_treatment).toBe('outside_scope')
  })

  it('list rejects unknown filters with a 400', async () => {
    expect((await call('list', { query: { status: 'nope' } })).status).toBe(400)
    expect((await call('list', { query: { gig_id: 'not-a-uuid' } })).status).toBe(400)
  })
})
