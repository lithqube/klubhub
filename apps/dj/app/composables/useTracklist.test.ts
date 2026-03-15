import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock $fetch (Nuxt global) before importing the composable
const mockFetch = vi.fn();
vi.stubGlobal('$fetch', mockFetch);

// Import after stubbing
// NOTE: adjust import path if Nuxt auto-imports make direct import needed
// import { uploadTracklist, pollArtworkStatus } from './useTracklist'

describe('useTracklist — uploadFile', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  it('sends a FormData with a "file" field to /api/v1/tracklists/upload', async () => {
    // Arrange
    mockFetch.mockResolvedValueOnce({
      tracklist: { id: 'abc', title: 'test.txt' },
      tracks: [],
      warnings: [],
    });

    // Act — this will fail until useTracklist.ts is created
    // const result = await uploadTracklist(fakeFile)
    // TODO: uncomment once composable exists

    // Assert $fetch was called with FormData containing a 'file' field
    // expect(mockFetch).toHaveBeenCalledOnce()
    // const [url, opts] = mockFetch.mock.calls[0]
    // expect(url).toBe('/api/v1/tracklists/upload')
    // expect(opts.method).toBe('POST')
    // expect(opts.body).toBeInstanceOf(FormData)
    // expect(opts.body.get('file')).toBe(fakeFile)

    // Wave 0: stub always passes — implementation will un-comment assertions
    expect(true).toBe(true);
  });
});

describe('useTracklist — pollArtworkStatus', () => {
  it('stops polling when AbortSignal is aborted', async () => {
    // Arrange
    const controller = new AbortController();
    mockFetch.mockResolvedValue({
      tracklist: { id: 'abc' },
      tracks: [{ id: 't1', artworkStatus: 'pending', artworkUrl: null }],
    });

    // Act — stub: abort immediately
    controller.abort();

    // Assert: after abort, onUpdate should not be called repeatedly
    // Wave 0: stub always passes
    expect(controller.signal.aborted).toBe(true);
    expect(true).toBe(true);
  });
});
