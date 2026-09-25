// Nitro adapter for the in-memory invoicing mock (frontend-only dev). The
// rules live in shared/finance-mock and are shared with the browser demo
// (app/demo). The leading "-" keeps Nitro from treating this file as a
// route; it lives under server/api/v1 so production builds drop it together
// with the other mocks (scripts/build-prod.mjs).

import type { H3Event } from 'h3'
import { FinanceMockDb, type MockGig } from '../../../../shared/finance-mock/db'
import type { MockResult } from '../../../../shared/finance-mock/rules'
import { party } from '../../../../shared/finance-mock/rules'
import { FINANCE_GIG_IDS, seedInvoices, seedMockGigs } from '../../../../shared/finance-mock/seed'
import { parseMoney } from '../../../../app/utils/money'

export const GIG_IDS = FINANCE_GIG_IDS

// The dev mock pins "today" so its seeded dates never move.
const TODAY = '2026-09-25'

export const gigs = new Map<string, MockGig>(seedMockGigs(TODAY).map((g) => [g.id, g]))

export const db = new FinanceMockDb({
  lookupGig: (id) => gigs.get(id),
  clockStart: Date.parse(`${TODAY}T09:00:00Z`),
})
seedInvoices(db, (id) => gigs.get(id)!.date)

/**
 * Learns a real gig (fee, currency, date, promoter) through the gig route,
 * which proxies to the Go API when one is running. Without it the gig
 * stays unknown and gets placeholder fee info.
 */
export async function learnGig(id: string): Promise<void> {
  if (gigs.has(id)) return
  try {
    const res = await $fetch<unknown>(`/api/v1/gigs/${id}`)
    const g = ((res && typeof res === 'object' && 'data' in res ? (res as { data: unknown }).data : res) ?? {}) as Record<string, unknown>
    if (!g.id) return
    const currency = String(g.fee_currency || 'EUR')
    gigs.set(id, {
      id,
      date: String(g.date ?? '').slice(0, 10) || new Date().toISOString().slice(0, 10),
      label: String(g.event_name || g.venue || 'Gig'),
      fee_minor: parseMoney(String(g.fee_amount ?? 0), currency) ?? 0,
      currency,
      customer: party({ legal_name: String(g.promoter_name ?? ''), email: String(g.promoter_email ?? '') }),
    })
  } catch {
    // No backend (or no such gig): the db falls back to placeholder fee info.
  }
}

/** Writes a MockResult to the event: status code + JSON body. */
export function send(event: H3Event, r: MockResult) {
  setResponseStatus(event, r.status)
  return r.body
}

export const idOf = (event: H3Event) => getRouterParam(event, 'id') ?? ''

export async function bodyOf(event: H3Event): Promise<Record<string, unknown>> {
  return ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
}
