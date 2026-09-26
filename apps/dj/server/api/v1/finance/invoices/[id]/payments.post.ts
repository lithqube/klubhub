import * as ops from '../../../../../../shared/finance-mock/ops'
import { bodyOf, db, idOf, send } from '../../-mockDb'

export default defineEventHandler(async (event) => send(event, ops.createPayment(db, idOf(event), await bodyOf(event))))
