import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TrackCardTrackList from '../TrackCardTrackList.vue'
import TrackCardFooter from '../TrackCardFooter.vue'
import TrackCard from '../TrackCard.vue'
import type { Track, Tracklist } from '../../../types/tracklist'

const colors = { bg: '#1a1a2e', primary: '#e94560', accent: '#0f3460', text: '#ffffff' }
const tracks = (n: number): Track[] =>
  Array.from({ length: n }, (_, i) => ({
    id: `t${i}`, tracklistId: 'tl', position: i + 1, title: `Track ${i + 1}`, artist: 'Artist', album: '', genre: '',
    bpm: 120, rating: 0, durationSecs: 0, musicalKey: '8A', dateAdded: '2026-10-01', artworkStatus: 'placeholder',
    artworkUrl: '', artworkSource: '',
  }))
const mountList = (n: number, extra: Record<string, unknown> = {}) =>
  mount(TrackCardTrackList, { props: { tracks: tracks(n), colors, maxTracks: 100, visibleFields: ['bpm'], ...extra } })

describe('TrackCardTrackList: density for long sets', () => {
  // The card has ~1500px for the list: 83px rows hold 18, 66px rows 22, 50px rows 28.
  // Each tier stops short of full so a wrapped header still fits.
  it.each([
    [1, 'comfortable'], [17, 'comfortable'],
    [18, 'compact'], [21, 'compact'],
    [22, 'dense'], [28, 'dense'],
    [29, 'dense-2col'], [50, 'dense-2col'],
  ])('%i tracks use the %s tier', (n, tier) => {
    expect(mountList(n as number).attributes('data-density')).toBe(tier)
  })

  it('renders every track, once', () => {
    for (const n of [12, 20, 26, 40]) {
      expect(mountList(n).text().match(/Track \d+/g)).toHaveLength(n)
    }
  })

  it('goes into two columns past 28 tracks, filling the left column first', () => {
    const style = (n: number) => mountList(n).attributes('style') ?? ''
    expect(style(28)).not.toContain('grid')
    const long = style(31)
    expect(long).toContain('display: grid')
    expect(long).toContain('grid-auto-flow: column')
    expect(long).toContain('repeat(16, auto)') // 31 tracks: 16 on the left, 15 on the right
  })

  it('counts the tracks that are actually shown, not the ones uploaded', () => {
    const list = mountList(60, { maxTracks: 15 })
    expect(list.attributes('data-density')).toBe('comfortable')
    expect(list.text().match(/Track \d+/g)).toHaveLength(15)
    const range = mountList(60, { trackRangeStart: 0, trackRangeEnd: 24 })
    expect(range.attributes('data-density')).toBe('dense')
  })

  it('takes the space between header and footer and clips instead of overflowing', () => {
    const style = mountList(10).attributes('style') ?? ''
    expect(style).toContain('flex: 1')
    expect(style).toContain('min-height: 0')
    expect(style).toContain('overflow: hidden')
  })
})

describe('TrackCardFooter: stays out of the way of the list', () => {
  const tracklist = { id: 'tl', title: 'Set', createdAt: '2026-10-02T00:00:00Z' } as Tracklist

  it('is part of the layout, not floated over it', () => {
    const style = mount(TrackCardFooter, { props: { tracklist, colors } }).attributes('style') ?? ''
    expect(style).not.toContain('position: absolute')
    expect(style).toContain('flex-shrink: 0')
  })

  it('says what made the card, with the domain as the highlight', () => {
    const w = mount(TrackCardFooter, { props: { tracklist, colors } })
    expect(w.text()).toBe('Made with klubhub.io')
    expect(w.find('span').text()).toBe('klubhub.io')
  })
})

describe('TrackCard: header, list and footer stack in a column', () => {
  it('lays its content out as a flex column inside the fixed 1080 x 1920 canvas', () => {
    const w = mount(TrackCard, { props: { colors }, slots: { default: '<p>content</p>' } })
    expect(w.attributes('style')).toContain('height: 1920px')
    const inner = w.find('p').element.parentElement!
    expect(inner.style.display).toBe('flex')
    expect(inner.style.flexDirection).toBe('column')
  })
})
