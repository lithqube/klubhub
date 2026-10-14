// Editable invoice lines: the draft-editor row model and its conversion to
// the API's `lines` payload. Prices are typed in major units and converted
// to integer minor units with string arithmetic (utils/money.ts), so no float
// ever touches money. Line totals shown here are previews only; the server
// derives the real total and tax rate on save.

import type { InvoiceLine, InvoiceLineInput, UnitCode } from '../types/finance'
import { DEFAULT_UNIT_CODE, UNIT_CODES } from '../types/finance'
import { minorToDecimalString, parseMoney } from './money'

/** Limits from the API (invoice_validation.go). */
export const MAX_LINES = 100
export const MAX_LINE_DESCRIPTION = 500
export const MAX_LINE_QUANTITY = 1_000_000
const MAX_LINE_TOTAL_MINOR = 10_000_000_000_000n

/** One editable row. Numbers stay strings while typing. */
export interface LineDraft {
  /** Stable client key for v-for; never sent. */
  key: string
  description: string
  quantity: string
  /** Unit price in major units, as typed ("80", "12,50"). */
  unit: string
  unit_code: UnitCode
}

let keySeq = 0
export function newLineKey(): string {
  keySeq += 1
  return `line-${keySeq}`
}

export function emptyLine(): LineDraft {
  return { key: newLineKey(), description: '', quantity: '1', unit: '', unit_code: DEFAULT_UNIT_CODE }
}

export function isUnitCode(v: unknown): v is UnitCode {
  return typeof v === 'string' && UNIT_CODES.some((u) => u.value === v)
}

/** Rows for the editor from the invoice's saved lines (never empty). */
export function linesToDrafts(lines: InvoiceLine[], currency: string): LineDraft[] {
  if (!lines.length) return [emptyLine()]
  return [...lines]
    .sort((a, b) => a.sort_order - b.sort_order)
    .map((l) => ({
      key: newLineKey(),
      description: l.description,
      quantity: String(l.quantity),
      unit: minorToDecimalString(l.unit_minor, currency),
      unit_code: isUnitCode(l.unit_code) ? l.unit_code : DEFAULT_UNIT_CODE,
    }))
}

/** Stable comparison form without the client-only `key`. */
export function draftsSignature(drafts: LineDraft[]): string {
  return JSON.stringify(drafts.map(({ description, quantity, unit, unit_code }) => ({ description, quantity, unit, unit_code })))
}

export interface LineProblems {
  description?: string
  quantity?: string
  unit?: string
}

/** Parses a quantity string: integer 1..1,000,000, else null. */
export function parseQuantity(raw: string): number | null {
  const s = raw.trim()
  if (!/^\d{1,7}$/.test(s)) return null
  const n = Number(s)
  return n >= 1 && n <= MAX_LINE_QUANTITY ? n : null
}

/** quantity × unit as a BigInt-safe integer; null when either side is invalid. */
export function lineTotalMinor(quantity: string, unit: string, currency: string): number | null {
  const q = parseQuantity(quantity)
  const u = parseMoney(unit, currency)
  if (q === null || u === null) return null
  const total = BigInt(q) * BigInt(u)
  return total > MAX_LINE_TOTAL_MINOR ? null : Number(total)
}

/** Per-row problems, keyed by row index; empty object when every row is valid. */
export function lineProblems(drafts: LineDraft[], currency: string): Record<number, LineProblems> {
  const out: Record<number, LineProblems> = {}
  drafts.forEach((d, i) => {
    const p: LineProblems = {}
    const desc = d.description.trim()
    if (!desc) p.description = 'Describe what you are billing.'
    else if ([...desc].length > MAX_LINE_DESCRIPTION) p.description = `Use at most ${MAX_LINE_DESCRIPTION} characters.`
    if (parseQuantity(d.quantity) === null) p.quantity = `Whole number from 1 to ${MAX_LINE_QUANTITY.toLocaleString('en')}.`
    const unit = parseMoney(d.unit, currency)
    if (unit === null) p.unit = 'Enter the price per unit, e.g. 80 or 12.50.'
    else if (p.quantity === undefined && lineTotalMinor(d.quantity, d.unit, currency) === null) p.unit = 'Line total is too large.'
    if (p.description || p.quantity || p.unit) out[i] = p
  })
  return out
}

/** Sum of the valid row totals (preview only). */
export function previewSubtotalMinor(drafts: LineDraft[], currency: string): number {
  return drafts.reduce((s, d) => s + (lineTotalMinor(d.quantity, d.unit, currency) ?? 0), 0)
}

/**
 * Payload for `lines`: the FULL list, minor units, no derived fields.
 * Callers must check `lineProblems` first; invalid rows throw.
 */
export function draftsToInput(drafts: LineDraft[], currency: string): InvoiceLineInput[] {
  return drafts.map((d) => {
    const quantity = parseQuantity(d.quantity)
    const unit_minor = parseMoney(d.unit, currency)
    if (quantity === null || unit_minor === null) throw new Error('Invalid invoice line.')
    return { description: d.description.trim(), quantity, unit_minor, unit_code: d.unit_code }
  })
}

/** Label for a unit code ("HUR" → "Hour"); unknown codes fall back to the code. */
export function unitLabel(code: string): string {
  return UNIT_CODES.find((u) => u.value === code)?.label ?? code
}

/** Lower-case short unit for "2 × hour" style display; empty for the default piece. */
export function unitSuffix(code: string): string {
  return code === DEFAULT_UNIT_CODE || !code ? '' : unitLabel(code).toLowerCase()
}

/**
 * Splits a 400 validation message ("validation failed: a: x; lines[0].b: y")
 * into field → message, keeping the first message per field.
 */
export function parseValidationMessage(message: string): Record<string, string> {
  const out: Record<string, string> = {}
  const body = message.replace(/^validation failed:\s*/, '')
  for (const part of body.split(/;\s+/)) {
    const m = /^([a-z_]+(?:\[\d+\])?(?:\.[a-z_0-9]+(?:\[\d+\])?)*):\s+(.*)$/.exec(part.trim())
    if (m && !out[m[1]!]) out[m[1]!] = m[2]!
  }
  return out
}
