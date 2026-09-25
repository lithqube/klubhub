import type { EventSummary } from '~/types/event'

export type AttentionKind = 'conflict' | 'untimed' | 'goes_live' | 'reveal' | 'draft' | 'security'
export type AttentionTone = 'failed' | 'pending' | 'scheduled' | 'draft'

export interface AttentionItem {
  kind: AttentionKind
  tone: AttentionTone
  label: string
  title: string
  detail: string
  to: string
  action: string
}

const HOUR = 3_600_000
const DAY = 24 * HOUR

const TONE: Record<AttentionKind, AttentionTone> = {
  conflict: 'failed', untimed: 'pending', goes_live: 'scheduled', reveal: 'scheduled', draft: 'draft', security: 'pending',
}
/** Queue order (UX §4.1): conflicts, untimed, go-live / reveal, drafts, security. */
const ORDER: AttentionKind[] = ['conflict', 'untimed', 'goes_live', 'reveal', 'draft', 'security']

function inHours(iso: string, now: number): string {
  const h = Math.max(0, Math.round((Date.parse(iso) - now) / HOUR))
  return h < 1 ? 'in under an hour' : h < 48 ? `in ${h} h` : `in ${Math.round(h / 24)} days`
}
const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`

/**
 * What needs the team's attention, most urgent first. `upcoming` and `drafts`
 * are the events list views; past events never appear.
 */
export function attentionQueue(upcoming: EventSummary[], drafts: EventSummary[], opts: { now?: number, needsMfa?: boolean } = {}): AttentionItem[] {
  const now = opts.now ?? Date.now()
  const items: AttentionItem[] = []
  const live = [...upcoming, ...drafts].filter(e => e.status !== 'cancelled' && Date.parse(e.ends_at) > now)
  for (const e of live) {
    const lineup = `/events/${e.id}/lineup`
    if (e.error_count > 0) {
      items.push({ kind: 'conflict', tone: TONE.conflict, label: 'CONFLICT', title: e.title, detail: `${plural(e.error_count, 'timetable problem')} · publishing and export are blocked`, to: lineup, action: 'FIX' })
    }
    if (e.untimed_count > 0) {
      items.push({ kind: 'untimed', tone: TONE.untimed, label: 'UNTIMED', title: e.title, detail: `${plural(e.untimed_count, 'act has', 'acts have')} no set time`, to: lineup, action: 'SCHEDULE' })
    }
    if (e.status === 'scheduled' && e.publish_at && Date.parse(e.publish_at) > now && Date.parse(e.publish_at) - now <= 48 * HOUR) {
      items.push({ kind: 'goes_live', tone: TONE.goes_live, label: 'GOES LIVE', title: e.title, detail: `public ${inHours(e.publish_at, now)}`, to: `/events/${e.id}/details`, action: 'REVIEW' })
    }
    if (e.location_mode === 'secret' && e.location_reveal_at && Date.parse(e.location_reveal_at) > now && Date.parse(e.location_reveal_at) - now <= 48 * HOUR) {
      items.push({ kind: 'reveal', tone: TONE.reveal, label: 'REVEAL', title: e.title, detail: `venue unlocks ${inHours(e.location_reveal_at, now)} · rebuild the export after`, to: `/events/${e.id}/export`, action: 'EXPORT' })
    }
    if (e.status === 'draft' && Date.parse(e.starts_at) - now <= 21 * DAY) {
      const missing = [!e.venue_id && e.location_mode === 'venue' && 'no venue', e.act_count === 0 && 'no lineup'].filter(Boolean)
      items.push({ kind: 'draft', tone: TONE.draft, label: 'DRAFT', title: e.title, detail: missing.length ? missing.join(', ') : `not published · starts ${inHours(e.starts_at, now)}`, to: `/events/${e.id}/details`, action: 'CONTINUE' })
    }
  }
  if (opts.needsMfa) {
    items.push({ kind: 'security', tone: TONE.security, label: 'SECURITY', title: 'Add an authenticator app', detail: 'your role needs a second factor for members, venue contacts and money', to: '/account/security', action: 'SET UP' })
  }
  return items.sort((a, b) => ORDER.indexOf(a.kind) - ORDER.indexOf(b.kind))
}

/** Keeps at most `max` items of each kind; returns the hidden counts. */
export function collapseQueue(items: AttentionItem[], max = 3): { shown: AttentionItem[], more: Partial<Record<AttentionKind, number>> } {
  const seen: Partial<Record<AttentionKind, number>> = {}
  const more: Partial<Record<AttentionKind, number>> = {}
  const shown = items.filter((i) => {
    seen[i.kind] = (seen[i.kind] ?? 0) + 1
    if (seen[i.kind]! <= max) return true
    more[i.kind] = (more[i.kind] ?? 0) + 1
    return false
  })
  return { shown, more }
}

/** The event /now opens: running now, else the next one within 7 days. */
export function pickNow(upcoming: EventSummary[], now = Date.now()): EventSummary | null {
  const active = upcoming
    .filter(e => e.status !== 'cancelled' && Date.parse(e.ends_at) > now)
    .sort((a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at))
  const running = active.find(e => Date.parse(e.starts_at) <= now)
  if (running) return running
  const next = active[0]
  return next && Date.parse(next.starts_at) - now <= 7 * DAY ? next : null
}
