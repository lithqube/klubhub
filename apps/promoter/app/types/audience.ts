// Mirrors the Go API (api/internal/promoter/audience). Times are ISO-8601 instants.

export type ContactStatus = 'active' | 'unsubscribed' | 'bounced' | 'complained'
export type ContactSource = 'rsvp' | 'follow' | 'notify_me' | 'csv' | 'door'
export type ConsentBasis = 'consent' | 'soft_opt_in'

export interface ConsentInput {
  basis: ConsentBasis
  recorded_at?: string
  ip?: string
  form_text: string
  double_opt_in_confirmed_at?: string | null
}

export interface Contact {
  id: string
  name: string
  email: string
  phone: string
  status: ContactStatus
  source: ContactSource
  consent_basis: ConsentBasis
  consent_recorded_at: string
  consent_ip?: string
  consent_form_text: string
  double_opt_in_confirmed_at?: string | null
  created_at: string
  updated_at: string
}

export interface ContactInput {
  name: string
  email: string
  phone: string
  source: ContactSource
  status?: ContactStatus
  consent: ConsentInput
}

export interface StatusInput {
  status: ContactStatus
}

export interface Counts {
  all: number
  active: number
  unsubscribed: number
  bounced: number
  complained: number
}

export interface Page {
  contacts: Contact[]
  counts: Counts
}

export interface ImportRow {
  name?: string
  email?: string
  phone?: string
}

export interface ImportResult {
  added: number
  updated: number
  duplicates: number
  invalid: number
}

export interface SegmentFilter {
  status?: ContactStatus | ''
  source?: ContactSource | ''
  since_days?: number | null
}

export interface SegmentInput {
  name: string
  filter: SegmentFilter
}

export interface Segment {
  id: string
  name: string
  filter: SegmentFilter
  matching: number
  created_at: string
  updated_at: string
}

export const CONTACT_STATUSES: ContactStatus[] = ['active', 'unsubscribed', 'bounced', 'complained']
export const CONTACT_SOURCES: ContactSource[] = ['rsvp', 'follow', 'notify_me', 'csv', 'door']
