import type { Tracklist, Track, ParseWarning } from '../types/tracklist';

// Define response interfaces for API calls
interface UploadTracklistResponse {
  tracklist: Tracklist;
  tracks: Track[];
  warnings: ParseWarning[];
  error?: string;
  message?: string;
}

interface TracklistDetailResponse {
  tracklist: Tracklist;
  tracks: Track[];
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
    method: 'post',
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
    // Check if the response is an error object
    if (res && typeof res === 'object' && !(Array.isArray(res)) && (res as any).error) {
      throw new Error((res as any).message ?? (res as any).error);
    }
    // The Go API wraps the list as `{ data: Tracklist[] | null }` (null when
    // empty); older mocks returned a raw array. Always hand back an array —
    // returning the envelope made v-for iterate its values and crash on null.
    if (Array.isArray(res)) return res as Tracklist[];
    const data = (res as { data?: Tracklist[] | null } | null)?.data;
    return Array.isArray(data) ? data : [];
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
    method: 'put',
    body: fields,
  }).then((res) => {
    // Check if the response is an error object
    if (res && typeof res === 'object' && !(Array.isArray(res)) && (res as any).error) {
      throw new Error((res as any).message ?? (res as any).error);
    }
    // The API returns the updated track directly
    return res as Track;
  });
}

// Upload track artwork (TRKL-14)
export function uploadTrackArtwork(
  tracklistId: string,
  trackId: string,
  file: File,
): Promise<{
  artworkUrl: string;
  artworkStatus: Track['artworkStatus'];
  artworkSource: Track['artworkSource'];
}> {
  const formData = new FormData();
  formData.append('file', file);

  return $fetch(`/api/v1/tracklists/${tracklistId}/tracks/${trackId}/artwork`, {
    method: 'put',
    body: formData,
  }).then((res) => {
    // Check if the response is an error object
    if (res && typeof res === 'object' && !(Array.isArray(res)) && (res as any).error) {
      throw new Error((res as any).message ?? (res as any).error);
    }
    // The API returns { track: Track }
    const track = (res as { track: Track }).track;
    return {
      artworkUrl: track.artworkUrl,
      artworkStatus: track.artworkStatus,
      artworkSource: track.artworkSource,
    };
  });
}

// Delete a tracklist
export function deleteTracklist(id: string): Promise<void> {
  return $fetch(`/api/v1/tracklists/${id}`, {
    method: 'delete',
  }).then((res) => {
    if (res && typeof res === 'object' && !(Array.isArray(res)) && (res as any).error) {
      throw new Error((res as any).message ?? (res as any).error);
    }
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
      // Check if the response is an error object
      if (res && typeof res === 'object' && !(Array.isArray(res)) && (res as any).error) {
        throw new Error((res as any).message ?? (res as any).error);
      }
      const response = res as { tracklist: Tracklist; tracks: Track[] };
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
      method: 'post',
    }
  ).then((res) => {
    if (res && typeof res === 'object' && !(Array.isArray(res)) && (res as any).error) {
      throw new Error((res as any).message ?? (res as any).error);
    }
    return res as { story?: string; square?: string };
  });
}