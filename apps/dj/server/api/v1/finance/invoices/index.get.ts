import * as ops from '../../../../../shared/finance-mock/ops'
import { db, send } from '../-mockDb'

export default defineEventHandler((event) => send(event, ops.listInvoices(db, getQuery(event))))
