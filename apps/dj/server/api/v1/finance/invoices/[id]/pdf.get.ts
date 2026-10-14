import * as ops from '../../../../../../shared/finance-mock/ops'
import { buildPdf } from '../../../../../../app/demo/lib/pdf'
import { db, idOf, send } from '../../-mockDb'
import { fail } from '../../../../../../shared/finance-mock/rules'

// Dev stand-in for the Go renderer: a plain-text PDF attachment.
export default defineEventHandler((event) => {
  const pdf = ops.invoicePdfLines(db, idOf(event))
  if (!pdf) return send(event, fail(404, 'not_found', 'invoice not found'))
  setHeader(event, 'Content-Type', 'application/pdf')
  setHeader(event, 'Content-Disposition', `attachment; filename="${pdf.name}"`)
  return buildPdf(pdf.lines)
})
