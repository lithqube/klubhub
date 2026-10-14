import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import RaEventImport from '../RaEventImport.vue'
import type { RAEVENT, RAImportResult } from '../../../types/ra'

const mockFetch = vi.fn()

function mountWith(raImport: boolean) {
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { features: { raImport } } }))
  vi.stubGlobal('$fetch', mockFetch)
  setActivePinia(createPinia())
  return mount(RaEventImport)
}

function makeEvent(over: Partial<RAEVENT> = {}): RAEVENT {
  return {
    id: 'ev-1',
    title: 'Klubnacht',
    date: '2026-11-01',
    startTime: '23:59',
    endTime: '06:00',
    venueId: 'v1',
    venueName: 'Berghain',
    venueUrl: 'https://ra.co/venues/1',
    artists: [],
    hosts: [],
    attending: 42,
    contentUrl: 'https://ra.co/events/1',
    isPick: false,
    isSoldOut: false,
    promo: 'All night long',
    ...over,
  }
}

function previewResult(events: RAEVENT[], skipped: string[] = []): RAImportResult {
  return {
    success: true,
    artist_slug: 'dj-phantom',
    events_imported: 0,
    events_skipped: skipped.length,
    gigs_created: 0,
    gigs_skipped: 0,
    gig_ids: [],
    skipped_reasons: skipped,
    dry_run: true,
    events,
  }
}

function buttonWithText(wrapper: ReturnType<typeof mount>, text: string) {
  return wrapper.findAll('button').find((b) => b.text().includes(text))
}

async function fetchEvents(wrapper: ReturnType<typeof mount>, slug = 'dj-phantom') {
  const input = wrapper.find('input[placeholder="e.g. dj-phantom"]')
  await input.setValue(slug)
  await buttonWithText(wrapper, 'FETCH')!.trigger('click')
  await flushPromises()
}

/** Wait until $fetch was called with url (the component lazy-imports the gig store). */
async function waitForFetch(url: string, tries = 100): Promise<void> {
  for (let i = 0; i < tries; i++) {
    if (mockFetch.mock.calls.some((c) => c[0] === url)) return
    await new Promise((r) => setTimeout(r, 10))
  }
  throw new Error(`$fetch never called with ${url}`)
}

describe('RaEventImport (licensed feature)', () => {
  beforeEach(() => {
    mockFetch.mockReset()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders nothing when RA import is disabled', () => {
    const wrapper = mountWith(false)
    expect(wrapper.find('.glass-panel').exists()).toBe(false)
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).not.toMatch(/\bRA\b|Resident Advisor|ra\.co/i)
    expect(mockFetch).not.toHaveBeenCalled()
  })

  it('renders the event import panel when RA import is enabled', () => {
    const wrapper = mountWith(true)
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
    expect(wrapper.text()).toContain('IMPORT FROM RA')
  })
})

describe('RaEventImport interactions', () => {
  beforeEach(() => {
    mockFetch.mockReset()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('keeps FETCH disabled until a slug is entered', async () => {
    const wrapper = mountWith(true)
    expect(buttonWithText(wrapper, 'FETCH')!.attributes('disabled')).toBeDefined()
    await wrapper.find('input[placeholder="e.g. dj-phantom"]').setValue('dj-phantom')
    expect(buttonWithText(wrapper, 'FETCH')!.attributes('disabled')).toBeUndefined()
  })

  it('fetches a dry-run preview and lists every event as selected', async () => {
    const events = [makeEvent(), makeEvent({ id: 'ev-2', title: 'Tresor Night', date: '2020-01-01' })]
    mockFetch.mockResolvedValue(previewResult(events))
    const wrapper = mountWith(true)

    await fetchEvents(wrapper)

    expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/import-ra', {
      method: 'POST',
      body: { artist_slug: 'dj-phantom', dry_run: true },
    })
    expect(wrapper.text()).toContain('2 OF 2 SELECTED')
    expect(wrapper.text()).toContain('Klubnacht')
    expect(wrapper.text()).toContain('Tresor Night')
    expect(wrapper.text()).toContain('Berghain')
    expect(wrapper.text()).toContain('42 attending')
    expect(wrapper.text()).toContain('All night long')
    // past events are badged
    expect(wrapper.text()).toContain('PAST EVENT')
    // ticket link points at the RA content url
    const link = wrapper.find('a[href="https://ra.co/events/1"]')
    expect(link.exists()).toBe(true)
    expect(link.attributes('target')).toBe('_blank')
  })

  it('toggles selection per event and with SELECT ALL / SELECT NONE', async () => {
    const events = [makeEvent(), makeEvent({ id: 'ev-2', title: 'Tresor Night' })]
    mockFetch.mockResolvedValue(previewResult(events))
    const wrapper = mountWith(true)
    await fetchEvents(wrapper)

    await wrapper.find('[data-testid="ra-event-ev-1"]').trigger('click')
    expect(wrapper.text()).toContain('1 OF 2 SELECTED')

    await buttonWithText(wrapper, 'SELECT NONE')!.trigger('click')
    expect(wrapper.text()).toContain('0 OF 2 SELECTED')
    // import is disabled without a selection
    expect(buttonWithText(wrapper, 'PREVIEW IMPORT')!.attributes('disabled')).toBeDefined()

    await buttonWithText(wrapper, 'SELECT ALL')!.trigger('click')
    expect(wrapper.text()).toContain('2 OF 2 SELECTED')
    expect(buttonWithText(wrapper, 'PREVIEW IMPORT')!.attributes('disabled')).toBeUndefined()
  })

  it('shows the empty state with skipped reasons when nothing is importable', async () => {
    mockFetch.mockResolvedValue(previewResult([], ['event too old', 'no venue match']))
    const wrapper = mountWith(true)
    await fetchEvents(wrapper)

    const empty = wrapper.find('[data-testid="ra-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('NO UPCOMING EVENTS TO IMPORT FOR "dj-phantom"')
    expect(empty.text()).toContain('2 events skipped — event too old')
  })

  it('previews an import with the selected event ids', async () => {
    const events = [makeEvent(), makeEvent({ id: 'ev-2', title: 'Tresor Night' })]
    mockFetch.mockResolvedValueOnce(previewResult(events))
    const wrapper = mountWith(true)
    await fetchEvents(wrapper)
    await wrapper.find('[data-testid="ra-event-ev-2"]').trigger('click')

    mockFetch.mockResolvedValueOnce({
      ...previewResult([]),
      gigs_created: 1,
      gigs_skipped: 1,
      skipped_reasons: ['r1', 'r2', 'r3', 'r4'],
      dry_run: true,
    })
    await buttonWithText(wrapper, 'PREVIEW IMPORT')!.trigger('click')
    await flushPromises()

    expect(mockFetch).toHaveBeenLastCalledWith('/api/v1/gigs/import-ra', {
      method: 'POST',
      body: { artist_slug: 'dj-phantom', dry_run: true, event_ids: ['ev-1'] },
    })
    expect(wrapper.text()).toContain('1')
    expect(wrapper.text()).toContain('GIGS WOULD BE CREATED')
    expect(wrapper.text()).toContain('SKIPPED')
    // at most 3 reasons, then the overflow note
    expect(wrapper.text()).toContain('r3')
    expect(wrapper.text()).not.toContain('r4')
    expect(wrapper.text()).toContain('...and 1 more')
  })

  it('runs a real import (dry run off) and refreshes the gig list', async () => {
    const events = [makeEvent()]
    mockFetch.mockResolvedValueOnce(previewResult(events))
    const wrapper = mountWith(true)
    await fetchEvents(wrapper)

    // turn the dry run off
    const checkbox = wrapper.find('input[type="checkbox"]')
    await checkbox.setValue(false)
    expect(buttonWithText(wrapper, 'IMPORT EVENTS')).toBeDefined()

    mockFetch.mockResolvedValueOnce({ ...previewResult([]), gigs_created: 1, dry_run: false })
    mockFetch.mockResolvedValueOnce([]) // gigStore.fetchGigs()
    await buttonWithText(wrapper, 'IMPORT EVENTS')!.trigger('click')
    // the component lazy-imports the gig store before refreshing the list
    await waitForFetch('/api/v1/gigs')

    expect(mockFetch).toHaveBeenNthCalledWith(2, '/api/v1/gigs/import-ra', {
      method: 'POST',
      body: { artist_slug: 'dj-phantom', dry_run: false, event_ids: ['ev-1'] },
    })
    expect(wrapper.text()).toContain('GIGS CREATED')
    // the gig list was refreshed after a real import
    expect(mockFetch).toHaveBeenLastCalledWith('/api/v1/gigs')
  })

  it('shows the store error when the fetch fails', async () => {
    mockFetch.mockRejectedValue({ data: { error: 'artist not found' } })
    const wrapper = mountWith(true)
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})

    await fetchEvents(wrapper)

    expect(wrapper.text()).toContain('artist not found')
    spy.mockRestore()
  })

  it('CLEAR resets the form and the store', async () => {
    const events = [makeEvent()]
    mockFetch.mockResolvedValue(previewResult(events))
    const wrapper = mountWith(true)
    await fetchEvents(wrapper)
    expect(wrapper.find('[data-testid="ra-event-ev-1"]').exists()).toBe(true)

    await buttonWithText(wrapper, 'CLEAR')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="ra-event-ev-1"]').exists()).toBe(false)
    expect((wrapper.find('input[placeholder="e.g. dj-phantom"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.text()).not.toContain('1 OF 1 SELECTED')
  })
})
