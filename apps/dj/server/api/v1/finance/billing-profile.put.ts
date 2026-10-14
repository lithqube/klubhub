import * as ops from '../../../../shared/finance-mock/ops'
import { bodyOf, db, send } from './-mockDb'

// Same validation and normalisation as the Go API (tax number, IBAN, BIC).
export default defineEventHandler(async (event) => send(event, ops.updateBillingProfile(db, await bodyOf(event))))
