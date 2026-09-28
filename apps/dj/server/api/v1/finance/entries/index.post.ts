import * as ops from '../../../../shared/finance-mock/ops'
import { bodyOf, db, send } from '../-mockDb'

export default defineEventHandler(async (event) =>
  send(event, ops.createEntry(db, await bodyOf(event))),
)
