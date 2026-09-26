import { describe, expect, it } from 'vitest'
import { contrast, DEFAULT_ACCENT, EXPORT_BG } from '../color'

describe('contrast', () => {
  it('matches WCAG reference values', () => {
    expect(contrast('#ffffff', '#000000')).toBeCloseTo(21, 0)
    expect(contrast('#777777', '#ffffff')).toBeCloseTo(4.48, 1)
  })

  it('keeps the default accent readable on export backgrounds', () => {
    expect(contrast(DEFAULT_ACCENT, EXPORT_BG)).toBeGreaterThan(4.5)
    expect(contrast('#1a1a40', EXPORT_BG)).toBeLessThan(3)
  })
})
