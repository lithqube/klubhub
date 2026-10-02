import { describe, expect, it } from 'vitest'
import { riderErrorMessage } from '../riderErrors'

describe('riderErrorMessage', () => {
  const FALLBACK = 'Could not do the thing.'

  it('uses the rider handlers\' {"error": "<text>"} body (e.g. the 422 limits)', () => {
    const e = { message: '[PUT] "/api/v1/rider/templates/x": 422', data: { error: 'invalid rider input: name is too long' } }
    expect(riderErrorMessage(e, FALLBACK)).toBe('invalid rider input: name is too long')
  })

  it('prefers `message` over `error` for the mux\'s {"error": "<code>", "message": "<text>"} body', () => {
    const e = { data: { error: 'not_found', message: 'unknown rider resource: nope' } }
    expect(riderErrorMessage(e, FALLBACK)).toBe('unknown rider resource: nope')
  })

  it('understands the dev mock\'s statusMessage', () => {
    expect(riderErrorMessage({ data: { statusMessage: 'template not found' } }, FALLBACK)).toBe('template not found')
    expect(riderErrorMessage({ statusMessage: 'updatedAt is required' }, FALLBACK)).toBe('updatedAt is required')
  })

  it('never shows ofetch\'s generic message; uses the fallback instead', () => {
    expect(riderErrorMessage(new Error('[POST] "/api": 500 Internal Server Error'), FALLBACK)).toBe(FALLBACK)
    expect(riderErrorMessage({ message: 'fetch failed', statusCode: 0 }, FALLBACK)).toBe(FALLBACK)
  })

  it('falls back for empty, non-string and missing bodies, and for non-errors', () => {
    expect(riderErrorMessage({ data: { error: '   ' } }, FALLBACK)).toBe(FALLBACK)
    expect(riderErrorMessage({ data: { error: 42, message: { nested: true } } }, FALLBACK)).toBe(FALLBACK)
    expect(riderErrorMessage({ data: null }, FALLBACK)).toBe(FALLBACK)
    expect(riderErrorMessage(null, FALLBACK)).toBe(FALLBACK)
    expect(riderErrorMessage(undefined, FALLBACK)).toBe(FALLBACK)
    expect(riderErrorMessage('oops', FALLBACK)).toBe(FALLBACK)
  })

  it('caps very long server text', () => {
    const out = riderErrorMessage({ data: { error: 'x'.repeat(5000) } }, FALLBACK)
    expect(out.length).toBeLessThanOrEqual(301)
    expect(out.endsWith('…')).toBe(true)
  })
})
