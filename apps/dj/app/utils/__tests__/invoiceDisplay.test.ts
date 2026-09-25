import { describe, it, expect } from 'vitest'
import {
  addDays,
  emptyParty,
  invoiceDisplayStatus,
  invoiceNumberLabel,
  isActiveInvoice,
  partyDisplayName,
  signedTotal,
  statusLabel,
  toApiTime,
} from '../invoiceDisplay'

const TODAY = '2026-09-25'

describe('invoiceDisplayStatus', () => {
  it('derives overdue from issued + past due date', () => {
    expect(invoiceDisplayStatus({ status: 'issued', due_at: '2026-09-24T00:00:00Z' }, TODAY)).toBe('overdue')
    expect(invoiceDisplayStatus({ status: 'issued', due_at: '2026-09-25T00:00:00Z' }, TODAY)).toBe('issued')
    expect(invoiceDisplayStatus({ status: 'issued', due_at: null }, TODAY)).toBe('issued')
  })

  it('maps cancelled/credited/corrected to void', () => {
    for (const s of ['cancelled', 'credited', 'corrected'] as const) {
      expect(invoiceDisplayStatus({ status: s, due_at: null }, TODAY)).toBe('void')
    }
  })

  it('keeps draft and paid', () => {
    expect(invoiceDisplayStatus({ status: 'draft', due_at: '2020-01-01' }, TODAY)).toBe('draft')
    expect(invoiceDisplayStatus({ status: 'paid', due_at: '2020-01-01' }, TODAY)).toBe('paid')
  })

  it('labels void statuses with the raw status', () => {
    expect(statusLabel({ status: 'credited', due_at: null, kind: 'invoice' }, TODAY)).toBe('CREDITED')
    expect(statusLabel({ status: 'issued', due_at: '2026-01-01', kind: 'invoice' }, TODAY)).toBe('OVERDUE')
  })
})

describe('helpers', () => {
  it('active invoice = invoice kind in draft/issued/paid', () => {
    expect(isActiveInvoice({ kind: 'invoice', status: 'draft' })).toBe(true)
    expect(isActiveInvoice({ kind: 'invoice', status: 'cancelled' })).toBe(false)
    expect(isActiveInvoice({ kind: 'credit_note', status: 'issued' })).toBe(false)
  })

  it('credit notes read negative', () => {
    expect(signedTotal({ kind: 'credit_note', total_minor: 1000 })).toBe(-1000)
    expect(signedTotal({ kind: 'invoice', total_minor: 1000 })).toBe(1000)
  })

  it('prefers company over legal name', () => {
    expect(partyDisplayName({ ...emptyParty(), legal_name: 'Sam Example', company: 'Club Alpha' })).toBe('Club Alpha')
    expect(partyDisplayName({ ...emptyParty(), legal_name: 'Jane Doe' })).toBe('Jane Doe')
  })

  it('labels drafts without a number', () => {
    expect(invoiceNumberLabel({ invoice_number: null, kind: 'invoice' })).toBe('DRAFT')
    expect(invoiceNumberLabel({ invoice_number: 'INV-0001-EUR', kind: 'invoice' })).toBe('INV-0001-EUR')
  })

  it('date math', () => {
    expect(addDays('2026-09-20', 14)).toBe('2026-10-04')
    expect(addDays('2026-09-20T00:00:00Z', 14)).toBe('2026-10-04')
    expect(toApiTime('2026-10-04')).toBe('2026-10-04T00:00:00Z')
    expect(toApiTime('')).toBeNull()
  })
})
