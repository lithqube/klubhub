import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TrackCardTrackRow from '../TrackCardTrackRow.vue'
import TrackMediaIcon from '../TrackMediaIcon.vue'
import type { Track } from '../../../types/tracklist'

const colors = { bg: '#1a1a2e', primary: '#e94560', accent: '#0f3460', text: '#ffffff' }
const track = (over: Partial<Track> = {}): Track => ({
  id: 't1', tracklistId: 'tl', position: 1, title: 'Secret Weapon', artist: 'Mystery Artist', album: '', genre: '',
  bpm: 128, rating: 0, durationSecs: 0, musicalKey: '8A', dateAdded: '2026-10-01', artworkStatus: 'fetched',
  artworkUrl: 'https://img.example/cover.jpg', artworkSource: '', ...over,
})
const mountRow = (t: Track) =>
  mount(TrackCardTrackRow, { props: { track: t, index: 0, visibleFields: ['bpm', 'musical_key'], colors } })

describe('TrackCardTrackRow: markers on the exported card', () => {
  it('shows a normal track with its title, artist and cover', () => {
    const w = mountRow(track())
    expect(w.text()).toContain('Secret Weapon')
    expect(w.text()).toContain('Mystery Artist')
    expect(w.html()).toContain('cover.jpg')
    expect(w.find('[data-testid="card-unreleased"]').exists()).toBe(false)
    expect(w.find('[data-testid="track-media"]').exists()).toBe(false)
  })

  it('a hidden gem keeps its place but gives away neither title, artist nor cover', () => {
    const w = mountRow(track({ hiddenGem: true }))
    expect(w.find('[data-testid="card-hidden-title"]').text()).toBe('HIDDEN GEM')
    expect(w.find('[data-testid="card-hidden-artist"]').exists()).toBe(true)
    const html = w.html()
    expect(html).not.toContain('Secret Weapon')
    expect(html).not.toContain('Mystery Artist')
    expect(html).not.toContain('cover.jpg')
    expect(w.text()).toContain('128') // the BPM and key are not what gives a track away
  })

  it('tags an unreleased track', () => {
    const w = mountRow(track({ unreleased: true }))
    expect(w.find('[data-testid="card-unreleased"]').text()).toBe('UNRELEASED')
    expect(w.text()).toContain('Secret Weapon')
  })

  it('shows what it was played from: vinyl or digital, with different icons', () => {
    const vinyl = mountRow(track({ media: 'vinyl' }))
    const digital = mountRow(track({ media: 'digital' }))
    expect(vinyl.find('[data-testid="track-media"]').attributes('data-media')).toBe('vinyl')
    expect(vinyl.text()).toContain('VINYL')
    expect(digital.find('[data-testid="track-media"]').attributes('data-media')).toBe('digital')
    expect(digital.text()).toContain('DIGITAL')
    // distinct glyphs, not just distinct words
    const glyph = (w: ReturnType<typeof mountRow>) => w.find('[data-testid="track-media"] svg').classes().join(' ')
    expect(glyph(vinyl)).toContain('disc')
    expect(glyph(digital)).toContain('file')
    expect(glyph(vinyl)).not.toBe(glyph(digital))
  })

  it('combines the markers', () => {
    const w = mountRow(track({ hiddenGem: true, unreleased: true, media: 'vinyl' }))
    expect(w.find('[data-testid="card-hidden-title"]').exists()).toBe(true)
    expect(w.find('[data-testid="card-unreleased"]').exists()).toBe(true)
    expect(w.find('[data-media="vinyl"]').exists()).toBe(true)
  })
})

describe('TrackMediaIcon', () => {
  it('renders nothing when the media is not set', () => {
    expect(mount(TrackMediaIcon, { props: { media: '' } }).find('[data-testid="track-media"]').exists()).toBe(false)
  })

  it('says it once for screen readers: alone it has a text alternative, decorative it has none', () => {
    const alone = mount(TrackMediaIcon, { props: { media: 'digital' } })
    expect(alone.find('.sr-only').text()).toBe('Digital')
    expect(alone.attributes('title')).toBe('Digital')

    const decorative = mount(TrackMediaIcon, { props: { media: 'digital', decorative: true } })
    expect(decorative.find('.sr-only').exists()).toBe(false)
    expect(decorative.attributes('aria-hidden')).toBe('true')

    const labelled = mount(TrackMediaIcon, { props: { media: 'vinyl', label: true } })
    expect(labelled.find('.sr-only').exists()).toBe(false)
    expect(labelled.text()).toBe('VINYL')
  })
})
