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

  it('loadFromApi restores the preset from the Go API\'s default_template', async () => {
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn().mockResolvedValue(row({ default_template: 'minimal' }));
    const store = useSettingsStore();
    await store.loadFromApi();
    expect(store.preset).toBe('minimal');
  });
});
