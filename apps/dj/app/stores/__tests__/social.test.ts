import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useSocialStore } from '../social';
import type { ScheduledPost, SocialAccount } from '../../types/social';

// Mock useUiStore
vi.mock('../ui', () => ({
  useUiStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}));

const mockPost: ScheduledPost = {
  id: 'post-1',
  accountId: 'acc-1',
  status: 'scheduled',
  postType: 'feed',
  caption: 'Test caption',
  imageMinioPath: 'tracklists/img.png',
  scheduledAtUtc: '2026-10-24T21:45:00Z',
  timezoneName: 'Europe/Berlin',
  retryCount: 0,
  nextRetryAt: null,
  lastError: '',
  createdAt: '2026-10-01T00:00:00Z',
  updatedAt: '2026-10-01T00:00:00Z',
};

const mockAccount: SocialAccount = {
  id: 'acc-1',
  platform: 'instagram',
  igUserId: '12345',
  accountName: 'dj_techno',
  tokenExpiry: '2027-01-01T00:00:00Z',
  status: 'connected',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
};

describe('useSocialStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    // Reset $fetch mock
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn();
  });

  describe('initial state', () => {
    it('posts starts as empty array', () => {
      const store = useSocialStore();
      expect(store.posts).toEqual([]);
    });

    it('account starts as null', () => {
      const store = useSocialStore();
      expect(store.account).toBeNull();
    });

    it('loading starts as false', () => {
      const store = useSocialStore();
      expect(store.loading).toBe(false);
    });

    it('composePanelOpen starts as false', () => {
      const store = useSocialStore();
      expect(store.composePanelOpen).toBe(false);
    });

    it('prefilledImageId starts as null', () => {
      const store = useSocialStore();
      expect(store.prefilledImageId).toBeNull();
    });
  });

  describe('loadPosts()', () => {
    it('populates posts ref from API response', async () => {
      const store = useSocialStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: [mockPost] });
      await store.loadPosts();
      expect(store.posts).toEqual([mockPost]);
    });

    it('calls GET /api/v1/social/posts', async () => {
      const store = useSocialStore();
      const mockFetch = vi.fn().mockResolvedValue({ data: [] });
      // @ts-expect-error - mocking global $fetch
      global.$fetch = mockFetch;
      await store.loadPosts();
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/social/posts');
    });

    it('sets loading true during fetch then false after', async () => {
      const store = useSocialStore();
      let loadingDuringFetch = false;
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockImplementation(async () => {
        loadingDuringFetch = store.loading;
        return { data: [] };
      });
      await store.loadPosts();
      expect(loadingDuringFetch).toBe(true);
      expect(store.loading).toBe(false);
    });
  });

  describe('loadAccount()', () => {
    it('populates account ref from API response', async () => {
      const store = useSocialStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: mockAccount });
      await store.loadAccount();
      expect(store.account).toEqual(mockAccount);
    });

    it('sets account to null when API returns null', async () => {
      const store = useSocialStore();
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: null });
      await store.loadAccount();
      expect(store.account).toBeNull();
    });
  });

  describe('retryPost()', () => {
    it('calls POST /api/v1/social/posts/{id}/retry', async () => {
      const store = useSocialStore();
      store.posts = [mockPost];
      const mockFetch = vi.fn()
        .mockResolvedValueOnce({ data: { ...mockPost, status: 'scheduled' } });
      // @ts-expect-error - mocking global $fetch
      global.$fetch = mockFetch;
      await store.retryPost('post-1');
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/social/posts/post-1/retry', { method: 'POST' });
    });

    it('updates the post in the store after retry', async () => {
      const store = useSocialStore();
      const failedPost = { ...mockPost, status: 'failed' as const };
      store.posts = [failedPost];
      const retriedPost = { ...mockPost, status: 'scheduled' as const };
      // @ts-expect-error - mocking global $fetch
      global.$fetch = vi.fn().mockResolvedValue({ data: retriedPost });
      await store.retryPost('post-1');
      expect(store.posts[0].status).toBe('scheduled');
    });
  });

  describe('openComposePanel()', () => {
    it('sets composePanelOpen to true', () => {
      const store = useSocialStore();
      store.openComposePanel();
      expect(store.composePanelOpen).toBe(true);
    });

    it('sets prefilledImageId when imageId provided', () => {
      const store = useSocialStore();
      store.openComposePanel('img-123');
      expect(store.prefilledImageId).toBe('img-123');
    });

    it('sets prefilledImageId to null when no imageId provided', () => {
      const store = useSocialStore();
      store.prefilledImageId = 'old-id';
      store.openComposePanel();
      expect(store.prefilledImageId).toBeNull();
    });
  });

  describe('closeComposePanel()', () => {
    it('sets composePanelOpen to false', () => {
      const store = useSocialStore();
      store.composePanelOpen = true;
      store.closeComposePanel();
      expect(store.composePanelOpen).toBe(false);
    });

    it('clears prefilledImageId', () => {
      const store = useSocialStore();
      store.prefilledImageId = 'img-456';
      store.closeComposePanel();
      expect(store.prefilledImageId).toBeNull();
    });
  });

  describe('createPost()', () => {
    it('builds FormData with imageFile when file provided', async () => {
      const store = useSocialStore();
      const mockFetch = vi.fn().mockResolvedValue({ data: mockPost });
      // @ts-expect-error - mocking global $fetch
      global.$fetch = mockFetch;
      // Mock loadPosts to avoid second fetch
      vi.spyOn(store, 'loadPosts').mockResolvedValue();

      const file = new File(['image content'], 'test.jpg', { type: 'image/jpeg' });
      await store.createPost({
        postType: 'feed',
        caption: 'Test',
        scheduledAt: '2026-10-24T23:45',
        timezoneName: 'Europe/Berlin',
        imageFile: file,
      });

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/social/posts', expect.objectContaining({
        method: 'POST',
        body: expect.any(FormData),
      }));

      const formData = mockFetch.mock.calls[0][1].body as FormData;
      expect(formData.get('imageFile')).toBe(file);
    });

    it('refreshes posts after creating post (calls GET /api/v1/social/posts)', async () => {
      const store = useSocialStore();
      const mockFetch = vi.fn()
        .mockResolvedValueOnce({ data: mockPost })  // POST create
        .mockResolvedValueOnce({ data: [mockPost] }); // GET loadPosts
      // @ts-expect-error - mocking global $fetch
      global.$fetch = mockFetch;
      await store.createPost({
        postType: 'feed',
        caption: 'Test',
        scheduledAt: '2026-10-24T23:45',
        timezoneName: 'Europe/Berlin',
      });
      // Should have been called twice: POST then GET
      expect(mockFetch).toHaveBeenCalledTimes(2);
      expect(mockFetch).toHaveBeenLastCalledWith('/api/v1/social/posts');
    });
  });
});
