/**
 * useQuickPreview — data behind the status bar's quick-preview readouts.
 *
 * Reads the whole gig list itself instead of borrowing the gig store: that
 * store holds whatever the Gigs page has filtered it to, and a readout that
 * changes when a filter is applied would be wrong. Failures stay silent (no
 * toast): this is a background strip, and the readouts fall back to "—".
 */
import { buildQuickPreview, type PreviewPost, type QuickPreview } from '../utils/quickPreview'
import type { Gig } from '../types/gig'

/** Route changes and focus events refresh at most this often. */
const STALE_MS = 30_000

interface RawPost {
  status?: string
  scheduledAtUtc?: string
  scheduled_at_utc?: string
  scheduledAt?: string
}

// The Go API returns a bare array (or JSON null for an empty list); older
// handlers wrapped it in { data }.
function toList<T>(value: unknown): T[] {
  if (Array.isArray(value)) return value as T[]
  const inner = (value as { data?: unknown } | null)?.data
  return Array.isArray(inner) ? (inner as T[]) : []
}

export const useQuickPreview = () => {
  const preview = useState<QuickPreview | null>('quickPreview', () => null)
  const failed = useState<boolean>('quickPreviewFailed', () => false)
  const fetchedAt = useState<number>('quickPreviewAt', () => 0)

  async function refresh(force = false): Promise<void> {
    if (!import.meta.client) return
    if (!force && Date.now() - fetchedAt.value < STALE_MS) return
    // Stamp first so a burst of triggers (route change + focus) is one fetch.
    fetchedAt.value = Date.now()

    const [gigsRes, postsRes] = await Promise.allSettled([
      $fetch<unknown>('/api/v1/gigs'),
      $fetch<unknown>('/api/v1/social/posts'),
    ])
    if (gigsRes.status === 'rejected') {
      // Keep the last good readouts; only flag that they may be stale.
      failed.value = true
      return
    }
    const posts: PreviewPost[] | null = postsRes.status === 'fulfilled'
      ? toList<RawPost>(postsRes.value).map((p) => ({
          status: p.status ?? '',
          scheduledAt: p.scheduledAtUtc ?? p.scheduled_at_utc ?? p.scheduledAt ?? null,
        }))
      : null

    preview.value = buildQuickPreview({ gigs: toList<Gig>(gigsRes.value), posts, now: new Date() })
    failed.value = false
  }

  return { preview, failed, refresh }
}
