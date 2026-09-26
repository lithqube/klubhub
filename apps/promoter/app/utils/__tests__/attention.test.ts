import { describe, expect, it } from 'vitest'
import type { EventSummary } from '~/types/event'
import { attentionQueue, collapseQueue, pickNow } from '../attention'

const NOW = Date.parse('2026-10-01T12:00:00Z')
const H = 3_600_000
const at = (h: number) => new Date(NOW + h * H).toISOString()

function ev(p: Partial<EventSummary>): EventSummary {
  return {
    id: 'e', title: 'Night', slug: 'night', status: 'published', visibility: 'public', publish_at: null,
    starts_at: at(48), ends_at: at(56), doors_at: null, timezone: 'Europe/Berlin', venue_id: 'v', city: 'Berlin',
    location_mode: 'venue', location_reveal_at: null, min_age: null, genres: [], description_md: '', cost_text: '',
    external_ticket_url: null, capacity: null, version: 1, created_at: at(-100), updated_at: at(-1),
    venue_name: 'Club', act_count: 3, stage_count: 1, untimed_count: 0, error_count: 0, warning_count: 0, ...p,
  }
}

describe('attentionQueue', () => {
  it('orders conflicts first and security last', () => {
    const q = attentionQueue(
      [ev({ id: 'a', untimed_count: 1 }), ev({ id: 'b', error_count: 2 })],
      [ev({ id: 'c', status: 'draft', act_count: 0 })],
      { now: NOW, needsMfa: true },
    )
    expect(q.map(i => i.kind)).toEqual(['conflict', 'untimed', 'draft', 'security'])
    expect(q[0]!.to).toBe('/events/b/lineup')
    expect(q[2]!.detail).toBe('no lineup')
  })

  it('flags go-live and reveal only inside 48 hours', () => {
    const q = attentionQueue([
      ev({ id: 'soon', status: 'scheduled', publish_at: at(20) }),
      ev({ id: 'later', status: 'scheduled', publish_at: at(72) }),
      ev({ id: 'secret', location_mode: 'secret', location_reveal_at: at(10) }),
    ], [], { now: NOW })
    expect(q.map(i => `${i.kind}:${i.to}`)).toEqual(['goes_live:/events/soon/details', 'reveal:/events/secret/export'])
    expect(q[0]!.detail).toBe('public in 20 h')
  })

  it('ignores cancelled and far-off drafts', () => {
    expect(attentionQueue([ev({ status: 'cancelled', error_count: 1 })], [ev({ status: 'draft', starts_at: at(24 * 30), ends_at: at(24 * 30 + 8) })], { now: NOW })).toEqual([])
  })

  it('collapses more than three of a kind', () => {
    const q = attentionQueue([], [1, 2, 3, 4, 5].map(i => ev({ id: `d${i}`, status: 'draft' })), { now: NOW })
    const { shown, more } = collapseQueue(q)
    expect(shown).toHaveLength(3)
    expect(more.draft).toBe(2)
  })
})

describe('pickNow', () => {
  it('prefers the running event, then the next within 7 days', () => {
    expect(pickNow([ev({ id: 'next', starts_at: at(24), ends_at: at(32) }), ev({ id: 'live', starts_at: at(-2), ends_at: at(4) })], NOW)?.id).toBe('live')
    expect(pickNow([ev({ id: 'next', starts_at: at(24), ends_at: at(32) })], NOW)?.id).toBe('next')
    expect(pickNow([ev({ id: 'far', starts_at: at(24 * 9), ends_at: at(24 * 9 + 8) })], NOW)).toBeNull()
  })
})
