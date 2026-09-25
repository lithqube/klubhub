import type { Party } from '../../../../../app/types/finance'
import {
  badRequest, fail, gigFor, learnGig, invoices, isDateOnly, isTreatment, newDraft, normalizeDueAt, normalizeParty,
  prefixProblem, recompute, serialize, suggest, supplier, NOTES,
} from '../-mockDb'

export default defineEventHandler(async (event) => {
  const body = (await readBody(event).catch(() => null)) as Record<string, unknown> | null
  if (!body || typeof body.gig_id !== 'string' || !body.gig_id) return badRequest(event, ['gig_id: is required'])
  const gigId = body.gig_id
  await learnGig(gigId)
  const gig = gigFor(gigId)

  const errs: string[] = []
  if (body.vat_treatment !== undefined && !isTreatment(body.vat_treatment)) errs.push('vat_treatment: unknown treatment')
  if (body.currency !== undefined && body.currency !== gig.currency) errs.push(`currency: must match the gig fee currency ${gig.currency}`)
  if (body.tax_rate_bps !== undefined && (!Number.isInteger(body.tax_rate_bps) || (body.tax_rate_bps as number) < 0 || (body.tax_rate_bps as number) > 10000)) {
    errs.push('tax_rate_bps: must be between 0 and 10000')
  }
  if (body.withholding_rate_bps !== undefined && (!Number.isInteger(body.withholding_rate_bps) || (body.withholding_rate_bps as number) < 0 || (body.withholding_rate_bps as number) > 10000)) {
    errs.push('withholding_rate_bps: must be between 0 and 10000')
  }
  if (body.supply_date !== undefined && body.supply_date !== null && !isDateOnly(body.supply_date)) errs.push('supply_date: must be a date in YYYY-MM-DD format')
  const due = normalizeDueAt(body.due_at)
  if (due === undefined) errs.push('due_at: must be RFC 3339 or YYYY-MM-DD')
  if (body.number_prefix !== undefined && String(body.number_prefix).trim() !== '') {
    const pp = prefixProblem(body.number_prefix)
    if (pp) errs.push(pp)
  }
  if (errs.length) return badRequest(event, errs)

  const active = [...invoices.values()].find(
    (i) => i.gig_id === gigId && i.kind === 'invoice' && ['draft', 'issued', 'paid'].includes(i.status),
  )
  if (active) return fail(event, 409, 'bad_state', 'this gig already has an active invoice')

  const inv = newDraft(gigId)
  if (body.customer && typeof body.customer === 'object') inv.customer = normalizeParty(body.customer as Partial<Party>, inv.customer)
  if (isTreatment(body.vat_treatment)) {
    const suggested = suggest(inv.customer)
    if (body.vat_treatment !== suggested.vat_treatment) {
      // §4.1: a treatment other than the suggestion takes its default rate/note.
      inv.vat_treatment = body.vat_treatment
      inv.tax_rate_bps = body.vat_treatment === 'domestic' ? supplier.default_vat_rate_bps : 0
      inv.tax_note = NOTES[body.vat_treatment] ?? ''
    }
  }
  if (typeof body.tax_rate_bps === 'number') inv.tax_rate_bps = body.tax_rate_bps
  if (typeof body.withholding_rate_bps === 'number') inv.withholding_rate_bps = body.withholding_rate_bps
  if (typeof body.supply_date === 'string') inv.supply_date = body.supply_date
  if (due) inv.due_at = due
  if (typeof body.number_prefix === 'string' && body.number_prefix.trim()) inv.number_prefix = body.number_prefix.trim().toUpperCase()
  recompute(inv)
  setResponseStatus(event, 201)
  return { data: serialize(inv) }
})
