// @vitest-environment node
// Legal notes per supplier country in the browser demo (shared finance-mock
// core), mirroring the Go API: GET /invoices/tax-notes, and the wording on
// suggestions and new drafts.
import { describe, expect, it } from 'vitest'
import { createDemoBackend } from '../app/demo/backend'
import { FINANCE_GIG_IDS } from '../shared/finance-mock/seed'
import { notesFor } from '../shared/finance-mock/rules'
import type { Invoice } from '../app/types/finance'

const NOW = new Date('2026-09-25T12:00:00Z')
const backend = () => createDemoBackend({ storage: null, baseURL: '/demo/', now: () => NOW, createObjectUrl: () => 'blob:x' })
type B = ReturnType<typeof backend>

async function call(b: B, method: string, path: string, query = '', body: unknown = null) {
  const res = await b.handle({ method, path, query: new URLSearchParams(query), body })
// eslint-disable-next-line @typescript-eslint/no-explicit-any
  return { status: res.status, body: res.body as Record<string, any> }
}

describe('notesFor (mirrors tax.NotesFor in Go)', () => {
  it('has German, bilingual wording for a DE supplier', () => {
    const de = notesFor('de')
    expect(de.exempt).toContain('Gemäß § 19 UStG wird keine Umsatzsteuer berechnet')
    expect(de.reverse_charge).toContain('Steuerschuldnerschaft des Leistungsempfängers')
    expect(de.reverse_charge).toContain('Art. 196')
    expect(de.outside_scope).toContain('§ 3a Abs. 2 UStG')
    for (const n of Object.values(de)) expect(n).toContain(' / ')
    expect(Object.keys(de).sort()).toEqual(['exempt', 'outside_scope', 'reverse_charge'])
  })

  it('keeps the generic English wording elsewhere', () => {
    expect(notesFor('AT').exempt).toBe('VAT exempt: small business scheme.')
    expect(notesFor('').reverse_charge).toContain('Art. 196 Directive 2006/112/EC')
    expect(notesFor('AT').reverse_charge).not.toContain('Steuerschuldnerschaft')
  })
})

describe('demo GET /invoices/tax-notes', () => {
  it('returns the demo supplier\'s country and wording', async () => {
    const r = await call(backend(), 'GET', '/api/v1/finance/invoices/tax-notes')
    expect(r.status).toBe(200)
    expect(r.body.data.country).toBe('DE')
    expect(r.body.data.notes).toEqual(notesFor('DE'))
  })

  it('is GET only', async () => {
    const r = await call(backend(), 'POST', '/api/v1/finance/invoices/tax-notes')
    expect(r.status).not.toBe(200)
  })
})

describe('demo suggestions and drafts print the supplier country\'s wording', () => {
  it('a suggestion for a French business customer carries the German reverse-charge note', async () => {
    const r = await call(backend(), 'GET', '/api/v1/finance/invoices/tax-suggestion',
      'customer_country=FR&customer_vat_id=FR12345678901&customer_is_business=true')
    expect(r.body.data.vat_treatment).toBe('reverse_charge')
    expect(r.body.data.tax_note).toBe(notesFor('DE').reverse_charge)
  })

  it('a draft created with an explicit treatment takes the same note', async () => {
    const b = backend()
    const created = await call(b, 'POST', '/api/v1/finance/invoices', '', {
      gig_id: FINANCE_GIG_IDS.unbilled, vat_treatment: 'exempt',
    })
    expect(created.status).toBe(201)
    expect((created.body.data as Invoice).tax_note).toBe(notesFor('DE').exempt)
  })
})
