// @vitest-environment node
// EN 16931 contract in the browser demo (shared finance-mock core): editable
// lines, references, unit codes, billing profile bank details and the PDF.
import { describe, expect, it } from 'vitest'
import { createDemoBackend } from '../app/demo/backend'
import { FINANCE_GIG_IDS } from '../shared/finance-mock/seed'
import type { BillingProfile, Invoice, InvoiceLine } from '../app/types/finance'

const NOW = new Date('2026-09-25T12:00:00Z')
const backend = () => createDemoBackend({ storage: null, baseURL: '/demo/', now: () => NOW, createObjectUrl: () => 'blob:x' })
type B = ReturnType<typeof backend>

async function call(b: B, method: string, path: string, body: unknown = null) {
  const res = await b.handle({ method, path, query: new URLSearchParams(), body })
// eslint-disable-next-line @typescript-eslint/no-explicit-any
  return { status: res.status, body: res.body as Record<string, any>, blob: res.blob, headers: res.headers }
}

async function newDraft(b: B) {
  const res = await call(b, 'POST', '/api/v1/finance/invoices', { gig_id: FINANCE_GIG_IDS.unbilled })
  expect(res.status).toBe(201)
  return res.body.data as Invoice
}

/** The PUT body the editor sends: current fields plus overrides. */
function putBody(inv: Invoice, over: Record<string, unknown> = {}) {
  return {
    customer: inv.customer, vat_treatment: inv.vat_treatment, tax_rate_bps: inv.tax_rate_bps, tax_note: inv.tax_note,
    withholding_rate_bps: inv.withholding_rate_bps, supply_date: inv.supply_date, due_at: inv.due_at,
    number_prefix: inv.number_prefix, internal_notes: inv.internal_notes,
    buyer_reference: '', purchase_order_ref: '', contract_ref: '', payment_terms: '',
    updated_at: inv.updated_at, ...over,
  }
}
const lines = async (b: B, id: string) => (await call(b, 'GET', `/api/v1/finance/invoices/${id}`)).body.lines as InvoiceLine[]

describe('demo invoice lines', () => {
  it('a new draft has references defaulting to empty and one piece line', async () => {
    const b = backend()
    const d = await newDraft(b)
    expect(d).toMatchObject({ buyer_reference: '', purchase_order_ref: '', contract_ref: '', payment_terms: '' })
    const ls = await lines(b, d.id)
    expect(ls).toHaveLength(1)
    expect(ls[0]!.unit_code).toBe('C62')
  })

  it('replaces ALL lines, derives totals/tax server side and returns recomputed totals', async () => {
    const b = backend()
    const d = await newDraft(b)
    const res = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, {
      lines: [
        { description: 'DJ set', quantity: 2, unit_minor: 15000, unit_code: 'HUR' },
        { description: 'Travel', quantity: 120, unit_minor: 35 },
      ],
    }))
    expect(res.status).toBe(200)
    const inv = res.body.data as Invoice
    expect(inv.subtotal_minor).toBe(34200)
    expect(inv.tax_minor).toBe(Math.round(34200 * inv.tax_rate_bps / 10000))
    expect(inv.total_minor).toBe(inv.subtotal_minor + inv.tax_minor)
    expect(inv.updated_at).not.toBe(d.updated_at)

    const stored = await lines(b, d.id)
    expect(stored.map((l) => [l.description, l.quantity, l.unit_minor, l.unit_code, l.line_total_minor, l.sort_order])).toEqual([
      ['DJ set', 2, 15000, 'HUR', 30000, 0],
      ['Travel', 120, 35, 'C62', 4200, 1],
    ])
    expect(stored.every((l) => l.tax_bps === inv.tax_rate_bps)).toBe(true)
  })

  it('ignores client-sent derived fields', async () => {
    const b = backend()
    const d = await newDraft(b)
    await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, {
      lines: [{ description: 'Set', quantity: 2, unit_minor: 100, line_total_minor: 999999, tax_bps: 5 }],
    }))
    const [l] = await lines(b, d.id)
    expect(l).toMatchObject({ line_total_minor: 200, tax_bps: d.tax_rate_bps })
  })

  it('keeps existing lines when `lines` is omitted or null', async () => {
    const b = backend()
    const d = await newDraft(b)
    const before = await lines(b, d.id)
    const r1 = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d))
    expect(r1.status).toBe(200)
    const r2 = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(r1.body.data, { lines: null }))
    expect(r2.status).toBe(200)
    expect(await lines(b, d.id)).toEqual(before)
  })

  it('rejects an empty list and more than 100 lines, leaving the invoice untouched', async () => {
    const b = backend()
    const d = await newDraft(b)
    const empty = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, { lines: [] }))
    expect(empty.status).toBe(400)
    expect(empty.body).toMatchObject({ error: 'validation_failed' })
    expect(empty.body.message).toMatch(/lines: an invoice needs at least one line/)
    const many = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, {
      lines: Array.from({ length: 101 }, () => ({ description: 'x', quantity: 1, unit_minor: 1 })),
    }))
    expect(many.status).toBe(400)
    expect(many.body.message).toMatch(/at most 100 lines/)
    expect((await call(b, 'GET', `/api/v1/finance/invoices/${d.id}`)).body.data.updated_at).toBe(d.updated_at)
  })

  it('reports each bad field with its lines[i] path in the Go message format', async () => {
    const b = backend()
    const d = await newDraft(b)
    const res = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, {
      lines: [
        { description: 'ok', quantity: 1, unit_minor: 100 },
        { description: '   ', quantity: 0, unit_minor: -1, unit_code: 'XXX' },
        { description: 'x'.repeat(501), quantity: 1_000_001, unit_minor: 1 },
      ],
    }))
    expect(res.status).toBe(400)
    expect(res.body.error).toBe('validation_failed')
    const m: string = res.body.message
    expect(m.startsWith('validation failed: ')).toBe(true)
    for (const part of [
      'lines[1].description: required',
      'lines[1].quantity: must be between 1 and 1000000',
      'lines[1].unit_minor: must not be negative',
      'lines[1].unit_code: must be one of C62, HUR, DAY, LS, KMT',
      'lines[2].description: exceeds 500 characters',
      'lines[2].quantity: must be between 1 and 1000000',
    ]) expect(m).toContain(part)
    expect(m).not.toContain('lines[0]')
  })

  it('rejects fractional quantities and an overflowing line total', async () => {
    const b = backend()
    const d = await newDraft(b)
    const frac = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, { lines: [{ description: 'x', quantity: 1.5, unit_minor: 100 }] }))
    expect(frac.body.message).toContain('lines[0].quantity')
    const big = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, { lines: [{ description: 'x', quantity: 1_000_000, unit_minor: 20_000_000 }] }))
    expect(big.body.message).toContain('lines[0].unit_minor: line total exceeds the maximum amount')
  })

  it('defaults a missing unit_code to C62 and accepts each allowed code', async () => {
    const b = backend()
    const d = await newDraft(b)
    const codes = ['C62', 'HUR', 'DAY', 'LS', 'KMT']
    const res = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, {
      lines: [{ description: 'none', quantity: 1, unit_minor: 1 }, ...codes.map((c) => ({ description: c, quantity: 1, unit_minor: 1, unit_code: c }))],
    }))
    expect(res.status).toBe(200)
    expect((await lines(b, d.id)).map((l) => l.unit_code)).toEqual(['C62', ...codes])
  })

  it('still enforces the optimistic-concurrency token', async () => {
    const b = backend()
    const d = await newDraft(b)
    const stale = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, { updated_at: '2020-01-01T00:00:00Z', lines: [{ description: 'x', quantity: 1, unit_minor: 1 }] }))
    expect(stale.status).toBe(409)
    expect(await lines(b, d.id)).toHaveLength(1)
    expect((await lines(b, d.id))[0]!.description).not.toBe('x')
  })

  it('only drafts accept line edits', async () => {
    const b = backend()
    const issued = ((await call(b, 'GET', '/api/v1/finance/invoices')).body.data as Invoice[]).find((i) => i.status === 'issued')!
    const res = await call(b, 'PUT', `/api/v1/finance/invoices/${issued.id}`, putBody(issued, { lines: [{ description: 'x', quantity: 1, unit_minor: 1 }] }))
    expect(res.status).toBe(409)
  })
})

describe('demo invoice references', () => {
  it('stores trimmed references and payment terms and returns them', async () => {
    const b = backend()
    const d = await newDraft(b)
    const res = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, {
      buyer_reference: ' 04011000-12345-34 ', purchase_order_ref: 'PO-7', contract_ref: 'C-2026-1', payment_terms: 'Payable within 14 days',
    }))
    expect(res.body.data).toMatchObject({
      buyer_reference: '04011000-12345-34', purchase_order_ref: 'PO-7', contract_ref: 'C-2026-1', payment_terms: 'Payable within 14 days',
    })
    expect((await call(b, 'GET', `/api/v1/finance/invoices/${d.id}`)).body.data.buyer_reference).toBe('04011000-12345-34')
  })

  it('enforces 100 / 100 / 100 / 500 character limits with the field name', async () => {
    const b = backend()
    const d = await newDraft(b)
    const ok = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, { buyer_reference: 'a'.repeat(100), payment_terms: 'b'.repeat(500) }))
    expect(ok.status).toBe(200)
    const res = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(ok.body.data, {
      buyer_reference: 'a'.repeat(101), purchase_order_ref: 'b'.repeat(101), contract_ref: 'c'.repeat(101), payment_terms: 'd'.repeat(501),
    }))
    expect(res.status).toBe(400)
    for (const f of ['buyer_reference: exceeds 100', 'purchase_order_ref: exceeds 100', 'contract_ref: exceeds 100', 'payment_terms: exceeds 500']) {
      expect(res.body.message).toContain(f)
    }
  })

  it('a PUT that omits them clears them, like the Go API', async () => {
    const b = backend()
    const d = await newDraft(b)
    const set = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, { buyer_reference: 'X-1' }))
    const { buyer_reference: _b, ...rest } = putBody(set.body.data)
    const cleared = await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, rest)
    expect(cleared.body.data.buyer_reference).toBe('')
  })
})

describe('demo billing profile', () => {
  const full = (p: BillingProfile, over: Record<string, unknown> = {}) => {
    const { id: _id, created_at: _c, ...rest } = p
    return { ...rest, ...over }
  }
  const get = async (b: B) => (await call(b, 'GET', '/api/v1/finance/billing-profile')).body.data as BillingProfile

  it('returns the new fields (empty by default)', async () => {
    expect(await get(backend())).toMatchObject({ tax_number: '', iban: '', bic: '' })
  })

  it('normalises IBAN and BIC (spaces removed, upper-cased) and persists the new fields', async () => {
    const b = backend()
    const p = await get(b)
    const res = await call(b, 'PUT', '/api/v1/finance/billing-profile', full(p, { tax_number: ' 21/815/08150 ', iban: 'de89 3704 0044 0532 0130 00', bic: 'cobadeffxxx' }))
    expect(res.status).toBe(200)
    expect(res.body.data).toMatchObject({ tax_number: '21/815/08150', iban: 'DE89370400440532013000', bic: 'COBADEFFXXX' })
    expect(await get(b)).toMatchObject({ iban: 'DE89370400440532013000' })
  })

  it('names iban and bic in the 400 when invalid', async () => {
    const b = backend()
    const p = await get(b)
    const res = await call(b, 'PUT', '/api/v1/finance/billing-profile', full(p, { iban: 'DE89370400440532013001', bic: 'ABC' }))
    expect(res.status).toBe(400)
    expect(res.body.error).toBe('validation_failed')
    expect(res.body.message).toContain('iban: not a valid IBAN')
    expect(res.body.message).toContain('bic: must be an 8 or 11 character BIC/SWIFT code')
    for (const bad of ['XX0000000000', 'DE8937040044053201300', 'DE893704004405320130000']) {
      expect((await call(b, 'PUT', '/api/v1/finance/billing-profile', full(p, { iban: bad }))).body.message).toContain('iban:')
    }
  })

  it('accepts an 8 character BIC and rejects an over-long tax number', async () => {
    const b = backend()
    const p = await get(b)
    expect((await call(b, 'PUT', '/api/v1/finance/billing-profile', full(p, { bic: 'COBADEFF' }))).status).toBe(200)
    const next = await get(b)
    const res = await call(b, 'PUT', '/api/v1/finance/billing-profile', full(next, { tax_number: 'x'.repeat(41) }))
    expect(res.status).toBe(400)
    expect(res.body.message).toContain('tax_number: exceeds 40 characters')
  })

  it('requires the version token and rejects a stale one', async () => {
    const b = backend()
    const p = await get(b)
    const { updated_at: _u, ...noToken } = full(p)
    expect((await call(b, 'PUT', '/api/v1/finance/billing-profile', noToken)).status).toBe(400)
    expect((await call(b, 'PUT', '/api/v1/finance/billing-profile', full(p, { updated_at: '2020-01-01T00:00:00Z' }))).status).toBe(409)
  })

  it('survives a reload through the persisted demo snapshot', async () => {
    const data = new Map<string, string>()
    const storage = { getItem: (k: string) => data.get(k) ?? null, setItem: (k: string, v: string) => void data.set(k, v), removeItem: (k: string) => void data.delete(k) }
    const mk = () => createDemoBackend({ storage, baseURL: '/demo/', now: () => NOW, createObjectUrl: () => 'blob:x' })
    const b1 = mk()
    const p = await get(b1)
    await call(b1, 'PUT', '/api/v1/finance/billing-profile', full(p, { iban: 'DE89 3704 0044 0532 0130 00' }))
    expect(await get(mk())).toMatchObject({ iban: 'DE89370400440532013000' })
  })
})

describe('demo invoice PDF', () => {
  it('serves a PDF attachment that carries the references and lines', async () => {
    const b = backend()
    const d = await newDraft(b)
    await call(b, 'PUT', `/api/v1/finance/invoices/${d.id}`, putBody(d, {
      buyer_reference: 'LEITWEG-1', lines: [{ description: 'Resident set', quantity: 2, unit_minor: 15000, unit_code: 'HUR' }],
    }))
    const res = await call(b, 'GET', `/api/v1/finance/invoices/${d.id}/pdf`)
    expect(res.status).toBe(200)
    expect(res.blob?.type).toBe('application/pdf')
    expect(res.headers?.['content-disposition']).toMatch(/^attachment; filename=/)
    const text = await res.blob!.text()
    expect(text.startsWith('%PDF-')).toBe(true)
    expect(text).toContain('LEITWEG-1')
    expect(text).toContain('Resident set')
    expect(text).toContain('HUR')
  })

  it('404s for an unknown invoice', async () => {
    expect((await call(backend(), 'GET', '/api/v1/finance/invoices/nope/pdf')).status).toBe(404)
  })
})
