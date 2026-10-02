export interface Tracklist {
  id: string;
  title: string;
  sourceFormat: string;
  rawFilePath: string;
  preset: string;
  visibleFields: string[];
  bgMode: 'solid' | 'upload' | 'mosaic';
  bgValue: string;
  maxTracks: number;
  trackRangeStart: number;
  trackRangeEnd: number;
  createdAt: string;
  updatedAt: string;
}

export interface Track {
  id: string;
  tracklistId: string;
  position: number;
  title: string;
  artist: string;
  album: string;
  genre: string;
  bpm: number;
  rating: number;
  durationSecs: number;
  musicalKey: string;
  dateAdded: string;
  artworkStatus: 'pending' | 'fetched' | 'placeholder' | 'manual';
  artworkUrl: string;
  artworkSource: string;
  // Markers shown on the exported card. Optional: tracklists saved before they
  // existed, and the static dev mocks, do not send them.
  hiddenGem?: boolean;
  unreleased?: boolean;
  media?: TrackMedia;
}

/** What a track was played from: vinyl or digital. '' = not set. */
export type TrackMedia = '' | 'vinyl' | 'digital';

export const TRACK_MEDIA: ReadonlyArray<{ value: Exclude<TrackMedia, ''>; label: string }> = [
  { value: 'vinyl', label: 'VINYL' },
  { value: 'digital', label: 'DIGITAL' },
];

export const isDigitalMedia = (m: TrackMedia | undefined): boolean => m === 'digital';

export interface ParseWarning {
  row: number;
  field: string;
  message: string;
}
