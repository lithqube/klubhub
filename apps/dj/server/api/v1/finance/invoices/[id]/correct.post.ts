import { badRequest, checkToken, creditNoteFor, fail, findInvoice, newDraft, serialize, stamp } from '../../-mockDb'

export default defineEventHandler(async (event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  const body = ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
  const tokenErr = checkToken(event, inv, body.updated_at)
  if (tokenErr) return tokenErr
  if (inv.kind !== 'invoice' || (inv.status !== 'issued' && inv.status !== 'paid')) {
    return fail(event, 409, 'bad_state', 'only issued or paid invoices can be corrected')
  }
  const reason = String(body.reason ?? '').trim()
  if (reason.length > 1000) return badRequest(event, ['reason: exceeds 1000 characters'])
  const cn = creditNoteFor(inv, reason)
  const replacement = newDraft(inv.gig_id, {
    currency: inv.currency,
    customer: structuredClone(inv.customer),
    vat_treatment: inv.vat_treatment,
    tax_rate_bps: inv.tax_rate_bps,
    tax_note: inv.tax_note,
    withholding_rate_bps: inv.withholding_rate_bps,
    supply_date: inv.supply_date,
    due_at: inv.due_at,
    number_prefix: inv.number_prefix,
    internal_notes: `Replaces ${inv.invoice_number}`,
  })
  inv.status = 'corrected'
  inv.replaced_by_invoice_id = replacement.id
  inv.updated_at = stamp()
  setResponseStatus(event, 201)
  return { data: { credit_note: serialize(cn), original: serialize(inv), replacement: serialize(replacement) } }
})
