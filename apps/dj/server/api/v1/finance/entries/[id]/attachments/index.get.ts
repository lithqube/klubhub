import * as att from '../../../../../../../shared/finance-mock/attachments'
import { db, idOf, send } from '../../../-mockDb'

export default defineEventHandler((event) => send(event, att.listAttachments(db, idOf(event))))
