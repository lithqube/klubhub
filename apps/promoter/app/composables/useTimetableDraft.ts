import { computed, onMounted, ref, watch, type Ref } from 'vue'
import type { EventDetail, LineupEntry, LineupInput } from '~/types/event'
import { validateTimetable } from '~/utils/timetable'

/** What a draft was started from: any difference means the saved lineup moved on. */
export function lineupSignature(lineup: LineupEntry[]): string {
  return JSON.stringify(lineup.map(l => [l.id, l.stage_id, l.display_name, l.profile_url, l.billing_order, l.b2b_group, l.set_start, l.set_end]))
}

/**
 * Local lineup draft (UX §2: "the device keeps the draft, the server
 * decides"). Survives reloads and bad signal backstage via localStorage;
 * cleared once the server accepts a save. Conflicted timetables still save
 * (decision 3) — the server's issues come back with the detail.
 *
 * A draft remembers the saved lineup it started from. If a teammate saves
 * in between, the draft is `stale`: saving it would silently replace their
 * work, so the page blocks SAVE until the user chooses.
 */
export function useTimetableDraft(event: Ref<EventDetail | null>) {
  const key = computed(() => (event.value ? `klubhub-promoter:lineup-draft:${event.value.id}` : ''))
  const draft = ref<LineupEntry[] | null>(null)
  /** Signature of the saved lineup the draft started from; '' when unknown. */
  const base = ref('')
  /** Show the saved lineup instead of the draft, to compare. Read-only. */
  const viewSaved = ref(false)

  function load() {
    if (!import.meta.client || !key.value) return
    viewSaved.value = false
    try {
      const raw = localStorage.getItem(key.value)
      const stored = raw ? (JSON.parse(raw) as LineupEntry[] | { base: string, lineup: LineupEntry[] }) : null
      // Drafts saved before the base was recorded count as stale: unknown is not safe.
      draft.value = Array.isArray(stored) ? stored : stored?.lineup ?? null
      base.value = Array.isArray(stored) ? '' : stored?.base ?? ''
    } catch {
      draft.value = null
    }
  }
  function persist() {
    if (!import.meta.client || !key.value) return
    try {
      if (draft.value) localStorage.setItem(key.value, JSON.stringify({ base: base.value, lineup: draft.value }))
      else localStorage.removeItem(key.value)
    } catch { /* storage unavailable: the draft lives in memory only */ }
  }
  onMounted(load)
  watch(key, load)

  const mine = computed<LineupEntry[]>(() => draft.value ?? event.value?.lineup ?? [])
  const lineup = computed<LineupEntry[]>(() => (viewSaved.value ? event.value?.lineup ?? [] : mine.value))
  const issues = computed(() => (event.value ? validateTimetable(event.value, event.value.stages, lineup.value) : []))

  const dirtyCount = computed(() => {
    if (!draft.value || !event.value) return 0
    const saved = new Map(event.value.lineup.map(l => [l.id, JSON.stringify({ ...l, billing_order: 0 })]))
    let n = Math.abs(draft.value.length - event.value.lineup.length)
    for (const l of draft.value) if (saved.get(l.id) !== JSON.stringify({ ...l, billing_order: 0 })) n++
    const order = (xs: LineupEntry[]) => xs.map(x => x.id).join()
    if (n === 0 && order(draft.value) !== order(event.value.lineup)) n = 1
    return n
  })

  const stale = computed(() => dirtyCount.value > 0 && !!event.value && base.value !== lineupSignature(event.value.lineup))

  // The comparison ends with the conflict: without it there are no controls to
  // leave the read-only view, and SAVE would send a draft that is not on screen.
  watch(stale, (conflict) => { if (!conflict) viewSaved.value = false })

  function update(next: LineupEntry[]) {
    if (viewSaved.value) return
    if (!draft.value) base.value = lineupSignature(event.value?.lineup ?? [])
    draft.value = next.map((l, i) => ({ ...l, billing_order: i }))
    persist()
  }
  function discard() {
    draft.value = null
    viewSaved.value = false
    persist()
  }
  /** The user chose their draft over the newer saved lineup: allow SAVE again. */
  function keepMine() {
    base.value = lineupSignature(event.value?.lineup ?? [])
    viewSaved.value = false
    persist()
  }
  function toInput(): LineupInput[] {
    return mine.value.map(l => ({
      id: l.id.startsWith('tmp-') ? undefined : l.id,
      stage_id: l.stage_id, display_name: l.display_name, profile_url: l.profile_url,
      b2b_group: l.b2b_group, set_start: l.set_start, set_end: l.set_end,
    }))
  }
  return { lineup, issues, dirtyCount, stale, viewSaved, update, discard, keepMine, toInput }
}
