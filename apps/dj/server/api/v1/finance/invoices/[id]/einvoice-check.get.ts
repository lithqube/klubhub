import * as ops from '../../../../../../shared/finance-mock/ops'
import { db, idOf, send } from '../../-mockDb'

// Dev stand-in: like a server without EINVOICE_URL, it answers 503.
export default defineEventHandler((event) => send(event, ops.einvoiceCheck(db, idOf(event), String(getQuery(event).format ?? ''))))
