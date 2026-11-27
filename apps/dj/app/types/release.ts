// Types for the release planner (Phase 6). The shapes mirror the API
// payloads in api/internal/release.

export type ReleaseStatus =
  | 'demo_sent'
  | 'accepted'
  | 'mastering'
  | 'artwork'
  | 'promo'
  | 'released'

export type ReleaseFormat = 'digital' | 'vinyl' | 'both'

export type ReleasePlatform = 'beatport' | 'bandcamp' | 'spotify' | 'soundcloud'

export interface Release {
  id: string
  title: string
  artists: string
  label: string
  distributor: string
  release_date: string
  format: ReleaseFormat
  notes: string
  status: ReleaseStatus
  created_at?: string
  updated_at?: string
}

export interface ReleaseInput {
  title: string
  artists?: string
  label?: string
  distributor?: string
  release_date: string
  format?: ReleaseFormat
  notes?: string
}

export interface Deadline {
  id: string
  release_id: string
  name: string
  due_date: string
  completed: boolean
  created_at?: string
  updated_at?: string
}

export interface PlatformLink {
  id: string
  release_id: string
  platform: ReleasePlatform
  url: string
  created_at?: string
  updated_at?: string
}

export interface ChecklistItem {
  id: string
  release_id: string
  position: number
  label: string
  done: boolean
  created_at?: string
  updated_at?: string
}

// The workflow order of REL-02. A release moves one step forward or one
// step back; 'released' is terminal.
export const RELEASE_STATUS_ORDER: ReleaseStatus[] = [
  'demo_sent',
  'accepted',
  'mastering',
  'artwork',
  'promo',
  'released',
]

export const RELEASE_STATUS_LABELS: Record<ReleaseStatus, string> = {
  demo_sent: 'Demo sent',
  accepted: 'Accepted',
  mastering: 'Mastering',
  artwork: 'Artwork',
  promo: 'Promo',
  released: 'Released',
}

export const RELEASE_PLATFORM_LABELS: Record<ReleasePlatform, string> = {
  beatport: 'Beatport',
  bandcamp: 'Bandcamp',
  spotify: 'Spotify',
  soundcloud: 'SoundCloud',
}

/** The status a release may move to next, or null when it is terminal. */
export function nextReleaseStatus(status: ReleaseStatus): ReleaseStatus | null {
  const index = RELEASE_STATUS_ORDER.indexOf(status)
  if (index < 0 || index >= RELEASE_STATUS_ORDER.length - 1) return null
  return RELEASE_STATUS_ORDER[index + 1]
}

/** The status a release may fall back to, or null at the start. */
export function previousReleaseStatus(status: ReleaseStatus): ReleaseStatus | null {
  const index = RELEASE_STATUS_ORDER.indexOf(status)
  if (index <= 0) return null
  return RELEASE_STATUS_ORDER[index - 1]
}

/** Whether a release is past its date and not out yet. */
export function isReleaseOverdue(release: Release, now = new Date()): boolean {
  if (release.status === 'released') return false
  const due = new Date(release.release_date)
  if (Number.isNaN(due.getTime())) return false
  return due.getTime() < now.getTime()
}
