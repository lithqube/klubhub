// Phase 5 finance-mock contract: shared by Nitro dev mocks and the browser
// demo. Tests live under app/utils/__tests__/ so the existing vitest
// include `{src,app,tests}` picks them up.
import { describe, it, expect } from 'vitest'
import { FinanceMockDb, type FinanceSnapshot } from '../../../shared/finance-mock/db'
import * as ops from '../../../shared/finance-mock/ops'

function freshDb() {
  return new FinanceMockDb({
    lookupGig: () => ({
      id: 'gig-1',
      date: '2026-09-15',
      label: 'Test Gig',
      fee_minor: 100000,
      currency: 'EUR',
      customer: {} as never,
    }),
  })
}

describe('finance-mock entry ops', () => {
  it('createEntry validates required fields and persists with auto values', () => {
    const db = freshDb()
    const r = ops.createEntry(db, {
      kind: 'income',
      amount_minor: 100000,
      currency: 'EUR',
      category: 'gig_fee',
      entry_date: '2026-09-15',
      description: 'DJ set',
    })
    expect(r.status).toBe(201)
    const e = (r.body as { data: { id: string; auto_generated: boolean; source_kind: string } }).data
    expect(e.auto_generated).toBe(false)
    expect(e.source_kind).toBe('manual')
    expect(db.entries.size).toBe(1)
  })

  it('createEntry rejects bad currency / date / kind', () => {
    const db = freshDb()
    const r = ops.createEntry(db, {
      kind: 'income',
      amount_minor: -1,
      currency: 'eur',
      category: '',
      entry_date: '2026/09/15',
      description: '',
    })
    expect(r.status).toBe(400)
    expect((r.body as { error: string }).error).toBe('validation_failed')
  })

  it('updateEntry refuses auto-generated entries with 409 immutable', () => {
    const db = freshDb()
    const auto = db.addEntry({
      kind: 'income', amount_minor: 100, currency: 'EUR',
      category: 'gig_fee', entry_date: '2026-09-15', description: 'auto',
      auto_generated: true, source_kind: 'gig_payment',
    })
    const r = ops.updateEntry(db, auto.id, {
      kind: 'income', amount_minor: 1, currency: 'EUR',
      category: 'gig_fee', entry_date: '2026-09-15', description: 'x', notes: '',
      updated_at: auto.updated_at,
    })
    expect(r.status).toBe(409)
    expect((r.body as { error: string }).error).toBe('immutable')
  })

  it('deleteEntry soft-deletes and emits 204', () => {
    const db = freshDb()
    const e = db.addEntry({
      kind: 'expense', amount_minor: 5000, currency: 'EUR',
      category: 'gear', entry_date: '2026-09-15', description: '', notes: 'cable',
    })
    const r = ops.deleteEntry(db, e.id, { updated_at: e.updated_at })
    expect(r.status).toBe(204)
    expect(db.entries.get(e.id)?.deleted_at).not.toBeNull()
  })

  it('voidEntry flips status and survives a stale updated_at as 409', () => {
    const db = freshDb()
    const e = db.addEntry({
      kind: 'income', amount_minor: 100, currency: 'EUR',
      category: 'gig_fee', entry_date: '2026-09-15', description: 'x',
    })
    const r = ops.voidEntry(db, e.id, { updated_at: 'stale' })
    expect(r.status).toBe(409)
    const ok = ops.voidEntry(db, e.id, { updated_at: e.updated_at })
    expect(ok.status).toBe(200)
    expect(db.entries.get(e.id)?.status).toBe('voided')
  })

  it('summaryEntries groups active entries by currency, dropping voided / deleted', () => {
    const db = freshDb()
    db.addEntry({ kind: 'income', amount_minor: 100_000, currency: 'EUR', category: 'gig_fee', entry_date: '2026-09-15', description: '1' })
    db.addEntry({ kind: 'expense', amount_minor: 20_000, currency: 'EUR', category: 'gear', entry_date: '2026-09-15', description: '', notes: 'cable' })
    db.addEntry({ kind: 'income', amount_minor: 250_000, currency: 'USD', category: 'gig_fee', entry_date: '2026-09-15', description: 'us gig' })
    const voided = db.addEntry({ kind: 'income', amount_minor: 999_999, currency: 'EUR', category: 'gig_fee', entry_date: '2026-09-15', description: 'to void' })
    voided.status = 'voided'
    voided.updated_at = db.stamp()

    const r = ops.summaryEntries(db, { scope: 'year', year: 2026 })
    const body = r.body as { data: Record<string, { income_minor: number; expense_minor: number }> }
    expect(body.data.EUR?.income_minor).toBe(100_000)
    expect(body.data.EUR?.expense_minor).toBe(20_000)
    expect(body.data.USD?.income_minor).toBe(250_000)
  })

  it('profitLossScope computes currency-grouped profit / loss', () => {
    const db = freshDb()
    db.addEntry({ kind: 'income', amount_minor: 100_000, currency: 'EUR', category: 'gig_fee', entry_date: '2026-09-15', description: '1' })
    db.addEntry({ kind: 'expense', amount_minor: 30_000, currency: 'EUR', category: 'gear', entry_date: '2026-09-15', description: '', notes: 'cable' })
    const r = ops.profitLossScope(db, { scope: 'month', year: 2026, month: 9 })
    const body = r.body as { data: Record<string, { profit_loss_minor: number }> }
    expect(body.data.EUR?.profit_loss_minor).toBe(70_000)
  })

  it('getReconciliationByGig returns the durable pending row, 404 otherwise', () => {
    const db = freshDb()
    const gigId = '00000000-0000-4000-8000-000000000001'
    db.upsertReconciliation({
      gig_id: gigId, entry_id: 'ent-1', reason: 'fee_changed',
      allowed_actions: ['update', 'keep'], gig_amount_minor: 1, gig_currency: 'EUR',
      gig_payment_status: 'paid', entry_updated_at: 'x',
    })
    const hit = ops.getReconciliationByGig(db, { gig_id: gigId })
    expect(hit.status).toBe(200)
    const miss = ops.getReconciliationByGig(db, { gig_id: '00000000-0000-4000-8000-000000000999' })
    expect(miss.status).toBe(404)
  })

  it('getReconciliationByGig rejects non-UUID gig_id with 400', () => {
    const db = freshDb()
    const r = ops.getReconciliationByGig(db, { gig_id: 'not-a-uuid' })
    expect(r.status).toBe(400)
  })

  it('resolveReconciliation applies the action and rejects disallowed ones', () => {
    const db = freshDb()
    const entry = db.addEntry({
      kind: 'income', amount_minor: 1, currency: 'EUR',
      category: 'gig_fee', entry_date: '2026-09-15', description: 'auto',
      auto_generated: true, source_kind: 'gig_payment',
    })
    const rec = db.upsertReconciliation({
      gig_id: 'gig-1', entry_id: entry.id, reason: 'fee_changed',
      allowed_actions: ['update', 'keep'], gig_amount_minor: 5, gig_currency: 'USD',
      gig_payment_status: 'paid', entry_updated_at: entry.updated_at,
    })
    const conflict = ops.resolveReconciliation(db, rec!.id, { action: 'update', updated_at: 'stale' })
    expect(conflict.status).toBe(409)
    const disallowed = ops.resolveReconciliation(db, rec!.id, { action: 'delete', updated_at: rec!.updated_at })
    expect(disallowed.status).toBe(400)
    const ok = ops.resolveReconciliation(db, rec!.id, { action: 'update', updated_at: rec!.updated_at })
    expect(ok.status).toBe(200)
    expect(db.entries.get(entry.id)?.amount_minor).toBe(5)
    expect(db.entries.get(entry.id)?.currency).toBe('USD')
    expect(db.reconciliations.get(rec!.id)?.status).toBe('resolved')
  })

  it('upsertGigIncome is idempotent across calls with the same snapshot', () => {
    const db = freshDb()
    const a = db.upsertGigIncome({
      gigId: 'gig-1', amount_minor: 100, currency: 'EUR',
      entry_date: '2026-09-15', description: 'DJ set',
    })
    const b = db.upsertGigIncome({
      gigId: 'gig-1', amount_minor: 100, currency: 'EUR',
      entry_date: '2026-09-15', description: 'DJ set',
    })
    expect(a.created).toBe(true)
    expect(b.created).toBe(false)
    expect(db.entries.size).toBe(1)
  })

  it('processGigPaymentTransition raises reconciliation on fee shift', () => {
    const db = freshDb()
    db.upsertGigIncome({
      gigId: 'gig-1', amount_minor: 100, currency: 'EUR',
      entry_date: '2026-09-15', description: 'DJ set',
    })
    const result = db.processGigPaymentTransition({
      gig_id: 'gig-1', amount_minor: 150, currency: 'EUR',
      entry_date: '2026-09-15', description: 'DJ set',
      prev_payment_status: 'paid', next_payment_status: 'paid',
    })
    expect(result.metadata?.reason).toBe('fee_changed')
    expect(result.metadata?.allowed_actions).toEqual(['update', 'keep'])
  })

  it('processGigPaymentTransition raises a delete/void/keep prompt on payment revert', () => {
    const db = freshDb()
    db.upsertGigIncome({
      gigId: 'gig-1', amount_minor: 100, currency: 'EUR',
      entry_date: '2026-09-15', description: 'DJ set',
    })
    const result = db.processGigPaymentTransition({
      gig_id: 'gig-1', amount_minor: 100, currency: 'EUR',
      entry_date: '2026-09-15', description: 'DJ set',
      prev_payment_status: 'paid', next_payment_status: 'unpaid',
    })
    expect(result.metadata?.reason).toBe('payment_reversed')
    expect(result.metadata?.allowed_actions).toEqual(['delete', 'void', 'keep'])
  })

  it('snapshot round-trips through toJSON / load() with the new collections', () => {
    const db = freshDb()
    db.addEntry({ kind: 'income', amount_minor: 1, currency: 'EUR', category: 'gig_fee', entry_date: '2026-09-15', description: 'x' })
    db.upsertReconciliation({
      gig_id: 'gig-1', entry_id: 'ent-1', reason: 'fee_changed',
      allowed_actions: ['update', 'keep'], gig_amount_minor: 1, gig_currency: 'EUR',
      gig_payment_status: 'paid', entry_updated_at: 'x',
    })
    const snap: FinanceSnapshot = db.toJSON()
    expect(snap.entries.length).toBe(1)
    expect(snap.reconciliations.length).toBe(1)
    const other = new FinanceMockDb({ lookupGig: () => undefined, clockStart: snap.clock })
    other.load(snap)
    expect(other.entries.size).toBe(1)
    expect(other.reconciliations.size).toBe(1)
  })
})
