import { useRiderStore } from '../stores/rider'
import type {
  RiderAttachmentUpdateInput,
  RiderTemplateUpdateInput,
} from '../types/rider'

// ─── Per-target autosave queues ────────────────────────────────────────────
//
// One module-level Map so the same composable handles both template and
// attachment editing without two separate imports. Each target gets its
// own queue: pending patches merged until the timer fires, then flushed
// in one round-trip. Mirrors useEpkAutosave.ts, plus two guarantees:
//   - saves for one target run strictly one after another, so responses
//     (which the store writes into its cache) can never arrive out of order
//     and an older snapshot can never overwrite a newer one;
//   - a failed save keeps its patch (merged under any newer edits) so the
//     edit is retried by the next save instead of being lost.

type RiderAutosaveTarget =
  | { kind: 'template'; id: string }
  | { kind: 'attachment'; id: string }

type Patch = RiderTemplateUpdateInput | RiderAttachmentUpdateInput

type Queue = {
  pending: Patch
  timer: number | null
  /** Tail of this target's save chain; never rejects. */
  chain: Promise<void>
  /** Flushes requested but not finished (queued behind the chain or running). */
  active: number
  /** The last save failed; its patch is back in `pending`. */
  failed: boolean
}

const queues = new Map<string, Queue>()

function keyOf(t: RiderAutosaveTarget): string {
  return `${t.kind}:${t.id}`
}

export function useRiderAutosave(delayMs = 1500) {
  const store = useRiderStore()

  // One indicator for every target: busy while any target has a timer or a
  // save in progress, failed while any idle target's last save failed.
  function refreshStatus(): void {
    let busy = false
    let failed = false
    for (const q of queues.values()) {
      if (q.active > 0 || q.timer !== null) busy = true
      else if (q.failed) failed = true
    }
    store.saveStatus = busy ? 'saving' : failed ? 'error' : 'saved'
  }

  async function send(target: RiderAutosaveTarget, q: Queue): Promise<void> {
    let attempted = false
    try {
      // Take the patch only when it is this save's turn, so edits made while
      // an earlier save was in flight are coalesced into this one.
      const patch = q.pending
      if (Object.keys(patch).length === 0) return
      q.pending = {}
      attempted = true
      try {
        if (target.kind === 'template') {
          await store.updateTemplate(target.id, patch as RiderTemplateUpdateInput)
        } else {
          await store.updateAttachment(target.id, patch as RiderAttachmentUpdateInput)
        }
        q.failed = false
      } catch {
        // Keep the failed edit; anything typed since wins per key.
        q.pending = { ...patch, ...q.pending }
        q.failed = true
      }
    } finally {
      q.active--
      if (attempted) refreshStatus()
    }
  }

  function flush(target: RiderAutosaveTarget): Promise<void> {
    const q = queues.get(keyOf(target))
    if (!q) return Promise.resolve()
    if (q.timer !== null) {
      clearTimeout(q.timer)
      q.timer = null
    }
    q.active++
    q.chain = q.chain.then(() => send(target, q))
    return q.chain
  }

  async function flushAll(): Promise<void> {
    const targets = Array.from(queues.keys())
    await Promise.all(
      targets.map(k => {
        const [kind, id] = k.split(':') as ['template' | 'attachment', string]
        return flush({ kind, id })
      }),
    )
  }

  function scheduleSave(target: RiderAutosaveTarget, patch: Patch): void {
    const k = keyOf(target)
    let q = queues.get(k)
    if (!q) {
      q = { pending: {}, timer: null, chain: Promise.resolve(), active: 0, failed: false }
      queues.set(k, q)
    }
    q.pending = { ...q.pending, ...patch }
    if (q.timer !== null) clearTimeout(q.timer)
    q.timer = window.setTimeout(() => {
      q.timer = null
      void flush(target)
    }, delayMs)
    refreshStatus()
  }

  return { scheduleSave, flush, flushAll }
}
