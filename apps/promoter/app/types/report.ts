// Mirrors the P2.4 post-event report contract (GET /events/{id}/report).
// Aggregates only: no guest names, emails or phones; allocation submitter
// labels are the one name-like field (public class, as in P2.1).
import type { ListType } from '~/types/guest'

export interface ReportEvent {
  id: string
  title: string
  starts_at: string
  ends_at: string
  timezone: string
  capacity: number | null
}

export interface ReportTotals {
  guests_going: number
  guests_arrived: number
  /** Going guests not arrived / going guests; null when nobody is going. */
  no_show_rate: number | null
  heads_expected: number
  heads_admitted: number
  plus_ones_allowed: number
  plus_ones_used: number
  tickets_valid: number
  tickets_scanned: number
  walkups: number
  peak_occupancy: number
  peak_at: string | null
  conflicts: number
}

export interface ReportListRow {
  list_id: string
  name: string
  type: ListType
  going: number
  arrived: number
  no_show_rate: number | null
  heads_expected: number
  heads_admitted: number
  plus_ones_allowed: number
  plus_ones_used: number
}

export interface ReportSubmitterRow {
  allocation_id: string
  list_id: string
  list_name: string
  list_type: ListType
  submitter: string
  quota: number
  going: number
  arrived: number
  no_show_rate: number | null
  heads_admitted: number
  revoked: boolean
}

export interface ReportTicketType {
  ticket_type_id: string
  name: string
  valid: number
  scanned: number
}

/** One 15-minute bucket; occupancy is the value at the end of the bucket. */
export interface CurvePoint {
  bucket_start: string
  in: number
  out: number
  walkups: number
  occupancy: number
}

export interface EventReport {
  event: ReportEvent
  generated_at: string
  /** true while now < ends_at */
  live: boolean
  totals: ReportTotals
  by_list: ReportListRow[]
  by_submitter: ReportSubmitterRow[]
  tickets_by_type: ReportTicketType[]
  curve: CurvePoint[]
}
