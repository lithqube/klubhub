export interface Tracklist {
  id: string;
  title: string;
  sourceFormat: string;
  rawFilePath: string | null;
  preset: string;
  visibleFields: string[];
  bgMode: 'solid' | 'upload' | 'mosaic';
  bgValue: string | null;
  maxTracks: number;
  trackRangeStart: number | null;
  trackRangeEnd: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface Track {
  id: string;
  tracklistId: string;
  position: number;
  title: string;
  artist: string | null;
  album: string | null;
  genre: string | null;
  bpm: number | null;
  rating: number | null;
  durationSecs: number | null;
  musicalKey: string | null;
  dateAdded: string | null;
  artworkStatus: 'pending' | 'fetched' | 'placeholder' | 'manual';
  artworkUrl: string | null;
}

export interface ParseWarning {
  row: number;
  field: string;
  message: string;
}
