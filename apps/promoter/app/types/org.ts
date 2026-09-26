/** Public collective profile used by the export pack (P1.5). */
export interface OrgProfile {
  bio: string
  website_url: string | null
  instagram_url: string | null
  soundcloud_url: string | null
  ra_url: string | null
  /** `#rrggbb`, lowercase; null = the Kinetic HUD cyan. */
  accent_color: string | null
}

/** The collective / organisation that owns this Promoter workspace (tenant). */
export interface Organization extends OrgProfile {
  id: string
  name: string
  slug: string
  /** IANA timezone used for event times unless a venue overrides it. */
  timezone: string
  /** ISO 4217 default currency for budgets and tickets. */
  currency: string
}
