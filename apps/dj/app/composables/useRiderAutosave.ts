import { useRiderStore } from '../stores/rider'
import { RiderConflictError } from '../types/rider'
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
//     edit is retried by the next save instead of being lost;
//   - a 409 (the record changed elsewhere; the store sends the updatedAt it
//     last saw) is NOT retried: resending would only be rejected again, and
//     overwriting would lose the other writer's changes. The target is
//     flagged, its edits are kept, and only discard() (explicit user choice)
//     reloads the latest copy and moves on.

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
  /** The server rejected the last save as stale; waiting for discard(). */
  conflict: boolean
}

const queues = new Map<string, Queue>()

function keyOf(t: RiderAutosaveTarget): string {
  return `${t.kind}:${t.id}`
}

export function useRiderAutosave(delayMs = 1500) {
  const store = useRiderStore()

  // One indicator for every target. A conflict needs the user, so it wins;
  // then busy while any target has a timer or a save in progress; then failed
  // while any idle target's last save failed.
  function refreshStatus(): void {
    let busy = false
    let failed = false
    let conflict = false
    for (const q of queues.values()) {
      if (q.conflict) conflict = true
      else if (q.active > 0 || q.timer !== null) busy = true
      else if (q.failed) failed = true
    }
    store.saveStatus = conflict ? 'conflict' : busy ? 'saving' : failed ? 'error' : 'saved'
  }

  async function send(target: RiderAutosaveTarget, q: Queue): Promise<void> {
    let attempted = false
    try {
      // Waiting for the user to resolve a conflict: nothing is sent.
      if (q.conflict) return
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
      } catch (e) {
        // Keep the failed edit; anything typed since wins per key.
        q.pending = { ...patch, ...q.pending }
        if (e instanceof RiderConflictError) {
          q.conflict = true
          store.markConflict(keyOf(target))
        } else {
          q.failed = true
        }
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
      q = { pending: {}, timer: null, chain: Promise.resolve(), active: 0, failed: false, conflict: false }
      queues.set(k, q)
    }
    q.pending = { ...q.pending, ...patch }
    if (q.conflict) {
      // Keep collecting edits, but do not hammer the server with saves that
      // will only be rejected again.
      refreshStatus()
      return
    }
    if (q.timer !== null) clearTimeout(q.timer)
    q.timer = window.setTimeout(() => {
      q.timer = null
      void flush(target)
    }, delayMs)
    refreshStatus()
  }

  // The user chose to drop their unsaved edits and take the server's copy:
  // re-read the record (which also refreshes its updatedAt token), clear the
  // queue and the conflict, and tell editors to reseed. If the re-read fails
  // nothing is cleared, so the conflict stays visible and can be retried.
  async function discard(target: RiderAutosaveTarget): Promise<void> {
    const k = keyOf(target)
    const q = queues.get(k)
    if (q) {
      if (q.timer !== null) {
        clearTimeout(q.timer)
        q.timer = null
      }
      await q.chain
    }
    if (target.kind === 'template') await store.reloadTemplate(target.id)
    else await store.reloadAttachment(target.id)
    if (q) {
      q.pending = {}
      q.failed = false
      q.conflict = false
    }
    store.clearConflict(k)
    store.reloadVersion++
    refreshStatus()
  }

  return { scheduleSave, flush, flushAll, discard }
}
