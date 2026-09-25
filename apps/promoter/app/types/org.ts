/** The collective / organisation that owns this Promoter workspace (tenant). */
export interface Organization {
  id: string
  name: string
  slug: string
  /** IANA timezone used for event times unless a venue overrides it. */
  timezone: string
  /** ISO 4217 default currency for budgets and tickets. */
  currency: string
}
