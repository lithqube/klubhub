import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia, storeToRefs as piniaStoreToRefs } from 'pinia'
import TracklistMeta from '../TracklistMeta.vue'
import { useTracklistStore } from '../../../stores/tracklist'
import { useGigStore } from '../../../stores/gig'
import type { Gig } from '../../../types/gig'
import type { Tracklist } from '../../../types/tracklist'

// The component relies on Nuxt auto-imports; make them resolve at runtime.
;(globalThis as Record<string, unknown>).useTracklistStore = useTracklistStore
;(globalThis as Record<string, unknown>).useGigStore = useGigStore
;(globalThis as Record<string, unknown>).storeToRefs = piniaStoreToRefs

const TL = 'tl-1'
const tracklist = (over: Partial<Tracklist> = {}): Tracklist => ({
  id: TL, title: 'rekordbox_sample.txt', sourceFormat: 'rekordbox', rawFilePath: '', preset: 'default',
  visibleFields: ['title'], bgMode: 'solid', bgValue: '', maxTracks: 50, trackRangeStart: 0, trackRangeEnd: 0,
  createdAt: '2026-09-21T00:00:00Z', updatedAt: '2026-09-21T00:00:00Z', ...over,
} as Tracklist)
const gig = (id: string, date: string, venue: string): Gig => ({ id, date, venue, city: 'Berlin', event_name: '' } as Gig)

type Call = { method: string; url: string; body?: unknown }
let calls: Call[]
let linkedOnServer: Array<{ id: string; date: string; venue: string; city: string; eventName: string }>
let failNext: { match: RegExp; error: unknown } | null

const fetchStub = async (url: string, opts?: { method?: string; body?: unknown }) => {
  const method = opts?.method ?? 'GET'
  calls.push({ method, url, body: opts?.body })
  if (failNext && failNext.match.test(`${method} ${url}`)) {
    const e = failNext.error
    failNext = null
    throw e
  }
  if (method === 'PUT' && url === `/api/v1/tracklists/${TL}`) {
    return tracklist({ title: String((opts?.body as { title: string }).title).trim(), updatedAt: '2026-10-02T10:00:00Z' })
  }
  if (method === 'GET' && url === `/api/v1/tracklists/${TL}/gigs`) return linkedOnServer
  if (method === 'POST' && url.startsWith('/api/v1/gigs/')) {
    const gigId = url.split('/')[4]!
    const g = useGigStore().gigs.find(x => x.id === gigId)!
    linkedOnServer = [...linkedOnServer, { id: g.id, date: g.date, venue: g.venue, city: g.city, eventName: '' }]
    return undefined
  }
  if (method === 'DELETE' && url.startsWith('/api/v1/gigs/')) {
    const gigId = url.split('/')[4]!
    linkedOnServer = linkedOnServer.filter(x => x.id !== gigId)
    return undefined
  }
  throw new Error(`unexpected ${method} ${url}`)
}

function setup(over: { linked?: typeof linkedOnServer } = {}) {
  setActivePinia(createPinia())
  calls = []
  failNext = null
  linkedOnServer = over.linked ?? []
  vi.stubGlobal('$fetch', vi.fn(fetchStub))
  const store = useTracklistStore()
  store.tracklist = tracklist()
  store.pastTracklists = [tracklist()]
  useGigStore().gigs = [
    gig('g-old', '2026-08-01T22:00:00Z', 'Old Club'),
    gig('g-new', '2026-10-01T22:00:00Z', 'New Club'),
    gig('g-mid', '2026-09-01T22:00:00Z', 'Mid Club'),
  ]
  return { store, wrapper: mount(TracklistMeta) }
}

afterEach(() => { vi.unstubAllGlobals() })

const sel = (id: string) => `[data-testid="${id}"]`

describe('TracklistMeta: rename', () => {
  beforeEach(() => vi.spyOn(console, 'error').mockImplementation(() => undefined))

  it('renames with Enter, trims, updates the store and the recent-imports row', async () => {
    const { store, wrapper } = setup()
    await flushPromises()
    expect(wrapper.find(sel('tracklist-title')).text()).toBe('rekordbox_sample.txt')

    await wrapper.find(sel('tracklist-rename')).trigger('click')
    const input = wrapper.find(sel('tracklist-title-input'))
    expect((input.element as HTMLInputElement).value).toBe('rekordbox_sample.txt')
    await input.setValue('  Friday at Tresor ')
    await input.trigger('keydown.enter')
    await flushPromises()

    expect(calls.filter(c => c.method === 'PUT')).toEqual([{ method: 'PUT', url: `/api/v1/tracklists/${TL}`, body: { title: 'Friday at Tresor' } }])
    expect(store.tracklist?.title).toBe('Friday at Tresor')
    expect(store.pastTracklists[0]!.title).toBe('Friday at Tresor')
    expect(wrapper.find(sel('tracklist-title')).text()).toBe('Friday at Tresor')
    expect(wrapper.find(sel('tracklist-title-input')).exists()).toBe(false)
  })

  it('keeps editing and shows the API\'s reason when the rename is refused', async () => {
    const { store, wrapper } = setup()
    await flushPromises()
    await wrapper.find(sel('tracklist-rename')).trigger('click')
    failNext = { match: /^PUT /, error: Object.assign(new Error('422'), { data: { error: 'title must be 1-200 characters' } }) }
    await wrapper.find(sel('tracklist-title-input')).setValue('x')
    await wrapper.find(sel('tracklist-title-save')).trigger('click')
    await flushPromises()

    expect(wrapper.find(sel('tracklist-rename-error')).text()).toContain('title must be 1-200 characters')
    expect(wrapper.find(sel('tracklist-title-input')).exists()).toBe(true)
    expect(store.tracklist?.title).toBe('rekordbox_sample.txt')
  })

  it('does not send an empty name, and Escape or an unchanged name sends nothing', async () => {
    const { wrapper } = setup()
    await flushPromises()

    await wrapper.find(sel('tracklist-rename')).trigger('click')
    await wrapper.find(sel('tracklist-title-input')).setValue('   ')
    await wrapper.find(sel('tracklist-title-save')).trigger('click')
    await flushPromises()
    expect(wrapper.find(sel('tracklist-rename-error')).text()).toContain('cannot be empty')

    await wrapper.find(sel('tracklist-title-input')).trigger('keydown.esc')
    expect(wrapper.find(sel('tracklist-title-input')).exists()).toBe(false)

    await wrapper.find(sel('tracklist-rename')).trigger('click')
    await wrapper.find(sel('tracklist-title-save')).trigger('click') // unchanged
    await flushPromises()
    expect(wrapper.find(sel('tracklist-title-input')).exists()).toBe(false)
    expect(calls.some(c => c.method === 'PUT')).toBe(false)
  })
})

describe('TracklistMeta: gig links', () => {
  beforeEach(() => vi.spyOn(console, 'error').mockImplementation(() => undefined))

  it('shows the linked gigs and offers only the others, newest first', async () => {
    const { wrapper } = setup({ linked: [{ id: 'g-mid', date: '2026-09-01T22:00:00Z', venue: 'Mid Club', city: 'Berlin', eventName: '' }] })
    await flushPromises()

    const chips = wrapper.findAll(sel('tracklist-linked-gig'))
    expect(chips).toHaveLength(1)
    expect(chips[0]!.text()).toContain('Mid Club')
    expect(wrapper.find(sel('tracklist-no-gig')).exists()).toBe(false)

    const options = wrapper.findAll(`${sel('tracklist-link-gig')} option`).map(o => o.text())
    expect(options[0]).toContain('LINK_TO_GIG')
    expect(options.slice(1).map(o => o.match(/New Club|Old Club/)?.[0])).toEqual(['New Club', 'Old Club'])
  })

  it('links a gig from the select and shows it as a chip', async () => {
    const { wrapper } = setup()
    await flushPromises()
    expect(wrapper.find(sel('tracklist-no-gig')).exists()).toBe(true)

    await wrapper.find(sel('tracklist-link-gig')).setValue('g-new')
    await flushPromises()

    expect(calls).toContainEqual({ method: 'POST', url: `/api/v1/gigs/g-new/tracklists/${TL}`, body: undefined })
    expect(wrapper.findAll(sel('tracklist-linked-gig'))).toHaveLength(1)
    expect(wrapper.find(sel('tracklist-linked-gig')).text()).toContain('New Club')
    expect((wrapper.find(sel('tracklist-link-gig')).element as HTMLSelectElement).value).toBe('')
  })

  it('unlinks a gig', async () => {
    const { wrapper } = setup({ linked: [{ id: 'g-new', date: '2026-10-01T22:00:00Z', venue: 'New Club', city: 'Berlin', eventName: '' }] })
    await flushPromises()
    await wrapper.find(sel('tracklist-unlink-gig')).trigger('click')
    await flushPromises()

    expect(calls).toContainEqual({ method: 'DELETE', url: `/api/v1/gigs/g-new/tracklists/${TL}`, body: undefined })
    expect(wrapper.findAll(sel('tracklist-linked-gig'))).toHaveLength(0)
    expect(wrapper.find(sel('tracklist-no-gig')).exists()).toBe(true)
  })

  it('says so when linking fails and keeps the list unchanged', async () => {
    const { wrapper } = setup()
    await flushPromises()
    failNext = { match: /^POST /, error: new Error('boom') }
    await wrapper.find(sel('tracklist-link-gig')).setValue('g-new')
    await flushPromises()

    expect(wrapper.find(sel('tracklist-link-error')).text()).toContain('Could not link')
    expect(wrapper.findAll(sel('tracklist-linked-gig'))).toHaveLength(0)
  })
})
