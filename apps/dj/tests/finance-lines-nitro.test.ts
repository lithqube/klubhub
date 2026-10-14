// @vitest-environment node
// The Nitro dev routes for the EN 16931 contract answer like the shared core.
import { beforeAll, describe, expect, it, vi } from 'vitest'

interface FakeEvent { params: Record<string, string>; body?: unknown; status: number; headers: Record<string, string> }

vi.stubGlobal('defineEventHandler', (h: unknown) => h)
vi.stubGlobal('setResponseStatus', (e: FakeEvent, s: number) => { e.status = s })
vi.stubGlobal('setHeader', (e: FakeEvent, k: string, v: string) => { e.headers[k] = v })
vi.stubGlobal('getRouterParam', (e: FakeEvent, name: string) => e.params[name])
vi.stubGlobal('readBody', async (e: FakeEvent) => e.body)

type Handler = (e: FakeEvent) => unknown
const base = '../server/api/v1/finance'
const h: Record<string, Handler> = {}
let db: typeof import('../server/api/v1/finance/-mockDb')

beforeAll(async () => {
  db = await import(`${base}/-mockDb`)
  for (const [k, p] of [['getBp', 'billing-profile.get'], ['putBp', 'billing-profile.put'], ['pdf', 'invoices/[id]/pdf.get'], ['get', 'invoices/[id].get'], ['put', 'invoices/[id].put']] as const) {
    h[k] = (await import(`${base}/${p}`)).default as Handler
  }
})

async function call(name: string, opts: Partial<FakeEvent> = {}) {
  const e: FakeEvent = { params: {}, status: 200, headers: {}, ...opts }
  const res = await h[name]!(e)
// eslint-disable-next-line @typescript-eslint/no-explicit-any
  return { status: e.status, res: res as any, headers: e.headers }
}

describe('Nitro finance mocks: EN 16931', () => {
  it('serves and validates the billing profile bank details', async () => {
    const { res: got } = await call('getBp')
    const p = got.data
    expect(p).toMatchObject({ tax_number: '', iban: '', bic: '' })
    const { id: _i, created_at: _c, ...rest } = p
    const bad = await call('putBp', { body: { ...rest, iban: 'nope' } })
    expect(bad.status).toBe(400)
    expect(bad.res.message).toContain('iban:')
    const good = await call('putBp', { body: { ...rest, iban: 'gb82 west 1234 5698 7654 32', bic: 'nwbkgb2l' } })
    expect(good.status).toBe(200)
    expect(good.res.data).toMatchObject({ iban: 'GB82WEST12345698765432', bic: 'NWBKGB2L' })
  })

  it('replaces lines through the PUT route and serves a PDF attachment', async () => {
    const inv = db.db.invoices.values().next().value!
    const draft = [...db.db.invoices.values()].find((i) => i.status === 'draft') ?? inv
    const id = draft.id
    const cur = (await call('get', { params: { id } })).res.data
    const put = await call('put', {
      params: { id },
      body: { ...cur, customer: cur.customer, lines: [{ description: 'Set', quantity: 3, unit_minor: 1000, unit_code: 'DAY' }] },
    })
    expect(put.status).toBe(200)
    expect(put.res.data.subtotal_minor).toBe(3000)
    const got = (await call('get', { params: { id } })).res
    expect(got.lines).toHaveLength(1)
    expect(got.lines[0]).toMatchObject({ unit_code: 'DAY', line_total_minor: 3000 })

    const pdf = await call('pdf', { params: { id } })
    expect(pdf.headers['Content-Type']).toBe('application/pdf')
    expect(pdf.headers['Content-Disposition']).toMatch(/^attachment; filename=/)
    expect(String(pdf.res).startsWith('%PDF-')).toBe(true)
    expect((await call('pdf', { params: { id: 'nope' } })).status).toBe(404)
  })
})
