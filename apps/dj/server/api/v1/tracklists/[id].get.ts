const MOCK_TRACKS: Record<string, any[]> = {
  'tl-001': [
    { id: 'tr-001', tracklistId: 'tl-001', position: 1,  title: 'Parallel Universe',         artist: 'Alignment',        album: null, genre: 'Techno', bpm: 138, rating: 5, durationSecs: 421, musicalKey: '8A', dateAdded: '2026-01-10', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-002', tracklistId: 'tl-001', position: 2,  title: 'Monolith',                   artist: 'Rebekah',          album: null, genre: 'Techno', bpm: 142, rating: 5, durationSecs: 507, musicalKey: '2A', dateAdded: '2025-11-04', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-003', tracklistId: 'tl-001', position: 3,  title: 'Digital Ceremony',           artist: 'Surgeon',          album: null, genre: 'Techno', bpm: 145, rating: 4, durationSecs: 388, musicalKey: '5A', dateAdded: '2025-12-20', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-004', tracklistId: 'tl-001', position: 4,  title: 'Covenant',                   artist: 'Ancient Methods',  album: null, genre: 'Techno', bpm: 147, rating: 5, durationSecs: 612, musicalKey: '9B', dateAdded: '2026-01-22', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-005', tracklistId: 'tl-001', position: 5,  title: 'Soma',                       artist: 'Blawan',           album: null, genre: 'Techno', bpm: 140, rating: 4, durationSecs: 445, musicalKey: '11A', dateAdded: '2025-10-14', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-006', tracklistId: 'tl-001', position: 6,  title: 'Subjugation Protocol',       artist: 'Drumcell',        album: null, genre: 'Techno', bpm: 144, rating: 4, durationSecs: 523, musicalKey: '4A', dateAdded: '2026-02-07', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-007', tracklistId: 'tl-001', position: 7,  title: 'Phantom Limb',               artist: 'Phase',           album: null, genre: 'Techno', bpm: 139, rating: 5, durationSecs: 468, musicalKey: '7A', dateAdded: '2026-03-01', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-008', tracklistId: 'tl-001', position: 8,  title: 'Neural Oscillation',         artist: 'Orphx',           album: null, genre: 'Techno', bpm: 148, rating: 3, durationSecs: 397, musicalKey: '1A', dateAdded: '2025-09-18', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-009', tracklistId: 'tl-001', position: 9,  title: 'Cassini Division',           artist: 'Shifted',         album: null, genre: 'Techno', bpm: 141, rating: 5, durationSecs: 554, musicalKey: '3B', dateAdded: '2026-01-30', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-010', tracklistId: 'tl-001', position: 10, title: 'Persistence of Vision',      artist: 'Headless Horseman',album: null, genre: 'Techno', bpm: 145, rating: 4, durationSecs: 489, musicalKey: '6A', dateAdded: '2026-02-14', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-011', tracklistId: 'tl-001', position: 11, title: 'Signal Loss',                artist: 'Varg',            album: null, genre: 'Techno', bpm: 143, rating: 4, durationSecs: 412, musicalKey: '10B', dateAdded: '2025-12-03', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-012', tracklistId: 'tl-001', position: 12, title: 'Black Mirror Protocol',      artist: 'Dj Pete',         album: null, genre: 'Techno', bpm: 138, rating: 5, durationSecs: 601, musicalKey: '2B', dateAdded: '2026-03-10', artworkStatus: 'placeholder', artworkUrl: null },
  ],
  'tl-002': [
    { id: 'tr-101', tracklistId: 'tl-002', position: 1,  title: 'Sleeparchive 01',            artist: 'Sleeparchive',    album: null, genre: 'Techno', bpm: 138, rating: 5, durationSecs: 520, musicalKey: '5A', dateAdded: '2025-08-12', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-102', tracklistId: 'tl-002', position: 2,  title: 'Interzone',                  artist: 'Perc',            album: null, genre: 'Techno', bpm: 135, rating: 5, durationSecs: 443, musicalKey: '8A', dateAdded: '2025-10-22', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-103', tracklistId: 'tl-002', position: 3,  title: 'Acid Rain',                  artist: 'Paula Temple',    album: null, genre: 'Techno', bpm: 137, rating: 4, durationSecs: 388, musicalKey: '11B', dateAdded: '2025-11-30', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-104', tracklistId: 'tl-002', position: 4,  title: 'Tresor',                     artist: 'Surgeon',         album: null, genre: 'Techno', bpm: 141, rating: 5, durationSecs: 567, musicalKey: '3A', dateAdded: '2026-01-07', artworkStatus: 'placeholder', artworkUrl: null },
    { id: 'tr-105', tracklistId: 'tl-002', position: 5,  title: 'Collapse',                   artist: 'Blawan',          album: null, genre: 'Techno', bpm: 140, rating: 4, durationSecs: 492, musicalKey: '6B', dateAdded: '2026-02-11', artworkStatus: 'placeholder', artworkUrl: null },
  ],
}

const MOCK_TRACKLISTS: Record<string, any> = {
  'tl-001': { id: 'tl-001', title: 'TECHNO RITUAL VOL. 4', sourceFormat: 'rekordbox', rawFilePath: null, preset: 'dark', visibleFields: ['title', 'artist', 'bpm', 'key', 'durationSecs'], bgMode: 'solid', bgValue: null, maxTracks: 47, trackRangeStart: null, trackRangeEnd: null, createdAt: '2026-03-29T22:00:00Z', updatedAt: '2026-03-29T23:45:00Z' },
  'tl-002': { id: 'tl-002', title: 'CLUB ALPHA CLOSING SET', sourceFormat: 'rekordbox', rawFilePath: null, preset: 'dark', visibleFields: ['title', 'artist', 'bpm', 'key', 'durationSecs'], bgMode: 'solid', bgValue: null, maxTracks: 31, trackRangeStart: null, trackRangeEnd: null, createdAt: '2026-03-08T02:00:00Z', updatedAt: '2026-03-08T04:30:00Z' },
  'tl-003': { id: 'tl-003', title: 'STUDIO WARMUP SESSION', sourceFormat: 'serato', rawFilePath: null, preset: 'dark', visibleFields: ['title', 'artist', 'bpm', 'key', 'durationSecs'], bgMode: 'solid', bgValue: null, maxTracks: 18, trackRangeStart: null, trackRangeEnd: null, createdAt: '2026-03-02T14:00:00Z', updatedAt: '2026-03-02T16:00:00Z' },
  'tl-004': { id: 'tl-004', title: 'GAMMA HALL ROOM 1 PREP', sourceFormat: 'traktor', rawFilePath: null, preset: 'dark', visibleFields: ['title', 'artist', 'bpm', 'key', 'durationSecs'], bgMode: 'solid', bgValue: null, maxTracks: 60, trackRangeStart: null, trackRangeEnd: null, createdAt: '2026-02-18T10:00:00Z', updatedAt: '2026-02-18T11:30:00Z' },
}

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  const tracklist = MOCK_TRACKLISTS[id!]
  if (!tracklist) {
    throw createError({ statusCode: 404, message: 'Tracklist not found' })
  }
  const tracks = MOCK_TRACKS[id!] ?? []
  return { tracklist, tracks }
})
