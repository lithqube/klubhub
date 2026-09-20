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
}

export interface ParseWarning {
  row: number;
  field: string;
  message: string;
}
