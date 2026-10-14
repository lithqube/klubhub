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

describe('useSettingsStore.save', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  const row = (over: Record<string, unknown> = {}) => ({
    dj_name: 'Figu Ds', logo_path: '', default_colors: {}, default_template: 'default',
    visible_fields: {}, social_links: {}, bio_short: 'short', bio_long: 'long',
    contact_info: 'c', invoice_prefix: 'INV', updated_at: 'T1', ...over,
  });

  it('sends the current updated_at token and keeps the fields it was not asked to change', async () => {
    const calls: Array<[string, { method?: string; body?: Record<string, unknown> } | undefined]> = [];
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn(async (url: string, opts?: { method?: string; body?: Record<string, unknown> }) => {
      calls.push([url, opts]);
      return opts?.method === 'PUT' ? row({ ...opts.body, updated_at: 'T2' }) : row();
    });
    const store = useSettingsStore();
    await store.save({ default_template: 'neon' });
    const put = calls.find(([, o]) => o?.method === 'PUT')![1]!.body!;
    expect(put.updated_at).toBe('T1');
    expect(put.default_template).toBe('neon');
    expect(put.dj_name).toBe('Figu Ds');
    expect(put.bio_long).toBe('long');
  });

  it('serialises overlapping saves so the second one carries the first one\'s new token', async () => {
    let token = 1;
    const sentTokens: string[] = [];
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn(async (_url: string, opts?: { method?: string; body?: Record<string, unknown> }) => {
      await new Promise(r => setTimeout(r, 5));
      if (opts?.method !== 'PUT') return row({ updated_at: `T${token}` });
      sentTokens.push(String(opts.body!.updated_at));
      if (opts.body!.updated_at !== `T${token}`) throw Object.assign(new Error('409'), { statusCode: 409 });
      token += 1;
      return row({ ...opts.body, updated_at: `T${token}` });
    });
    const store = useSettingsStore();
    await Promise.all([store.save({ default_template: 'dark' }), store.save({ default_template: 'light' })]);
    expect(sentTokens).toEqual(['T1', 'T2']);
  });

  it('a failed save does not block the next one', async () => {
    let fail = true;
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn(async (_url: string, opts?: { method?: string; body?: Record<string, unknown> }) => {
      if (opts?.method === 'PUT') {
        if (fail) { fail = false; throw new Error('boom'); }
        return row({ ...opts.body });
      }
      return row();
    });
    const store = useSettingsStore();
    await expect(store.save({ default_template: 'dark' })).rejects.toThrow('boom');
    await expect(store.save({ default_template: 'light' })).resolves.toBeUndefined();
  });

  it('a saved default_template wins over the preset a demo response still carries', async () => {
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn().mockResolvedValue(row({ default_template: 'neon', preset: 'dark' }))
    const store = useSettingsStore()
    await store.loadFromApi()
    expect(store.preset).toBe('neon')
  })

  it('falls back to preset when there is no default_template', async () => {
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn().mockResolvedValue(row({ default_template: '', preset: 'light' }))
    const store = useSettingsStore()
    await store.loadFromApi()
    expect(store.preset).toBe('light')
  })

  it('loadFromApi restores the preset from the Go API\'s default_template', async () => {
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn().mockResolvedValue(row({ default_template: 'minimal' }));
    const store = useSettingsStore();
    await store.loadFromApi();
    expect(store.preset).toBe('minimal');
  });
});
