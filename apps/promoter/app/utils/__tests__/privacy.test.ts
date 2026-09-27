import { describe, expect, it } from 'vitest'
import {
  authIsStale, confirmMatches, countsText, daysAfterEnd, daysAfterText, ERASED_ITEMS, isStepUp, KEPT_ITEMS, normalizeTitle, privacyBanner, purgeDate,
  purgeErrorText, retentionErrorText, retentionSaveButton, shortDate, signInAgainRoute, stepUpLoginText, validateRetentionDays, wouldPurgeText,
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
    // Not ended yet (the settings list running events too).
    expect(daysAfterText(30, false)).toBe('30 days after it ends')
    expect(daysAfterText(0, false)).toBe('the day it ends')
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

  it('says the erase is running once the date passed and the job has not run yet', () => {
    const due = privacyBanner(EVENT, { ...p, purge_after: '2026-10-18T05:00:00Z', retention_days: 7 }, NOW)
    expect(due.state).toBe('due')
    expect(due.text).toBe('Guest names and contacts for this event are being erased now (scheduled for 18 Oct, 7 days after it ended).')
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
    expect(purgeErrorText({ error: 'reauthentication_required' })).toMatch(/recent sign-in.*come straight back here/)
    expect(retentionErrorText({ error: 'reauthentication_required' })).toMatch(/recent sign-in/)
    expect(retentionErrorText({ error: 'retention_would_purge' })).toMatch(/changed/)
    expect(purgeErrorText({ error: 'event_not_ended' })).toMatch(/has not ended yet/)
    expect(purgeErrorText({ error: 'invalid', field: 'confirm' })).toBe('The title you typed does not match the event title.')
    expect(purgeErrorText({ error: 'no_role_grant' })).toBe('Only owners and admins can erase guest data.')
    expect(purgeErrorText({ error: 'network_error' })).toMatch(/Check your connection/)
    expect(retentionErrorText({ error: 'invalid' })).toBe('Enter a whole number of days from 1 to 365.')
    expect(retentionErrorText({ error: 'no_role_grant' })).toMatch(/Only owners and admins/)
  })

  it('matches the typed title after normalising both sides (as the server does)', () => {
    expect(confirmMatches(' Klubnacht 02 ', 'Klubnacht 02')).toBe(true)
    expect(confirmMatches('klubnacht 02', 'Klubnacht 02')).toBe(true)
    const title = 'Klubnacht 03 – Tresor’s “Late” Edition'
    for (const typed of [
      'klubnacht 03 - tresor\'s "late" edition',
      '  KLUBNACHT   03 — Tresor\'s "Late"\tEdition  ',
      'Ｋｌｕｂｎａｃｈｔ ０３ - tresor\'s "late" edition',
      'klubnacht\u00a003 \u2010 tresor‘s „late“ edition',
    ]) expect(confirmMatches(typed, title), typed).toBe(true)
    for (const typed of ['', 'Klubnacht 03', 'Klubnacht 03 Tresors Late Edition', 'Klubnacht 3 – Tresor’s “Late” Edition']) {
      expect(confirmMatches(typed, title), typed).toBe(false)
    }
    expect(confirmMatches('', '')).toBe(false)
    expect(confirmMatches(' ', '  ')).toBe(false)
    expect(normalizeTitle('  A\u2013B  “C”  ')).toBe('a-b "c"')
  })
})

describe('step-up before an erase', () => {
  it('treats a sign-in older than 14 minutes as stale and leaves unknown times to the server', () => {
    expect(authIsStale(new Date(NOW - 13 * 60_000).toISOString(), NOW)).toBe(false)
    expect(authIsStale(new Date(NOW - 14 * 60_000).toISOString(), NOW)).toBe(false)
    expect(authIsStale(new Date(NOW - 14 * 60_000 - 1000).toISOString(), NOW)).toBe(true)
    expect(authIsStale(new Date(NOW - 3 * 3_600_000).toISOString(), NOW)).toBe(true)
    expect(authIsStale(undefined, NOW)).toBe(false)
    expect(authIsStale('not a date', NOW)).toBe(false)
  })

  it('signs in again and comes straight back, saying why', () => {
    expect(signInAgainRoute('/events/e1/guests?erase=1', 'erase')).toEqual({ path: '/login', query: { next: '/events/e1/guests?erase=1', why: 'erase' } })
    expect(stepUpLoginText('erase')).toBe('Confirm it\'s you to erase guest data.')
    expect(stepUpLoginText('retention')).toMatch(/Confirm it's you to change how long guest data is kept/)
    expect(stepUpLoginText(undefined)).toBe('')
    expect(stepUpLoginText('other')).toBe('')
  })
})

describe('shortening retention', () => {
  const preview = (titles: string[]) => ({ would_purge: titles.map((title, i) => ({ event_id: `e${i}`, title, ends_at: '2026-10-11T05:00:00Z' })), count: titles.length })

  it('warns with the count and up to three titles', () => {
    expect(wouldPurgeText(7, preview(['A', 'B', 'C']))).toBe('With 7 days, guest names and contacts of 3 ended events (A, B, C) are erased within the hour. This can\'t be undone.')
    expect(wouldPurgeText(7, preview(['A', 'B', 'C', 'D']))).toBe('With 7 days, guest names and contacts of 4 ended events (A, B, C, …) are erased within the hour. This can\'t be undone.')
    expect(wouldPurgeText(1, preview(['Solo']))).toBe('With 1 day, guest names and contacts of 1 ended event (Solo) are erased within the hour. This can\'t be undone.')
    expect(wouldPurgeText(7, preview([]))).toBe('')
    expect(wouldPurgeText(7, null)).toBe('')
  })

  it('turns the save into SAVE AND ERASE N, enabled only once acknowledged with a recent sign-in', () => {
    const base = { busy: false, changed: true, shorter: true, previewing: false, erasing: 0, ack: false, stale: false }
    expect(retentionSaveButton(base)).toEqual({ label: 'SAVE RETENTION', danger: false, disabled: false })
    expect(retentionSaveButton({ ...base, previewing: true }).disabled).toBe(true)
    expect(retentionSaveButton({ ...base, shorter: false, previewing: true }).disabled).toBe(false)
    expect(retentionSaveButton({ ...base, erasing: 3 })).toEqual({ label: 'SAVE AND ERASE 3', danger: true, disabled: true })
    expect(retentionSaveButton({ ...base, erasing: 3, ack: true })).toEqual({ label: 'SAVE AND ERASE 3', danger: true, disabled: false })
    expect(retentionSaveButton({ ...base, erasing: 3, ack: true, stale: true }).disabled).toBe(true)
    expect(retentionSaveButton({ ...base, changed: false }).disabled).toBe(true)
    expect(retentionSaveButton({ ...base, erasing: 3, ack: true, busy: true })).toMatchObject({ label: 'SAVING…', disabled: true })
  })
})

describe('erased and kept', () => {
  it('is one list for the settings and the dialog', () => {
    expect(ERASED_ITEMS).toContain('Guest names, emails, phone numbers and notes')
    expect(KEPT_ITEMS.some(k => k.startsWith('The report'))).toBe(true)
  })
})
