import type { Tracklist, Track, ParseWarning } from '~/types/tracklist';

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
    if (res.error) throw new Error(res.message ?? res.error);
    return res as {
      tracklist: Tracklist;
      tracks: Track[];
      warnings: ParseWarning[];
    };
  });
}

// List all tracklists
export function listTracklists(): Promise<Tracklist[]> {
  return $fetch(`/api/v1/tracklists`).then((res) => {
    if (res.error) throw new Error(res.message ?? res.error);
    return res as Tracklist[];
  });
}

// Get a single tracklist by ID
export function getTracklist(id: string): Promise<{
  tracklist: Tracklist;
  tracks: Track[];
}> {
  return $fetch(`/api/v1/tracklists/${id}`).then((res) => {
    if (res.error) throw new Error(res.message ?? res.error);
    return res as {
      tracklist: Tracklist;
      tracks: Track[];
    };
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
    if (res.error) throw new Error(res.message ?? res.error);
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
  artworkStatus: 'manual';
  artworkSource: 'manual';
}> {
  const formData = new FormData();
  formData.append('file', file);

  return $fetch(`/api/v1/tracklists/${tracklistId}/tracks/${trackId}/artwork`, {
    method: 'PUT',
    body: formData,
  }).then((res) => {
    if (res.error) throw new Error(res.message ?? res.error);
    return res as {
      artworkUrl: string;
      artworkStatus: 'manual';
      artworkSource: 'manual';
    };
  });
}

// Delete a tracklist
export function deleteTracklist(id: string): Promise<void> {
  return $fetch(`/api/v1/tracklists/${id}`, {
    method: 'DELETE',
  }).then((res) => {
    if (res.error) throw new Error(res.message ?? res.error);
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
      const res = await $fetch<{ tracklist: Tracklist; tracks: Track[] }>(
        `/api/v1/tracklists/${tracklistId}`,
      );
      if (res.error) throw new Error(res.message ?? res.error);
      onUpdate(res.tracks);
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
