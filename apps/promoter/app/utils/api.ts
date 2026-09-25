/**
 * apiFetch — the only way promoter stores talk to /api/v1.
 *
 * State-changing requests carry X-KlubHub-CSRF: the Go API refuses unsafe
 * methods without it (plus a matching Origin), and browsers cannot add a
 * custom header cross-site without a CORS preflight the API never grants.
 * During SSR the incoming session cookie is forwarded so the Nitro proxy
 * reaches the Go API as the same user.
 */
export const CSRF_HEADER = 'X-KlubHub-CSRF'

const SAFE = new Set(['GET', 'HEAD', 'OPTIONS'])

type FetchOptions = NonNullable<Parameters<typeof $fetch>[1]>

export function withCsrf(opts: FetchOptions = {}): FetchOptions {
  const method = String(opts.method ?? 'GET').toUpperCase()
  if (SAFE.has(method)) return opts
  return { ...opts, headers: { ...(opts.headers as Record<string, string> | undefined), [CSRF_HEADER]: '1' } }
}

export function apiFetch<T>(url: string, opts: FetchOptions = {}): Promise<T> {
  let o = withCsrf(opts)
  if (import.meta.server) {
    const cookie = useRequestHeaders(['cookie']).cookie
    if (cookie) o = { ...o, headers: { ...(o.headers as Record<string, string> | undefined), cookie } }
  }
  return $fetch<T>(url, o as Parameters<typeof $fetch>[1]) as Promise<T>
}
