import { describe, expect, it } from 'vitest'
import { isNavActive } from './shell'

describe('isNavActive', () => {
  it('matches the root route exactly', () => {
    expect(isNavActive('/', '/')).toBe(true)
    expect(isNavActive('/events', '/')).toBe(false)
  })

  it('matches nested routes by prefix', () => {
    expect(isNavActive('/events', '/events')).toBe(true)
    expect(isNavActive('/events/abc/guests', '/events')).toBe(true)
    expect(isNavActive('/guests', '/events')).toBe(false)
  })
})
