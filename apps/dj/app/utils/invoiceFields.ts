// Field ids and error lookups shared by the invoice editors and the issue
// checklist. The API names problems by dotted path ("customer.vat_id"); the
// editors give each input a DOM id derived from that path so a checklist
// item can move focus to exactly the field it is about.

import type { IssueProblem } from '../types/finance'

/** DOM id for an invoice field path ("customer.vat_id" → "invf-customer-vat_id"). */
export function fieldId(path: string): string {
  return `invf-${path.replace(/[^a-zA-Z0-9_]+/g, '-')}`
}

/** Id of the message element that describes a field (aria-describedby). */
export function fieldMessageId(path: string): string {
  return `${fieldId(path)}-msg`
}

/**
 * Where a problem can be fixed. Supplier fields live in the billing profile,
 * which has no editor in this sheet yet (docs/INVOICING.md §6), and totals
 * come from the gig fee.
 */
export type ProblemTarget = 'field' | 'billing_profile' | 'gig'

export function problemTarget(field: string): ProblemTarget {
  if (field.startsWith('supplier.')) return 'billing_profile'
  if (field === 'total_minor' || field === 'currency' || field === 'status') return 'gig'
  return 'field'
}

/** Moves focus to the input for a problem; returns false when none is rendered. */
export function focusField(path: string, root: ParentNode = document): boolean {
  const el = root.querySelector<HTMLElement>(`#${fieldId(path)}`)
  if (!el) return false
  el.focus()
  if (typeof el.scrollIntoView === 'function') el.scrollIntoView({ block: 'center' })
  return true
}

/** Groups problems by field so each input can show its own message. */
export function problemsByField(problems: IssueProblem[] | null | undefined): Record<string, string> {
  const out: Record<string, string> = {}
  for (const p of problems ?? []) {
    if (!out[p.field]) out[p.field] = p.message
  }
  return out
}

/**
 * Number prefix rule from the API (invoice_validation.go): 1–20 letters or
 * digits, upper-cased, and never "CN" (reserved for credit notes).
 * Returns '' when valid.
 */
export function prefixError(raw: string): string {
  const p = raw.trim().toUpperCase()
  if (p.length < 1 || p.length > 20) return 'Use 1 to 20 characters.'
  if (!/^[A-Z0-9]+$/.test(p)) return 'Use letters and digits only.'
  if (p === 'CN') return 'CN is reserved for credit notes.'
  return ''
}

/** Country must be empty or a two-letter ISO code. Returns '' when valid. */
export function countryError(raw: string): string {
  const c = raw.trim()
  return c === '' || /^[A-Za-z]{2}$/.test(c) ? '' : 'Use a two-letter country code, e.g. DE.'
}

/**
 * Inline message for a failed invoice action, or null when the store has
 * already surfaced it (409 conflict / bad_state show the sheet banner).
 */
export function inlineErrorMessage(err: { code: string; message: string }): string | null {
  switch (err.code) {
    case 'conflict':
    case 'bad_state':
      return null
    case 'not_issuable':
      return 'Not ready to issue yet: fix the items in the issue check.'
    case 'exceeds_balance':
      return 'That amount is more than the invoice balance allows. Balances are refreshed; check the amount.'
    case 'invoice_not_payable':
      return 'This invoice no longer accepts payments. Showing the latest state.'
    case 'network':
      return 'Could not reach the server. Check your connection and try again.'
    default:
      return err.message || 'Something went wrong. Try again.'
  }
}
