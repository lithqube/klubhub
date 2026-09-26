import type { EventDetail, Issue, LineupEntry, Stage } from '~/types/event'

/**
 * Client mirror of api/internal/promoter/event/timetable.go for instant
 * feedback while editing. The server result is authoritative; parity is
 * pinned by utils/__tests__/timetable.test.ts (same cases as the Go tests).
 */
type Window = Pick<EventDetail, 'starts_at' | 'ends_at' | 'doors_at'>

interface Slot { start: number, end: number, entries: string[] }

const ms = (iso: string) => Date.parse(iso)

export function validateTimetable(ev: Window, stages: Stage[], lineup: LineupEntry[]): Issue[] {
  const issues: Issue[] = []
  const byId = new Map(stages.map(s => [s.id, s]))
  const windowStart = ms(ev.doors_at ?? ev.starts_at)
  const windowEnd = ms(ev.ends_at)
  const perStage = new Map<string, Slot[]>()
  const groups = new Map<string, Slot & { stage: string }>()

  for (const e of lineup) {
    if (!e.set_start || !e.set_end) {
      issues.push({ code: 'untimed', severity: 'warning', entry_ids: [e.id] })
      continue
    }
    if (!e.stage_id) {
      issues.push({ code: 'no_stage', severity: 'error', entry_ids: [e.id] })
      continue
    }
    const st = byId.get(e.stage_id)
    if (!st) {
      issues.push({ code: 'unknown_stage', severity: 'error', entry_ids: [e.id] })
      continue
    }
    const start = ms(e.set_start)
    const end = ms(e.set_end)
    if (start < windowStart || end > windowEnd) {
      issues.push({ code: 'outside_event', severity: 'error', stage_id: st.id, entry_ids: [e.id], from: e.set_start, to: e.set_end })
    }
    if (st.curfew_at && end > ms(st.curfew_at)) {
      issues.push({ code: 'past_curfew', severity: 'error', stage_id: st.id, entry_ids: [e.id], from: st.curfew_at, to: e.set_end, minutes: Math.round((end - ms(st.curfew_at)) / 60000) })
    }
    if (e.b2b_group != null) {
      const key = `${st.id}:${e.b2b_group}`
      const g = groups.get(key)
      if (g) {
        if (g.start !== start || g.end !== end) {
          issues.push({ code: 'b2b_mismatch', severity: 'error', stage_id: st.id, entry_ids: [...g.entries, e.id] })
        }
        g.entries.push(e.id)
        continue
      }
      groups.set(key, { start, end, entries: [e.id], stage: st.id })
      continue
    }
    perStage.set(st.id, [...(perStage.get(st.id) ?? []), { start, end, entries: [e.id] }])
  }
  for (const g of groups.values()) perStage.set(g.stage, [...(perStage.get(g.stage) ?? []), g])

  const ids = [...perStage.keys()].sort((a, b) => (byId.get(a)!.position - byId.get(b)!.position))
  for (const id of ids) {
    const st = byId.get(id)!
    const slots = perStage.get(id)!.sort((a, b) => a.start - b.start)
    const changeover = st.changeover_minutes * 60000
    // Compare with the latest end so far (a short set inside a long one).
    let last = slots[0]!
    for (let i = 1; i < slots.length; i++) {
      const next = slots[i]!
      const gap = next.start - last.end
      const entries = [...last.entries, ...next.entries]
      if (gap < 0) {
        const to = Math.min(last.end, next.end)
        issues.push({ code: 'overlap', severity: 'error', stage_id: id, entry_ids: entries, from: new Date(next.start).toISOString(), to: new Date(to).toISOString(), minutes: Math.round((to - next.start) / 60000) })
      } else if (gap < changeover) {
        issues.push({ code: 'short_changeover', severity: 'warning', stage_id: id, entry_ids: entries, minutes: Math.round(gap / 60000) })
      } else if (gap > changeover) {
        const from = last.end + changeover
        issues.push({ code: 'dead_air', severity: 'warning', stage_id: id, from: new Date(from).toISOString(), to: new Date(next.start).toISOString(), minutes: Math.round((next.start - from) / 60000) })
      }
      if (next.end > last.end) last = next
    }
  }
  return issues
}

export const hasErrors = (issues: Issue[]) => issues.some(i => i.severity === 'error')
