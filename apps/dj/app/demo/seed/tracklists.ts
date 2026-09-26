// Fictional tracklists for the demo. Artist names are deliberately
// placeholder-like so none can be mistaken for a real act.

import type { Track, Tracklist } from '../../types/tracklist'
import { coverDataUrl } from '../lib/art'
import { daysFrom } from '../lib/util'

const ARTISTS = [
  'Artist Alpha', 'Artist Beta', 'Artist Gamma', 'Artist Delta', 'Sample Collective',
  'Demo Unit', 'Example Duo', 'Placeholder Trio', 'Mock Ensemble', 'Lorem Ipsum Sound System',
]

const TITLES = [
  'Concrete Echo', 'Low Orbit', 'Signal Path', 'Fog Machine', 'Second Room', 'Late Checkout',
  'Empty Platform', 'Blue Hour Sketch', 'Loop Theory', 'Soft Reset', 'Afterglow Protocol',
  'Night Bus Home', 'Warm Static', 'Floor Plan', 'Paper Moon Dub', 'Hall Pressure',
  'Glass Corridor', 'Slow Burner', 'Open Window', 'Last Call Edit',
]

const KEYS = ['8A', '9A', '10A', '11A', '12A', '1A', '2A', '3A', '4B', '5B', '6B', '7B']

export const DEMO_TRACKLIST_IDS = {
  warehouse: '00000000-0000-4000-8000-000000000301',
  openAir: '00000000-0000-4000-8000-000000000302',
  resident: '00000000-0000-4000-8000-000000000303',
} as const

interface Spec {
  id: string
  title: string
  sourceFormat: string
  daysAgo: number
  count: number
  offset: number
  bpmFrom: number
  genre: string
}

const SPECS: Spec[] = [
  { id: DEMO_TRACKLIST_IDS.warehouse, title: 'WAREHOUSE SESSION 03', sourceFormat: 'rekordbox', daysAgo: 12, count: 14, offset: 0, bpmFrom: 132, genre: 'Techno' },
  { id: DEMO_TRACKLIST_IDS.openAir, title: 'OPEN AIR SUNSET SET', sourceFormat: 'serato', daysAgo: 4, count: 10, offset: 7, bpmFrom: 118, genre: 'House' },
  { id: DEMO_TRACKLIST_IDS.resident, title: 'RESIDENT NIGHT WARM-UP', sourceFormat: 'traktor', daysAgo: 47, count: 12, offset: 3, bpmFrom: 124, genre: 'Minimal' },
]

export const DEFAULT_VISIBLE_FIELDS = ['title', 'artist', 'bpm', 'key', 'durationSecs']

export function newTracklist(id: string, title: string, sourceFormat: string, createdAt: string, count: number): Tracklist {
  return {
    id, title, sourceFormat, rawFilePath: '', preset: 'default', visibleFields: [...DEFAULT_VISIBLE_FIELDS],
    bgMode: 'solid', bgValue: '', maxTracks: Math.max(count, 1), trackRangeStart: 0, trackRangeEnd: 0,
    createdAt, updatedAt: createdAt,
  }
}

export function seedTracklists(now: Date): { tracklists: Tracklist[]; tracks: Record<string, Track[]> } {
  const tracklists: Tracklist[] = []
  const tracks: Record<string, Track[]> = {}
  for (const s of SPECS) {
    const created = daysFrom(now, -s.daysAgo, 23, 30)
    tracklists.push(newTracklist(s.id, s.title, s.sourceFormat, created, s.count))
    tracks[s.id] = Array.from({ length: s.count }, (_, i) => {
      const n = s.offset + i
      const title = TITLES[n % TITLES.length]!
      const artist = ARTISTS[(n * 3) % ARTISTS.length]!
      return {
        id: `00000000-0000-4000-8001-${s.id.slice(-3)}${String(i + 1).padStart(9, '0')}`,
        tracklistId: s.id,
        position: i + 1,
        title,
        artist,
        album: '',
        genre: s.genre,
        bpm: s.bpmFrom + (i % 5),
        rating: 3 + (n % 3),
        durationSecs: 300 + ((n * 37) % 240),
        musicalKey: KEYS[n % KEYS.length]!,
        dateAdded: daysFrom(now, -(60 + n)),
        artworkStatus: 'fetched',
        artworkUrl: coverDataUrl(`${artist}|${title}`),
        artworkSource: 'demo',
      }
    })
  }
  return { tracklists: tracklists.sort((a, b) => b.createdAt.localeCompare(a.createdAt)), tracks }
}

/** Sample rekordbox-style TXT for the "Use sample file" button. */
export const SAMPLE_TRACKLIST_TXT = [
  '#\tTrack Title\tArtist\tGenre\tBPM\tTime\tKey',
  ...TITLES.slice(8, 20).map((t, i) =>
    [i + 1, t, ARTISTS[(i * 7) % ARTISTS.length], 'Techno', 126 + i, `${5 + (i % 3)}:${String(10 + i * 3).padStart(2, '0')}`, KEYS[i % KEYS.length]].join('\t'),
  ),
].join('\n')
