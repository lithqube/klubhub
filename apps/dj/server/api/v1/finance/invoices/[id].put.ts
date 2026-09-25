import * as ops from '../../../../../shared/finance-mock/ops'
import { bodyOf, db, idOf, send } from '../-mockDb'

// Full replacement of the listed fields (§4.1); draft only.
export default defineEventHandler(async (event) => send(event, ops.updateInvoice(db, idOf(event), await bodyOf(event))))
