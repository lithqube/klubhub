import { describe, it, expect } from 'vitest'
import {
  applyBps,
  bpsToPercent,
  currencyDigits,
  formatMinor,
  minorToDecimalString,
  parseMoney,
  percentToBps,
} from '../money'

describe('currencyDigits', () => {
  it('knows 2-digit and 0-digit currencies', () => {
    expect(currencyDigits('EUR')).toBe(2)
    expect(currencyDigits('JPY')).toBe(0)
  })
  it('falls back to 2 for an invalid code', () => {
    expect(currencyDigits('??')).toBe(2)
  })
})

describe('parseMoney', () => {
  it.each([
    ['1500', 150000],
    ['1500.5', 150050],
    ['1500,50', 150050],
    ['1,500.50', 150050],
    ['1.500,50', 150050],
    ['1,500', 150000],
    ['1.234.567', 123456700],
    ['€ 80', 8000],
    ['0.01', 1],
    ['12.', 1200],
    ['.5', 50],
    // three digits after a lone separator read as a thousands group
    ['1.234', 123400],
    ['10.001', 1000100],
  ])('parses %s as %i minor units', (input, expected) => {
    expect(parseMoney(input, 'EUR')).toBe(expected)
  })

  it('does not use float math (0.1 + 0.2 style values stay exact)', () => {
    expect(parseMoney('0.29', 'EUR')).toBe(29)
    expect(parseMoney('1.15', 'EUR')).toBe(115)
    expect(parseMoney('4.35', 'EUR')).toBe(435)
  })

  it.each(['', '-5', 'abc', '1,2,3', '10.0012', '1.2.3,4,5', '1,50.5'])('rejects %s', (input) => {
    expect(parseMoney(input, 'EUR')).toBeNull()
  })

  it('respects zero-decimal currencies', () => {
    expect(parseMoney('1500', 'JPY')).toBe(1500)
    expect(parseMoney('1,500', 'JPY')).toBe(1500)
    expect(parseMoney('15.5', 'JPY')).toBeNull()
  })

  it('rejects absurdly long numbers', () => {
    expect(parseMoney('9'.repeat(20), 'EUR')).toBeNull()
  })

  it('handles null/undefined', () => {
    expect(parseMoney(null, 'EUR')).toBeNull()
    expect(parseMoney(undefined, 'EUR')).toBeNull()
  })
})

describe('minorToDecimalString', () => {
  it('splits minor units exactly', () => {
    expect(minorToDecimalString(123456, 'EUR')).toBe('1234.56')
    expect(minorToDecimalString(5, 'EUR')).toBe('0.05')
    expect(minorToDecimalString(-150, 'EUR')).toBe('-1.50')
    expect(minorToDecimalString(1500, 'JPY')).toBe('1500')
  })
})

describe('formatMinor', () => {
  it('formats with the currency symbol and digits', () => {
    expect(formatMinor(150000, 'EUR')).toBe('€1,500.00')
    expect(formatMinor(99, 'USD')).toBe('US$0.99')
    expect(formatMinor(1500, 'JPY')).toBe('JP¥1,500')
  })
  it('formats negatives (credit notes)', () => {
    expect(formatMinor(-150000, 'EUR')).toBe('-€1,500.00')
  })
  it('treats non-finite values as zero', () => {
    expect(formatMinor(Number.NaN, 'EUR')).toBe('€0.00')
    expect(formatMinor(null, 'EUR')).toBe('€0.00')
  })
  it('honours locale', () => {
    expect(formatMinor(150000, 'EUR', { locale: 'de-DE' })).toBe('1.500,00 €')
  })
})

describe('percent / basis points', () => {
  it('converts bps to percent text', () => {
    expect(bpsToPercent(1900)).toBe('19')
    expect(bpsToPercent(1950)).toBe('19.5')
    expect(bpsToPercent(725)).toBe('7.25')
    expect(bpsToPercent(5)).toBe('0.05')
    expect(bpsToPercent(0)).toBe('0')
  })
  it('parses percent text into bps', () => {
    expect(percentToBps('19')).toBe(1900)
    expect(percentToBps('7,5')).toBe(750)
    expect(percentToBps('7.25 %')).toBe(725)
    expect(percentToBps('0')).toBe(0)
    expect(percentToBps('')).toBeNull()
    expect(percentToBps('-1')).toBeNull()
    expect(percentToBps('1.234')).toBeNull()
  })
  it('applies bps with half-up rounding', () => {
    expect(applyBps(150000, 1900)).toBe(28500)
    expect(applyBps(1005, 1500)).toBe(151) // 150.75 → 151
    expect(applyBps(1003, 1500)).toBe(150) // 150.45 → 150
  })
})
