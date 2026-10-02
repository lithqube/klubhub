import * as att from '../../../../../../../shared/finance-mock/attachments'
import { fail } from '../../../../../../../shared/finance-mock/rules'
import { db, idOf, send } from '../../../-mockDb'

// Multipart upload: exactly one part named `file`, like the Go handler.
export default defineEventHandler(async (event) => {
  const parts = await readMultipartFormData(event).catch(() => undefined)
  if (!parts) return send(event, fail(400, 'bad_request', 'send the file as multipart/form-data in a "file" part'))
  const part = parts.find((p) => p.name === 'file' && p.filename !== undefined)
  const file = part ? { name: part.filename ?? '', bytes: new Uint8Array(part.data) } : null
  return send(event, att.addAttachment(db, idOf(event), file))
})
