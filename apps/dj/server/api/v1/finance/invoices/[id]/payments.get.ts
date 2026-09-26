import * as ops from '../../../../../../shared/finance-mock/ops'
import { db, idOf, send } from '../../-mockDb'

export default defineEventHandler((event) => send(event, ops.listPayments(db, idOf(event))))
