import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia, storeToRefs as piniaStoreToRefs } from 'pinia'
import TracklistExporter from '../TracklistExporter.vue'
import { useTracklistStore } from '../../../stores/tracklist'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import type { Track, Tracklist } from '../../../types/tracklist'

// The component relies on Nuxt auto-imports; make them resolve at runtime.
;(globalThis as Record<string, unknown>).useTracklistStore = useTracklistStore
;(globalThis as Record<string, unknown>).useSettingsStore = useSettingsStore
;(globalThis as Record<string, unknown>).useUiStore = useUiStore
;(globalThis as Record<string, unknown>).storeToRefs = piniaStoreToRefs

const track = (id: string, title: string, artist: string, over: Partial<Track> = {}): Track => ({
  id, tracklistId: 'tl', position: 0, title, artist, album: '', genre: '', bpm: 0, rating: 0,
  durationSecs: 0, musicalKey: '', dateAdded: '', artworkStatus: 'placeholder', artworkUrl: '',
  artworkSource: '', ...over,
})

const Button = { template: '<button v-bind="$attrs"><slot /></button>' }

describe('TracklistExporter text export', () => {
  let writeText: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  })

  const setup = () => {
    const store = useTracklistStore()
    store.tracklist = { id: 'tl' } as Tracklist
    store.tracks = [
      track('1', 'Intro', 'A'),
      track('2', 'Strings of Life', 'Rhythim Is Rhythim'),
      track('3', 'Secret Weapon', 'Dubplate Guy', { hiddenGem: true }),
      track('4', 'Energy Flash', 'Joey Beltram'),
      track('5', 'Outro', 'Z'),
    ]
    const settings = useSettingsStore()
    settings.trackRangeStart = 1
    settings.trackRangeEnd = 4
    settings.maxTracks = 3
    return { store, wrapper: mount(TracklistExporter, { global: { stubs: { Button } } }) }
  }

  it('copies the card range, capped at max tracks, with manual edits', async () => {
    const { store, wrapper } = setup()
    store.tracks[1].title = 'Strings of Life (Edit)'
    await wrapper.find('[data-testid="export-text-numbered"]').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('1. Rhythim Is Rhythim - Strings of Life (Edit)\n2. ID - ID\n3. Joey Beltram - Energy Flash')
    expect(wrapper.find('[data-testid="export-text-numbered"]').text()).toBe('COPIED')
  })

  it('copies the plain format and never leaks a hidden gem', async () => {
    const { wrapper } = setup()
    await wrapper.find('[data-testid="export-text-plain"]').trigger('click')
    await flushPromises()
    const text = writeText.mock.calls[0][0] as string
    expect(text).toBe('Rhythim Is Rhythim - Strings of Life\nID - ID\nJoey Beltram - Energy Flash')
    expect(text).not.toMatch(/Secret|Dubplate/)
  })

  it('shows an error when the clipboard refuses', async () => {
    const { wrapper } = setup()
    writeText.mockRejectedValueOnce(new Error('denied'))
    await wrapper.find('[data-testid="export-text-plain"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('Could not copy')
  })
})
