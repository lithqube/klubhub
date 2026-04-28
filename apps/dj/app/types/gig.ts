export type GigStatus = 'inquiry' | 'confirmed' | 'advanced' | 'played' | 'cancelled'
export type PaymentStatus = 'unpaid' | 'deposit_paid' | 'paid' | 'overdue' | 'waived'

export interface Gig {
  id: string
  date: string
  venue: string
  city: string
  country: string
  event_name: string
  promoter_name: string
  promoter_email: string
  promoter_phone: string
  fee_amount: number
  fee_currency: string
  set_length_minutes: number
  notes: string
  status: GigStatus
  payment_status: PaymentStatus
  gig_reader_venue_id: string | null
  gig_reader_contact_id: string | null
  created_at: string
  updated_at: string
  deleted_at: string | null
}

export interface GigCreate {
  date: string
  venue: string
  city: string
  country: string
  event_name: string
  promoter_name: string
  promoter_email: string
  promoter_phone: string
  fee_amount: number
  fee_currency: string
  set_length_minutes?: number
  notes?: string
  status?: GigStatus
  payment_status?: PaymentStatus
}

export interface GigUpdate {
  date?: string
  venue?: string
  city?: string
  country?: string
  event_name?: string
  promoter_name?: string
  promoter_email?: string
  promoter_phone?: string
  fee_amount?: number
  fee_currency?: string
  set_length_minutes?: number
  notes?: string
  status?: GigStatus
  payment_status?: PaymentStatus
  updated_at?: string
}

export interface Venue {
  id: string
  name: string
  city: string
  country: string
  capacity: number | null
  website: string | null
  tech_contact_name: string
  tech_contact_email: string
  tech_contact_phone: string
  notes: string
  created_at: string
  updated_at: string
  deleted_at: string | null
}

export interface VenueCreate {
  name: string
  city: string
  country: string
  capacity?: number
  website?: string
  tech_contact_name?: string
  tech_contact_email?: string
  tech_contact_phone?: string
  notes?: string
}

export interface Contact {
  id: string
  name: string
  company: string | null
  email: string
  phone: string
  type: ContactType
  notes: string
  created_at: string
  updated_at: string
  deleted_at: string | null
}

export type ContactType = 'promoter' | 'agent' | 'label' | 'other'

export interface ContactCreate {
  name: string
  company?: string
  email: string
  phone: string
  type: ContactType
  notes?: string
}

export interface GigFilter {
  status?: string
  venue?: string
  city?: string
  fee_min?: number
  fee_max?: number
  from?: string
  to?: string
}
