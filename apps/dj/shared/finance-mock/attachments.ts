// Receipts on ledger entries: the in-memory twin of the Go routes
//   GET|POST /finance/entries/{id}/attachments
//   GET|DELETE /finance/entries/{id}/attachments/{aid}
// (api/internal/finance/attachment*.go). Pure functions over a FinanceMockDb,
// shared by the Nitro dev routes and the browser demo. The file type is read
// from the bytes (JPEG, PNG, WebP, PDF) and never from the name or the
// client's claim; limits and error shapes match the API.

import type { EntryAttachment } from '../../app/types/finance'
import type { FinanceMockDb } from './db'
import { fail, ok, uuid, type MockResult } from './rules'

export const MAX_ATTACHMENT_BYTES = 15 * 1024 * 1024
export const MAX_ATTACHMENTS_PER_ENTRY = 10
const MAX_FILENAME = 200

export type AttachmentMime = EntryAttachment['mime_type']

export interface StoredAttachment extends EntryAttachment {
  bytes: Uint8Array
}

const startsWith = (b: Uint8Array, sig: number[], at = 0) => sig.every((v, i) => b[at + i] === v)

/** Detects the stored types from the file's own bytes; anything else is null. */
export function sniffMime(b: Uint8Array): AttachmentMime | null {
  if (startsWith(b, [0xff, 0xd8, 0xff])) return 'image/jpeg'
  if (startsWith(b, [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])) return 'image/png'
  if (startsWith(b, [0x52, 0x49, 0x46, 0x46]) && startsWith(b, [0x57, 0x45, 0x42, 0x50], 8)) return 'image/webp'
  if (startsWith(b, [0x25, 0x50, 0x44, 0x46, 0x2d])) return 'application/pdf'
  return null
}

/** Same rules as the Go sanitizer: base name only, no control characters. */
export function sanitizeFilename(name: string): string {
  const base = name.replace(/\\/g, '/').trim().split('/').pop() ?? ''
  // eslint-disable-next-line no-control-regex
  const clean = base.replace(/[\u0000-\u001f\u007f-\u009f]/g, '').trim()
  if (clean === '' || clean === '.' || clean === '..') return 'receipt'
  return [...clean].slice(0, MAX_FILENAME).join('')
}

// ── SHA-256 (sync, so the shared ops stay synchronous) ──

const K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
])

export function sha256Hex(data: Uint8Array): string {
  const h = new Uint32Array([0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19])
  const padded = new Uint8Array(Math.ceil((data.length + 9) / 64) * 64)
  padded.set(data)
  padded[data.length] = 0x80
  const view = new DataView(padded.buffer)
  view.setUint32(padded.length - 8, Math.floor((data.length * 8) / 0x100000000))
  view.setUint32(padded.length - 4, (data.length * 8) >>> 0)
  const w = new Uint32Array(64)
  const rotr = (x: number, n: number) => (x >>> n) | (x << (32 - n))
  for (let off = 0; off < padded.length; off += 64) {
    for (let i = 0; i < 16; i++) w[i] = view.getUint32(off + i * 4)
    for (let i = 16; i < 64; i++) {
      const s0 = rotr(w[i - 15]!, 7) ^ rotr(w[i - 15]!, 18) ^ (w[i - 15]! >>> 3)
      const s1 = rotr(w[i - 2]!, 17) ^ rotr(w[i - 2]!, 19) ^ (w[i - 2]! >>> 10)
      w[i] = (w[i - 16]! + s0 + w[i - 7]! + s1) >>> 0
    }
    let [a, b, c, d, e, f, g, hh] = h as unknown as number[]
    for (let i = 0; i < 64; i++) {
      const t1 = (hh! + (rotr(e!, 6) ^ rotr(e!, 11) ^ rotr(e!, 25)) + ((e! & f!) ^ (~e! & g!)) + K[i]! + w[i]!) >>> 0
      const t2 = ((rotr(a!, 2) ^ rotr(a!, 13) ^ rotr(a!, 22)) + ((a! & b!) ^ (a! & c!) ^ (b! & c!))) >>> 0
      hh = g; g = f; f = e; e = (d! + t1) >>> 0; d = c; c = b; b = a; a = (t1 + t2) >>> 0
    }
    const next = [a, b, c, d, e, f, g, hh] as number[]
    for (let i = 0; i < 8; i++) h[i] = (h[i]! + next[i]!) >>> 0
  }
  return [...h].map((v) => v.toString(16).padStart(8, '0')).join('')
}

// ── Routes ──

const entryNotFound = () => fail(404, 'not_found', 'finance entry not found')
const attachmentNotFound = () => fail(404, 'not_found', 'attachment not found')

/** Entry exists (404) and is still active (409 inactive). */
function ensureEntry(db: FinanceMockDb, entryId: string): MockResult | null {
  const e = db.entries.get(entryId)
  if (!e || e.deleted_at) return entryNotFound()
  if (e.status !== 'active') return fail(409, 'inactive', 'voided finance entries cannot change their attachments')
  return null
}

const publicView = ({ bytes: _bytes, ...a }: StoredAttachment): EntryAttachment => ({ ...a })

/** GET /entries/{id}/attachments: oldest first. A voided entry still lists its files. */
export function listAttachments(db: FinanceMockDb, entryId: string): MockResult {
  const e = db.entries.get(entryId)
  if (!e || e.deleted_at) return entryNotFound()
  return ok({ data: db.attachmentsOf(entryId).map(publicView) })
}

/** POST /entries/{id}/attachments: `bytes` is the one `file` part (null when it is missing). */
export function addAttachment(db: FinanceMockDb, entryId: string, file: { name: string; bytes: Uint8Array } | null): MockResult {
  const blocked = ensureEntry(db, entryId)
  if (blocked) return blocked
  if (!file) return fail(400, 'bad_request', 'missing "file" part')
  const { bytes } = file
  if (bytes.length === 0) return fail(400, 'validation_failed', 'invalid attachment: the file is empty')
  const mime = sniffMime(bytes)
  if (!mime) return fail(415, 'unsupported_media_type', 'unsupported attachment type: use a JPEG, PNG, WebP or PDF file')
  if (bytes.length > MAX_ATTACHMENT_BYTES) {
    return fail(413, 'too_large', `attachment too large: the limit is ${MAX_ATTACHMENT_BYTES >> 20} MB per file`)
  }
  if (db.attachmentsOf(entryId).length >= MAX_ATTACHMENTS_PER_ENTRY) {
    return fail(409, 'limit_reached', `an entry can hold at most ${MAX_ATTACHMENTS_PER_ENTRY} files`)
  }
  const a = db.putAttachment({
    id: uuid(),
    entry_id: entryId,
    filename: sanitizeFilename(file.name),
    mime_type: mime,
    size_bytes: bytes.length,
    checksum_sha256: sha256Hex(bytes),
    created_at: db.stamp(),
    bytes: bytes.slice(),
  })
  return ok({ data: publicView(a) }, 201)
}

export interface AttachmentFile {
  bytes: Uint8Array
  mime: AttachmentMime
  filename: string
  /** `inline` only for an image asked for with ?inline=1; a PDF is always an attachment. */
  disposition: 'inline' | 'attachment'
}

/** GET /entries/{id}/attachments/{aid}: the bytes, or an error result. */
export function readAttachment(db: FinanceMockDb, entryId: string, id: string, inline: boolean): AttachmentFile | MockResult {
  const e = db.entries.get(entryId)
  if (!e || e.deleted_at) return entryNotFound()
  const a = db.attachmentsOf(entryId).find((x) => x.id === id)
  if (!a) return attachmentNotFound()
  return {
    bytes: a.bytes,
    mime: a.mime_type,
    filename: a.filename,
    disposition: inline && a.mime_type !== 'application/pdf' ? 'inline' : 'attachment',
  }
}

export function isAttachmentFile(r: AttachmentFile | MockResult): r is AttachmentFile {
  return 'bytes' in r
}

/** DELETE /entries/{id}/attachments/{aid}: a voided entry's evidence cannot be removed. */
export function removeAttachment(db: FinanceMockDb, entryId: string, id: string): MockResult {
  const e = db.entries.get(entryId)
  if (!e || e.deleted_at) return entryNotFound()
  if (!db.attachmentsOf(entryId).some((x) => x.id === id)) return attachmentNotFound()
  if (e.status !== 'active') return fail(409, 'inactive', 'voided finance entries cannot change their attachments')
  db.dropAttachment(entryId, id)
  return { status: 204, body: {} }
}
