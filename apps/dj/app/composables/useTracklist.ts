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

// A gig a tracklist is linked to (GET /tracklists/{id}/gigs).
export interface LinkedGig {
  id: string;
  date: string;
  venue: string;
  city: string;
  eventName: string;
}

function throwIfError(res: unknown): void {
  if (res && typeof res === 'object' && !Array.isArray(res) && (res as { error?: unknown }).error) {
    const r = res as { error: string; message?: string };
    throw new Error(r.message ?? r.error);
  }
}

// Rename a tracklist. The API trims the title and answers 422 for an empty or
// over-long one; the thrown message carries the API's reason.
export async function renameTracklist(id: string, title: string): Promise<Tracklist> {
  const res = await $fetch(`/api/v1/tracklists/${id}`, { method: 'PUT', body: { title } });
  throwIfError(res);
  return res as Tracklist;
}

// The gigs this tracklist is linked to. Linking and unlinking go through the
// gig store (POST/DELETE /gigs/{id}/tracklists/{tracklistId}).
export async function listLinkedGigs(tracklistId: string): Promise<LinkedGig[]> {
  const res = await $fetch(`/api/v1/tracklists/${tracklistId}/gigs`);
  throwIfError(res);
  return Array.isArray(res) ? (res as LinkedGig[]) : [];
}

export type NewTrack = Partial<Pick<Track, 'artist' | 'bpm' | 'musicalKey' | 'media' | 'hiddenGem' | 'unreleased'>> & {
  title: string;
};

// Add a track by hand; the API appends it after the last one (201). A blank
// title or unknown media is a 422.
export async function addTrack(tracklistId: string, fields: NewTrack): Promise<Track> {
  const res = await $fetch(`/api/v1/tracklists/${tracklistId}/tracks`, { method: 'POST', body: fields });
  throwIfError(res);
  return res as Track;
}

// Persist a new track order: every track id of the tracklist, once, in order.
// Returns the tracks as the server now has them.
export async function reorderTracks(tracklistId: string, trackIds: string[]): Promise<Track[]> {
  const res = await $fetch(`/api/v1/tracklists/${tracklistId}/tracks/order`, { method: 'PUT', body: { trackIds } });
  throwIfError(res);
  return Array.isArray(res) ? (res as Track[]) : [];
}

// Update a track
export function updateTrack(
  tracklistId: string,
  trackId: string,
  fields: Partial<
    Pick<Track, 'title' | 'artist' | 'bpm' | 'musicalKey' | 'album' | 'genre' | 'hiddenGem' | 'unreleased' | 'media'>
  >,
): Promise<Track> {
  return $fetch(`/api/v1/tracklists/${tracklistId}/tracks/${trackId}`, {
    method: 'PUT',
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
    method: 'PUT',
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
    method: 'DELETE',
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
      method: 'POST',
    }
  ).then((res) => {
    if (res && typeof res === 'object' && !(Array.isArray(res)) && (res as any).error) {
      throw new Error((res as any).message ?? (res as any).error);
    }
    return res as { story?: string; square?: string };
  });
}