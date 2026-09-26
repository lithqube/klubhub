// Wraps globalThis.fetch so same-origin requests to <baseURL>api/v1/* are
// answered by the demo backend. Everything else passes through untouched.

import type { DemoBackend } from './backend'
import type { DemoResponse } from './types'

/** Returns "/api/v1/..." for demo API URLs, or null for anything else. */
export function demoApiPath(url: URL, origin: string, baseURL: string): string | null {
  if (url.origin !== origin) return null
  const base = baseURL.endsWith('/') ? baseURL : `${baseURL}/`
  let path = url.pathname
  if (base !== '/' && path.startsWith(base)) path = path.slice(base.length - 1)
  return path.startsWith('/api/v1/') || path === '/api/v1' ? path : null
}

async function readBody(input: RequestInfo | URL, init?: RequestInit): Promise<unknown> {
  let raw: unknown = init?.body
  if (raw === undefined && input instanceof Request) {
    const type = input.headers.get('content-type') ?? ''
    raw = type.includes('multipart/form-data') ? await input.clone().formData() : await input.clone().text()
  }
  if (raw === undefined || raw === null || raw === '') return null
  if (typeof raw === 'string') {
    try { return JSON.parse(raw) } catch { return raw }
  }
  if (raw instanceof URLSearchParams) return Object.fromEntries(raw)
  return raw
}

export function toResponse(r: DemoResponse): Response {
  const headers = new Headers(r.headers)
  if (r.status === 204 || r.status === 304) return new Response(null, { status: r.status, headers })
  if (r.blob) {
    if (!headers.has('content-type') && r.blob.type) headers.set('content-type', r.blob.type)
    return new Response(r.blob, { status: r.status, headers })
  }
  headers.set('content-type', 'application/json')
  return new Response(JSON.stringify(r.body ?? null), { status: r.status, headers })
}

export interface InstallOptions {
  baseURL: string
  /** Simulated latency so loading states are visible (ms). */
  latencyMs?: number
}

export function installDemoFetch(backend: DemoBackend, opts: InstallOptions): () => void {
  const native = globalThis.fetch
  const original = native.bind(globalThis)
  const origin = globalThis.location?.origin ?? 'http://localhost'

  async function demoFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const href = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    let url: URL
    try {
      url = new URL(href, globalThis.location?.href ?? origin)
    } catch {
      return original(input, init)
    }
    const path = demoApiPath(url, origin, opts.baseURL)
    if (!path) return original(input, init)
    const method = (init?.method ?? (input instanceof Request ? input.method : 'GET')).toUpperCase()
    const body = await readBody(input, init)
    if (opts.latencyMs) await new Promise((r) => setTimeout(r, opts.latencyMs))
    return toResponse(await backend.handle({ method, path, query: url.searchParams, body }))
  }

  globalThis.fetch = demoFetch as typeof fetch
  return () => { globalThis.fetch = native }
}
