/**
 * Heuristic for contact details typed into unencrypted fields (tech notes).
 * A hint, not a guarantee: it nudges people to use the protected fields.
 */
const EMAIL = /[^\s@]+@[^\s@]+\.[a-z]{2,}/i
const PHONE = /(?:\+|\b00)?\d[\d\s().-]{7,}\d/

export function looksLikeContact(text: string): 'email' | 'phone' | null {
  if (EMAIL.test(text)) return 'email'
  // Ignore long runs that are plainly model numbers or times ("CDJ-3000", "22:00-06:00").
  const m = text.match(PHONE)
  if (m && m[0].replace(/\D/g, '').length >= 9) return 'phone'
  return null
}
