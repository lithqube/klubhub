import type { Tracklist, Track, ParseWarning } from '../types/tracklist';

// Define response interfaces for API calls
interface UploadTracklistResponse {
  tracklist: Tracklist;
  tracks: Track[];
  warnings: ParseWarning[];
  error?: string;
  message?: string;
}

interface TracklistResponse {
  tracklists: Tracklist[];
  error?: string;
  message?: string;
}

interface TracklistDetailResponse {
  tracklist: Tracklist;
  tracks: Track[];
  error?: string;
  message?: string;
}

interface TrackResponse {
  track: Track;
  error?: string;
  message?: string;
}

interface ArtworkResponse {
  artwork_url: string;
  artwork_source: 'manual';
  artwork_status: 'manual';
  error?: string;
  message?: string;
}

// Upload a tracklist file (TXT format)
export function uploadTracklist(file: File): Promise<{
  tracklist: Tracklist;
  tracks: Track[];
  warnings: ParseWarning[];
}> {
  const formData = new FormData();
  formData.append('file', file);

  return $fetch(`/api/v1/tracklists/upload`, {
    method: 'POST',
    body: formData,
  }).then((res) => {
    const response = res as UploadTracklistResponse;
    if (response.error) throw new Error(response.message ?? response.error);
    return response;
  });
}

// List all tracklists
export function listTracklists(): Promise<Tracklist[]> {
  return $fetch(`/api/v1/tracklists`).then((res) => {
    const response = res as TracklistResponse;
    if (response.error) throw new Error(response.message ?? response.error);
    return response.tracklists;
  });
}

// Get a single tracklist by ID
export function getTracklist(id: string): Promise<{
  tracklist: Tracklist;
  tracks: Track[];
}> {
  return $fetch(`/api/v1/tracklists/${id}`).then((res) => {
    const response = res as TracklistDetailResponse;
    if (response.error) throw new Error(response.message ?? response.error);
    return response;
  });
}

// Update a track
export function updateTrack(
  tracklistId: string,
  trackId: string,
  fields: Partial<
    Pick<Track, 'title' | 'artist' | 'bpm' | 'musicalKey' | 'album' | 'genre'>
  >,
): Promise<Track> {
  return $fetch(`/api/v1/tracklists/${tracklistId}/tracks/${trackId}`, {
    method: 'PUT',
    body: fields,
  }).then((res) => {
    const response = res as TrackResponse;
    if (response.error) throw new Error(response.message ?? response.error);
    return response.track;
  });
}

// Upload track artwork (TRKL-14)
export function uploadTrackArtwork(
  tracklistId: string,
  trackId: string,
  file: File,
): Promise<{
  artworkUrl: string;
  artworkStatus: 'manual';
  artworkSource: 'manual';
}> {
  const formData = new FormData();
  formData.append('file', file);

  return $fetch(`/api/v1/tracklists/${tracklistId}/tracks/${trackId}/artwork`, {
    method: 'PUT',
    body: formData,
  }).then((res) => {
    const response = res as ArtworkResponse;
    if (response.error) throw new Error(response.message ?? response.error);
    return {
      artworkUrl: response.artwork_url,
      artworkStatus: response.artwork_status,
      artworkSource: response.artwork_source,
    };
  });
}

// Delete a tracklist
export function deleteTracklist(id: string): Promise<void> {
  return $fetch(`/api/v1/tracklists/${id}`, {
    method: 'DELETE',
  }).then((res) => {
    const response = res as { error?: string; message?: string };
    if (response.error) throw new Error(response.message ?? response.error);
  });
}

// Poll artwork status (TRKL-16)
export function pollArtworkStatus(
  tracklistId: string,
  intervalMs: number,
  onUpdate: (tracks: Track[]) => void,
  signal: AbortSignal,
): void {
  const fetchTracks = async () => {
    try {
      const res = await $fetch(`/api/v1/tracklists/${tracklistId}`);
      const response = res as TracklistDetailResponse;
      if (response.error) throw new Error(response.message ?? response.error);
      onUpdate(response.tracks);
    } catch (err) {
      console.error('Failed to poll artwork status:', err);
    }
  };

  const poll = () => {
    fetchTracks();
    if (!signal.aborted) {
      setTimeout(poll, intervalMs);
    }
  };

  poll();
}

// Generate tracklist image (TRKL-17)
export function generateImage(
  tracklistId: string,
  format: 'story' | 'square' | 'both',
): Promise<{ story?: string; square?: string }> {
  return $fetch(
    `/api/v1/tracklists/${tracklistId}/generate-image?format=${format}`,
    {
      method: 'POST',
    },
  ).then((res) => {
    const response = res as {
      story?: string;
      square?: string;
      error?: string;
      message?: string;
    };
    if (response.error) throw new Error(response.message ?? response.error);
    return response;
  });
}
