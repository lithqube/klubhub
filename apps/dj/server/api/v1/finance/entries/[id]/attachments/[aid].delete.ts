import * as att from '../../../../../../../shared/finance-mock/attachments'
import { db, idOf, send } from '../../../-mockDb'

export default defineEventHandler((event) =>
  send(event, att.removeAttachment(db, idOf(event), getRouterParam(event, 'aid') ?? '')),
)
