// @vitest-environment node
// Rules enforced by the in-memory rider mocks (apps/dj/server/api/v1/rider/**).
// They must answer like the Go API (api/internal/rider): required updatedAt
// (422), stale updatedAt (409), unknown template (404), size / NUL / blank-name
// limits (422). Otherwise frontend-only dev hides exactly the errors the real
// backend returns. Nitro auto-imports are stubbed so handlers run as functions.
import { beforeAll, describe, expect, it, vi } from 'vitest'

interface FakeEvent {
  params: Record<string, string>
  body?: unknown
  status: number
}
class HttpError extends Error {
  constructor(public statusCode: number, public statusMessage: string) { super(statusMessage) }
}

vi.stubGlobal('defineEventHandler', (h: unknown) => h)
vi.stubGlobal('setResponseStatus', (e: FakeEvent, s: number) => { e.status = s })
vi.stubGlobal('getRouterParam', (e: FakeEvent, name: string) => e.params[name])
vi.stubGlobal('readBody', async (e: FakeEvent) => e.body)
vi.stubGlobal('createError', (o: { statusCode: number; statusMessage: string }) => new HttpError(o.statusCode, o.statusMessage))

type Handler = (e: FakeEvent) => unknown
const base = '../server/api/v1/rider'
const h: Record<string, Handler> = {}
let state: typeof import('../server/api/v1/rider/_state')

beforeAll(async () => {
  state = await import(`${base}/_state`)
  const load = async (key: string, path: string) => { h[key] = (await import(`${base}/${path}`)).default as Handler }
  await load('tplCreate', 'templates/index.post')
  await load('tplPut', 'templates/[id].put')
  await load('attCreate', 'attachments/index.post')
  await load('attPut', 'attachments/[id].put')
})

/** Runs a handler; returns the response, or the HttpError it threw. */
async function call(name: string, opts: Partial<FakeEvent> = {}) {
  const e: FakeEvent = { params: {}, status: 200, ...opts }
  try {
    const out = (await h[name]!(e)) as { data: Record<string, unknown> }
    // Nitro serialises the response; without a copy the test would see the
    // mock's live state object change underneath it.
    const res = JSON.parse(JSON.stringify(out)) as typeof out
    return { status: e.status, res, err: null as HttpError | null }
  } catch (err) {
    return { status: (err as HttpError).statusCode, res: null, err: err as HttpError }
  }
}
const gigId = (() => { let n = 0; return () => `gig-mock-${++n}` })()

describe('rider mock handlers: every module loads', () => {
  // Several handlers once imported _state from a path that does not exist, so
  // frontend-only dev (no NUXT_PUBLIC_API_BASE) failed on those endpoints
  // while every test that mocked the module still passed.
  const modules = import.meta.glob('../server/api/v1/rider/**/*.ts')

  it('finds every handler file (11) plus the shared state', () => {
    expect(Object.keys(modules).filter(p => !p.endsWith('/_state.ts'))).toHaveLength(11)
  })

  it.each(Object.entries(modules))('%s imports cleanly', async (_path, load) => {
    await expect(load()).resolves.toBeDefined()
  })
})

describe('rider mock handlers', () => {
  it('create template: 201, but 422 for a blank name or oversized text', async () => {
    const ok = await call('tplCreate', { body: { name: 'Mock club', technical: 't' } })
    expect(ok.status).toBe(201)
    expect((await call('tplCreate', { body: { name: '  ' } })).status).toBe(422)
    const big = await call('tplCreate', { body: { name: 'x', technical: 'x'.repeat(20001) } })
    expect(big.status).toBe(422)
    expect(big.err?.statusMessage).toContain('technical is too long')
  })

  it('update template: token required (422), stale (409), valid (200 with a new token)', async () => {
    const created = (await call('tplCreate', { body: { name: 'To update' } })).res!.data as { id: string; updatedAt: string }

    expect((await call('tplPut', { params: { id: created.id }, body: { name: 'x' } })).status).toBe(422)
    const stale = await call('tplPut', { params: { id: created.id }, body: { name: 'x', updatedAt: '1999-01-01T00:00:00.000Z' } })
    expect(stale.status).toBe(409)

    const ok = await call('tplPut', { params: { id: created.id }, body: { technical: 'new', updatedAt: created.updatedAt } })
    expect(ok.status).toBe(200)
    const next = ok.res!.data as { updatedAt: string; technical: string }
    expect(next.technical).toBe('new')
    expect(next.updatedAt).not.toBe(created.updatedAt)

    // The old token is now stale; the new one works.
    expect((await call('tplPut', { params: { id: created.id }, body: { technical: 'again', updatedAt: created.updatedAt } })).status).toBe(409)
    expect((await call('tplPut', { params: { id: created.id }, body: { technical: 'again', updatedAt: next.updatedAt } })).status).toBe(200)
    expect((await call('tplPut', { params: { id: 'nope' }, body: { name: 'x', updatedAt: next.updatedAt } })).status).toBe(404)
  })

  it('updating one section leaves the others alone', async () => {
    const created = (await call('tplCreate', { body: { name: 'Sections', technical: 't', hospitality: 'h' } })).res!.data as { id: string; updatedAt: string }
    const ok = await call('tplPut', { params: { id: created.id }, body: { technical: 't2', updatedAt: created.updatedAt } })
    expect(ok.res!.data).toMatchObject({ technical: 't2', hospitality: 'h' })
  })

  it('update with an over-long section is 422 and changes nothing', async () => {
    const created = (await call('tplCreate', { body: { name: 'Limits' } })).res!.data as { id: string; updatedAt: string }
    const bad = await call('tplPut', { params: { id: created.id }, body: { technical: 'x'.repeat(20001), updatedAt: created.updatedAt } })
    expect(bad.status).toBe(422)
    expect(state.db.getTemplate(created.id)?.updatedAt).toBe(created.updatedAt)
  })

  it('create attachment: 404 for a template that does not exist, 409 for a second one, 422 over the limit', async () => {
    const g = gigId()
    expect((await call('attCreate', { body: { gigId: g, templateId: 'no-such-template' } })).status).toBe(404)
    expect(state.db.getAttachmentByGig(g)).toBeNull() // nothing was stored with a dangling id

    expect((await call('attCreate', { body: { gigId: g, technical: 'x'.repeat(20001) } })).status).toBe(422)
    expect((await call('attCreate', { body: { gigId: g } })).status).toBe(201)
    expect((await call('attCreate', { body: { gigId: g } })).status).toBe(409)
  })

  it('update attachment: same token rules as the Go API', async () => {
    const g = gigId()
    const created = (await call('attCreate', { body: { gigId: g, technical: 't' } })).res!.data as { id: string; updatedAt: string }

    expect((await call('attPut', { params: { id: created.id }, body: { technical: 'x' } })).status).toBe(422)
    expect((await call('attPut', { params: { id: created.id }, body: { technical: 'x', updatedAt: '1999-01-01T00:00:00.000Z' } })).status).toBe(409)
    const ok = await call('attPut', { params: { id: created.id }, body: { backline: 'b', updatedAt: created.updatedAt } })
    expect(ok.status).toBe(200)
    expect(ok.res!.data).toMatchObject({ technical: 't', backline: 'b' })
    expect((await call('attPut', { params: { id: 'nope' }, body: { technical: 'x', updatedAt: created.updatedAt } })).status).toBe(404)
  })

  it('two writes inside one millisecond still produce different tokens', async () => {
    const created = (await call('tplCreate', { body: { name: 'Fast writer' } })).res!.data as { id: string; updatedAt: string }
    const a = (await call('tplPut', { params: { id: created.id }, body: { technical: '1', updatedAt: created.updatedAt } })).res!.data as { updatedAt: string }
    const b = (await call('tplPut', { params: { id: created.id }, body: { technical: '2', updatedAt: a.updatedAt } })).res!.data as { updatedAt: string }
    expect(new Set([created.updatedAt, a.updatedAt, b.updatedAt]).size).toBe(3)
  })
})
