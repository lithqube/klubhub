import { describe, expect, it } from 'vitest'
import type { Guest, Ticket } from '~/types/guest'
import {
  allocationState, countByStatus, csvCell, cutoffInstant, fold, guestErrorText, headsHeld, parseEmails, parsePastedGuests, quotaFill,
  searchGuests, searchTickets, toCsv,
} from '../guests'

const g = (over: Partial<Guest>): Guest => ({
  id: 'g', list_id: 'l', allocation_id: null, name: 'X', email: '', phone: '', note: '', plus_n: 0, status: 'going',
  source: 'manual', created_at: '', updated_at: '', ...over,
})

describe('parsePastedGuests', () => {
  it('reads names with +N in the usual shapes', () => {
    const text = 'Mara Weiss +2\n- Tom (+1)\n3. Ines\n\n• Kim, +1\nJosé Müller [+ 3]\n   \n'
    expect(parsePastedGuests(text)).toEqual([
      { name: 'Mara Weiss', plus_n: 2, email: '' },
      { name: 'Tom', plus_n: 1, email: '' },
      { name: 'Ines', plus_n: 0, email: '' },
      { name: 'Kim', plus_n: 1, email: '' },
      { name: 'José Müller', plus_n: 3, email: '' },
    ])
  })

  it('separates an email from the name', () => {
    expect(parsePastedGuests('Lena W <lena@example.org> +1\nsam@example.org')).toEqual([
      { name: 'Lena W', plus_n: 1, email: 'lena@example.org' },
    ])
  })

  it('keeps names that merely contain a plus, digits or brackets', () => {
    expect(parsePastedGuests('DJ +Minus\nRoom 2 crew\nTom (DJ) +1\nAnne-Marie; ')).toEqual([
      { name: 'DJ +Minus', plus_n: 0, email: '' },
      { name: 'Room 2 crew', plus_n: 0, email: '' },
      { name: 'Tom (DJ)', plus_n: 1, email: '' },
      { name: 'Anne-Marie', plus_n: 0, email: '' },
    ])
  })
})

describe('parseEmails', () => {
  it('finds unique emails in any pasted layout', () => {
    expect(parseEmails('a@example.org, B@example.org\n"Sam" <b@example.org>; c@x.io nope@local')).toEqual([
      'a@example.org', 'B@example.org', 'c@x.io',
    ])
  })
})

describe('search', () => {
  it('folds case, accents and spacing like the server', () => {
    expect(fold('  José   MÜLLER ')).toBe('jose muller')
    expect(fold('Łukasz Ørsted')).toBe('lukasz orsted')
  })

  it('matches every word across name, email and note', () => {
    const list = [g({ id: '1', name: 'José Müller', note: 'label' }), g({ id: '2', name: 'Mara', email: 'mara@x.org' })]
    expect(searchGuests(list, 'jose lab').map(x => x.id)).toEqual(['1'])
    expect(searchGuests(list, 'MARA@')).toHaveLength(1)
    expect(searchGuests(list, '  ')).toHaveLength(2)
  })
})

describe('counts and quota', () => {
  it('counts statuses and going heads', () => {
    const c = countByStatus([g({ plus_n: 2 }), g({}), g({ status: 'pending' }), g({ status: 'declined', plus_n: 5 })])
    expect(c).toEqual({ all: 4, going: 2, pending: 1, waitlist: 0, invited: 0, declined: 1, going_heads: 4, tickets: 0 })
  })

  it('only going, pending and invited hold quota', () => {
    expect(headsHeld([g({ plus_n: 1 }), g({ status: 'invited' }), g({ status: 'waitlist', plus_n: 3 }), g({ status: 'declined' })])).toBe(3)
  })

  it('tones the quota bar', () => {
    expect(quotaFill(2, 10)).toEqual({ pct: 20, tone: 'ok' })
    expect(quotaFill(8, 10)).toEqual({ pct: 80, tone: 'near' })
    expect(quotaFill(10, 10)).toEqual({ pct: 100, tone: 'full' })
    expect(quotaFill(3, 0)).toEqual({ pct: 0, tone: 'ok' })
  })

  it('reports whether an allocation takes guests', () => {
    const now = Date.parse('2026-10-03T12:00:00Z')
    expect(allocationState({ revoked_at: null, deadline: null }, now)).toBe('open')
    expect(allocationState({ revoked_at: null, deadline: '2026-10-03T11:00:00Z' }, now)).toBe('closed')
    expect(allocationState({ revoked_at: '2026-10-01T00:00:00Z', deadline: null }, now)).toBe('revoked')
  })
})

describe('cutoffInstant', () => {
  const start = '2026-10-03T21:00:00Z' // 23:00 in Berlin (CEST)
  it('resolves after-midnight cutoffs to the next morning', () => {
    expect(cutoffInstant(start, 'Europe/Berlin', '01:00')).toBe('2026-10-03T23:00:00.000Z')
  })
  it('keeps evening cutoffs on the same day', () => {
    expect(cutoffInstant(start, 'Europe/Berlin', '23:30')).toBe('2026-10-03T21:30:00.000Z')
    expect(cutoffInstant(start, 'Europe/Berlin', '22:00')).toBe('2026-10-03T20:00:00.000Z')
  })
  it('wraps times earlier than 12 h before the start', () => {
    expect(cutoffInstant(start, 'Europe/Berlin', '10:59')).toBe('2026-10-04T08:59:00.000Z')
  })
})

describe('csv', () => {
  it('neutralises formulas and quotes separators', () => {
    expect(csvCell('=SUM(A1)')).toBe("'=SUM(A1)")
    expect(csvCell('+49 30 1')).toBe("'+49 30 1")
    expect(csvCell('-1')).toBe("'-1")
    expect(csvCell('@cmd')).toBe("'@cmd")
    expect(csvCell('Anne-Marie')).toBe('Anne-Marie')
    expect(csvCell('O"Brien, Sam')).toBe('"O""Brien, Sam"')
    expect(csvCell('=a,b')).toBe('"\'=a,b"')
    expect(csvCell(2)).toBe('2')
  })

  it('joins rows', () => {
    expect(toCsv([['name', 'plus_n'], ['Mara', 1]])).toBe('name,plus_n\nMara,1\n')
  })
})

describe('guestErrorText', () => {
  it('explains a full allocation with numbers', () => {
    expect(guestErrorText({ error: 'quota_exceeded', detail: { label: 'Ben Klock', quota: 6, used: 5, requested: 2 } }))
      .toBe('Ben Klock is at 5 of 6 heads. These guests need 2; only 1 left. Raise the quota or waitlist them.')
    expect(guestErrorText({ error: 'quota_exceeded', detail: { label: 'X', quota: 2, used: 2, requested: 1 } })).toContain('none are left')
  })

  it('points validation errors at the pasted line', () => {
    expect(guestErrorText({ error: 'invalid', field: 'guests[2].plus_n', problem: 'more than this allocation allows' }))
      .toBe('Line 3: +N — more than this allocation allows')
  })

  it('covers closed and revoked allocations', () => {
    expect(guestErrorText({ error: 'allocation_closed' })).toMatch(/deadline/)
    expect(guestErrorText({ error: 'allocation_revoked' })).toMatch(/revoked/)
    expect(guestErrorText({ error: 'network_error' })).toMatch(/connection/)
  })
})

describe('searchTickets', () => {
  const t = (over: Partial<Ticket>): Ticket => ({
    id: 't', order_id: 'o', source: 'dice', order_ref: 'D-1', ticket_type_id: 'tt', ticket_type: 'Early bird', name: 'X', email: '',
    status: 'valid', imported_at: '', ...over,
  })
  it('matches holder, email, ticket type and order number, accent-insensitively', () => {
    const list = [t({ id: '1', name: 'José Müller' }), t({ id: '2', name: 'Kim', email: 'kim@example.org', ticket_type: 'Regular', order_ref: 'D-77' })]
    expect(searchTickets(list, 'jose').map(x => x.id)).toEqual(['1'])
    expect(searchTickets(list, 'regular d-77').map(x => x.id)).toEqual(['2'])
    expect(searchTickets(list, 'kim@')).toHaveLength(1)
    expect(searchTickets(list, '  ')).toHaveLength(2)
  })
})
