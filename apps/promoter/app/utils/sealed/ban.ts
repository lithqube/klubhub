/**
 * Ban list entries (P2.6): JSON {name, email?, reason, note?} encrypted with
 * AES-256-GCM under the OSK, AAD "klubhub-ban|<tenant>|<entry id>|<key
 * version>", stored as entry_sealed = 0x01 ‖ nonce ‖ ct+tag. Plus the
 * form rules (name and reason required, expiry presets, custom ≤ 3 years)
 * and the search. Pure, no Vue.
 */
import type { BanEntry, BanPlain } from '~/types/sealed'
import { normalise } from '~/utils/doorSearch'
import { b64url, fromB64url, fromUtf8, utf8 } from './bytes'
import { banAad } from './keys'
import { aesOpen, aesSeal, SealedError } from './seal'

export const NAME_MAX = 120
export const REASON_MAX = 500
export const NOTE_MAX = 1000
export const EMAIL_MAX = 254

const clean = (s: string | undefined) => (s ?? '').replace(/\s+/g, ' ').trim()

/** Trim and drop empty optionals (so the ciphertext carries nothing extra). */
export function tidyPlain(p: BanPlain): BanPlain {
  const out: BanPlain = { name: clean(p.name), reason: (p.reason ?? '').trim() }
  const email = clean(p.email).toLowerCase()
  const note = (p.note ?? '').trim()
  if (email) out.email = email
  if (note) out.note = note
  return out
}

export type BanField = 'name' | 'reason' | 'email' | 'note' | 'expires_at'

/** Field errors for the form (empty object = valid). */
export function validatePlain(p: BanPlain): Partial<Record<BanField, string>> {
  const t = tidyPlain(p)
  const e: Partial<Record<BanField, string>> = {}
  if (!t.name) e.name = 'Enter the person\'s name.'
  else if (t.name.length > NAME_MAX) e.name = `At most ${NAME_MAX} characters.`
  if (!t.reason) e.reason = 'Say why, so a manager at the door can decide.'
  else if (t.reason.length > REASON_MAX) e.reason = `At most ${REASON_MAX} characters.`
  if (t.email && (t.email.length > EMAIL_MAX || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(t.email))) e.email = 'Enter a valid email, or leave it empty.'
  if (t.note && t.note.length > NOTE_MAX) e.note = `At most ${NOTE_MAX} characters.`
  return e
}

export async function encryptBan(osk: Uint8Array, tenant: string, id: string, version: number, p: BanPlain): Promise<string> {
  return b64url(await aesSeal(osk, utf8(JSON.stringify(tidyPlain(p))), banAad(tenant, id, version)))
}

/** Decrypt an entry; throws SealedError when it does not open (wrong key, version, id or tampered). */
export async function decryptBan(osk: Uint8Array, tenant: string, id: string, version: number, sealed: string): Promise<BanPlain> {
  let blob: Uint8Array
  try {
    blob = fromB64url(sealed)
  } catch {
    throw new SealedError('malformed')
  }
  const pt = await aesOpen(osk, blob, banAad(tenant, id, version))
  let v: unknown
  try {
    v = JSON.parse(fromUtf8(pt))
  } catch {
    throw new SealedError('malformed')
  }
  const o = v as Partial<BanPlain>
  if (!o || typeof o.name !== 'string' || typeof o.reason !== 'string') throw new SealedError('malformed')
  return tidyPlain({ name: o.name, reason: o.reason, email: typeof o.email === 'string' ? o.email : undefined, note: typeof o.note === 'string' ? o.note : undefined })
}

// ---------------------------------------------------------------- expiry

const DAY = 86_400_000
export type ExpiryPreset = '30d' | '6m' | '1y' | 'custom'

export const EXPIRY_PRESETS: { id: Exclude<ExpiryPreset, 'custom'>, label: string }[] = [
  { id: '30d', label: '30 DAYS' },
  { id: '6m', label: '6 MONTHS' },
  { id: '1y', label: '1 YEAR' },
]

function addMonths(d: Date, n: number): Date {
  const out = new Date(d)
  const day = out.getUTCDate()
  out.setUTCDate(1)
  out.setUTCMonth(out.getUTCMonth() + n)
  const last = new Date(Date.UTC(out.getUTCFullYear(), out.getUTCMonth() + 1, 0)).getUTCDate()
  out.setUTCDate(Math.min(day, last))
  return out
}

/** The expiry instant for a preset, counted from now. */
export function presetExpiry(p: Exclude<ExpiryPreset, 'custom'>, now = Date.now()): string {
  const d = new Date(now)
  if (p === '30d') return new Date(now + 30 * DAY).toISOString()
  return addMonths(d, p === '6m' ? 6 : 12).toISOString()
}

/** YYYY-MM-DD in UTC (the date input's value). */
export const dateInput = (d: Date) => d.toISOString().slice(0, 10)

/** The last date a custom expiry may pick (3 years ahead, a day under the server's limit). */
export const maxExpiryDate = (now = Date.now()) => dateInput(new Date(addMonths(new Date(now), 36).getTime() - DAY))
/** The first date a custom expiry may pick (the server wants at least a day from now). */
export const minExpiryDate = (now = Date.now()) => dateInput(new Date(now + 2 * DAY))

/**
 * A custom date (YYYY-MM-DD; the entry expires at the end of that day,
 * UTC) → ISO, or why not: from the day after tomorrow to 3 years ahead.
 */
export function customExpiry(date: string, now = Date.now()): { at: string, error: null } | { at: null, error: string } {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || !Number.isFinite(Date.parse(`${date}T23:59:00Z`))) return { at: null, error: 'Pick a date.' }
  if (date < minExpiryDate(now)) return { at: null, error: 'Pick a later date: an entry lasts at least a day.' }
  if (date > maxExpiryDate(now)) return { at: null, error: 'An entry can last at most 3 years.' }
  return { at: new Date(Date.parse(`${date}T23:59:00Z`)).toISOString(), error: null }
}

/** Which preset an existing expiry is closest to (for editing): always custom, prefilled. */
export function expiryDate(iso: string): string {
  return iso.slice(0, 10)
}

// ---------------------------------------------------------------- search

/** Entries whose name or email contains every typed token (accent-folded, prefix per token). */
export function searchBan(entries: BanEntry[], q: string): BanEntry[] {
  const tokens = normalise(q).split(' ').filter(Boolean)
  if (!tokens.length) return entries
  return entries.filter((e) => {
    if (!e.plain) return false
    const hay = normalise(`${e.plain.name} ${e.plain.email ?? ''}`).split(' ')
    return tokens.every(t => hay.some(h => h.startsWith(t)))
  })
}

/** Readable entries first by name, unreadable ones last. */
export function sortBan(entries: BanEntry[]): BanEntry[] {
  return [...entries].sort((a, b) => {
    if (!a.plain || !b.plain) return a.plain ? -1 : b.plain ? 1 : 0
    return normalise(a.plain.name).localeCompare(normalise(b.plain.name))
  })
}
