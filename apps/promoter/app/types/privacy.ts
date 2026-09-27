// Mirrors the Go API (P2.5 privacy and retention). Times are ISO-8601 instants.

export type PurgeTrigger = 'schedule' | 'manual'

/** An event whose guest data is waiting to be erased. */
export interface RetentionUpcoming {
  event_id: string
  title: string
  ends_at: string
  /** ends_at + retention at scheduling time (recomputed until purged). */
  purge_after: string
}

/** An event whose guest data was erased. */
export interface RetentionRecent {
  event_id: string
  title: string
  purged_at: string
  trigger: PurgeTrigger
  /** Rows anonymised per table (guests, orders, order_positions, …). */
  counts: Record<string, number>
}

/** GET /api/v1/org/retention */
export interface RetentionOverview {
  /** 1..365; 30 when never set. */
  retention_days: number
  /** Next 20 by purge_after. */
  upcoming: RetentionUpcoming[]
  /** Last 20. */
  recent: RetentionRecent[]
}

/** GET /api/v1/events/{eventID}/privacy */
export interface EventPrivacy {
  purge_after: string | null
  purged_at: string | null
  retention_days: number
  /** Non-purged guests + ticket positions that still hold personal data. */
  personal_rows: number
}

/** POST /api/v1/events/{eventID}/purge — the body is not part of the contract; read defensively. */
export interface PurgeResponse {
  purged_at?: string
  counts?: Record<string, number>
}
