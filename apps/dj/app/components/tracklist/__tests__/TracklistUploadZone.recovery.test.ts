import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia, storeToRefs as piniaStoreToRefs } from 'pinia'
import TracklistUploadZone from '../TracklistUploadZone.vue'
import { useUiStore } from '../../../stores/ui'
import { useTracklistStore } from '../../../stores/tracklist'

// Component relies on Nuxt auto-imports — make them resolve at runtime.
;(globalThis as Record<string, unknown>).useUiStore = useUiStore
;(globalThis as Record<string, unknown>).useTracklistStore = useTracklistStore
;(globalThis as Record<string, unknown>).storeToRefs = piniaStoreToRefs
;(globalThis as Record<string, unknown>).useDemo = () => ({
  isDemo: false,
  sampleTracklist: '',
  reset: () => undefined,
  downloadCalendar: async () => undefined,
})

// Mock the @vueuse/core useDropZone helper so jsdom can mount the component.
vi.mock('@vueuse/core', async () => {
  const actual = await vi.importActual<typeof import('@vueuse/core')>('@vueuse/core')
  return {
    ...actual,
    useDropZone: () => ({ isOverDropZone: { value: false } }),
  }
})

// useDemo reads runtime config / Nuxt app context we don't have in jsdom,
// so it is provided via globalThis above.

function makeFile(name = 'history.txt'): File {
  return new File(['track1\ntrack2'], name, { type: 'text/plain' })
}

describe('TracklistUploadZone — failure does not lie about progress', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  async function pickFile(wrapper: ReturnType<typeof mount>) {
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [makeFile()] })
    await input.trigger('change')
  }

  it('clears the progress interval and shows error on rejection', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockRejectedValue(new Error('parse failed')))
    const wrapper = mount(TracklistUploadZone)
    const ui = useUiStore()
    const tracklist = useTracklistStore()

    await pickFile(wrapper)
    // The upload is in-flight; progress is shown via uploadLoading.
    // With fake timers the rejected promise resolves on the next tick,
    // so we cannot observe uploadLoading=true unless we control timing
    // — see the "leak" test for the orphan-interval assertion which is
    // observable in the post-failure state. Here we check the post-state.
    vi.advanceTimersByTime(2000)
    await flushPromises()

    expect(ui.uploadLoading).toBe(false)
    expect(ui.uploadError).toBe('parse failed')
    expect(ui.step).toBe('upload')
    expect(tracklist.tracks).toEqual([])
    expect(tracklist.tracklist).toBeNull()
  })

  it('does not leak a setInterval when upload rejects', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockRejectedValue(new Error('boom')))
    const wrapper = mount(TracklistUploadZone)

    await pickFile(wrapper)
    await flushPromises()
    expect(useUiStore().uploadLoading).toBe(false)

    // vi.getTimerCount() counts every outstanding fake timer (setInterval
    // and setTimeout). With the orphan interval cleared we expect 0
    // timers left over from the failed upload.
    expect(vi.getTimerCount()).toBe(0)

    // Advancing time must not start a fresh upload.
    vi.advanceTimersByTime(10_000)
    await flushPromises()
    expect(useUiStore().uploadLoading).toBe(false)
  })

  it('advances step to edit only on successful parse', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({
      tracklist: { id: 'tl-1' },
      tracks: [{ id: 't1' }],
      warnings: [],
    }))
    const wrapper = mount(TracklistUploadZone)
    const ui = useUiStore()

    await pickFile(wrapper)
    await flushPromises()

    expect(ui.uploadLoading).toBe(false)
    expect(ui.uploadError).toBeNull()
    expect(ui.step).toBe('edit')
    expect(useTracklistStore().tracks).toHaveLength(1)
  })
})
