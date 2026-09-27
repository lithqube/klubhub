import { describe, expect, it } from 'vitest'
import { buildIndex, damerau, nameTokens, normalise, searchDoor, TIER, tokenTier, type SearchEntry } from '../doorSearch'

const entries: SearchEntry[] = [
  { kind: 'guest', id: 'g1', name: 'Mara Weiss' },
  { kind: 'guest', id: 'g2', name: 'Zoë Lindqvist' },
  { kind: 'guest', id: 'g3', name: 'Emil Sørensen' },
  { kind: 'guest', id: 'g4', name: "Jean-Luc O'Neil" },
  { kind: 'guest', id: 'g5', name: 'Marta Weissmann' },
  { kind: 'guest', id: 'g6', name: 'Ines Duarte' },
  { kind: 'ticket', id: 't1', name: 'Hana Kim', order_ref: 'D-7001', secret: 'DICE-0001' },
  { kind: 'ticket', id: 't2', name: 'Mika Kim', order_ref: 'D-7001', secret: 'DICE-0002' },
]
const index = buildIndex(entries)
const ids = (q: string) => searchDoor(index, q).map(h => h.entry.id)

describe('normalise', () => {
  it('folds accents, case, special letters and punctuation', () => {
    expect(normalise('  Zoë   ÅKESSON ')).toBe('zoe akesson')
    expect(normalise('Sørensen-Łukasz, Straße')).toBe('sorensen lukasz strasse')
    expect(normalise("O'Neil")).toBe('o neil')
    expect(normalise('Æsa')).toBe('aesa')
  })

  it('keeps hyphen and apostrophe compounds as extra tokens', () => {
    expect(nameTokens("Jean-Luc O'Neil").sort()).toEqual(['jean', 'jeanluc', 'luc', 'neil', 'o', 'oneil'].sort())
  })
})

describe('damerau', () => {
  it('counts one edit for a swap, a missing, an extra or a wrong letter', () => {
    expect(damerau('wiess', 'weiss')).toBe(1)
    expect(damerau('weis', 'weiss')).toBe(1)
    expect(damerau('weisss', 'weiss')).toBe(1)
    expect(damerau('weiss', 'weigs')).toBe(1)
    expect(damerau('mara', 'mara')).toBe(0)
  })

  it('caps at max + 1 for distant words', () => {
    expect(damerau('abcd', 'wxyz')).toBeGreaterThan(1)
    expect(damerau('ab', 'abcdef')).toBe(2)
  })
})

describe('tokenTier', () => {
  it('ranks exact over prefix over fuzzy', () => {
    expect(tokenTier('mara', 'mara')).toBe(TIER.exact)
    expect(tokenTier('m', 'mara')).toBe(TIER.prefix)
    expect(tokenTier('amra', 'mara')).toBe(TIER.fuzzy)
    expect(tokenTier('lindqvsit', 'lindqvist')).toBe(TIER.fuzzy)
  })

  it('does not fuzz short tokens', () => {
    expect(tokenTier('amr', 'mar')).toBe(TIER.none)
    expect(tokenTier('xar', 'mara')).toBe(TIER.none)
  })

  it('tolerates a typo in a name still being typed', () => {
    expect(tokenTier('lnidq', 'lindqvist')).toBe(TIER.fuzzy)
  })
})

describe('searchDoor', () => {
  it('matches from the first letter', () => {
    expect(ids('m')).toEqual(expect.arrayContaining(['g1', 'g5', 't2']))
    expect(ids('z')).toEqual(['g2'])
  })

  it('ignores accents both ways', () => {
    expect(ids('zoe')).toEqual(['g2'])
    expect(ids('sörensen')).toEqual(['g3'])
    expect(ids('SORENSEN')).toEqual(['g3'])
  })

  it('finds a name with one typo per token of 4+ letters', () => {
    expect(ids('zoe lindqvsit')).toEqual(['g2'])
    expect(ids('ines duatre')).toEqual(['g6'])
    expect(ids('ines dxxxte')).toEqual([])
  })

  it('needs every query token to match', () => {
    expect(ids('mara kim')).toEqual([])
    expect(ids('jean oneil')).toEqual(['g4'])
    expect(ids('luc')).toEqual(['g4'])
  })

  it('ranks exact over prefix over fuzzy, then by name', () => {
    // "weiss": exact for Mara Weiss, prefix for Marta Weissmann.
    expect(ids('weiss')).toEqual(['g1', 'g5'])
    // "mara": exact Mara, fuzzy Marta (one insertion) — and Marta ranks below.
    const hits = searchDoor(index, 'mara')
    expect(hits[0]!.entry.id).toBe('g1')
    expect(hits[0]!.tier).toBe(TIER.exact)
    expect(hits.find(h => h.entry.id === 'g5')?.tier).toBe(TIER.fuzzy)
  })

  it('matches ticket order refs and secrets only exactly', () => {
    expect(ids('D-7001').sort()).toEqual(['t1', 't2'])
    expect(ids('d-7001').sort()).toEqual(['t1', 't2'])
    expect(ids('DICE-0002')).toEqual(['t2'])
    expect(searchDoor(index, 'DICE-0002')[0]!.exactRef).toBe(true)
    // A partial secret or order ref reveals nothing.
    expect(ids('DICE-000')).toEqual([])
    expect(ids('dice')).toEqual([])
    expect(ids('7001')).toEqual([])
  })

  it('returns nothing for an empty query and honours the limit', () => {
    expect(ids('   ')).toEqual([])
    expect(searchDoor(index, 'm', 2)).toHaveLength(2)
  })
})
