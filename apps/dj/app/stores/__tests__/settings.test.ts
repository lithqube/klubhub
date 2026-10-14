import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useSettingsStore } from '../settings';

describe('useSettingsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('initial djName is empty string', () => {
    const store = useSettingsStore();
    expect(store.djName).toBe('');
  });

  it('initial preset is "default"', () => {
    const store = useSettingsStore();
    expect(store.preset).toBe('default');
  });

  it('initial logoPosition is "top-left"', () => {
    const store = useSettingsStore();
    expect(store.logoPosition).toBe('top-left');
  });

  it('initial bgMode is "solid"', () => {
    const store = useSettingsStore();
    expect(store.bgMode).toBe('solid');
  });

  it('initial visibleFields contains expected defaults', () => {
    const store = useSettingsStore();
    expect(store.visibleFields).toEqual(['title', 'artist', 'bpm', 'key', 'durationSecs']);
  });

  it('initial maxTracks is 50', () => {
    const store = useSettingsStore();
    expect(store.maxTracks).toBe(50);
  });

  it('loadFromApi() with mocked $fetch sets preset to returned value', async () => {
    const store = useSettingsStore();
    const mockFetch = vi.fn().mockResolvedValue({ preset: 'neon' });
    // @ts-expect-error - mocking global $fetch
    global.$fetch = mockFetch;
    await store.loadFromApi();
    expect(store.preset).toBe('neon');
  });

  it('loadFromApi() with mocked $fetch sets djName to returned value', async () => {
    const store = useSettingsStore();
    const mockFetch = vi.fn().mockResolvedValue({ dj_name: 'DJ Techno' });
    // @ts-expect-error - mocking global $fetch
    global.$fetch = mockFetch;
    await store.loadFromApi();
    expect(store.djName).toBe('DJ Techno');
  });

  it('loadFromApi() maps every profile field', async () => {
    const store = useSettingsStore();
    const mockFetch = vi.fn().mockResolvedValue({
      dj_name: 'DJ X',
      logo_path: 'logos/x.png',
      logo_position: 'bottom-right',
      custom_placeholder_path: 'ph/custom.png',
      preset: 'neon',
      bg_mode: 'gradient',
      bg_value: '#000,#fff',
      visible_fields: ['title', 'bpm'],
      max_tracks: 25,
      track_range_start: 2,
      track_range_end: 40,
      social_links: { instagram: 'https://ig/djx' },
      contact_info: 'dj@example.com',
    });
    // @ts-expect-error - mocking global $fetch
    global.$fetch = mockFetch;
    await store.loadFromApi();
    expect(store.djName).toBe('DJ X');
    expect(store.logoPath).toBe('logos/x.png');
    expect(store.logoPosition).toBe('bottom-right');
    expect(store.customPlaceholderPath).toBe('ph/custom.png');
    expect(store.preset).toBe('neon');
    expect(store.bgMode).toBe('gradient');
    expect(store.bgValue).toBe('#000,#fff');
    expect(store.visibleFields).toEqual(['title', 'bpm']);
    expect(store.maxTracks).toBe(25);
    expect(store.trackRangeStart).toBe(2);
    expect(store.trackRangeEnd).toBe(40);
    expect(store.socialLinks).toEqual({ instagram: 'https://ig/djx' });
    expect(store.contactInfo).toBe('dj@example.com');
  });

  it('loadFromApi() ignores an empty visible_fields array (keeps defaults)', async () => {
    const store = useSettingsStore();
    const mockFetch = vi.fn().mockResolvedValue({ visible_fields: [] });
    // @ts-expect-error - mocking global $fetch
    global.$fetch = mockFetch;
    await store.loadFromApi();
    expect(store.visibleFields).toEqual(['title', 'artist', 'bpm', 'key', 'durationSecs']);
  });

  it('loadFromApi() ignores a non-array visible_fields value', async () => {
    const store = useSettingsStore();
    const mockFetch = vi.fn().mockResolvedValue({ visible_fields: {} });
    // @ts-expect-error - mocking global $fetch
    global.$fetch = mockFetch;
    await store.loadFromApi();
    expect(store.visibleFields).toEqual(['title', 'artist', 'bpm', 'key', 'durationSecs']);
  });

  it('save() does read-modify-write: GET, merge patch over all writable keys, PUT', async () => {
    const store = useSettingsStore();
    const current = {
      updated_at: '2026-01-01T00:00:00Z',
      dj_name: 'Old Name',
      logo_path: 'logos/old.png',
      default_colors: { bg: '#000' },
      default_template: 'classic',
      visible_fields: ['title'],
      social_links: { instagram: 'https://ig/old' },
      bio_short: 'short',
      bio_long: 'long',
      contact_info: 'old@example.com',
      invoice_prefix: 'INV-',
    };
    const saved = { ...current, dj_name: 'New Name' };
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(current) // GET
      .mockResolvedValueOnce(saved);   // PUT
    // @ts-expect-error - mocking global $fetch
    global.$fetch = mockFetch;

    await store.save({ dj_name: 'New Name' });

    expect(mockFetch).toHaveBeenNthCalledWith(1, '/api/v1/settings');
    const [putUrl, putOpts] = mockFetch.mock.calls[1];
    expect(putUrl).toBe('/api/v1/settings');
    expect(putOpts.method).toBe('PUT');
    // concurrency token + every writable key round-tripped, patch applied last
    expect(putOpts.body).toEqual({ ...current, dj_name: 'New Name' });
    expect(store.saveStatus).toBe('saved');
    expect(store.djName).toBe('New Name');
  });

  it('save() applies the profile fields from the PUT response', async () => {
    const store = useSettingsStore();
    const mockFetch = vi.fn()
      .mockResolvedValueOnce({ updated_at: 't', dj_name: 'A' })
      .mockResolvedValueOnce({
        dj_name: 'A',
        social_links: { tiktok: 'https://tt/a' },
        contact_info: 'a@example.com',
      });
    // @ts-expect-error - mocking global $fetch
    global.$fetch = mockFetch;

    await store.save({ dj_name: 'A' });

    expect(store.socialLinks).toEqual({ tiktok: 'https://tt/a' });
    expect(store.contactInfo).toBe('a@example.com');
  });

  it('save() sets saveStatus to error and rethrows on failure', async () => {
    const store = useSettingsStore();
    const mockFetch = vi.fn().mockRejectedValue(new Error('409 conflict'));
    // @ts-expect-error - mocking global $fetch
    global.$fetch = mockFetch;

    await expect(store.save({ dj_name: 'X' })).rejects.toThrow('409 conflict');
    expect(store.saveStatus).toBe('error');
  });
});
