import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useEpkStore } from '../epk';
import type { EPKContent, EPKExport, EPKExportCreateResult } from '../../types/epk';

const mockEpkContent: EPKContent = {
  id: 'epk-1',
  bioShort: 'Short bio',
  bioLong: 'Long bio text',
  techRider: 'Technical rider info',
  stagePlotPath: 'epk/stage-plot.png',
  gigHighlights: ['Club Alpha', 'Gamma Hall'],
  pressQuotes: [{ text: 'Amazing set', source: 'DJ Mag' }],
  photoPaths: ['epk/photo1.jpg', 'epk/photo2.jpg'],
  sectionVisibility: { bio: true, techRider: false },
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
};

const mockExport: EPKExport = {
  id: 'export-1',
  minioPath: 'epk/exports/export-1.pdf',
  createdAt: '2026-01-01T00:00:00Z',
};

const mockExportCreateResult: EPKExportCreateResult = {
  id: 'export-1',
  downloadUrl: 'https://minio.example.com/presigned?token=abc',
  createdAt: '2026-01-01T00:00:00Z',
};

describe('useEpkStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn();
  });

  describe('initial state', () => {
    it('bioShort starts as empty string', () => {
      const store = useEpkStore();
      expect(store.bioShort).toBe('');
    });

    it('bioLong starts as empty string', () => {
      const store = useEpkStore();
      expect(store.bioLong).toBe('');
    });

    it('gigHighlights starts as empty array', () => {
      const store = useEpkStore();
      expect(store.gigHighlights).toEqual([]);
    });

    it('pressQuotes starts as empty array', () => {
      const store = useEpkStore();
      expect(store.pressQuotes).toEqual([]);
    });

    it('photoPaths starts as empty array', () => {
      const store = useEpkStore();
      expect(store.photoPaths).toEqual([]);
    });

    it('exports starts as empty array', () => {
      const store = useEpkStore();
      expect(store.exports).toEqual([]);
    });

    it('saveStatus starts as idle', () => {
      const store = useEpkStore();
      expect(store.saveStatus).toBe('idle');
    });

    it('sectionVisibility starts as empty object', () => {
      const store = useEpkStore();
      expect(store.sectionVisibility).toEqual({});
    });
  });

  describe('computed properties', () => {
    it('photoCount reflects photoPaths array length', () => {
      const store = useEpkStore();
      store.photoPaths = ['path1.jpg', 'path2.jpg'];
      expect(store.photoCount).toBe(2);
    });

    it('canAddPhoto is true when fewer than 20 photos', () => {
      const store = useEpkStore();
      store.photoPaths = Array.from({ length: 19 }, (_, i) => `photo${i}.jpg`);
      expect(store.canAddPhoto).toBe(true);
    });

    it('canAddPhoto is false when 20 or more photos', () => {
      const store = useEpkStore();
      store.photoPaths = Array.from({ length: 20 }, (_, i) => `photo${i}.jpg`);
      expect(store.canAddPhoto).toBe(false);
    });
  });

  describe('loadFromApi()', () => {
    it('populates bioShort from API response', async () => {
      const store = useEpkStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: mockEpkContent });
      await store.loadFromApi();
      expect(store.bioShort).toBe('Short bio');
    });

    it('populates bioLong from API response', async () => {
      const store = useEpkStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: mockEpkContent });
      await store.loadFromApi();
      expect(store.bioLong).toBe('Long bio text');
    });

    it('populates gigHighlights from API response', async () => {
      const store = useEpkStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: mockEpkContent });
      await store.loadFromApi();
      expect(store.gigHighlights).toEqual(['Club Alpha', 'Gamma Hall']);
    });

    it('populates photoPaths from API response', async () => {
      const store = useEpkStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: mockEpkContent });
      await store.loadFromApi();
      expect(store.photoPaths).toEqual(['epk/photo1.jpg', 'epk/photo2.jpg']);
    });

    it('calls GET /api/v1/epk/content', async () => {
      const store = useEpkStore();
      const mockFetch = vi.fn().mockResolvedValue({ data: mockEpkContent });
      // @ts-expect-error - mocking global $fetch
      global.$fetch = mockFetch;
      await store.loadFromApi();
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/epk/content');
    });
  });

  describe('updateContent()', () => {
    it('sets saveStatus to saving before fetch', async () => {
      const store = useEpkStore();
      let statusDuringFetch = '';
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockImplementation(async () => {
        statusDuringFetch = store.saveStatus;
        return { data: mockEpkContent };
      });
      await store.updateContent({ bioShort: 'New bio' });
      expect(statusDuringFetch).toBe('saving');
    });

    it('sets saveStatus to saved on success', async () => {
      const store = useEpkStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: mockEpkContent });
      await store.updateContent({ bioShort: 'New bio' });
      expect(store.saveStatus).toBe('saved');
    });

    it('sets saveStatus to error on failure', async () => {
      const store = useEpkStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockRejectedValue(new Error('Network error'));
      await expect(store.updateContent({ bioShort: 'New bio' })).rejects.toThrow('Network error');
      expect(store.saveStatus).toBe('error');
    });

    it('calls PUT /api/v1/epk/content with patch body', async () => {
      const store = useEpkStore();
      const mockFetch = vi.fn().mockResolvedValue({ data: mockEpkContent });
      // @ts-expect-error - mocking global $fetch
      global.$fetch = mockFetch;
      await store.updateContent({ bioShort: 'Updated bio' });
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/epk/content', {
        method: 'PUT',
        body: { bioShort: 'Updated bio' },
      });
    });

    it('updates store state on success', async () => {
      const store = useEpkStore();
      const updated = { ...mockEpkContent, bioShort: 'Updated bio' };
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: updated });
      await store.updateContent({ bioShort: 'Updated bio' });
      expect(store.bioShort).toBe('Updated bio');
    });
  });

  describe('uploadPhoto()', () => {
    it('appends new path to photoPaths on success', async () => {
      const store = useEpkStore();
      store.photoPaths = ['existing.jpg'];
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ path: 'epk/photos/new.jpg' });
      const file = new File(['img'], 'photo.jpg', { type: 'image/jpeg' });
      await store.uploadPhoto(file);
      expect(store.photoPaths).toContain('epk/photos/new.jpg');
      expect(store.photoPaths).toHaveLength(2);
    });

    it('calls POST /api/v1/epk/photos as FormData', async () => {
      const store = useEpkStore();
      const mockFetch = vi.fn().mockResolvedValue({ path: 'epk/photos/new.jpg' });
      // @ts-expect-error - mocking global $fetch
      global.$fetch = mockFetch;
      const file = new File(['img'], 'photo.jpg', { type: 'image/jpeg' });
      await store.uploadPhoto(file);
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/epk/photos', expect.objectContaining({
        method: 'POST',
        body: expect.any(FormData),
      }));
    });
  });

  describe('deletePhoto()', () => {
    it('removes path from photoPaths on success', async () => {
      const store = useEpkStore();
      store.photoPaths = ['epk/photo1.jpg', 'epk/photo2.jpg'];
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({});
      await store.deletePhoto('epk/photo1.jpg');
      expect(store.photoPaths).not.toContain('epk/photo1.jpg');
      expect(store.photoPaths).toHaveLength(1);
    });

    it('calls DELETE /api/v1/epk/photos/{encoded-path}', async () => {
      const store = useEpkStore();
      store.photoPaths = ['epk/photo1.jpg'];
      const mockFetch = vi.fn().mockResolvedValue({});
      // @ts-expect-error - mocking global $fetch
      global.$fetch = mockFetch;
      await store.deletePhoto('epk/photo1.jpg');
      expect(mockFetch).toHaveBeenCalledWith(
        `/api/v1/epk/photos/${encodeURIComponent('epk/photo1.jpg')}`,
        { method: 'DELETE' },
      );
    });
  });

  describe('uploadStagePlot()', () => {
    it('posts the file using the image multipart field', async () => {
      const store = useEpkStore();
      const mockFetch = vi.fn().mockResolvedValue({ path: 'epk/stage-plot/new.png' });
      // @ts-expect-error - mocking global $fetch
      global.$fetch = mockFetch;
      const file = new File(['img'], 'stage.png', { type: 'image/png' });

      await store.uploadStagePlot(file);

      const options = mockFetch.mock.calls[0]?.[1] as { body: FormData };
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/epk/stage-plot', {
        method: 'POST',
        body: expect.any(FormData),
      });
      expect(options.body.get('stagePlot')).toBe(file);
      expect(options.body.has('image')).toBe(false);
    });
  });

  describe('generateExport()', () => {
    it('returns the create result without treating it as a list entry', async () => {
      const store = useEpkStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue(mockExportCreateResult);
      await expect(store.generateExport()).resolves.toEqual(mockExportCreateResult);
      expect(store.exports).toEqual([]);
    });
  });

  describe('loadExports()', () => {
    it('replaces exports list from API response', async () => {
      const store = useEpkStore();
      store.exports = [mockExport];
      const newExport = { ...mockExport, id: 'export-2' };
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [newExport] });
      await store.loadExports();
      expect(store.exports).toEqual([newExport]);
    });
  });

  describe('deleteExport()', () => {
    it('removes export from list by id', async () => {
      const store = useEpkStore();
      store.exports = [mockExport, { ...mockExport, id: 'export-2' }];
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({});
      await store.deleteExport('export-1');
      expect(store.exports.find((e) => e.id === 'export-1')).toBeUndefined();
      expect(store.exports).toHaveLength(1);
    });
  });
});
