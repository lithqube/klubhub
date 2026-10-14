# Finance hardening execution plan

Branch: `fix/finance-hardening`; worktree `/Users/admin/dev/klubhub-finance`.
Baseline: `84b75d74f832712ff5b4e14ed6d3e3db030faed8` (merged PR #40).
The original `/Users/admin/dev/klubhub` checkout is out of scope; preserve all its WIP.

## Scope and execution
Address the 19 audit findings, starting with six HIGH defects. No automatic feature expansion into OCR, accounting exports, e-invoicing or tax certification. No pushes, PR publication, merges, real email sends or production restore. Product-contract changes require documented semantics rather than invented tax/accounting rules.

Wave A: backend currency and gig-payment correctness (BE-01/02/03); frontend kind and edit-version integrity (FE-1/2); safe isolated restore (SR-01). Three disjoint ownership tracks. Review each track for spec compliance then code/security quality; parent independently verifies.
Wave B: remaining backend and frontend findings, frontend operation lifetimes/list/detail/aggregate ordering, and checked money arithmetic. Keep backend and UI contract changes coordinated.
Wave C: origin/content-type boundaries and durable email/document workflows. Prefer explicit rejection to falsely successful unsupported attachments or unresolved documents. Validate provider contracts from official current documentation before implementation; never guess them.

## Mandatory acceptance
Strict RED -> GREEN per behavior; regression tests assert desired behavior, not the old audit defect. Include real migrated disposable PostgreSQL tests for ledger/payment/concurrency, real-Pinia mounted deferred transport tests for editor/list races, and non-production isolated restore drills. Preserve original edit field/version snapshots; explicit discard/reload after conflicts. Never substitute cache timestamps. No tests/builds with masked exit codes. Record all evidence outside product credentials.

Build/vet/race Go gates run from `api/`; prepare both Nuxt projects before frontend gates; run full DJ tests, scoped lint and Nuxt-aware typecheck; compare inherited full-project diagnostics with baseline, do not silently exclude errors. No success claims about unexecuted Garage/Plunk/browser acceptance. No live production writes.

## Finding checklist
- [ ] BE-01 (HIGH, Wave A): Invoice gig-fee conversion hardcodes two decimal places
- [ ] BE-02 (HIGH, Wave A): Per-invoice balance overwrites shared gig payment state
- [ ] BE-03 (HIGH, Wave A): Payment hook consumes stale unlocked gig snapshot
- [ ] BE-04 (MEDIUM, follow-up): Credit or correction prevents recording subsequent refunds
- [ ] BE-05 (MEDIUM, follow-up): Accepted ledger amounts can overflow report result types
- [ ] BE-06 (MEDIUM, follow-up): Generated-source lookup is ambiguous after void/re-pay replacement
- [ ] BE-07 (MEDIUM, follow-up): Concurrent generated-income unique violation recovery queries an aborted transaction
- [ ] FE-1 (HIGH, Wave A): Editing an entry borrows the last create kind and can reverse expense into income
- [ ] FE-2 (HIGH, Wave A): Entry and invoice saves combine stale form fields with a fresh cache token
- [ ] FE-3 (MEDIUM, follow-up): An old entry save closes a newly reopened dialog
- [ ] FE-4 (MEDIUM, follow-up): Entry list responses can roll back filters or resurrect confirmed deletion
- [ ] FE-5 (MEDIUM, follow-up): Late invoice detail response blanks the newly selected invoice sheet
- [ ] FE-6 (MEDIUM, follow-up): Selecting a historical P&L year removes newer years from the picker
- [x] SR-01 (HIGH, Wave A): Restore cannot recover the current stack — SPEC PASS and QUALITY APPROVED; real PostgreSQL/Garage destructive-loss and same-size/newer-object recovery verified against retained current-source ledger.
- [ ] SR-02 (MEDIUM, follow-up): DJ finance accepts cross-origin simple POST without CSRF enforcement
- [ ] SR-03 (MEDIUM, follow-up): Email retry logic is not wired into runtime and initial attempts are not persisted
- [ ] SR-04 (MEDIUM, follow-up): Email attachment IDs are accepted and silently omitted during delivery
- [ ] SR-05 (MEDIUM, follow-up): Malformed HTTP 2xx provider acknowledgment is accepted as delivery success
- [ ] SR-06 (MEDIUM, follow-up): Agreement GeneratePDF stores unresolved markdown and does not link document to agreement

## Evidence ledger
Pending: implementation, spec review, quality review, parent regression and integration verification.
