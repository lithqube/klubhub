// Phase 5 earnings store front door (`useEarningsStore`). Mirrors the
// pattern of `stores/invoice.ts` — same module name, same Pinia setup store
// convention, shared `FinanceApiError` from invoice.ts. The implementation
// lives in `earnings.ts` (kept there for test discoverability); this file is
// the canonical Phase-5 surface consumed by finance.vue and GigFormDialog.
//
// Endpoints exposed by /api/v1/finance/*:
//   GET    /entries              list with kind/status/currency/category/gig_id/from/to
//   POST   /entries              create income/expense
//   GET    /entries/:id          fetch one
//   PUT    /entries/:id          full replace (requires updated_at token)
//   DELETE /entries/:id          soft delete (requires updated_at token)
//   POST   /entries/:id/void     mark voided (FIN-10 friendly)
//   GET    /summary              ?scope=month&year&month OR ?from&to
//   GET    /profit-loss          ?scope=gig&gig_id OR ?scope=month|year
//   GET    /reconciliations      ?gig_id=
//   POST   /reconciliations/:id/resolve   FIN-04 / FIN-05 resolution

export { useEarningsStore } from './earnings'
export type { SummaryScopeArg, ProfitLossArg } from './earnings'
