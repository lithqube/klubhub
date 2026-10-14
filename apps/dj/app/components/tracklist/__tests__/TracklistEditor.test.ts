import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia, storeToRefs as piniaStoreToRefs } from 'pinia'
import TracklistEditor from '../TracklistEditor.vue'
import { useTracklistStore } from '../../../stores/tracklist'
import { useUiStore } from '../../../stores/ui'
import type { Track, Tracklist } from '../../../types/tracklist'

// The editor polls the server for artwork on mount and merges its tracks into the
// store, which would mask a missing revert in these tests. Turn the poller off.
vi.mock('@/composables/useTracklist', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/composables/useTracklist')>()),
  pollArtworkStatus: vi.fn(),
}))

// The component relies on Nuxt auto-imports; make them resolve at runtime.
;(globalThis as Record<string, unknown>).useTracklistStore = useTracklistStore
;(globalThis as Record<string, unknown>).useUiStore = useUiStore
;(globalThis as Record<string, unknown>).storeToRefs = piniaStoreToRefs
;(globalThis as Record<string, unknown>).onMounted = (await import('vue')).onMounted
;(globalThis as Record<string, unknown>).onBeforeUnmount = (await import('vue')).onBeforeUnmount

const TL = 'tl-1'
const track = (id: string, title: string, position: number, over: Partial<Track> = {}): Track => ({
  id, tracklistId: TL, position, title, artist: `${title} artist`, album: '', genre: '', bpm: 120, rating: 0,
  durationSecs: 0, musicalKey: '8A', dateAdded: '2026-10-01', artworkStatus: 'placeholder', artworkUrl: '',
  artworkSource: '', hiddenGem: false, unreleased: false, media: '', ...over,
})

type Call = { method: string; url: string; body?: any }
let calls: Call[]
let failNext: { match: RegExp; error: unknown } | null
let serverTracks: Track[]
let deferNextPut: Promise<never> | null

const fetchStub = async (url: string, opts?: { method?: string; body?: any }) => {
  const method = opts?.method ?? 'GET'
  calls.push({ method, url, body: opts?.body })
  if (deferNextPut && method === 'PUT') {
    const p = deferNextPut
    deferNextPut = null
    return p
  }
  if (failNext && failNext.match.test(`${method} ${url}`)) {
    const e = failNext.error
    failNext = null
    throw e
  }
  if (method === 'GET' && url === `/api/v1/tracklists/${TL}`) return { tracklist: {}, tracks: serverTracks }
  if (method === 'POST' && url === `/api/v1/tracklists/${TL}/tracks`) {
    const created = track(`new-${serverTracks.length + 1}`, opts!.body.title, serverTracks.length + 1, { ...opts!.body, artworkStatus: 'placeholder' })
    serverTracks = [...serverTracks, created]
    return created
  }
  if (method === 'PUT' && url === `/api/v1/tracklists/${TL}/tracks/order`) {
    const ids: string[] = opts!.body.trackIds
    serverTracks = ids.map((id, i) => ({ ...serverTracks.find(t => t.id === id)!, position: i + 1 }))
    return serverTracks
  }
  const m = url.match(new RegExp(`^/api/v1/tracklists/${TL}/tracks/([^/]+)$`))
  if (method === 'PUT' && m) {
    serverTracks = serverTracks.map(t => (t.id === m[1] ? { ...t, ...opts!.body } : t))
    return serverTracks.find(t => t.id === m[1])
  }
  throw new Error(`unexpected ${method} ${url}`)
}

function setup(initial: Track[] = [track('a', 'Alpha', 1), track('b', 'Bravo', 2), track('c', 'Charlie', 3)]) {
  setActivePinia(createPinia())
  calls = []
  failNext = null
  deferNextPut = null
  serverTracks = initial
  vi.stubGlobal('$fetch', vi.fn(fetchStub))
  const store = useTracklistStore()
  store.tracklist = { id: TL, title: 'Set' } as Tracklist
  store.tracks = [...initial]
  const wrapper = mount(TracklistEditor, { attachTo: document.body })
  return { store, ui: useUiStore(), wrapper }
}

afterEach(() => { vi.unstubAllGlobals(); document.body.innerHTML = '' })
beforeEach(() => { vi.spyOn(console, 'error').mockImplementation(() => undefined) })

const sel = (id: string) => `[data-testid="${id}"]`
// The UI-kit table parts are not registered in unit tests, so Vue renders them as <tablerow> elements.
const rows = (w: ReturnType<typeof setup>['wrapper']) => w.findAll('tablerow').filter(r => r.find('[data-handle]').exists())
const titles = (store: ReturnType<typeof setup>['store']) => store.tracks.map(t => t.title)
const puts = () => calls.filter(c => c.method === 'PUT')

describe('TracklistEditor: add a track', () => {
  it('opens the form, adds a track at the end and stays open for the next one', async () => {
    const { store, wrapper } = setup()
    expect(wrapper.find(sel('add-track-form')).exists()).toBe(false)
    await wrapper.find(sel('add-track-toggle')).trigger('click')
    expect(wrapper.find(sel('add-track-form')).exists()).toBe(true)

    await wrapper.find(sel('add-track-title')).setValue('  New ID ')
    await wrapper.find(sel('add-track-artist')).setValue('Me')
    await wrapper.find(sel('add-track-bpm')).setValue('128.5')
    await wrapper.find(sel('add-track-key')).setValue('4B')
    await wrapper.find(sel('add-track-form')).trigger('submit')
    await flushPromises()

    expect(calls.find(c => c.method === 'POST')!.body).toEqual({ title: 'New ID', artist: 'Me', bpm: 128.5, musicalKey: '4B' })
    expect(titles(store)).toEqual(['Alpha', 'Bravo', 'Charlie', 'New ID'])
    expect(wrapper.find(sel('add-track-form')).exists()).toBe(true)
    expect((wrapper.find(sel('add-track-title')).element as HTMLInputElement).value).toBe('')
  })

  it('needs a title and a sane BPM, and sends nothing otherwise', async () => {
    const { wrapper } = setup()
    await wrapper.find(sel('add-track-toggle')).trigger('click')

    await wrapper.find(sel('add-track-title')).setValue('   ')
    await wrapper.find(sel('add-track-form')).trigger('submit')
    expect(wrapper.find(sel('add-track-error')).text()).toContain('needs a title')

    await wrapper.find(sel('add-track-title')).setValue('x')
    await wrapper.find(sel('add-track-bpm')).setValue('fast')
    await wrapper.find(sel('add-track-form')).trigger('submit')
    expect(wrapper.find(sel('add-track-error')).text()).toContain('BPM')
    expect(calls.some(c => c.method === 'POST')).toBe(false)
  })

  it('keeps what you typed and shows the API reason when adding fails', async () => {
    const { store, wrapper } = setup()
    await wrapper.find(sel('add-track-toggle')).trigger('click')
    await wrapper.find(sel('add-track-title')).setValue('Keep me')
    failNext = { match: /^POST /, error: Object.assign(new Error('422'), { data: { error: 'a track needs a title' } }) }
    await wrapper.find(sel('add-track-form')).trigger('submit')
    await flushPromises()

    expect(wrapper.find(sel('add-track-error')).text()).toContain('a track needs a title')
    expect((wrapper.find(sel('add-track-title')).element as HTMLInputElement).value).toBe('Keep me')
    expect(store.tracks).toHaveLength(3)
  })

  it('shows the empty-state hint when there are no tracks', () => {
    const { wrapper } = setup([])
    expect(wrapper.text()).toContain('NO TRACKS YET')
  })
})

describe('TracklistEditor: marks', () => {
  it('toggles hidden gem and unreleased at once and saves them', async () => {
    const { store, wrapper } = setup()
    const first = rows(wrapper)[0]!
    await first.find(sel('mark-hidden-gem')).trigger('click')
    expect(first.find(sel('mark-hidden-gem')).attributes('aria-pressed')).toBe('true') // before the response
    await first.find(sel('mark-unreleased')).trigger('click')
    await flushPromises()

    expect(puts().map(c => c.body)).toEqual([{ hiddenGem: true }, { unreleased: true }])
    expect(store.tracks[0]).toMatchObject({ hiddenGem: true, unreleased: true })
    // and off again
    await first.find(sel('mark-hidden-gem')).trigger('click')
    await flushPromises()
    expect(puts().at(-1)!.body).toEqual({ hiddenGem: false })
    expect(store.tracks[0]!.hiddenGem).toBe(false)
  })

  it('puts the mark back and says why when saving fails', async () => {
    const { store, ui, wrapper } = setup()
    failNext = { match: /^PUT /, error: Object.assign(new Error('x'), { data: { error: 'invalid track fields' } }) }
    await rows(wrapper)[1]!.find(sel('mark-hidden-gem')).trigger('click')
    await flushPromises()

    expect(store.tracks[1]!.hiddenGem).toBe(false)
    expect(rows(wrapper)[1]!.find(sel('mark-hidden-gem')).attributes('aria-pressed')).toBe('false')
    expect(ui.editError).toBe('invalid track fields')
  })

  it('a failed save puts back only its own mark, not one that saved meanwhile', async () => {
    const { store, wrapper } = setup()
    const row = rows(wrapper)[0]!
    let failFirst!: (e: unknown) => void
    deferNextPut = new Promise<never>((_, reject) => { failFirst = reject })
    await row.find(sel('mark-hidden-gem')).trigger('click') // request A: still in flight
    await row.find(sel('mark-unreleased')).trigger('click') // request B: saves
    await flushPromises()
    expect(store.tracks[0]).toMatchObject({ hiddenGem: true, unreleased: true })

    failFirst(Object.assign(new Error('x'), { data: { error: 'boom' } }))
    await flushPromises()
    expect(store.tracks[0]).toMatchObject({ hiddenGem: false, unreleased: true })
    expect(serverTracks[0]).toMatchObject({ unreleased: true })
    expect(row.find(sel('mark-unreleased')).attributes('aria-pressed')).toBe('true')
  })

  it('sets what it was played from: vinyl or digital, or clears it', async () => {
    const { store, wrapper } = setup()
    const row = rows(wrapper)[2]!
    await row.find(sel('media-digital')).trigger('click')
    await flushPromises()
    expect(puts().at(-1)!.body).toEqual({ media: 'digital' })
    expect(store.tracks[2]!.media).toBe('digital')

    await row.find(sel('media-vinyl')).trigger('click')
    await flushPromises()
    expect(store.tracks[2]!.media).toBe('vinyl')

    await row.find(sel('media-none')).trigger('click')
    await flushPromises()
    expect(puts().at(-1)!.body).toEqual({ media: '' })
    expect(store.tracks[2]!.media).toBe('')
    // only the two real options are offered
    expect(row.findAll('[data-testid^="media-"]').map(e => e.attributes('data-testid'))).toEqual(['media-vinyl', 'media-digital', 'media-none'])
  })
})

describe('TracklistEditor: reorder', () => {
  const handle = (w: ReturnType<typeof setup>['wrapper'], i: number) => rows(w)[i]!.find('[data-handle]')

  it('moves a track down and up with Alt + arrow on its handle and saves the order', async () => {
    const { store, wrapper } = setup()
    await handle(wrapper, 0).trigger('keydown', { key: 'ArrowDown', altKey: true })
    await flushPromises()
    expect(titles(store)).toEqual(['Bravo', 'Alpha', 'Charlie'])
    expect(puts().at(-1)!.url).toBe(`/api/v1/tracklists/${TL}/tracks/order`)
    expect(puts().at(-1)!.body).toEqual({ trackIds: ['b', 'a', 'c'] })

    await handle(wrapper, 2).trigger('keydown', { key: 'ArrowUp', altKey: true })
    await flushPromises()
    expect(titles(store)).toEqual(['Bravo', 'Charlie', 'Alpha'])
  })

  it('does nothing past either end', async () => {
    const { store, wrapper } = setup()
    await handle(wrapper, 0).trigger('keydown', { key: 'ArrowUp', altKey: true })
    await handle(wrapper, 2).trigger('keydown', { key: 'ArrowDown', altKey: true })
    await flushPromises()
    expect(titles(store)).toEqual(['Alpha', 'Bravo', 'Charlie'])
    expect(puts()).toHaveLength(0)
  })

  it('drags a track onto another: before or after depending on the half it is dropped on', async () => {
    const { store, wrapper } = setup()
    await handle(wrapper, 0).trigger('dragstart')
    await rows(wrapper)[2]!.trigger('dragover', { clientY: 10 }) // jsdom rects are 0x0: below the middle = after
    await rows(wrapper)[2]!.trigger('drop')
    await flushPromises()
    expect(titles(store)).toEqual(['Bravo', 'Charlie', 'Alpha'])
    expect(puts().at(-1)!.body).toEqual({ trackIds: ['b', 'c', 'a'] })

    await handle(wrapper, 2).trigger('dragstart') // Alpha again
    await rows(wrapper)[0]!.trigger('dragover', { clientY: -10 }) // above the middle = before
    await rows(wrapper)[0]!.trigger('drop')
    await flushPromises()
    expect(titles(store)).toEqual(['Alpha', 'Bravo', 'Charlie'])
  })

  it('ignores a drop onto itself and a drop with nothing being dragged', async () => {
    const { store, wrapper } = setup()
    await handle(wrapper, 1).trigger('dragstart')
    await rows(wrapper)[1]!.trigger('dragover', { clientY: 10 })
    await rows(wrapper)[1]!.trigger('drop')
    await rows(wrapper)[0]!.trigger('drop')
    await flushPromises()
    expect(titles(store)).toEqual(['Alpha', 'Bravo', 'Charlie'])
    expect(puts()).toHaveLength(0)
  })

  it('puts the old order back and says why when the save fails', async () => {
    const { store, ui, wrapper } = setup()
    failNext = { match: /^PUT .*order$/, error: Object.assign(new Error('x'), { data: { error: 'trackIds must list every track' } }) }
    await handle(wrapper, 0).trigger('keydown', { key: 'ArrowDown', altKey: true })
    await flushPromises()
    expect(titles(store)).toEqual(['Alpha', 'Bravo', 'Charlie'])
    expect(ui.editError).toBe('trackIds must list every track')
  })
})
