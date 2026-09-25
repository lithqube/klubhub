import * as ops from '../../../../../shared/finance-mock/ops'
import { db, send } from '../-mockDb'

// §4.1: invoices only (credit notes excluded).
export default defineEventHandler((event) => send(event, ops.summaries(db)))
