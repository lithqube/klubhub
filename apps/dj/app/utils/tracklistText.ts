import type { Track } from '../types/tracklist'

/** The tracks the card shows: the range (inclusive, only when both ends are set), then the max-tracks cap. */
export function selectCardTracks(
  tracks: Track[],
  rangeStart: number | null,
  rangeEnd: number | null,
  maxTracks: number,
): Track[] {
  let shown = tracks
  if (rangeStart !== null && rangeEnd !== null) shown = shown.slice(rangeStart, rangeEnd + 1)
  return maxTracks > 0 ? shown.slice(0, maxTracks) : [...shown]
}

/** Plain-text tracklist, one `Artist - Title` per line; a hidden gem prints as `ID - ID`. */
export function tracklistText(tracks: Track[], numbered: boolean): string {
  return tracks
    .map((t, i) => {
      const line = t.hiddenGem
        ? 'ID - ID'
        : [t.artist, t.title].map((s) => s.trim()).filter(Boolean).join(' - ')
      return numbered ? `${i + 1}. ${line}`.trimEnd() : line
    })
    .join('\n')
}
