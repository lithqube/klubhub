// RA (Resident Advisor) TypeScript types for KlubHub DJ

export interface RAArtist {
  id: string
  name: string
  slug: string
  url: string
  biography: string // blurb text
  aliases: string[]
  facebook: string
  twitter: string
  instagram: string
  soundcloud: string
  bandcamp: string
  discogs: string
  website: string
  areas: RAArea[]
  venues: RAArtistVenue[]
  followers: number
  headerImage: string
  profileImage: string
}

export interface RAArea {
  areaId: string
  areaName: string
  countryId: string
  countryUrl: string
}

export interface RAArtistVenue {
  venueId: string
  venueName: string
}

export interface RAEVENT {
  id: string
  title: string
  date: string // ISO format YYYY-MM-DD
  startTime: string
  endTime: string
  venueId: string
  venueName: string
  venueUrl: string
  artists: RAArtistRef[]
  hosts: RAArtistRef[]
  attending: number
  contentUrl: string // ticket link
  isPick: boolean
  isSoldOut: boolean
  promo: string
}

export interface RAArtistRef {
  artistId: string
  name: string
  url: string
  slug: string
}

// Mirrors the Go RAImportResult JSON exactly (snake_case). The previous
// camelCase version matched nothing the API sends, so every field read
// as undefined.
export interface RAImportResult {
  success: boolean
  artist_slug: string
  events_imported: number
  events_skipped: number
  gigs_created: number
  gigs_skipped: number
  gig_ids: string[]
  skipped_reasons: string[]
  dry_run: boolean
  /** Events that passed the import filters; a dry run returns them as the preview. */
  events: RAEVENT[]
}

export interface RAImportRequest {
  artist_slug: string
  venue_override?: string
  contact_override?: string
  dry_run?: boolean
  /** Only import these RA event IDs (the selection made in the preview). */
  event_ids?: string[]
}
