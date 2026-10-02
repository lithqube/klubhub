// Builds the first-visit demo state. Dates are relative to `now`.

import { FinanceMockDb } from '../../../shared/finance-mock/db'
import { seedFinanceEntries, seedInvoices } from '../../../shared/finance-mock/seed'
import { RiderMockDb } from '../../../shared/rider-mock/db'
import type { DemoState } from '../types'
import { financeLookup } from '../handlers/finance'
import { seedAccount, seedEpk, seedPosts, seedSettings } from './content'
import { seedGigs } from './gigs'
import { seedTracklists } from './tracklists'

export function createSeedState(now: Date): DemoState {
  const { tracklists, tracks } = seedTracklists(now)
  const { gigs, customers, venues, contacts, gigLinks } = seedGigs(now)
  const { epk, assets } = seedEpk(now)
  const state: DemoState = {
    version: 1,
    seededAt: now.toISOString(),
    settings: seedSettings(now),
    tracklists,
    tracks,
    account: seedAccount(now),
    posts: seedPosts(now),
    epk,
    exports: [],
    assets,
    gigs,
    gigLinks,
    customers,
    venues,
    contacts,
    finance: { invoices: [], lines: {}, payments: [], series: {}, entries: [], reconciliations: [], seriesEntries: {}, clock: 0 },
    rider: new RiderMockDb({ now: () => now, seed: true }).toJSON(),
  }
  const db = new FinanceMockDb({ lookupGig: financeLookup(state), clockStart: now.getTime() - 60 * 86400_000 })
  const gigDate = (id: string) => state.gigs.find((g) => g.id === id)!.date.slice(0, 10)
  seedInvoices(db, gigDate)
  seedFinanceEntries(db, gigDate)
  state.finance = db.toJSON()
  return state
}
