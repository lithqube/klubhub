// Fictional profile, press kit and social queue for "Sam Example".

import type { EPKContent } from '../../types/epk'
import type { SocialAccount } from '../../types/social'
import type { DemoPost } from '../types'
import { photoSvg, svgDataUrl } from '../lib/art'
import { daysFrom } from '../lib/util'
import { DEFAULT_VISIBLE_FIELDS } from './tracklists'

export const DEMO_ACCOUNT_ID = '00000000-0000-4000-8000-000000000601'

export function seedSettings(now: Date): Record<string, unknown> {
  const stamp = daysFrom(now, -90, 12)
  return {
    id: '00000000-0000-4000-8000-000000000001',
    dj_name: 'Sam Example',
    logo_path: null,
    logo_position: 'top-left',
    custom_placeholder_path: null,
    preset: 'default',
    bg_mode: 'solid',
    bg_value: null,
    visible_fields: [...DEFAULT_VISIBLE_FIELDS],
    max_tracks: 50,
    track_range_start: null,
    track_range_end: null,
    default_colors: {},
    default_template: 'default',
    social_links: {
      instagram: 'https://example.com/instagram/sam.example',
      soundcloud: 'https://example.com/soundcloud/sam-example',
      website: 'https://sam.example.com',
    },
    bio_short: '',
    bio_long: '',
    contact_info: 'bookings@example.com',
    invoice_prefix: 'INV',
    created_at: stamp,
    updated_at: stamp,
  }
}

export function seedEpk(now: Date): { epk: EPKContent; assets: Record<string, string> } {
  const assets: Record<string, string> = {}
  const photoPaths = [1, 2, 3].map((n) => {
    const path = `demo/epk/photos/press-${n}.svg`
    assets[path] = svgDataUrl(photoSvg(`SAM EXAMPLE · PRESS ${n}`, `press-${n}`))
    return path
  })
  const stamp = daysFrom(now, -30, 12)
  return {
    assets,
    epk: {
      id: '00000000-0000-4000-8000-000000000701',
      bioShort: 'Sam Example is a fictional DJ who plays warm, driving techno and deep house. This press kit is demo content.',
      bioLong: 'Sam Example is a fictional selector created for the KlubHub DJ demo. Known for long, patient warm-up sets that '
        + 'build into peak-time techno, Sam has held a monthly residency at Club Alpha and played warehouse and open-air '
        + 'events across the (equally fictional) Alpha–Zeta circuit.\n\nEverything on this page — names, venues, quotes and '
        + 'photos — is placeholder content. Edit any field: your changes stay in this browser.',
      techRider: '2 × media players (USB, latest firmware)\n1 × four-channel club mixer\n2 × booth monitors, independent level control\n'
        + 'Table at waist height, stable, away from subwoofers\nPower strip at the booth',
      stagePlotPath: '',
      gigHighlights: [
        'Club Alpha · Berlin, DE · Monthly residency',
        'Club Beta · Paris, FR · Techno Night',
        'Gamma Hall · New York, US · Sunday Party',
        'Epsilon Festival · Amsterdam, NL',
      ],
      pressQuotes: [
        { text: 'A patient, hypnotic selector who never rushes the room.', source: 'Example Magazine' },
        { text: 'Sam Example turned a quiet Tuesday into the night of the month.', source: 'Sample Weekly' },
      ],
      photoPaths,
      photoUrls: Object.fromEntries(photoPaths.map((p) => [p, assets[p]!])),
      sectionVisibility: {},
      createdAt: daysFrom(now, -120, 12),
      updatedAt: stamp,
    },
  }
}

export function seedAccount(now: Date): SocialAccount {
  return {
    id: DEMO_ACCOUNT_ID,
    platform: 'instagram',
    igUserId: '0000000000',
    accountName: 'sam.example',
    tokenExpiry: daysFrom(now, 55),
    status: 'connected',
    createdAt: daysFrom(now, -100, 10),
    updatedAt: daysFrom(now, -5, 10),
  }
}

export function seedPosts(now: Date): DemoPost[] {
  const post = (n: number, p: Partial<DemoPost>): DemoPost => ({
    id: `00000000-0000-4000-8000-00000000080${n}`,
    accountId: DEMO_ACCOUNT_ID,
    status: 'scheduled',
    postType: 'feed',
    caption: '',
    imageMinioPath: `demo/social/post-${n}.svg`,
    scheduledAtUtc: daysFrom(now, 1, 18),
    timezoneName: 'Europe/Berlin',
    retryCount: 0,
    nextRetryAt: null,
    lastError: '',
    createdAt: daysFrom(now, -2, 10),
    updatedAt: daysFrom(now, -2, 10),
    ...p,
  })
  return [
    post(1, { caption: 'Warehouse Session 03 — 14 tracks from last weekend. Full list in the tracklist card. #techno #djset', scheduledAtUtc: daysFrom(now, 2, 19, 30) }),
    post(2, { postType: 'story', caption: 'Next up: Resident Night at Club Alpha. See you on the floor.', scheduledAtUtc: daysFrom(now, 6, 17) }),
    post(3, { status: 'draft', caption: 'Open Air Sunset Set — thank you Zeta Garden. #openair #house', scheduledAtUtc: daysFrom(now, 9, 12) }),
    post(4, { status: 'failed', caption: 'Studio warm-up session, new edits incoming.', scheduledAtUtc: daysFrom(now, -1, 18), retryCount: 2,
      nextRetryAt: daysFrom(now, 0, 23), lastError: 'Demo: simulated rate limit from Instagram. Press retry to publish.' }),
    post(5, { status: 'published', caption: 'Thank you Club Beta — what a night. #technonight', scheduledAtUtc: daysFrom(now, -5, 12), updatedAt: daysFrom(now, -5, 12) }),
  ]
}
