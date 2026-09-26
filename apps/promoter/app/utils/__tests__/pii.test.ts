import { describe, expect, it } from 'vitest'
import { looksLikeContact } from '../pii'

describe('looksLikeContact', () => {
  it('flags emails and phone numbers', () => {
    expect(looksLikeContact('ask sam@club.example for the key')).toBe('email')
    expect(looksLikeContact('call +49 30 1234 5678')).toBe('phone')
    expect(looksLikeContact('Sam 0171 2345678')).toBe('phone')
  })

  it('ignores gear and times', () => {
    expect(looksLikeContact('2× CDJ-3000, DJM-V10, Funktion-One')).toBeNull()
    expect(looksLikeContact('load-in 18:00-19:30, 4x4 m stage')).toBeNull()
  })
})
