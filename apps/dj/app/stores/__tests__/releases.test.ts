import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useReleaseStore } from '../releases';
import type { ChecklistItem, Deadline, PlatformLink, Release } from '../../types/release';

const mockRelease: Release = {
  id: 'r-1',
  title: 'Night Signal',
  artists: 'Ada, Juno',
  label: 'KlubHub',
  distributor: 'Self',
  release_date: '2026-12-11',
  format: 'both',
  notes: '',
  status: 'demo_sent',
};

const mockFetch = vi.fn();

function setup() {
  vi.stubGlobal('$fetch', mockFetch);
  setActivePinia(createPinia());
  return useReleaseStore();
}

describe('useReleaseStore', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('starts empty', () => {
    const store = setup();
    expect(store.releases).toEqual([]);
    expect(store.current).toBeNull();
    expect(store.error).toBeNull();
  });

  it('unwraps the data envelope on list', async () => {
    mockFetch.mockResolvedValue({ data: [mockRelease] });
    const store = setup();
    const releases = await store.fetchReleases();
    expect(releases).toHaveLength(1);
    expect(store.releases[0].title).toBe('Night Signal');
  });

  it('loads deadlines for the release timeline through the store', async () => {
    const deadline: Deadline = {
      id: 'd-1',
      release_id: 'r-1',
      name: 'Artwork',
      due_date: '2026-12-01',
      completed: false,
    };
    mockFetch.mockImplementation((url: string) => {
      if (url === '/api/v1/releases') return Promise.resolve({ data: [mockRelease] });
      return Promise.resolve({ data: [deadline] });
    });
    const store = setup();
    await store.fetchReleases();

    const timeline = await store.fetchDeadlineTimeline();
    expect(timeline).toEqual([{ releaseId: 'r-1', releaseTitle: 'Night Signal', deadline }]);
    expect(mockFetch).toHaveBeenLastCalledWith('/api/v1/releases/r-1/deadlines');
  });

  it('keeps the list empty and records an error when the request fails', async () => {
    mockFetch.mockRejectedValue(new Error('boom'));
    const store = setup();
    const releases = await store.fetchReleases();
    expect(releases).toEqual([]);
    expect(store.error).toBe('Failed to load releases');
  });

  it('transitions a release to the next status', async () => {
    mockFetch.mockResolvedValue({ data: { ...mockRelease, status: 'accepted' } });
    const store = setup();
    const updated = await store.transitionRelease('r-1', 'accepted');
    expect(updated?.status).toBe('accepted');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/releases/r-1/transition', {
      method: 'POST',
      body: { status: 'accepted' },
    });
  });

  it('reports a refused transition', async () => {
    mockFetch.mockRejectedValue(new Error('nope'));
    const store = setup();
    const updated = await store.transitionRelease('r-1', 'released');
    expect(updated).toBeNull();
    expect(store.error).toBe('That status change is not allowed');
  });

  it('loads deadlines, links and the checklist for a release', async () => {
    const deadline: Deadline = {
      id: 'd-1',
      release_id: 'r-1',
      name: 'Artwork',
      due_date: '2026-11-20',
      completed: false,
    };
    const link: PlatformLink = {
      id: 'l-1',
      release_id: 'r-1',
      platform: 'bandcamp',
      url: 'https://bandcamp.com/x',
    };
    const item: ChecklistItem = {
      id: 'c-1',
      release_id: 'r-1',
      position: 0,
      label: 'Artwork final',
      done: false,
    };
    mockFetch.mockImplementation((url: string) => {
      if (url.endsWith('/deadlines')) return Promise.resolve({ data: [deadline] });
      if (url.endsWith('/links')) return Promise.resolve({ data: [link] });
      return Promise.resolve({ data: [item] });
    });
    const store = setup();
    expect(await store.fetchDeadlines('r-1')).toHaveLength(1);
    expect(await store.fetchLinks('r-1')).toHaveLength(1);
    expect(await store.fetchChecklist('r-1')).toHaveLength(1);
    expect(store.checklist[0].label).toBe('Artwork final');
  });

  it('saves a platform link', async () => {
    mockFetch.mockResolvedValue({ data: { id: 'l-1', release_id: 'r-1', platform: 'spotify', url: 'https://open.spotify.com/x' } });
    const store = setup();
    const saved = await store.setLink('r-1', 'spotify', 'https://open.spotify.com/x');
    expect(saved?.platform).toBe('spotify');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/releases/r-1/links', {
      method: 'POST',
      body: { platform: 'spotify', url: 'https://open.spotify.com/x' },
    });
  });

  it('toggles a checklist step', async () => {
    mockFetch.mockResolvedValue({ data: { id: 'c-1', release_id: 'r-1', position: 0, label: 'Artwork final', done: true } });
    const store = setup();
    const toggled = await store.toggleChecklistItem('r-1', 'c-1', true);
    expect(toggled?.done).toBe(true);
  });

  it('saves the checklist as the default template', async () => {
    mockFetch.mockResolvedValue(undefined);
    const store = setup();
    expect(await store.saveChecklistAsTemplate('r-1')).toBe(true);
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/releases/r-1/checklist/save-as-default', { method: 'POST' });
  });
});
