// Turns a failed rider API call into text for the user.
//
// ofetch's own `message` is a generic "[POST] "/api/...": 422 Unprocessable
// Entity" and says nothing useful. The Go API puts the reason in the body:
//   rider handlers: {"error": "<human text>"}            (e.g. the 422 limits)
//   rider mux:      {"error": "<code>", "message": "<human text>"}
// and the dev mock sets `statusMessage`. Prefer `message`, then `error`, then
// the mock's `statusMessage`; otherwise the caller's fallback.

const MAX_LEN = 300

function text(v: unknown): string {
  return typeof v === 'string' ? v.trim() : ''
}

export function riderErrorMessage(e: unknown, fallback: string): string {
  const err = e as {
    data?: { message?: unknown; error?: unknown; statusMessage?: unknown }
    statusMessage?: unknown
  } | null
  const picked = text(err?.data?.message) || text(err?.data?.error)
    || text(err?.data?.statusMessage) || text(err?.statusMessage)
  if (!picked) return fallback
  return picked.length > MAX_LEN ? `${picked.slice(0, MAX_LEN)}…` : picked
}
