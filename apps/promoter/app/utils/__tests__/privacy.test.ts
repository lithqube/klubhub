import { describe, expect, it } from 'vitest'
import {
  confirmMatches, countsText, daysAfterEnd, daysAfterText, isStepUp, privacyBanner, purgeDate, purgeErrorText, retentionErrorText,
  shortDate, validateRetentionDays,
} from '../privacy'

const TZ = 'Europe/Berlin'
const NOW = Date.parse('2026-10-20T12:00:00Z')
// Ended Saturday 07:00 Berlin (CEST).
const EVENT = { ends_at: '2026-10-11T05:00:00Z', timezone: TZ }

describe('validateRetentionDays', () => {
  it('accepts whole days from 1 to 365', () => {
    expect(validateRetentionDays('1')).toEqual({ days: 1, error: null })
    expect(validateRetentionDays(' 45 ')).toEqual({ days: 45, error: null })
    expect(validateRetentionDays(365)).toEqual({ days: 365, error: null })
  })

  it('refuses zero, too many days, decimals, signs and text', () => {
    for (const bad of ['0', '366', '1000', '7.5', '-3', '+3', '', 'ten', '1e2']) {
      expect(validateRetentionDays(bad)).toEqual({ days: null, error: 'Enter a whole number of days from 1 to 365.' })
    }
  })
})

describe('purge dates', () => {
  it('formats a date in the event timezone, with the year only when it is not this year', () => {
    // 23:30 UTC on 11 Nov is already 12 Nov in Berlin.
    expect(shortDate('2026-11-11T23:30:00Z', TZ, NOW)).toBe('12 Nov')
    expect(shortDate('2027-01-05T10:00:00Z', TZ, NOW)).toBe('5 Jan 2027')
  })

  it('counts whole days between the end and the purge date', () => {
    expect(daysAfterEnd(EVENT.ends_at, '2026-11-10T05:00:00Z')).toBe(30)
    // A DST change in between (one hour shorter) still rounds to whole days.
    expect(daysAfterEnd(EVENT.ends_at, '2026-11-10T06:00:00Z')).toBe(30)
    expect(daysAfterEnd(EVENT.ends_at, EVENT.ends_at)).toBe(0)
    expect(daysAfterText(30)).toBe('30 days after it ended')
    expect(daysAfterText(1)).toBe('1 day after it ended')
    expect(daysAfterText(0)).toBe('the day it ended')
  })

  it('uses the server purge date, else the end plus retention', () => {
    expect(purgeDate(EVENT.ends_at, { purge_after: '2026-12-01T00:00:00Z', retention_days: 30 })).toBe('2026-12-01T00:00:00Z')
    expect(purgeDate(EVENT.ends_at, { purge_after: null, retention_days: 7 })).toBe('2026-10-18T05:00:00.000Z')
  })
})

describe('privacyBanner', () => {
  const p = { purge_after: '2026-11-10T06:00:00Z', purged_at: null, retention_days: 30, personal_rows: 12 }

  it('says nothing before the event ends or without the privacy state', () => {
    expect(privacyBanner({ ends_at: '2026-10-25T05:00:00Z', timezone: TZ }, p, NOW).state).toBe('none')
    expect(privacyBanner(EVENT, null, NOW).state).toBe('none')
  })

  it('says when names and contacts are erased after the event ended', () => {
    expect(privacyBanner(EVENT, p, NOW)).toEqual({
      state: 'scheduled', at: p.purge_after,
      text: 'Guest names and contacts for this event are erased on 10 Nov (30 days after it ended).',
    })
  })

  it('says the erase is due once the date passed and the job has not run yet', () => {
    const due = privacyBanner(EVENT, { ...p, purge_after: '2026-10-18T05:00:00Z', retention_days: 7 }, NOW)
    expect(due.state).toBe('due')
    expect(due.text).toBe('Guest names and contacts for this event are due to be erased: 18 Oct (7 days after it ended). This happens within the hour.')
  })

  it('says when they were erased', () => {
    const b = privacyBanner(EVENT, { ...p, purged_at: '2026-10-19T08:00:00Z', personal_rows: 0 }, NOW)
    expect(b).toEqual({ state: 'purged', at: '2026-10-19T08:00:00Z', text: 'Guest names and contacts were erased on 19 Oct.' })
  })
})

describe('countsText', () => {
  it('lists what was erased, singular and plural, zeros left out', () => {
    expect(countsText({ guests: 12, order_positions: 1, orders: 0, guest_allocations: 2, door_pins: 1 }))
      .toBe('12 guests · 1 ticket · 2 submitter contacts · 1 door PIN')
    expect(countsText({ guests: 1, orders: 3, checkins_kept: 5 })).toBe('1 guest · 3 orders · 5 other rows')
    expect(countsText({})).toBe('nothing to erase')
    expect(countsText(null)).toBe('nothing to erase')
  })
})

describe('erase copy', () => {
  it('recognises step-up refusals', () => {
    expect(isStepUp('reauthentication_required')).toBe(true)
    expect(isStepUp('step_up_required')).toBe(true)
    expect(isStepUp('mfa_required')).toBe(false)
  })

  it('maps refusals to plain words', () => {
    expect(purgeErrorText({ error: 'reauthentication_required' })).toMatch(/recent sign-in/)
    expect(purgeErrorText({ error: 'event_not_ended' })).toMatch(/has not ended yet/)
    expect(purgeErrorText({ error: 'invalid', field: 'confirm' })).toBe('The title you typed does not match the event title.')
    expect(purgeErrorText({ error: 'no_role_grant' })).toBe('Only owners and admins can erase guest data.')
    expect(purgeErrorText({ error: 'network_error' })).toMatch(/Check your connection/)
    expect(retentionErrorText({ error: 'invalid' })).toBe('Enter a whole number of days from 1 to 365.')
    expect(retentionErrorText({ error: 'no_role_grant' })).toMatch(/Only owners and admins/)
  })

  it('matches the typed title exactly, ignoring surrounding spaces', () => {
    expect(confirmMatches(' Klubnacht 02 ', 'Klubnacht 02')).toBe(true)
    expect(confirmMatches('klubnacht 02', 'Klubnacht 02')).toBe(false)
    expect(confirmMatches('', '')).toBe(false)
  })
})
