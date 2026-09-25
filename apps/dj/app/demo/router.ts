// Minimal method + path router for the demo backend. Patterns use `:name`
// segments; the first registered match wins, so register literals first.

import type { DemoResponse, Handler, Params } from './types'

interface Route {
  method: string
  parts: string[]
  handler: Handler
}

export class DemoRouter {
  private readonly routes: Route[] = []

  on(method: string, pattern: string, handler: Handler): this {
    this.routes.push({ method: method.toUpperCase(), parts: split(pattern), handler })
    return this
  }

  match(method: string, path: string): { handler: Handler; params: Params } | null {
    const parts = split(path)
    for (const r of this.routes) {
      if (r.method !== method.toUpperCase() || r.parts.length !== parts.length) continue
      const params: Params = {}
      let hit = true
      for (let i = 0; i < parts.length; i++) {
        const want = r.parts[i]!
        const got = parts[i]!
        if (want.startsWith(':')) params[want.slice(1)] = safeDecode(got)
        else if (want !== got) { hit = false; break }
      }
      if (hit) return { handler: r.handler, params }
    }
    return null
  }
}

function split(path: string): string[] {
  return path.split('/').filter(Boolean)
}

function safeDecode(s: string): string {
  try { return decodeURIComponent(s) } catch { return s }
}

// ── response helpers ──

export const json = (body: unknown, status = 200): DemoResponse => ({ status, body })
export const noContent = (): DemoResponse => ({ status: 204 })
/** Go API error shape for non-finance routes: `{ "error": message }`. */
export const apiError = (status: number, message: string): DemoResponse => ({ status, body: { error: message } })
export const notFound = (what = 'not found'): DemoResponse => apiError(404, what)
