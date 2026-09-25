import { describe, expect, it } from 'vitest'
import { CSRF_HEADER, withCsrf } from '../api'

describe('withCsrf', () => {
  it('leaves safe requests alone', () => {
    expect(withCsrf({ method: 'GET' })).toEqual({ method: 'GET' })
    expect(withCsrf()).toEqual({})
  })

  it('adds the CSRF header to state-changing requests', () => {
    for (const method of ['POST', 'put', 'PATCH', 'DELETE'] as const) {
      const o = withCsrf({ method, headers: { 'x-other': 'y' } })
      expect((o.headers as Record<string, string>)[CSRF_HEADER]).toBe('1')
      expect((o.headers as Record<string, string>)['x-other']).toBe('y')
    }
  })
})
