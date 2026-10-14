import * as ops from '../../../../shared/finance-mock/ops'
import { db, send } from './-mockDb'

// Dev stand-in: like a server without object storage, it keeps no archive (503).
export default defineEventHandler((event) => send(event, ops.listDocuments(db, String(getQuery(event).owner_id ?? ''))))
