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

export interface RAImportResult {
  success: boolean
  artistSlug: string
  eventsImported: number
  eventsSkipped: number
  gigsCreated: number
  gigsSkipped: number
  gigIDs: string[]
  skippedReasons: string[]
  dryRun: boolean
}

export interface RAImportRequest {
  artist_slug: string
  venue_override?: string
  contact_override?: string
  dry_run?: boolean
}
