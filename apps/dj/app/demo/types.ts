// Types for the in-browser demo backend (docs/DEMO.md).

import type { FinanceMockDb, FinanceSnapshot } from '../../shared/finance-mock/db'
import type { EPKContent, EPKExport } from '../types/epk'
import type { Party } from '../types/finance'
import type { Contact, Gig, Venue } from '../types/gig'
import type { ScheduledPost, SocialAccount } from '../types/social'
import type { Track, Tracklist } from '../types/tracklist'

export interface DemoRequest {
  method: string
  /** Path starting at /api/v1/, without the app base URL or query. */
  path: string
  query: URLSearchParams
  /** Parsed JSON, FormData, raw text, or null. */
  body: unknown
}

export interface DemoResponse {
  status: number
  /** JSON body (ignored when `blob` is set). */
  body?: unknown
  blob?: Blob
  headers?: Record<string, string>
}

export interface GigLinks {
  venues: { venue_id: string; is_primary: boolean }[]
  contacts: { contact_id: string; role: string }[]
  tracklists: string[]
}

export type DemoPost = ScheduledPost & { imageData?: string }

/** Everything the demo persists in localStorage. */
export interface DemoState {
  version: 1
  seededAt: string
  settings: Record<string, unknown>
  tracklists: Tracklist[]
  tracks: Record<string, Track[]>
  account: SocialAccount | null
  posts: DemoPost[]
  epk: EPKContent
  exports: EPKExport[]
  /** Storage key → data URL (uploaded photos, logos, artwork). */
  assets: Record<string, string>
  gigs: Gig[]
  gigLinks: Record<string, GigLinks>
  /** Billing party per gig (invoice customer defaults). */
  customers: Record<string, Party>
  venues: Venue[]
  contacts: Contact[]
  finance: FinanceSnapshot
}

export interface DemoContext {
  state: DemoState
  finance: FinanceMockDb
  now: () => Date
  /** App base URL, e.g. "/demo/". */
  baseURL: string
  /** Creates an object URL for a generated file (window.open / download). */
  objectUrl: (blob: Blob) => string
  /** Looks up a blob created with objectUrl() in this session. */
  blobFor: (url: string) => Blob | undefined
}

export type Params = Record<string, string>
export type Handler = (req: DemoRequest & { params: Params }, ctx: DemoContext) => DemoResponse | Promise<DemoResponse>
