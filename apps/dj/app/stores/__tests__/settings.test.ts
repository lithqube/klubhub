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
