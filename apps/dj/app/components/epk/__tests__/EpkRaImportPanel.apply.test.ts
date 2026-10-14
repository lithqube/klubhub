import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import EpkRaImportPanel from '../EpkRaImportPanel.vue'
import { useSettingsStore } from '../../../stores/settings'
import type { RAArtist } from '../../../types/ra'

// Real stores (no vi.mock): these tests pin the actual API calls the panel
// makes when applying an RA import to the EPK and settings.
const mockFetch = vi.fn()

function mountPanel() {
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { features: { raImport: true } } }))
  vi.stubGlobal('$fetch', mockFetch)
  setActivePinia(createPinia())
  return mount(EpkRaImportPanel)
}

function makeArtist(over: Partial<RAArtist> = {}): RAArtist {
  return {
    id: '1',
    name: 'Helena Hauff',
    slug: 'helenahauff',
    url: 'https://ra.co/dj/helenahauff',
    biography: 'Hamburg techno producer.',
    aliases: [],
    facebook: '',
    twitter: '',
    instagram: 'https://instagram.com/helenahauff',
    soundcloud: '',
    bandcamp: '',
    discogs: '',
    website: 'https://helenahauff.com',
    areas: [],
    venues: [],
    followers: 12345,
    headerImage: '',
    profileImage: '',
    ...over,
  }
}

function buttonWithText(wrapper: ReturnType<typeof mount>, text: string) {
  return wrapper.findAll('button').find((b) => b.text().includes(text))
}

async function fetchArtist(wrapper: ReturnType<typeof mount>, slug = 'helenahauff') {
  await wrapper.find('input[placeholder="e.g. helenahauff"]').setValue(slug)
  await buttonWithText(wrapper, 'FETCH')!.trigger('click')
  await flushPromises()
}

describe('EpkRaImportPanel apply flow (real stores)', () => {
  beforeEach(() => {
    mockFetch.mockReset()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('fetches the artist and renders the card with only the links RA has', async () => {
    mockFetch.mockResolvedValue({ data: makeArtist() })
    const wrapper = mountPanel()

    await fetchArtist(wrapper)

    expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/info/helenahauff')
    expect(wrapper.text()).toContain('Helena Hauff')
    expect(wrapper.text()).toContain('H')
    expect(wrapper.text()).toContain('12,345 FOLLOWERS')
    expect(wrapper.text()).toContain('Hamburg techno producer.')
    expect(wrapper.text()).toContain('https://instagram.com/helenahauff')
    expect(wrapper.text()).toContain('https://helenahauff.com')
    // empty RA links render no chip
    expect(wrapper.text()).not.toContain('soundcloud.com')
    const raLink = wrapper.find('a[href="https://ra.co/dj/helenahauff"]')
    expect(raLink.exists()).toBe(true)
    expect(raLink.attributes('target')).toBe('_blank')
  })

  it('keeps FETCH disabled until a slug is entered', async () => {
    const wrapper = mountPanel()
    expect(buttonWithText(wrapper, 'FETCH')!.attributes('disabled')).toBeDefined()
    await wrapper.find('input[placeholder="e.g. helenahauff"]').setValue('helenahauff')
    expect(buttonWithText(wrapper, 'FETCH')!.attributes('disabled')).toBeUndefined()
  })

  it('applies bio, name and only the non-empty RA links, preserving existing links', async () => {
    const artist = makeArtist()
    mockFetch.mockResolvedValueOnce({ data: artist }) // fetchArtist
    const wrapper = mountPanel()
    // a manually entered link must survive the import
    useSettingsStore().socialLinks = { spotify: 'https://open.spotify.com/dj/x' }
    await fetchArtist(wrapper)

    mockFetch.mockResolvedValueOnce({ data: { bio_long: artist.biography } }) // PUT epk/content
    mockFetch.mockResolvedValueOnce({ updated_at: 't0', dj_name: 'Old', social_links: { spotify: 'https://open.spotify.com/dj/x' } }) // GET settings
    mockFetch.mockResolvedValueOnce({ dj_name: artist.name }) // PUT settings
    await buttonWithText(wrapper, 'APPLY TO EPK')!.trigger('click')
    await flushPromises()

    // 1) bio goes to the EPK content endpoint, truncated to 200 chars for the short bio
    expect(mockFetch).toHaveBeenNthCalledWith(2, '/api/v1/epk/content', {
      method: 'PUT',
      body: { bioLong: artist.biography, bioShort: artist.biography.substring(0, 200) },
    })
    // 2) settings save is a read-modify-write
    expect(mockFetch).toHaveBeenNthCalledWith(3, '/api/v1/settings')
    const [putUrl, putOpts] = mockFetch.mock.calls[3]!
    expect(putUrl).toBe('/api/v1/settings')
    expect(putOpts.method).toBe('PUT')
    expect(putOpts.body.dj_name).toBe('Helena Hauff')
    expect(putOpts.body.social_links).toEqual({
      spotify: 'https://open.spotify.com/dj/x', // preserved
      instagram: 'https://instagram.com/helenahauff',
      residentAdvisor: 'https://ra.co/dj/helenahauff',
      website: 'https://helenahauff.com',
    })

    const success = wrapper.find('[data-testid="ra-apply-success"]')
    expect(success.exists()).toBe(true)
    expect(success.text()).toContain('Saved to your EPK: bio · name · 3 links')
  })

  it('skips the EPK write when the artist has no biography', async () => {
    mockFetch.mockResolvedValueOnce({ data: makeArtist({ biography: '' }) })
    const wrapper = mountPanel()
    await fetchArtist(wrapper)

    mockFetch.mockResolvedValueOnce({ updated_at: 't0' }) // GET settings
    mockFetch.mockResolvedValueOnce({}) // PUT settings
    await buttonWithText(wrapper, 'APPLY TO EPK')!.trigger('click')
    await flushPromises()

    const urls = mockFetch.mock.calls.map((c) => c[0])
    expect(urls).not.toContain('/api/v1/epk/content')
    expect(wrapper.find('[data-testid="ra-apply-success"]').text()).toContain('name · 3 links')
  })

  it('shows the API error message when applying fails', async () => {
    mockFetch.mockResolvedValueOnce({ data: makeArtist() })
    const wrapper = mountPanel()
    await fetchArtist(wrapper)

    mockFetch.mockRejectedValueOnce({ data: { error: 'epk write refused' } })
    await buttonWithText(wrapper, 'APPLY TO EPK')!.trigger('click')
    await flushPromises()

    const error = wrapper.find('[data-testid="ra-apply-error"]')
    expect(error.exists()).toBe(true)
    expect(error.text()).toContain('epk write refused')
  })

  it('falls back to a generic message when the failure has no body', async () => {
    mockFetch.mockResolvedValueOnce({ data: makeArtist() })
    const wrapper = mountPanel()
    await fetchArtist(wrapper)

    mockFetch.mockRejectedValueOnce(new Error('offline'))
    await buttonWithText(wrapper, 'APPLY TO EPK')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="ra-apply-error"]').text()).toContain('Could not apply the import. Try again.')
  })

  it('shows the store error when the artist fetch fails', async () => {
    mockFetch.mockRejectedValue({ data: { error: 'artist not found' } })
    const wrapper = mountPanel()
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})

    await fetchArtist(wrapper)

    expect(wrapper.text()).toContain('artist not found')
    spy.mockRestore()
  })

  it('CLEAR resets the form and hides the artist card', async () => {
    mockFetch.mockResolvedValue({ data: makeArtist() })
    const wrapper = mountPanel()
    await fetchArtist(wrapper)
    expect(wrapper.text()).toContain('Helena Hauff')

    await buttonWithText(wrapper, 'CLEAR')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain('Helena Hauff')
    expect((wrapper.find('input[placeholder="e.g. helenahauff"]').element as HTMLInputElement).value).toBe('')
  })
})
