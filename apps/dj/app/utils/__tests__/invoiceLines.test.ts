import { describe, expect, it } from 'vitest'
import type { InvoiceLine } from '../../types/finance'
import {
  draftsSignature,
  draftsToInput,
  emptyLine,
  lineProblems,
  lineTotalMinor,
  linesToDrafts,
  parseQuantity,
  parseValidationMessage,
  previewSubtotalMinor,
  unitSuffix,
  type LineDraft,
} from '../invoiceLines'

const row = (over: Partial<LineDraft> = {}): LineDraft => ({ ...emptyLine(), description: 'DJ set', unit: '80', ...over })

function saved(over: Partial<InvoiceLine> = {}): InvoiceLine {
  return {
    id: 'l1', invoice_id: 'inv-1', sort_order: 0, description: 'DJ set', quantity: 2, unit_minor: 8050,
    unit_code: 'HUR', tax_bps: 1900, line_total_minor: 16100, created_at: '2026-09-25T10:00:00Z', ...over,
  }
}

describe('draftsToInput', () => {
  it('converts typed major units to integer minor units without float math', () => {
    expect(draftsToInput([row({ unit: '80.50', quantity: '3' })], 'EUR')).toEqual([
      { description: 'DJ set', quantity: 3, unit_minor: 8050, unit_code: 'C62' },
    ])
    // 0.1 + 0.2 style inputs stay exact; comma decimals and thousands separators parse.
    expect(draftsToInput([row({ unit: '1.234,56' })], 'EUR')[0]!.unit_minor).toBe(123456)
    expect(draftsToInput([row({ unit: '19.99' })], 'EUR')[0]!.unit_minor).toBe(1999)
  })

  it('honours the currency minor-unit exponent', () => {
    expect(draftsToInput([row({ unit: '5000' })], 'JPY')[0]!.unit_minor).toBe(5000)
    expect(draftsToInput([row({ unit: '5' })], 'EUR')[0]!.unit_minor).toBe(500)
  })

  it('never emits derived fields and keeps the chosen unit code', () => {
    const [l] = draftsToInput([row({ unit_code: 'DAY' })], 'EUR')
    expect(Object.keys(l!).sort()).toEqual(['description', 'quantity', 'unit_code', 'unit_minor'])
    expect(l!.unit_code).toBe('DAY')
  })

  it('throws on an invalid row instead of sending a bad payload', () => {
    expect(() => draftsToInput([row({ quantity: '1.5' })], 'EUR')).toThrow()
    expect(() => draftsToInput([row({ unit: '' })], 'EUR')).toThrow()
  })
})

describe('line validation', () => {
  it('accepts whole quantities 1..1,000,000 only', () => {
    expect(parseQuantity('1')).toBe(1)
    expect(parseQuantity('1000000')).toBe(1_000_000)
    for (const bad of ['0', '1000001', '1.5', '-1', '', 'abc', '1e3']) expect(parseQuantity(bad)).toBeNull()
  })

  it('reports each bad field of each row and nothing for valid rows', () => {
    const problems = lineProblems([row(), row({ description: '  ', quantity: '0', unit: 'x' })], 'EUR')
    expect(problems[0]).toBeUndefined()
    expect(Object.keys(problems[1]!).sort()).toEqual(['description', 'quantity', 'unit'])
  })

  it('allows a zero unit price (the API allows unit_minor >= 0) but rejects negatives', () => {
    expect(lineProblems([row({ unit: '0' })], 'EUR')).toEqual({})
    expect(lineProblems([row({ unit: '-5' })], 'EUR')[0]?.unit).toBeTruthy()
  })

  it('rejects a line total beyond the API maximum', () => {
    expect(lineProblems([row({ quantity: '1000000', unit: '99999999' })], 'EUR')[0]?.unit).toMatch(/too large/i)
  })
})

describe('line totals', () => {
  it('multiplies quantity by unit in minor units', () => {
    expect(lineTotalMinor('3', '80.50', 'EUR')).toBe(24150)
    expect(lineTotalMinor('x', '80', 'EUR')).toBeNull()
  })

  it('sums only valid rows for the preview subtotal', () => {
    expect(previewSubtotalMinor([row({ quantity: '2' }), row({ unit: '' })], 'EUR')).toBe(16000)
  })
})

describe('linesToDrafts', () => {
  it('formats saved minor units back to major-unit text, ordered by sort_order', () => {
    const drafts = linesToDrafts([saved({ id: 'b', sort_order: 1, description: 'Travel', unit_minor: 100, unit_code: 'KMT' }), saved()], 'EUR')
    expect(drafts.map((d) => [d.description, d.quantity, d.unit, d.unit_code])).toEqual([
      ['DJ set', '2', '80.50', 'HUR'],
      ['Travel', '2', '1.00', 'KMT'],
    ])
  })

  it('always yields at least one row and falls back to piece for unknown codes', () => {
    expect(linesToDrafts([], 'EUR')).toHaveLength(1)
    expect(linesToDrafts([saved({ unit_code: 'XYZ' as never })], 'EUR')[0]!.unit_code).toBe('C62')
  })

  it('round-trips unchanged lines to an identical signature', () => {
    const a = linesToDrafts([saved()], 'EUR')
    const b = linesToDrafts([saved()], 'EUR')
    expect(a[0]!.key).not.toBe(b[0]!.key)
    expect(draftsSignature(a)).toBe(draftsSignature(b))
  })
})

describe('parseValidationMessage', () => {
  it('splits every field error of a 400 message, including indexed line paths', () => {
    expect(parseValidationMessage(
      'validation failed: lines[0].quantity: must be between 1 and 1000000; lines[1].unit_minor: must not be negative; buyer_reference: exceeds 100 characters',
    )).toEqual({
      'lines[0].quantity': 'must be between 1 and 1000000',
      'lines[1].unit_minor': 'must not be negative',
      buyer_reference: 'exceeds 100 characters',
    })
  })

  it('keeps the first message for a repeated field', () => {
    expect(parseValidationMessage('iban: first; iban: second')).toEqual({ iban: 'first' })
  })
})

describe('unitSuffix', () => {
  it('is empty for the default piece and a lower-case label otherwise', () => {
    expect(unitSuffix('C62')).toBe('')
    expect(unitSuffix('HUR')).toBe('hour')
    expect(unitSuffix('LS')).toBe('lump sum')
  })
})
