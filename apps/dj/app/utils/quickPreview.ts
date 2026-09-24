/**
 * Quick-preview readouts for the status bar.
 *
 * A pure derivation over the gig list and the social queue, so it can be tested
 * without a store, a DOM or a clock. Where the dashboard already has a number
 * (EARNED MTD = played gigs this month) the definition is the same; the rest
 * answers "what needs me next?": the next booked gig, money that is late or
 * outstanding, posts that failed.
 *
 * There is no invoice entity or due date in the data model yet (the finance
 * page is demo data), so money is read from each gig's `payment_status`. A
 * NEXT DUE readout belongs here once the finance phase adds invoices.
 */
import type { Gig } from '../types/gig'

export interface PreviewPost {
  status: string
  scheduledAt: string | null
}

export interface Money {
  amount: number
  currency: string
  /** More than one currency contributed; the amount shown is the largest one. */
  mixed: boolean
}

export interface QuickPreview {
  nextGig: { gig: Gig; day: Date; days: number } | null
  /** Played gigs in the current month. */
  earnedMtd: Money | null
  /** Upcoming confirmed or advanced gigs (inquiries are not money yet). */
  booked: Money | null
  payments: {
    overdueCount: number
    overdue: Money | null
    /** Played gigs still `unpaid`. `deposit_paid` is left out: the deposit is not stored, so the balance is unknown. */
    unpaidCount: number
    unpaid: Money | null
  }
  /** null = the queue could not be read, which is not the same as an empty queue. */
  social: { scheduled: number; failed: number; nextAt: string | null } | null
}

const DAY_MS = 86_400_000
const BOOKED_STATUSES = ['confirmed', 'advanced']
const SCHEDULED_STATUSES = ['scheduled', 'publishing']
const FAILED_STATUSES = ['failed', 'permanently_failed']

/** 'YYYY-MM-DD' or an ISO timestamp -> that calendar day at local midnight. */
function calendarDay(value: string | null | undefined): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(value ?? '')
  return m ? new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])) : null
}

// fee_amount is a decimal on the API and arrives as a string ("1500").
function sumMoney(gigs: Gig[]): Money | null {
  const totals = new Map<string, number>()
  for (const g of gigs) {
    const currency = g.fee_currency || 'EUR'
    totals.set(currency, (totals.get(currency) ?? 0) + (Number(g.fee_amount) || 0))
  }
  const largest = [...totals.entries()].sort((a, b) => b[1] - a[1])[0]
  if (!largest || largest[1] <= 0) return null
  return { amount: largest[1], currency: largest[0], mixed: totals.size > 1 }
}

export function buildQuickPreview(input: {
  gigs: Gig[]
  /** null when the social queue could not be read. */
  posts: PreviewPost[] | null
  now: Date
}): QuickPreview {
  const { gigs, posts, now } = input
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const live = gigs.filter((g) => g.status !== 'cancelled')

  const upcoming = live
    .filter((g) => BOOKED_STATUSES.includes(g.status))
    .map((gig) => ({ gig, day: calendarDay(gig.date) }))
    .filter((x): x is { gig: Gig; day: Date } => x.day !== null && x.day >= today)
    .sort((a, b) => a.day.getTime() - b.day.getTime())
  const next = upcoming[0]

  const thisMonth = (g: Gig) => {
    const d = calendarDay(g.date)
    return d !== null && d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth()
  }
  const overdue = live.filter((g) => g.payment_status === 'overdue')
  const unpaid = live.filter((g) => g.status === 'played' && g.payment_status === 'unpaid')

  let social: QuickPreview['social'] = null
  if (posts !== null) {
    const scheduled = posts.filter((p) => SCHEDULED_STATUSES.includes(p.status))
    const times = scheduled
      .map((p) => (p.scheduledAt ? new Date(p.scheduledAt).getTime() : NaN))
      .filter((t) => !Number.isNaN(t))
      .sort((a, b) => a - b)
    social = {
      scheduled: scheduled.length,
      failed: posts.filter((p) => FAILED_STATUSES.includes(p.status)).length,
      nextAt: times.length > 0 ? new Date(times[0]!).toISOString() : null,
    }
  }

  return {
    nextGig: next
      ? { gig: next.gig, day: next.day, days: Math.round((next.day.getTime() - today.getTime()) / DAY_MS) }
      : null,
    earnedMtd: sumMoney(live.filter((g) => g.status === 'played' && thisMonth(g))),
    booked: sumMoney(upcoming.map((x) => x.gig)),
    payments: {
      overdueCount: overdue.length,
      overdue: sumMoney(overdue),
      unpaidCount: unpaid.length,
      unpaid: sumMoney(unpaid),
    },
    social,
  }
}

/** Whole-euro style amounts; a trailing "+" marks a total that hides a second currency. */
export function formatMoney(money: Money | null): string {
  if (!money) return '—'
  let text: string
  try {
    text = new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: money.currency,
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(money.amount)
  } catch {
    // Intl throws on a malformed currency code; keep the number readable.
    text = `${Math.round(money.amount)} ${money.currency}`
  }
  return money.mixed ? `${text}+` : text
}

/** 'OCT 17' */
export function formatDayLabel(day: Date): string {
  return day.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }).toUpperCase()
}

/** 'TODAY' | 'TOMORROW' | 'IN 23D' */
export function countdownLabel(days: number): string {
  if (days <= 0) return 'TODAY'
  if (days === 1) return 'TOMORROW'
  return `IN ${days}D`
}
