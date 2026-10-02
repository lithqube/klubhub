// @vitest-environment node
// Receipts on ledger entries, frontend-only mocks: the four Nitro dev routes
// (server/api/v1/finance/entries/[id]/attachments/**) and the browser demo
// answer exactly like the Go API: 15 MB / 10 files / JPEG-PNG-WebP-PDF read
// from the bytes, error shapes, voided entries read-only, attachment_count on
// every entry response and the `receipt` list filter.
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { createDemoBackend } from '../app/demo/backend'
import type { KeyValueStorage } from '../app/demo/state'
import { sha256Hex, sniffMime } from '../shared/finance-mock/attachments'
import type { Entry, EntryAttachment } from '../app/types/finance'

// ── byte fixtures ──
const bytes = (...head: number[]) => Uint8Array.from([...head, ...Array(32).fill(7)])
const JPEG = () => bytes(0xff, 0xd8, 0xff, 0xe0)
const PNG = () => bytes(0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a)
const WEBP = () => bytes(0x52, 0x49, 0x46, 0x46, 1, 0, 0, 0, 0x57, 0x45, 0x42, 0x50)
const PDF = () => new TextEncoder().encode('%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF')
const HEIC = () => bytes(0, 0, 0, 0x18, 0x66, 0x74, 0x79, 0x70, 0x68, 0x65, 0x69, 0x63)
const SVG = () => new TextEncoder().encode('<svg xmlns="http://www.w3.org/2000/svg"></svg>')
const HTML = () => new TextEncoder().encode('<!doctype html><script>alert(1)</script>')

describe('file type detection and checksum', () => {
  it('reads the type from the bytes', () => {
    expect(sniffMime(JPEG())).toBe('image/jpeg')
    expect(sniffMime(PNG())).toBe('image/png')
    expect(sniffMime(WEBP())).toBe('image/webp')
    expect(sniffMime(PDF())).toBe('application/pdf')
    for (const nope of [HEIC(), SVG(), HTML(), new Uint8Array(), bytes(0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x41, 0x56, 0x45)]) {
      expect(sniffMime(nope)).toBeNull()
    }
  })

  it('computes SHA-256 like the server (known vectors, multi-block input)', () => {
    const hex = (s: string) => sha256Hex(new TextEncoder().encode(s))
    expect(hex('')).toBe('e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')
    expect(hex('abc')).toBe('ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
    expect(hex('abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq')).toBe('248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1')
    const big = new Uint8Array(1_000_000).fill(0x61)
    expect(sha256Hex(big)).toBe('cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0')
  })
})

// ───────────────────────── Nitro dev routes ─────────────────────────

interface FakeEvent {
  params: Record<string, string>
  query?: Record<string, string>
  body?: unknown
  parts?: { name?: string; filename?: string; data: Uint8Array }[]
  headers: Record<string, string | number>
  status: number
}

vi.stubGlobal('defineEventHandler', (h: unknown) => h)
vi.stubGlobal('setResponseStatus', (e: FakeEvent, s: number) => { e.status = s })
vi.stubGlobal('getRouterParam', (e: FakeEvent, name: string) => e.params[name])
vi.stubGlobal('readBody', async (e: FakeEvent) => e.body)
vi.stubGlobal('getQuery', (e: FakeEvent) => e.query ?? {})
vi.stubGlobal('setHeader', (e: FakeEvent, k: string, v: string | number) => { e.headers[k] = v })
vi.stubGlobal('readMultipartFormData', async (e: FakeEvent) => e.parts)

type Handler = (e: FakeEvent) => unknown
/** Loosely typed JSON bodies: the assertions name the fields they care about. */
type Res = Record<string, any> // eslint-disable-line @typescript-eslint/no-explicit-any
const base = '../server/api/v1/finance'
let mock: typeof import('../server/api/v1/finance/-mockDb')
const h: Record<string, Handler> = {}

beforeAll(async () => {
  mock = await import(`${base}/-mockDb`)
  const load = async (key: string, path: string) => { h[key] = (await import(`${base}/${path}`)).default as Handler }
  await load('list', 'entries/index.get')
  await load('get', 'entries/[id].get')
  await load('put', 'entries/[id].put')
  await load('void', 'entries/[id]/void.post')
  await load('delete', 'entries/[id].delete')
  await load('aList', 'entries/[id]/attachments/index.get')
  await load('aPost', 'entries/[id]/attachments/index.post')
  await load('aGet', 'entries/[id]/attachments/[aid].get')
  await load('aDelete', 'entries/[id]/attachments/[aid].delete')
})

async function call(name: string, opts: Partial<FakeEvent> = {}) {
  const e: FakeEvent = { params: {}, headers: {}, status: 200, ...opts }
  const res = await h[name]!(e)
  return { status: e.status, res: res as Res, headers: e.headers }
}

const newEntry = (over: Record<string, unknown> = {}) =>
  mock.db.addEntry({ kind: 'expense', amount_minor: 1000, currency: 'EUR', category: 'gear', entry_date: '2026-09-01', notes: 'Cable', ...over })

const upload = (entryId: string, data: Uint8Array, filename = 'receipt.bin') =>
  call('aPost', { params: { id: entryId }, parts: [{ name: 'file', filename, data }] })

describe('Nitro routes: upload', () => {
  it('stores a receipt and returns the API shape (no storage key, no bytes)', async () => {
    const e = newEntry()
    const { status, res } = await upload(e.id, PNG(), 'hotel.png')
    expect(status).toBe(201)
    expect(Object.keys(res.data).sort()).toEqual(['checksum_sha256', 'created_at', 'entry_id', 'filename', 'id', 'mime_type', 'size_bytes'])
    expect(res.data).toMatchObject({ entry_id: e.id, filename: 'hotel.png', mime_type: 'image/png', size_bytes: PNG().length })
    expect(res.data.checksum_sha256).toBe(sha256Hex(PNG()))
  })

  it('believes the bytes, not the file name or the claimed type', async () => {
    const e = newEntry()
    expect((await upload(e.id, JPEG(), 'scan.pdf')).res.data.mime_type).toBe('image/jpeg')
    expect((await upload(e.id, PDF(), 'photo.jpg')).res.data.mime_type).toBe('application/pdf')
    expect((await upload(e.id, WEBP(), 'x')).res.data.mime_type).toBe('image/webp')
  })

  it('keeps only a safe base name', async () => {
    const e = newEntry()
    expect((await upload(e.id, PNG(), '../../etc/passwd.png')).res.data.filename).toBe('passwd.png')
    expect((await upload(e.id, PNG(), 'C:\\Users\\dj\\bill.png')).res.data.filename).toBe('bill.png')
    expect((await upload(e.id, PNG(), '  ')).res.data.filename).toBe('receipt')
  })

  it.each([
    ['HEIC', HEIC], ['SVG', SVG], ['HTML', HTML],
  ])('415 unsupported_media_type for %s, even with an image name', async (_n, make) => {
    const e = newEntry()
    const { status, res } = await upload(e.id, make(), 'IMG_0001.jpg')
    expect(status).toBe(415)
    expect(res).toEqual({ error: 'unsupported_media_type', message: 'unsupported attachment type: use a JPEG, PNG, WebP or PDF file' })
    expect((await call('get', { params: { id: e.id } })).res.data.attachment_count).toBe(0)
  })

  it('400 validation_failed for an empty file; 400 bad_request without a file part or without multipart', async () => {
    const e = newEntry()
    expect(await upload(e.id, new Uint8Array())).toMatchObject({ status: 400, res: { error: 'validation_failed' } })
    expect(await call('aPost', { params: { id: e.id }, parts: [{ name: 'other', filename: 'x.png', data: PNG() }] }))
      .toMatchObject({ status: 400, res: { error: 'bad_request', message: 'missing "file" part' } })
    expect(await call('aPost', { params: { id: e.id }, parts: undefined })).toMatchObject({ status: 400, res: { error: 'bad_request' } })
  })

  it('413 too_large above 15 MB; exactly 15 MB is fine', async () => {
    const e = newEntry()
    const big = (n: number) => { const b = new Uint8Array(n); b.set(PNG()); return b }
    const over = await upload(e.id, big(15 * 1024 * 1024 + 1))
    expect(over.status).toBe(413)
    expect(over.res).toEqual({ error: 'too_large', message: 'attachment too large: the limit is 15 MB per file' })
    expect((await upload(e.id, big(15 * 1024 * 1024))).status).toBe(201)
  })

  it('409 limit_reached on the 11th file; the first ten stay', async () => {
    const e = newEntry()
    for (let i = 0; i < 10; i++) expect((await upload(e.id, PNG(), `r${i}.png`)).status).toBe(201)
    const eleventh = await upload(e.id, PNG(), 'r10.png')
    expect(eleventh.status).toBe(409)
    expect(eleventh.res).toEqual({ error: 'limit_reached', message: 'an entry can hold at most 10 files' })
    expect((await call('aList', { params: { id: e.id } })).res.data).toHaveLength(10)
  })

  it('404 for an unknown entry, a deleted entry, and 404 before any type check', async () => {
    expect(await upload('00000000-0000-4000-8000-00000000dead', PNG())).toMatchObject({ status: 404, res: { error: 'not_found' } })
    const gone = newEntry()
    await call('delete', { params: { id: gone.id }, body: { updated_at: gone.updated_at } })
    expect((await upload(gone.id, PNG())).status).toBe(404)
    expect((await call('aList', { params: { id: gone.id } })).status).toBe(404)
  })

  it('auto-generated gig-payment entries take receipts like any other active entry', async () => {
    const auto = newEntry({ kind: 'income', description: 'Gig fee', notes: '', category: 'gig_fee', auto_generated: true, source_kind: 'gig_payment' })
    expect((await upload(auto.id, PDF(), 'contract.pdf')).status).toBe(201)
  })
})

describe('Nitro routes: list, download, delete', () => {
  it('lists oldest first', async () => {
    const e = newEntry()
    for (const name of ['first.png', 'second.png', 'third.png']) await upload(e.id, PNG(), name)
    const { status, res } = await call('aList', { params: { id: e.id } })
    expect(status).toBe(200)
    expect(res.data.map((a: EntryAttachment) => a.filename)).toEqual(['first.png', 'second.png', 'third.png'])
  })

  it('lists an empty array, not null, for an entry without receipts', async () => {
    expect((await call('aList', { params: { id: newEntry().id } })).res).toEqual({ data: [] })
  })

  it('downloads the bytes as an attachment by default', async () => {
    const e = newEntry()
    const { res: { data } } = await upload(e.id, PNG(), 'hotel.png')
    const { status, res, headers } = await call('aGet', { params: { id: e.id, aid: data.id } })
    expect(status).toBe(200)
    expect(Array.from(res as Uint8Array)).toEqual(Array.from(PNG()))
    expect(headers['Content-Type']).toBe('image/png')
    expect(String(headers['Content-Disposition'])).toMatch(/^attachment; filename="hotel\.png"/)
    expect(headers['X-Content-Type-Options']).toBe('nosniff')
    expect(headers['Cache-Control']).toBe('private, no-store')
    expect(headers['Content-Length']).toBe(PNG().length)
  })

  it('?inline=1 shows an image inline, but a PDF is always a download', async () => {
    const e = newEntry()
    const img = (await upload(e.id, JPEG(), 'a.jpg')).res.data
    const pdf = (await upload(e.id, PDF(), 'b.pdf')).res.data
    const inlineImg = await call('aGet', { params: { id: e.id, aid: img.id }, query: { inline: '1' } })
    expect(String(inlineImg.headers['Content-Disposition'])).toMatch(/^inline;/)
    const inlinePdf = await call('aGet', { params: { id: e.id, aid: pdf.id }, query: { inline: '1' } })
    expect(String(inlinePdf.headers['Content-Disposition'])).toMatch(/^attachment;/)
  })

  it("404 for an unknown attachment and for another entry's attachment", async () => {
    const a = newEntry(); const b = newEntry()
    const { res: { data } } = await upload(a.id, PNG())
    expect((await call('aGet', { params: { id: a.id, aid: '00000000-0000-4000-8000-00000000dead' } })).status).toBe(404)
    expect((await call('aGet', { params: { id: b.id, aid: data.id } })).status).toBe(404)
    expect((await call('aDelete', { params: { id: b.id, aid: data.id } })).status).toBe(404)
  })

  it('DELETE answers 204 and the file is gone; a second DELETE is 404', async () => {
    const e = newEntry()
    const { res: { data } } = await upload(e.id, PNG())
    const params = { id: e.id, aid: data.id }
    expect((await call('aDelete', { params })).status).toBe(204)
    expect((await call('aGet', { params })).status).toBe(404)
    expect((await call('aDelete', { params })).status).toBe(404)
    expect((await call('get', { params: { id: e.id } })).res.data.attachment_count).toBe(0)
  })
})

describe('Nitro routes: voided entries', () => {
  async function voidedWithReceipt() {
    const e = newEntry()
    const { res: { data } } = await upload(e.id, PNG(), 'kept.png')
    await call('void', { params: { id: e.id }, body: { updated_at: e.updated_at } })
    return { e, a: data as EntryAttachment }
  }

  it('cannot gain or lose receipts (409 inactive)', async () => {
    const { e, a } = await voidedWithReceipt()
    const add = await upload(e.id, PNG())
    expect(add.status).toBe(409)
    expect(add.res).toEqual({ error: 'inactive', message: 'voided finance entries cannot change their attachments' })
    const del = await call('aDelete', { params: { id: e.id, aid: a.id } })
    expect(del.status).toBe(409)
    expect(del.res.error).toBe('inactive')
  })

  it('still lists and downloads what it has', async () => {
    const { e, a } = await voidedWithReceipt()
    expect((await call('aList', { params: { id: e.id } })).res.data.map((x: EntryAttachment) => x.filename)).toEqual(['kept.png'])
    expect((await call('aGet', { params: { id: e.id, aid: a.id } })).status).toBe(200)
    expect((await call('get', { params: { id: e.id } })).res.data).toMatchObject({ status: 'voided', attachment_count: 1 })
  })

  it('voiding does not remove files and the void response carries the count', async () => {
    const e = newEntry()
    await upload(e.id, PNG()); await upload(e.id, PDF())
    const voided = await call('void', { params: { id: e.id }, body: { updated_at: e.updated_at } })
    expect(voided.res.data).toMatchObject({ status: 'voided', attachment_count: 2 })
  })
})

describe('Nitro routes: attachment_count and the receipt filter', () => {
  it('is on every entry response: get, list, update', async () => {
    const e = newEntry()
    expect((await call('get', { params: { id: e.id } })).res.data.attachment_count).toBe(0)
    await upload(e.id, PNG()); await upload(e.id, PDF())
    expect((await call('get', { params: { id: e.id } })).res.data.attachment_count).toBe(2)
    const listed = (await call('list')).res.data.find((x: Entry) => x.id === e.id)
    expect(listed.attachment_count).toBe(2)
    const fresh = (await call('get', { params: { id: e.id } })).res.data as Entry
    const put = await call('put', { params: { id: e.id }, body: { kind: 'expense', amount_minor: 2000, currency: 'EUR', category: 'gear', entry_date: '2026-09-01', notes: 'Cable', updated_at: fresh.updated_at } })
    expect(put.res.data).toMatchObject({ amount_minor: 2000, attachment_count: 2 })
  })

  it('receipts do not change the entry version token', async () => {
    const e = newEntry()
    const before = (await call('get', { params: { id: e.id } })).res.data.updated_at
    await upload(e.id, PNG())
    expect((await call('get', { params: { id: e.id } })).res.data.updated_at).toBe(before)
  })

  it('receipt=missing and receipt=present split the list', async () => {
    const withReceipt = newEntry({ notes: 'with receipt filter test' })
    const without = newEntry({ notes: 'without receipt filter test' })
    await upload(withReceipt.id, PNG())
    const ids = async (receipt: string) => (await call('list', { query: { receipt } })).res.data.map((x: Entry) => x.id)
    const missing = await ids('missing'); const present = await ids('present')
    expect(missing).toContain(without.id); expect(missing).not.toContain(withReceipt.id)
    expect(present).toContain(withReceipt.id); expect(present).not.toContain(without.id)
    const all = (await call('list')).res.data.map((x: Entry) => x.id)
    expect(all).toEqual(expect.arrayContaining([withReceipt.id, without.id]))
  })

  it('anything else is a 400 validation_failed', async () => {
    const r = await call('list', { query: { receipt: 'maybe' } })
    expect(r.status).toBe(400)
    expect(r.res).toMatchObject({ error: 'validation_failed' })
    expect(r.res.message).toContain('receipt')
  })

  // Same definition as the Go API: "missing" = an ACTIVE EXPENSE with no files.
  it('receipt=missing never returns income or voided entries', async () => {
    const inc = newEntry({ kind: 'income', description: 'Fee', notes: '', category: 'gig_fee' })
    const expense = newEntry({ notes: 'needs a receipt' })
    const voided = newEntry({ notes: 'voided, no files' })
    await call('void', { params: { id: voided.id }, body: { updated_at: voided.updated_at } })

    const rows = (await call('list', { query: { receipt: 'missing' } })).res.data as Entry[]
    const ids = rows.map((x) => x.id)
    expect(ids).toContain(expense.id)
    expect(ids).not.toContain(inc.id)
    expect(ids).not.toContain(voided.id)
    expect(rows.every((x) => x.kind === 'expense' && x.status === 'active' && x.attachment_count === 0)).toBe(true)

    // Asking for income with receipt=missing is simply empty.
    const none = (await call('list', { query: { receipt: 'missing', kind: 'income' } })).res.data as Entry[]
    expect(none).toEqual([])
  })

  it('combines with the other filters', async () => {
    const expense = newEntry({ notes: 'eur expense without file', currency: 'EUR' })
    const rows = (await call('list', { query: { receipt: 'missing', kind: 'expense', currency: 'EUR' } })).res.data as Entry[]
    expect(rows.map((x) => x.id)).toContain(expense.id)
    expect(rows.every((x) => x.currency === 'EUR' && x.attachment_count === 0)).toBe(true)
  })
})

describe('seed data', () => {
  it('has an expense with two receipts (photo and PDF) and expenses with none', async () => {
    const rows = (await call('list', { query: { kind: 'expense' } })).res.data as Entry[]
    const travel = rows.find((x) => x.notes.startsWith('Travel & accommodation'))!
    expect(travel.attachment_count).toBe(2)
    // The other seeded expenses have none, so the list shows both states.
    for (const notes of ['USB drives + SD cards', 'Replacement headphones']) {
      expect(rows.find((x) => x.notes === notes)!.attachment_count).toBe(0)
    }
    const list = (await call('aList', { params: { id: travel.id } })).res.data as EntryAttachment[]
    expect(list.map((a) => a.mime_type)).toEqual(['image/png', 'application/pdf'])
    const png = await call('aGet', { params: { id: travel.id, aid: list[0]!.id }, query: { inline: '1' } })
    expect(sniffMime(png.res as Uint8Array)).toBe('image/png')
    const pdf = await call('aGet', { params: { id: travel.id, aid: list[1]!.id } })
    expect(sniffMime(pdf.res as Uint8Array)).toBe('application/pdf')
  })
})

describe('snapshot', () => {
  it('round-trips receipts through toJSON/load (the demo keeps state in localStorage)', async () => {
    const e = newEntry()
    const { res: { data } } = await upload(e.id, PNG(), 'persisted.png')
    const snap = JSON.parse(JSON.stringify(mock.db.toJSON()))
    mock.db.load(snap)
    expect((await call('get', { params: { id: e.id } })).res.data.attachment_count).toBe(1)
    const file = await call('aGet', { params: { id: e.id, aid: data.id } })
    expect(Array.from(file.res as Uint8Array)).toEqual(Array.from(PNG()))
  })

  it('leaves big receipts out of the snapshot instead of overflowing localStorage', async () => {
    const e = newEntry()
    const big = new Uint8Array(1_200_000); big.set(PNG())
    await upload(e.id, big, 'huge.png')
    const snap = mock.db.toJSON()
    const persisted = (snap.attachments ?? []).filter((a) => a.entry_id === e.id)
    expect(persisted).toHaveLength(0)
  })
})

// ───────────────────────── Browser demo ─────────────────────────

class MemoryStorage implements KeyValueStorage {
  data = new Map<string, string>()
  getItem(k: string) { return this.data.get(k) ?? null }
  setItem(k: string, v: string) { this.data.set(k, v) }
  removeItem(k: string) { this.data.delete(k) }
}

function demo(storage: KeyValueStorage = new MemoryStorage()) {
  return createDemoBackend({ storage, baseURL: '/demo/', now: () => new Date('2026-09-25T12:00:00Z'), createObjectUrl: () => 'blob:test' })
}
type Demo = ReturnType<typeof demo>
async function dcall(b: Demo, method: string, path: string, body: unknown = null) {
  const [p = '', qs = ''] = path.split('?')
  const res = await b.handle({ method, path: p, query: new URLSearchParams(qs), body })
  return { status: res.status, body: res.body as Res, blob: res.blob, headers: res.headers }
}
const form = (data: Uint8Array, filename: string, field = 'file') => {
  const fd = new FormData()
  fd.append(field, new File([data as BlobPart], filename))
  return fd
}
async function demoEntry(b: Demo, over: Record<string, unknown> = {}) {
  const r = await dcall(b, 'POST', '/api/v1/finance/entries', { kind: 'expense', amount_minor: 1500, currency: 'EUR', category: 'gear', entry_date: '2026-09-20', notes: 'Strings', ...over })
  return r.body.data as Entry
}

describe('demo backend', () => {
  it('uploads, lists, downloads and deletes a receipt, keeping attachment_count right', async () => {
    const b = demo()
    const e = await demoEntry(b)
    expect(e.attachment_count).toBe(0)

    const up = await dcall(b, 'POST', `/api/v1/finance/entries/${e.id}/attachments`, form(JPEG(), 'strings.jpg'))
    expect(up.status).toBe(201)
    expect(up.body.data).toMatchObject({ entry_id: e.id, filename: 'strings.jpg', mime_type: 'image/jpeg' })

    expect((await dcall(b, 'GET', `/api/v1/finance/entries/${e.id}`)).body.data.attachment_count).toBe(1)
    const list = await dcall(b, 'GET', `/api/v1/finance/entries/${e.id}/attachments`)
    expect(list.body.data).toHaveLength(1)

    const file = await dcall(b, 'GET', `/api/v1/finance/entries/${e.id}/attachments/${up.body.data.id}?inline=1`)
    expect(file.status).toBe(200)
    expect(file.blob!.type).toBe('image/jpeg')
    expect(file.headers!['content-disposition']).toMatch(/^inline;/)
    expect(new Uint8Array(await file.blob!.arrayBuffer())).toEqual(JPEG())

    const del = await dcall(b, 'DELETE', `/api/v1/finance/entries/${e.id}/attachments/${up.body.data.id}`)
    expect(del.status).toBe(204)
    expect((await dcall(b, 'GET', `/api/v1/finance/entries/${e.id}`)).body.data.attachment_count).toBe(0)
  })

  it('enforces the same type, size, count and voided rules', async () => {
    const b = demo()
    const e = await demoEntry(b)
    const post = (data: Uint8Array, name = 'f.png') => dcall(b, 'POST', `/api/v1/finance/entries/${e.id}/attachments`, form(data, name))

    expect((await post(HEIC(), 'IMG_1.jpg')).body.error).toBe('unsupported_media_type')
    expect((await post(SVG(), 'logo.png')).status).toBe(415)
    expect((await post(new Uint8Array(), 'e.png')).body.error).toBe('validation_failed')
    const over = new Uint8Array(15 * 1024 * 1024 + 1); over.set(PNG())
    expect(await post(over)).toMatchObject({ status: 413, body: { error: 'too_large' } })
    expect((await dcall(b, 'POST', `/api/v1/finance/entries/${e.id}/attachments`, form(PNG(), 'x.png', 'wrong'))).body.error).toBe('bad_request')
    expect((await dcall(b, 'POST', `/api/v1/finance/entries/${e.id}/attachments`, { not: 'multipart' })).body.error).toBe('bad_request')

    for (let i = 0; i < 10; i++) expect((await post(PNG(), `${i}.png`)).status).toBe(201)
    expect(await post(PNG())).toMatchObject({ status: 409, body: { error: 'limit_reached' } })

    const voided = await dcall(b, 'POST', `/api/v1/finance/entries/${e.id}/void`, { updated_at: (await dcall(b, 'GET', `/api/v1/finance/entries/${e.id}`)).body.data.updated_at })
    expect(voided.body.data.attachment_count).toBe(10)
    expect(await post(PNG())).toMatchObject({ status: 409, body: { error: 'inactive' } })
    const first = (await dcall(b, 'GET', `/api/v1/finance/entries/${e.id}/attachments`)).body.data[0]
    expect((await dcall(b, 'DELETE', `/api/v1/finance/entries/${e.id}/attachments/${first.id}`)).status).toBe(409)
    expect((await dcall(b, 'GET', `/api/v1/finance/entries/${e.id}/attachments/${first.id}`)).status).toBe(200)
    expect((await dcall(b, 'GET', '/api/v1/finance/entries/00000000-0000-4000-8000-00000000dead/attachments')).status).toBe(404)
  })

  it('a PDF stays a download even with ?inline=1', async () => {
    const b = demo()
    const e = await demoEntry(b)
    const up = await dcall(b, 'POST', `/api/v1/finance/entries/${e.id}/attachments`, form(PDF(), 'bill.pdf'))
    const file = await dcall(b, 'GET', `/api/v1/finance/entries/${e.id}/attachments/${up.body.data.id}?inline=1`)
    expect(file.blob!.type).toBe('application/pdf')
    expect(file.headers!['content-disposition']).toMatch(/^attachment;/)
  })

  it('filters the list by receipt and rejects other values', async () => {
    const b = demo()
    const e = await demoEntry(b, { notes: 'Receipt filter in the demo' })
    await dcall(b, 'POST', `/api/v1/finance/entries/${e.id}/attachments`, form(PNG(), 'r.png'))
    const ids = async (q: string) => (await dcall(b, 'GET', `/api/v1/finance/entries?${q}`)).body.data.map((x: Entry) => x.id)
    expect(await ids('receipt=present')).toContain(e.id)
    expect(await ids('receipt=missing')).not.toContain(e.id)
    expect((await dcall(b, 'GET', '/api/v1/finance/entries?receipt=bogus')).status).toBe(400)
  })

  it('is seeded with one expense carrying two receipts and expenses with none', async () => {
    const b = demo()
    const rows = (await dcall(b, 'GET', '/api/v1/finance/entries?kind=expense')).body.data as Entry[]
    expect(rows.filter((x) => x.attachment_count === 2)).toHaveLength(1)
    expect(rows.filter((x) => x.attachment_count === 0).length).toBeGreaterThanOrEqual(2)
    const unreceipted = (await dcall(b, 'GET', '/api/v1/finance/entries?receipt=missing&kind=expense')).body.data as Entry[]
    expect(unreceipted.map((x) => x.notes)).toEqual(expect.arrayContaining(['USB drives + SD cards', 'Replacement headphones']))
  })

  it('keeps small receipts across a reload and hands <img> a data URL', async () => {
    const storage = new MemoryStorage()
    const first = demo(storage)
    const e = await demoEntry(first)
    const up = await dcall(first, 'POST', `/api/v1/finance/entries/${e.id}/attachments`, form(PNG(), 'kept.png'))

    const second = demo(storage)
    expect((await dcall(second, 'GET', `/api/v1/finance/entries/${e.id}`)).body.data.attachment_count).toBe(1)
    const url = second.resolveAsset(`/api/v1/finance/entries/${e.id}/attachments/${up.body.data.id}?inline=1`)
    expect(url).toMatch(/^data:image\/png;base64,/)
    expect(Buffer.from(url!.split(',')[1]!, 'base64')).toEqual(Buffer.from(PNG()))
    expect(second.resolveAsset(`/api/v1/finance/entries/${e.id}/attachments/nope`)).toBeNull()
  })
})
