import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { ref } from 'vue';
import { useSocialPostForm } from '../useSocialPostForm';

// Mock useSettingsStore — must return a store-compatible object with reactive refs
// so that storeToRefs() can destructure djName correctly.
vi.mock('../../stores/settings', () => ({
  useSettingsStore: vi.fn(() => {
    const djName = ref('DJ Techno');
    return {
      djName,
      $id: 'settings',
      $patch: vi.fn(),
      $subscribe: vi.fn(),
      $onAction: vi.fn(),
      $dispose: vi.fn(),
      $state: {},
    };
  }),
}));

describe('useSocialPostForm', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  describe('initial state', () => {
    it('postType defaults to "feed"', () => {
      const { postType } = useSocialPostForm();
      expect(postType.value).toBe('feed');
    });

    it('caption defaults to empty string', () => {
      const { caption } = useSocialPostForm();
      expect(caption.value).toBe('');
    });

    it('scheduledAt defaults to empty string', () => {
      const { scheduledAt } = useSocialPostForm();
      expect(scheduledAt.value).toBe('');
    });

    it('imageId defaults to null', () => {
      const { imageId } = useSocialPostForm();
      expect(imageId.value).toBeNull();
    });

    it('imageFile defaults to null', () => {
      const { imageFile } = useSocialPostForm();
      expect(imageFile.value).toBeNull();
    });

    it('timezoneName defaults to browser IANA timezone', () => {
      const { timezoneName } = useSocialPostForm();
      const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone;
      expect(timezoneName.value).toBe(browserTz);
    });
  });

  describe('charLimit', () => {
    it('is 2200 for postType "feed"', () => {
      const { postType, charLimit } = useSocialPostForm();
      postType.value = 'feed';
      expect(charLimit.value).toBe(2200);
    });

    it('is null for postType "story"', () => {
      const { postType, charLimit } = useSocialPostForm();
      postType.value = 'story';
      expect(charLimit.value).toBeNull();
    });
  });

  describe('charCount', () => {
    it('reflects caption.length', () => {
      const { caption, charCount } = useSocialPostForm();
      caption.value = 'Hello World';
      expect(charCount.value).toBe(11);
    });

    it('is 0 for empty caption', () => {
      const { charCount } = useSocialPostForm();
      expect(charCount.value).toBe(0);
    });
  });

  describe('isOverLimit', () => {
    it('is false when caption is within feed limit', () => {
      const { caption, postType, isOverLimit } = useSocialPostForm();
      postType.value = 'feed';
      caption.value = 'A'.repeat(2200);
      expect(isOverLimit.value).toBe(false);
    });

    it('is true when caption exceeds feed limit of 2200', () => {
      const { caption, postType, isOverLimit } = useSocialPostForm();
      postType.value = 'feed';
      caption.value = 'A'.repeat(2201);
      expect(isOverLimit.value).toBe(true);
    });

    it('is always false for story type regardless of caption length', () => {
      const { caption, postType, isOverLimit } = useSocialPostForm();
      postType.value = 'story';
      caption.value = 'A'.repeat(9999);
      expect(isOverLimit.value).toBe(false);
    });
  });

  describe('timezoneOptions', () => {
    it('returns all timezones when search is empty', () => {
      const { timezoneOptions, timezoneSearch } = useSocialPostForm();
      timezoneSearch.value = '';
      const all = Intl.supportedValuesOf('timeZone');
      expect(timezoneOptions.value).toEqual(all);
    });

    it('filters timezones by search query (case-insensitive)', () => {
      const { timezoneOptions, timezoneSearch } = useSocialPostForm();
      timezoneSearch.value = 'berlin';
      expect(timezoneOptions.value).toContain('Europe/Berlin');
      // All results should include "berlin" (case-insensitive)
      timezoneOptions.value.forEach((tz) => {
        expect(tz.toLowerCase()).toContain('berlin');
      });
    });

    it('returns empty array when search matches nothing', () => {
      const { timezoneOptions, timezoneSearch } = useSocialPostForm();
      timezoneSearch.value = 'zzz_nonexistent_timezone_zzz';
      expect(timezoneOptions.value).toEqual([]);
    });
  });

  describe('generateCaption()', () => {
    it('generates caption with DJ name, title, track count, avg BPM', () => {
      const { caption, generateCaption } = useSocialPostForm();
      generateCaption({ title: 'Summer Rave', trackCount: 25, avgBpm: 140.6 });
      expect(caption.value).toBe('DJ Techno @ Summer Rave — 25 tracks • avg 141 BPM\n#techno #djset');
    });

    it('rounds avgBpm to nearest integer', () => {
      const { caption, generateCaption } = useSocialPostForm();
      generateCaption({ title: 'Dark Set', trackCount: 10, avgBpm: 133.4 });
      expect(caption.value).toContain('avg 133 BPM');
    });
  });

  describe('scheduledAtUTCPreview', () => {
    it('returns empty string when scheduledAt is empty', () => {
      const { scheduledAtUTCPreview, scheduledAt } = useSocialPostForm();
      scheduledAt.value = '';
      expect(scheduledAtUTCPreview.value).toBe('');
    });

    it('returns non-empty string when scheduledAt and timezoneName are set', () => {
      const { scheduledAtUTCPreview, scheduledAt, timezoneName } = useSocialPostForm();
      scheduledAt.value = '2026-10-24T23:45';
      timezoneName.value = 'Europe/Berlin';
      expect(scheduledAtUTCPreview.value).not.toBe('');
      expect(scheduledAtUTCPreview.value).toContain('(local)');
    });
  });

  describe('resetForm()', () => {
    it('resets postType to "feed"', () => {
      const { postType, resetForm } = useSocialPostForm();
      postType.value = 'story';
      resetForm();
      expect(postType.value).toBe('feed');
    });

    it('resets caption to empty string', () => {
      const { caption, resetForm } = useSocialPostForm();
      caption.value = 'Some caption';
      resetForm();
      expect(caption.value).toBe('');
    });

    it('resets scheduledAt to empty string', () => {
      const { scheduledAt, resetForm } = useSocialPostForm();
      scheduledAt.value = '2026-10-24T23:45';
      resetForm();
      expect(scheduledAt.value).toBe('');
    });

    it('resets imageId to null', () => {
      const { imageId, resetForm } = useSocialPostForm();
      imageId.value = 'some-id';
      resetForm();
      expect(imageId.value).toBeNull();
    });

    it('resets imageFile to null', () => {
      const { imageFile, resetForm } = useSocialPostForm();
      imageFile.value = new File(['content'], 'test.jpg');
      resetForm();
      expect(imageFile.value).toBeNull();
    });

    it('resets timezoneSearch to empty string', () => {
      const { timezoneSearch, resetForm } = useSocialPostForm();
      timezoneSearch.value = 'berlin';
      resetForm();
      expect(timezoneSearch.value).toBe('');
    });
  });
});
