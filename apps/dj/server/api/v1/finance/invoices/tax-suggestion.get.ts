import * as ops from '../../../../../shared/finance-mock/ops'
import { db, send } from '../-mockDb'

export default defineEventHandler((event) => send(event, ops.taxSuggestion(db, getQuery(event))))
