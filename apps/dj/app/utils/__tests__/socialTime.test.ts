import { describe, expect, it } from 'vitest'
import { toZonedInput } from '../socialTime'

describe('toZonedInput', () => {
  it('shows the wall time of the post zone, across daylight saving', () => {
    expect(toZonedInput('2026-07-01T18:00:00Z', 'Europe/Berlin')).toBe('2026-07-01T20:00')
    expect(toZonedInput('2026-12-01T19:00:00Z', 'Europe/Berlin')).toBe('2026-12-01T20:00')
  })

  it('crosses the date line and shows midnight as 00', () => {
    expect(toZonedInput('2026-07-01T22:00:00Z', 'Europe/Berlin')).toBe('2026-07-02T00:00')
  })

  it('returns an empty value for an unknown zone or date', () => {
    expect(toZonedInput('2026-07-01T18:00:00Z', 'Mars/Olympus')).toBe('')
    expect(toZonedInput('nope', 'UTC')).toBe('')
  })
})
