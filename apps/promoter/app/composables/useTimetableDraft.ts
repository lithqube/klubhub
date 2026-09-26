import type { EventDetail, LineupEntry, LineupInput } from '~/types/event'
import { validateTimetable } from '~/utils/timetable'

/**
 * Local lineup draft (UX §2: "the device keeps the draft, the server
 * decides"). Survives reloads and bad signal backstage via localStorage;
 * cleared once the server accepts a save. Conflicted timetables still save
 * (decision 3) — the server's issues come back with the detail.
 */
export function useTimetableDraft(event: Ref<EventDetail | null>) {
  const key = computed(() => (event.value ? `klubhub-promoter:lineup-draft:${event.value.id}` : ''))
  const draft = ref<LineupEntry[] | null>(null)

  function load() {
    if (!import.meta.client || !key.value) return
    try {
      const raw = localStorage.getItem(key.value)
      draft.value = raw ? (JSON.parse(raw) as LineupEntry[]) : null
    } catch {
      draft.value = null
    }
  }
  function persist() {
    if (!import.meta.client || !key.value) return
    try {
      if (draft.value) localStorage.setItem(key.value, JSON.stringify(draft.value))
      else localStorage.removeItem(key.value)
    } catch { /* storage unavailable: the draft lives in memory only */ }
  }
  onMounted(load)
  watch(key, load)

  const lineup = computed<LineupEntry[]>(() => draft.value ?? event.value?.lineup ?? [])
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

  function update(next: LineupEntry[]) {
    draft.value = next.map((l, i) => ({ ...l, billing_order: i }))
    persist()
  }
  function discard() {
    draft.value = null
    persist()
  }
  function toInput(): LineupInput[] {
    return lineup.value.map(l => ({
      id: l.id.startsWith('tmp-') ? undefined : l.id,
      stage_id: l.stage_id, display_name: l.display_name, profile_url: l.profile_url,
      b2b_group: l.b2b_group, set_start: l.set_start, set_end: l.set_end,
    }))
  }
  return { lineup, issues, dirtyCount, update, discard, toInput }
}
