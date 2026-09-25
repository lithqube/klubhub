import { describe, it, expect, vi, beforeEach } from 'vitest';
import { uploadTracklist, listTracklists, getTracklist, updateTrack, uploadTrackArtwork } from './useTracklist';

// Mock $fetch (Nuxt global). The composable resolves $fetch at call time,
// so stubbing after the (hoisted) import is sufficient.
const mockFetch = vi.fn();
vi.stubGlobal('$fetch', mockFetch);

beforeEach(() => {
  mockFetch.mockReset();
  console.log('Before each: mockFetch call count:', mockFetch.mock.calls.length);
});

describe('useTracklist — uploadFile', () => {
  const fakeFile = new File(['content'], 'test.txt', { type: 'text/plain' });

  it('sends a FormData with a "file" field to /api/v1/tracklists/upload', async () => {
    // Arrange
    mockFetch.mockResolvedValueOnce({
      tracklist: { id: 'abc', title: 'test.txt' },
      tracks: [],
      warnings: [],
    });

    // Act
    const result = await uploadTracklist(fakeFile);

    // Assert $fetch was called with FormData containing a 'file' field
    expect(mockFetch).toHaveBeenCalledOnce();
    const call = mockFetch.mock.calls[0];
    expect(call[0]).toBe('/api/v1/tracklists/upload');
    expect(call[1].method).toBe('POST');
    expect(call[1].body).toBeInstanceOf(FormData);
    expect(call[1].body.get('file')).toBe(fakeFile);

    // Also check the result shape
    expect(result).toEqual({
      tracklist: { id: 'abc', title: 'test.txt' },
      tracks: [],
      warnings: [],
    });
  });
});

describe('useTracklist — listTracklists', () => {
  it('returns a raw array of tracklists', async () => {
    mockFetch.mockResolvedValueOnce([
      { id: '1', title: 'List 1', sourceFormat: 'rekordbox', rawFilePath: '/tmp/1.txt', preset: 'story', visibleFields: ['title'], bgMode: 'solid', bgValue: '#000', maxTracks: 20, trackRangeStart: 1, trackRangeEnd: 10, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
      { id: '2', title: 'List 2', sourceFormat: 'serato', rawFilePath: '/tmp/2.txt', preset: 'club', visibleFields: ['artist'], bgMode: 'upload', bgValue: '#fff', maxTracks: 30, trackRangeStart: 5, trackRangeEnd: 15, createdAt: '2026-01-02T00:00:00Z', updatedAt: '2026-01-02T00:00:00Z' }
    ]);

    const result = await listTracklists();

    const call = mockFetch.mock.calls[0];
    expect(call[0]).toBe('/api/v1/tracklists');
    // If there is a second argument, check its method; otherwise, it's a GET by default
    if (call[1]) {
      expect(call[1].method).toBe('GET');
    }

    expect(result).toEqual([
      { id: '1', title: 'List 1', sourceFormat: 'rekordbox', rawFilePath: '/tmp/1.txt', preset: 'story', visibleFields: ['title'], bgMode: 'solid', bgValue: '#000', maxTracks: 20, trackRangeStart: 1, trackRangeEnd: 10, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
      { id: '2', title: 'List 2', sourceFormat: 'serato', rawFilePath: '/tmp/2.txt', preset: 'club', visibleFields: ['artist'], bgMode: 'upload', bgValue: '#fff', maxTracks: 30, trackRangeStart: 5, trackRangeEnd: 15, createdAt: '2026-01-02T00:00:00Z', updatedAt: '2026-01-02T00:00:00Z' }
    ]);
  });
});

describe('useTracklist — getTracklist', () => {
  it('returns an object with tracklist and tracks', async () => {
    mockFetch.mockResolvedValueOnce({
      tracklist: { id: '1', title: 'List 1', sourceFormat: 'rekordbox', rawFilePath: '/tmp/1.txt', preset: 'story', visibleFields: ['title'], bgMode: 'solid', bgValue: '#000', maxTracks: 20, trackRangeStart: 1, trackRangeEnd: 10, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
      tracks: [{ id: 't1', tracklistId: '1', position: 1, title: 'Track 1', artist: 'Artist', album: 'Album', genre: 'Genre', bpm: 120, rating: 5, durationSecs: 180, musicalKey: '8A', dateAdded: '2026-01-01T00:00:00Z', artworkStatus: 'fetched', artworkUrl: 'http://example.com/art.jpg', artworkSource: 'spotify' }]
    });

    const result = await getTracklist('1');

    const call = mockFetch.mock.calls[0];
    expect(call[0]).toBe('/api/v1/tracklists/1');
    // If there is a second argument, check its method; otherwise, it's a GET by default
    if (call[1]) {
      expect(call[1].method).toBe('GET');
    }

    expect(result).toEqual({
      tracklist: { id: '1', title: 'List 1', sourceFormat: 'rekordbox', rawFilePath: '/tmp/1.txt', preset: 'story', visibleFields: ['title'], bgMode: 'solid', bgValue: '#000', maxTracks: 20, trackRangeStart: 1, trackRangeEnd: 10, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
      tracks: [{ id: 't1', tracklistId: '1', position: 1, title: 'Track 1', artist: 'Artist', album: 'Album', genre: 'Genre', bpm: 120, rating: 5, durationSecs: 180, musicalKey: '8A', dateAdded: '2026-01-01T00:00:00Z', artworkStatus: 'fetched', artworkUrl: 'http://example.com/art.jpg', artworkSource: 'spotify' }]
    });
  });
});

describe('useTracklist — updateTrack', () => {
  it('sends a PUT request with the fields and returns the updated track', async () => {
    const updatedTrack = { id: 't1', tracklistId: '1', position: 1, title: 'Updated Title', artist: 'Artist', album: 'Album', genre: 'Genre', bpm: 120, rating: 5, durationSecs: 180, musicalKey: '8A', dateAdded: '2026-01-01T00:00:00Z', artworkStatus: 'fetched', artworkUrl: 'http://example.com/art.jpg', artworkSource: 'spotify' };
    mockFetch.mockResolvedValueOnce(updatedTrack);

    const result = await updateTrack('1', 't1', { title: 'Updated Title' });

    expect(mockFetch).toHaveBeenCalledOnce();
    const call = mockFetch.mock.calls[0];
    expect(call[0]).toBe('/api/v1/tracklists/1/tracks/t1');
    expect(call[1].method).toBe('PUT');
    expect(call[1].body).toEqual({ title: 'Updated Title' });

    expect(result).toEqual(updatedTrack);
  });
});

describe('useTracklist — uploadTrackArtwork', () => {
  it('sends a PUT request with FormData and returns the artwork info', async () => {
    const fakeFile = new File(['content'], 'art.jpg', { type: 'image/jpeg' });
    mockFetch.mockResolvedValueOnce({
      track: {
        artworkUrl: 'http://example.com/art.jpg',
        artworkStatus: 'manual',
        artworkSource: 'manual'
      }
    });

    const result = await uploadTrackArtwork('1', 't1', fakeFile);

    expect(mockFetch).toHaveBeenCalledOnce();
    const call = mockFetch.mock.calls[0];
    expect(call[0]).toBe('/api/v1/tracklists/1/tracks/t1/artwork');
    expect(call[1].method).toBe('PUT');
    expect(call[1].body).toBeInstanceOf(FormData);
    expect(call[1].body.get('file')).toBe(fakeFile);

    expect(result).toEqual({
      artworkUrl: 'http://example.com/art.jpg',
      artworkStatus: 'manual',
      artworkSource: 'manual'
    });
  });
});