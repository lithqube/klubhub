import type { Party } from '../../../../../app/types/finance'
import {
  badRequest, checkToken, fail, findInvoice, isDateOnly, isTreatment, normalizeDueAt, normalizeParty,
  prefixProblem, recompute, serialize, stamp,
} from '../-mockDb'

// Full replacement of the listed fields (§4.1); draft only.
export default defineEventHandler(async (event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  const body = ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
  const tokenErr = checkToken(event, inv, body.updated_at)
  if (tokenErr) return tokenErr
  if (inv.status !== 'draft') return fail(event, 409, 'bad_state', 'only draft invoices can be edited')

  const errs: string[] = []
  const customer = normalizeParty((body.customer ?? {}) as Partial<Party>, inv.customer)
  if (customer.country && !/^[A-Z]{2}$/.test(customer.country)) errs.push('customer.country: must be a 2-letter ISO 3166-1 alpha-2 code')
  if (!isTreatment(body.vat_treatment)) errs.push('vat_treatment: must be one of domestic, reverse_charge, exempt, outside_scope, us_sales_tax, none')
  const rate = Number(body.tax_rate_bps)
  if (!Number.isInteger(rate) || rate < 0 || rate > 10000) errs.push('tax_rate_bps: must be between 0 and 10000')
  // Drafts accept 0–10000; issuing requires ≤ 5000 (issue-check).
  const wh = Number(body.withholding_rate_bps)
  if (!Number.isInteger(wh) || wh < 0 || wh > 10000) errs.push('withholding_rate_bps: must be between 0 and 10000')
  const supply = body.supply_date === '' || body.supply_date == null ? null : body.supply_date
  if (supply !== null && !isDateOnly(supply)) errs.push('supply_date: must be a date in YYYY-MM-DD format')
  const due = normalizeDueAt(body.due_at)
  if (due === undefined) errs.push('due_at: must be RFC 3339 or YYYY-MM-DD')
  const prefix = String(body.number_prefix ?? '').trim() || 'INV'
  const pp = prefixProblem(prefix)
  if (pp) errs.push(pp)
  if (errs.length) return badRequest(event, errs)

  inv.customer = customer
  inv.vat_treatment = body.vat_treatment as typeof inv.vat_treatment
  inv.tax_rate_bps = rate
  inv.tax_note = String(body.tax_note ?? '')
  inv.withholding_rate_bps = wh
  inv.supply_date = supply as string | null
  inv.due_at = due ?? null
  inv.number_prefix = prefix.toUpperCase()
  inv.internal_notes = String(body.internal_notes ?? '')
  recompute(inv)
  inv.updated_at = stamp()
  return { data: serialize(inv) }
})
