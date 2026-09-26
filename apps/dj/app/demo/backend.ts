// The demo backend: routes /api/v1/* requests to in-memory handlers over a
// persisted DemoState. Framework-free so it runs in vitest and the browser.

import { FinanceMockDb } from '../../shared/finance-mock/db'
import { registerEpk } from './handlers/epk'
import { financeLookup, registerFinance } from './handlers/finance'
import { registerGigs } from './handlers/gigs'
import { registerSettings } from './handlers/settings'
import { postImage, registerSocial } from './handlers/social'
import { registerTracklists } from './handlers/tracklists'
import { apiError, DemoRouter } from './router'
import { createSeedState } from './seed'
import { DemoStore, type KeyValueStorage } from './state'
import type { DemoContext, DemoRequest, DemoResponse } from './types'

export interface DemoBackendOptions {
  storage: KeyValueStorage | null
  /** App base URL, e.g. "/demo/". */
  baseURL?: string
  now?: () => Date
  /** Object URL factory; defaults to URL.createObjectURL. */
  createObjectUrl?: (blob: Blob) => string
}

/** GET routes that change state (simulated publishing, account connect). */
const MUTATING_READS = new Set(['/api/v1/social/posts', '/api/v1/social/auth/url'])

export function buildRouter(): DemoRouter {
  const r = new DemoRouter()
  registerSettings(r)
  registerTracklists(r)
  registerSocial(r)
  registerEpk(r)
  registerGigs(r)
  registerFinance(r)
  return r
}

export function createDemoBackend(opts: DemoBackendOptions) {
  const now = opts.now ?? (() => new Date())
  const store = new DemoStore(opts.storage, () => createSeedState(now()))
  const router = buildRouter()
  const blobs = new Map<string, Blob>()
  let seq = 0
  const createUrl = opts.createObjectUrl ?? ((b: Blob) => URL.createObjectURL(b))

  const finance = new FinanceMockDb({ lookupGig: (id) => financeLookup(store.state)(id) })
  finance.load(store.state.finance)

  const ctx: DemoContext = {
    get state() { return store.state },
    finance,
    now,
    baseURL: opts.baseURL ?? '/',
    objectUrl(blob) {
      const url = createUrl(blob) || `blob:demo/${++seq}`
      blobs.set(url, blob)
      return url
    },
    blobFor: (url) => blobs.get(url),
  }

  async function handle(req: DemoRequest): Promise<DemoResponse> {
    const hit = router.match(req.method, req.path)
    if (!hit) return apiError(404, `${req.method} ${req.path} is not available in the demo`)
    let res: DemoResponse
    try {
      res = await hit.handler({ ...req, params: hit.params }, ctx)
    } catch (e) {
      console.warn('[demo] handler failed', req.method, req.path, e)
      return apiError(500, 'demo backend error')
    }
    if (req.method !== 'GET' || MUTATING_READS.has(req.path)) {
      store.state.finance = finance.toJSON()
      store.save()
    }
    return res
  }

  /** Synchronous URL for <img src> pointing at an API path (see utils/apiAssetUrl). */
  function resolveAsset(url: string): string | null {
    const [path = '', qs = ''] = url.split('?')
    const post = /^\/api\/v1\/social\/posts\/([^/]+)\/image$/.exec(path)
    if (post) {
      const p = store.state.posts.find((x) => x.id === post[1])
      return p ? postImage(p) : null
    }
    if (path === '/api/v1/storage/proxy') {
      return store.state.assets[new URLSearchParams(qs).get('path') ?? ''] ?? null
    }
    return null
  }

  function reset(): void {
    store.reset()
    finance.load(store.state.finance)
  }

  return {
    handle,
    resolveAsset,
    reset,
    get state() { return store.state },
    get persistent() { return store.persistent },
  }
}

export type DemoBackend = ReturnType<typeof createDemoBackend>
