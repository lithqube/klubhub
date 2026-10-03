import * as att from '../../../../../../../shared/finance-mock/attachments'
import { db, idOf, send } from '../../../-mockDb'

// The file itself. Default `attachment`; `?inline=1` shows images inline (never a PDF).
export default defineEventHandler((event) => {
  const aid = getRouterParam(event, 'aid') ?? ''
  const res = att.readAttachment(db, idOf(event), aid, getQuery(event).inline === '1')
  if (!att.isAttachmentFile(res)) return send(event, res)
  const ascii = res.filename.replace(/[^\x20-\x7e]/g, '_').replace(/["\\]/g, '_')
  setHeader(event, 'Content-Type', res.mime)
  setHeader(event, 'Content-Disposition', `${res.disposition}; filename="${ascii}"; filename*=UTF-8''${encodeURIComponent(res.filename)}`)
  setHeader(event, 'Content-Length', res.bytes.length)
  setHeader(event, 'Cache-Control', 'private, no-store')
  setHeader(event, 'X-Content-Type-Options', 'nosniff')
  setHeader(event, 'Content-Security-Policy', "default-src 'none'; sandbox")
  return res.bytes
})
