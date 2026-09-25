import * as ops from '../../../../../../shared/finance-mock/ops'
import { db, idOf, send } from '../../-mockDb'

export default defineEventHandler((event) => send(event, ops.issueCheck(db, idOf(event))))
