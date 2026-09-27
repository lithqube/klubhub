/**
 * Door search (P2.3, decision P2-D2: runs on the device, never on the
 * server). Pure: no Vue, no storage.
 *
 * - Names are folded: NFKD, diacritics stripped, lower case, special
 *   letters (ø, ß, ł…) spelled out, punctuation → space.
 * - Every query token must match a name token: exact, else prefix (from the
 *   first letter typed), else — for query tokens of 4+ characters — within
 *   one Damerau-Levenshtein edit (typo, missing/extra letter, swap).
 * - Ticket order refs and QR secrets match only exactly (whole query), so a
 *   partial secret never reveals a ticket.
 * - Ranking: exact > prefix > fuzzy, summed over tokens; ties by name.
 */
import type { SubjectKind } from '~/types/door'

const SPECIAL: Record<string, string> = { ł: 'l', đ: 'd', ø: 'o', æ: 'ae', œ: 'oe', ß: 'ss', þ: 'th', ı: 'i', ð: 'd' }

/** Folded, punctuation-free form of s (tokens separated by one space). */
export function normalise(s: string): string {
  return s.normalize('NFKD').replace(/\p{M}/gu, '').toLowerCase()
    .replace(/[łđøæœßþıð]/g, c => SPECIAL[c] ?? c)
    .replace(/[^\p{L}\p{N}]+/gu, ' ').trim()
}

/**
 * Tokens of a name: each word, plus hyphen/apostrophe compounds joined
 * ("Jean-Luc O'Neil" → jean, luc, jeanluc, o, neil, oneil).
 */
export function nameTokens(name: string): string[] {
  const out = new Set<string>()
  for (const word of name.split(/\s+/)) {
    const parts = normalise(word).split(' ').filter(Boolean)
    for (const p of parts) out.add(p)
    if (parts.length > 1) out.add(parts.join(''))
  }
  return [...out]
}

/** Optimal string alignment distance (Damerau-Levenshtein with adjacent swaps), capped at max + 1. */
export function damerau(a: string, b: string, max = 1): number {
  if (Math.abs(a.length - b.length) > max) return max + 1
  const rows = a.length + 1
  const cols = b.length + 1
  const d: number[][] = Array.from({ length: rows }, (_, i) => Array.from({ length: cols }, (_, j) => (i === 0 ? j : j === 0 ? i : 0)))
  for (let i = 1; i < rows; i++) {
    for (let j = 1; j < cols; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1
      let v = Math.min(d[i - 1]![j]! + 1, d[i]![j - 1]! + 1, d[i - 1]![j - 1]! + cost)
      if (i > 1 && j > 1 && a[i - 1] === b[j - 2] && a[i - 2] === b[j - 1]) v = Math.min(v, d[i - 2]![j - 2]! + 1)
      d[i]![j] = v
    }
  }
  return d[a.length]![b.length]!
}

export const FUZZY_MIN = 4

export const TIER = { exact: 3, prefix: 2, fuzzy: 1, none: 0 } as const

/** How well one query token matches one name token (TIER value). */
export function tokenTier(q: string, t: string): number {
  if (q === t) return TIER.exact
  if (t.startsWith(q)) return TIER.prefix
  if (q.length >= FUZZY_MIN) {
    if (damerau(q, t) <= 1) return TIER.fuzzy
    // Still typing a longer name: compare with the start of the token.
    if (t.length > q.length) {
      for (const n of [q.length - 1, q.length, q.length + 1]) {
        if (n > 0 && n <= t.length && damerau(q, t.slice(0, n)) <= 1) return TIER.fuzzy
      }
    }
  }
  return TIER.none
}

export interface SearchEntry {
  kind: SubjectKind
  id: string
  name: string
  /** Tickets: order number and QR secret (exact match only). */
  order_ref?: string
  secret?: string
}

export interface IndexedEntry extends SearchEntry {
  tokens: string[]
  sortKey: string
}

export interface SearchHit {
  entry: SearchEntry
  score: number
  /** Worst token tier (3 = every token exact). */
  tier: number
  /** Matched a ticket's order ref or QR secret exactly. */
  exactRef: boolean
}

export function buildIndex(entries: SearchEntry[]): IndexedEntry[] {
  return entries.map(e => ({ ...e, tokens: nameTokens(e.name), sortKey: normalise(e.name) }))
}

const REF_SCORE = 1000

/** Ranked hits for query q (empty query → none). */
export function searchDoor(index: IndexedEntry[], q: string, limit = 50): SearchHit[] {
  const raw = q.trim()
  if (!raw) return []
  const qTokens = normalise(raw).split(' ').filter(Boolean)
  const lowered = raw.toLowerCase()
  const hits: (SearchHit & { sortKey: string })[] = []
  for (const e of index) {
    const exactRef = (!!e.secret && e.secret === raw) || (!!e.order_ref && e.order_ref.toLowerCase() === lowered)
    if (exactRef) {
      hits.push({ entry: stripIndex(e), score: REF_SCORE, tier: TIER.exact, exactRef: true, sortKey: e.sortKey })
      continue
    }
    if (!qTokens.length) continue
    let score = 0
    let worst: number = TIER.exact
    let ok = true
    for (const qt of qTokens) {
      let best = 0
      for (const t of e.tokens) {
        const tier = tokenTier(qt, t)
        if (tier > best) best = tier
        if (best === TIER.exact) break
      }
      if (!best) {
        ok = false
        break
      }
      score += best
      worst = Math.min(worst, best)
    }
    if (ok) hits.push({ entry: stripIndex(e), score, tier: worst, exactRef: false, sortKey: e.sortKey })
  }
  hits.sort((a, b) => b.score - a.score || b.tier - a.tier || a.sortKey.localeCompare(b.sortKey))
  return hits.slice(0, limit).map(({ sortKey: _s, ...h }) => h)
}

function stripIndex(e: IndexedEntry): SearchEntry {
  const { tokens: _t, sortKey: _s, ...rest } = e
  return rest
}
