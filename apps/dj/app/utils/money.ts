// Money helpers. Amounts live as integer minor units everywhere; user input
// is parsed with string arithmetic so no float ever touches a stored value.

const DEFAULT_LOCALE = 'en-GB'

/** Number of minor-unit digits for a currency (EUR 2, JPY 0, ...). */
export function currencyDigits(currency: string): number {
  try {
    return new Intl.NumberFormat('en', { style: 'currency', currency: currency || 'EUR' })
      .resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

/** Currency symbol for a currency code, e.g. "€" for EUR. */
export function currencySymbol(currency: string, locale = DEFAULT_LOCALE): string {
  try {
    const part = new Intl.NumberFormat(locale, { style: 'currency', currency: currency || 'EUR' })
      .formatToParts(0)
      .find((p) => p.type === 'currency')
    return part?.value ?? currency
  } catch {
    return currency
  }
}

// Longest digit string we accept (keeps us far below 2^53).
const MAX_DIGITS = 15

/**
 * Parses a user-typed amount in major units ("1,234.50", "1234,5", "€ 80")
 * into integer minor units. Returns null when the text is not an amount,
 * is negative or has more decimals than the currency allows.
 *
 * Separator rule: when both `.` and `,` appear the last one is the decimal
 * separator. When only one kind appears once and is followed by no more
 * than the currency's decimals, it is the decimal separator; otherwise it
 * is a thousands separator.
 */
export function parseMoney(input: string | number | null | undefined, currency: string): number | null {
  if (input === null || input === undefined) return null
  let s = String(input).trim()
  if (s === '') return null
  // Strip currency symbols/codes and all kinds of spaces.
  s = s.replace(/[A-Za-z€$£¥₹₩₽¤\s\u00a0\u202f']/g, '')
  if (s === '' || s.startsWith('-')) return null
  if (!/^[0-9.,]+$/.test(s)) return null

  const digits = currencyDigits(currency)
  const lastDot = s.lastIndexOf('.')
  const lastComma = s.lastIndexOf(',')
  let intPart = s
  let fracPart = ''

  if (lastDot !== -1 && lastComma !== -1) {
    const decIdx = Math.max(lastDot, lastComma)
    intPart = s.slice(0, decIdx)
    fracPart = s.slice(decIdx + 1)
    const decChar = s.charAt(decIdx)
    // The other separator must not appear after the decimal one.
    if (fracPart.includes('.') || fracPart.includes(',')) return null
    const thousandsChar = decChar === '.' ? ',' : '.'
    if (intPart.includes(decChar)) return null
    const groups = intPart.split(thousandsChar)
    if (groups.slice(1).some((g) => g.length !== 3)) return null
    intPart = groups.join('')
  } else if (lastDot !== -1 || lastComma !== -1) {
    const sep = lastDot !== -1 ? '.' : ','
    const parts = s.split(sep)
    const tail = parts[parts.length - 1] ?? ''
    if (parts.length === 2 && tail.length > 0 && tail.length <= digits) {
      intPart = parts[0] ?? ''
      fracPart = tail
    } else if (parts.length === 2 && tail.length === 0) {
      // "12." → 12
      intPart = parts[0] ?? ''
    } else {
      // Thousands groups: every group after the first must be 3 digits.
      if (parts.slice(1).some((p) => p.length !== 3)) return null
      intPart = parts.join('')
    }
  }

  if (intPart === '') intPart = '0'
  if (!/^\d+$/.test(intPart) || (fracPart !== '' && !/^\d+$/.test(fracPart))) return null
  if (fracPart.length > digits) return null

  const minorStr = (intPart + fracPart.padEnd(digits, '0')).replace(/^0+(?=\d)/, '')
  if (minorStr.length > MAX_DIGITS) return null
  return Number(minorStr)
}

/** Splits minor units into a plain decimal string ("123456" → "1234.56"). */
export function minorToDecimalString(minor: number, currency: string): string {
  const digits = currencyDigits(currency)
  const negative = minor < 0
  const abs = String(Math.abs(Math.trunc(minor)))
  if (digits === 0) return (negative ? '-' : '') + abs
  const padded = abs.padStart(digits + 1, '0')
  const major = padded.slice(0, padded.length - digits)
  const frac = padded.slice(padded.length - digits)
  return `${negative ? '-' : ''}${major}.${frac}`
}

/** Formats minor units for display, e.g. 150000 EUR → "€1,500.00". */
export function formatMinor(
  minor: number | null | undefined,
  currency: string,
  opts: { locale?: string; signDisplay?: 'auto' | 'always' | 'exceptZero' | 'never' } = {},
): string {
  const value = Number.isFinite(minor) ? (minor as number) : 0
  const cur = currency || 'EUR'
  const digits = currencyDigits(cur)
  // The decimal string keeps the digits exact; Number() of a short decimal
  // string round-trips exactly for display at `digits` places.
  const asNumber = Number(minorToDecimalString(value, cur))
  try {
    return new Intl.NumberFormat(opts.locale ?? DEFAULT_LOCALE, {
      style: 'currency',
      currency: cur,
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
      signDisplay: opts.signDisplay ?? 'auto',
    }).format(asNumber)
  } catch {
    return `${minorToDecimalString(value, cur)} ${cur}`
  }
}

/** Basis points → percent string without trailing zeros (1950 → "19.5"). */
export function bpsToPercent(bps: number | null | undefined): string {
  const v = Math.trunc(Number(bps) || 0)
  const negative = v < 0
  const abs = String(Math.abs(v)).padStart(3, '0')
  const major = abs.slice(0, -2).replace(/^0+(?=\d)/, '')
  const frac = abs.slice(-2).replace(/0+$/, '')
  return `${negative ? '-' : ''}${major}${frac ? '.' + frac : ''}`
}

/**
 * Percent text → basis points with string math ("19" → 1900, "7,5" → 750).
 * Returns null for anything that is not a non-negative number with at most
 * two decimals.
 */
export function percentToBps(input: string | number | null | undefined): number | null {
  if (input === null || input === undefined) return null
  const s = String(input).trim().replace('%', '').trim().replace(',', '.')
  if (s === '') return null
  const m = /^(\d{1,5})(?:\.(\d{0,2}))?$/.exec(s)
  if (!m) return null
  const intPart = m[1] ?? '0'
  const frac = (m[2] ?? '').padEnd(2, '0')
  return Number(intPart) * 100 + Number(frac)
}

/** Applies a basis-point rate to an amount, rounding half away from zero. */
export function applyBps(minor: number, bps: number): number {
  // Integer math: minor * bps fits comfortably below 2^53 for real fees.
  const product = minor * bps
  const q = Math.trunc(product / 10000)
  const r = product - q * 10000
  if (Math.abs(r) * 2 >= 10000) return q + Math.sign(product)
  return q
}
