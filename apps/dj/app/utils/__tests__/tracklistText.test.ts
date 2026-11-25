import { describe, expect, it } from 'vitest'
import { selectCardTracks, tracklistText } from '../tracklistText'
import type { Track } from '../../types/tracklist'

const t = (title: string, artist: string, extra: Partial<Track> = {}): Track => ({
  id: title, tracklistId: 'tl', position: 0, title, artist, album: '', genre: '',
  bpm: 0, rating: 0, durationSecs: 0, musicalKey: '', dateAdded: '', artworkStatus: 'placeholder',
  artworkUrl: '', artworkSource: '', ...extra,
})

describe('tracklistText', () => {
  const set = [t('Strings of Life', 'Rhythim Is Rhythim'), t('Energy Flash', 'Joey Beltram')]

  it('prints one Artist - Title line per track, in set order', () => {
    expect(tracklistText(set, false)).toBe('Rhythim Is Rhythim - Strings of Life\nJoey Beltram - Energy Flash')
  })

  it('numbers the lines from 1', () => {
    expect(tracklistText(set, true)).toBe('1. Rhythim Is Rhythim - Strings of Life\n2. Joey Beltram - Energy Flash')
  })

  it('leaves no stray separator when the artist or title is missing', () => {
    const gaps = [t('Untitled', ''), t('', 'Anon'), t(' ', ' ')]
    expect(tracklistText(gaps, false)).toBe('Untitled\nAnon\n')
    expect(tracklistText(gaps, true)).toBe('1. Untitled\n2. Anon\n3.')
  })

  it('prints a hidden gem as ID - ID in both formats and never its name', () => {
    const gem = [t('Secret Weapon', 'Dubplate Guy', { hiddenGem: true })]
    expect(tracklistText(gem, false)).toBe('ID - ID')
    expect(tracklistText(gem, true)).toBe('1. ID - ID')
    expect(tracklistText(gem, true)).not.toMatch(/Secret|Dubplate/)
  })
})

describe('selectCardTracks', () => {
  const ten = Array.from({ length: 10 }, (_, i) => t(`T${i}`, 'A'))
  const titles = (ts: Track[]) => ts.map((x) => x.title)

  it('takes the range (inclusive) first, then caps at max tracks', () => {
    expect(titles(selectCardTracks(ten, 2, 6, 3))).toEqual(['T2', 'T3', 'T4'])
  })

  it('includes the range end track when no max is set', () => {
    expect(titles(selectCardTracks(ten, 2, 4, 0))).toEqual(['T2', 'T3', 'T4'])
  })

  it('ignores a half-set range and a max of 0', () => {
    expect(selectCardTracks(ten, 2, null, 0)).toHaveLength(10)
    expect(titles(selectCardTracks(ten, null, null, 2))).toEqual(['T0', 'T1'])
  })
})
