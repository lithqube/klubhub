import { describe, expect, it } from 'vitest'
import { PROMOTER_BOTTOM_NAV, PROMOTER_MORE_NAV, PROMOTER_NAV } from '../nav'

describe('promoter navigation', () => {
  it('has unique routes and labels', () => {
    const routes = PROMOTER_NAV.map(i => i.to)
    expect(new Set(routes).size).toBe(routes.length)
    expect(new Set(PROMOTER_NAV.map(i => i.label)).size).toBe(PROMOTER_NAV.length)
  })

  it('keeps each group contiguous so the sidebar prints one heading per group', () => {
    const seen = new Set<string>()
    let prev: string | undefined
    for (const item of PROMOTER_NAV) {
      if (item.group && item.group !== prev) {
        expect(seen.has(item.group)).toBe(false)
        seen.add(item.group)
      }
      prev = item.group
    }
  })

  it('keeps unbuilt sections out of the mobile bar and inside MORE', () => {
    for (const item of PROMOTER_BOTTOM_NAV) expect(item.tag).toBeUndefined()
    const more = PROMOTER_MORE_NAV.map(i => i.to)
    for (const tagged of PROMOTER_NAV.filter(i => i.tag)) expect(more).toContain(tagged.to)
    expect(PROMOTER_BOTTOM_NAV.length + 1).toBeLessThanOrEqual(5) // + MORE
  })
})
