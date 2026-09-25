import { describe, expect, it } from 'vitest'
import { PROMOTER_BOTTOM_NAV, PROMOTER_NAV } from '../nav'

describe('promoter navigation', () => {
  it('has unique routes and labels', () => {
    const routes = PROMOTER_NAV.map(i => i.to)
    expect(new Set(routes).size).toBe(routes.length)
    expect(new Set(PROMOTER_NAV.map(i => i.label)).size).toBe(PROMOTER_NAV.length)
  })

  it('starts with the dashboard at the root route', () => {
    expect(PROMOTER_NAV[0]?.to).toBe('/')
  })

  it('keeps the mobile bar to five destinations that exist in the main nav', () => {
    expect(PROMOTER_BOTTOM_NAV).toHaveLength(5)
    const main = new Set(PROMOTER_NAV.map(i => i.to))
    for (const item of PROMOTER_BOTTOM_NAV) expect(main.has(item.to)).toBe(true)
  })
})
