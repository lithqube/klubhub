import { describe, expect, it } from 'vitest'
import type { ExportData } from '~/types/event'
import { embedSnippet, platformFields, staticPage } from '../exportPack'

const x: ExportData = {
  event: {
    id: 'e1', title: 'Klubnacht <03>', slug: 'klubnacht-03', status: 'published', visibility: 'public', publish_at: null,
    starts_at: '2026-10-03T21:00:00Z', ends_at: '2026-10-04T05:00:00Z', doors_at: null, timezone: 'Europe/Berlin', venue_id: 'v',
    city: 'Berlin', location_mode: 'secret', location_reveal_at: '2026-10-03T10:00:00Z', min_age: 18, genres: ['techno'],
    description_md: '**Eight hours**', cost_text: '€20', external_ticket_url: 'https://t.example/k', capacity: null, version: 2,
    created_at: '', updated_at: '',
  },
  lineup: [{ id: 'l', stage_id: null, display_name: 'Ben Klock', profile_url: null, billing_order: 0, b2b_group: null, set_start: null, set_end: null }],
  stages: [], organizer: 'Nachtwerk', embargoed: false,
  organizer_profile: { name: 'Nachtwerk', url: 'https://nachtwerk.example', same_as: ['javascript:alert(1)', 'https://instagram.com/nachtwerk'], accent_color: '#c3a9ff' },
  location: { name: 'Berlin — location TBA', city: 'Berlin', withheld: true, reveal_at: '2026-10-03T10:00:00Z' },
}

describe('export pack', () => {
  it('marks withheld venues instead of dropping them', () => {
    const venue = platformFields(x, 'ra').find(f => f.label === 'VENUE')!
    expect(venue.withheld).toBe(true)
    expect(venue.value).toBe('Berlin — location TBA')
  })

  it('escapes HTML in the static page and embed, and inlines JSON-LD safely', () => {
    const html = staticPage(x, { name: '</script><script>alert(1)</script>' })
    expect(html).toContain('Klubnacht &lt;03&gt;')
    expect(html).not.toContain('</script><script>alert(1)')
    expect(embedSnippet(x)).toContain('Klubnacht &lt;03&gt;')
    expect(html).toContain('23:00 → 07:00 +1')
  })
})

describe('export pack links', () => {
  it('drops non-http ticket links from generated HTML', () => {
    const bad = { ...x, event: { ...x.event, external_ticket_url: 'javascript:alert(1)' } }
    expect(staticPage(bad, null)).not.toContain('javascript:')
    expect(embedSnippet(bad)).not.toContain('javascript:')
  })

  it('keeps unlisted pages out of search engines', () => {
    expect(staticPage({ ...x, event: { ...x.event, visibility: 'unlisted' } }, null)).toContain('noindex')
    expect(staticPage(x, null)).not.toContain('noindex')
  })
})

describe('export pack profile', () => {
  it('uses the collective accent and only safe profile links', () => {
    const html = staticPage(x, null)
    expect(html).toContain('background:#c3a9ff')
    expect(html).toContain('href="https://instagram.com/nachtwerk"')
    expect(html.match(/javascript:/g)).toBeNull()
    expect(platformFields(x, 'generic').at(-1)?.value).toBe('Nachtwerk · https://nachtwerk.example')
  })

  it('falls back to the default accent for malformed colours', () => {
    expect(embedSnippet({ ...x, organizer_profile: { name: 'N', accent_color: 'red;background:url(x)' } })).toContain('#96f8ff')
  })
})
