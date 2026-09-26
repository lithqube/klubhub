import { events, summary } from '../-mockDb'
export default defineEventHandler((event) => {
  const view = String(getQuery(event).view ?? 'upcoming')
  const now = Date.now()
  const rows = events.filter(e =>
    view === 'drafts' ? e.status === 'draft'
      : view === 'past' ? e.status !== 'draft' && Date.parse(e.ends_at) <= now
        : e.status !== 'draft' && Date.parse(e.ends_at) > now)
  return rows.sort((a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at)).map(summary)
})
