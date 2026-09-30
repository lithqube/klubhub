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
// in one round-trip. Mirrors useEpkAutosave.ts.

type RiderAutosaveTarget =
  | { kind: 'template'; id: string }
  | { kind: 'attachment'; id: string }

type Queue = { pending: RiderTemplateUpdateInput | RiderAttachmentUpdateInput; timer: number | null }

const queues = new Map<string, Queue>()

function keyOf(t: RiderAutosaveTarget): string {
  return `${t.kind}:${t.id}`
}

export function useRiderAutosave(delayMs = 1500) {
  const store = useRiderStore()

  async function flush(target: RiderAutosaveTarget): Promise<void> {
    const k = keyOf(target)
    const q = queues.get(k)
    if (!q) return
    if (q.timer) {
      clearTimeout(q.timer)
      q.timer = null
    }
    const patch = q.pending
    q.pending = {}
    if (Object.keys(patch).length === 0) return

    store.saveStatus = 'saving'
    try {
      if (target.kind === 'template') {
        await store.updateTemplate(target.id, patch as RiderTemplateUpdateInput)
      } else {
        await store.updateAttachment(target.id, patch as RiderAttachmentUpdateInput)
      }
      store.saveStatus = 'saved'
    } catch {
      store.saveStatus = 'error'
    }
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

  function scheduleSave(
    target: RiderAutosaveTarget,
    patch: RiderTemplateUpdateInput | RiderAttachmentUpdateInput,
  ): void {
    const k = keyOf(target)
    let q = queues.get(k)
    if (!q) {
      q = { pending: {}, timer: null }
      queues.set(k, q)
    }
    q.pending = { ...q.pending, ...patch }
    store.saveStatus = 'saving'
    if (q.timer) clearTimeout(q.timer)
    q.timer = window.setTimeout(() => {
      void flush(target)
    }, delayMs)
  }

  return { scheduleSave, flush, flushAll }
}