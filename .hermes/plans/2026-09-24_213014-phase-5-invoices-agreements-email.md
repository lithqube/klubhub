# Phase 5 — DJ Invoices, Event Agreements and Email Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** A DJ can turn a gig into an editable event agreement and invoice, generate durable PDFs, prepare an email to the promoter/booker, explicitly send it with attachments, and track payment without fabricated success states.

**Architecture:** Keep the existing Go handler → service → repository pattern, PostgreSQL for business records and delivery history, Garage S3 for immutable document versions, and Nuxt/Pinia for authoring and review. Finance owns invoices/payments; gig owns agreements; a small shared document/email layer handles exports and delivery without coupling the modules to a provider.

**Tech Stack:** Existing chi, pgx, goose, shopspring/decimal, go-pdf/fpdf, Garage via the existing S3 client, Nuxt/Vue/Pinia, Vitest and Playwright. Select an SMTP/MIME library only after checking maintenance, security and attachment support during implementation; no new dependency is approved by this document.

**Status:** Proposed next phase; planning only. Created 2026-09-24. Supersedes the vague Phase 5 scope for execution planning, not the historical release or connectivity plans.

---

## Priority and scope

Next product phase: **booking → agreement → invoice → email → payment**. Complete this vertical slice before Release Planner, Tour Manager or expanded analytics. Existing gig functionality is a dependency, not something to rebuild. Reusable rider-template management remains separate Phase 4.5; this phase can attach an existing exported rider/EPK without requiring that entire module.

Included:
- Structured DJ billing profile and recipient details, separate from public artist biography.
- Gig-linked invoices, deposits and balance invoices; standalone invoices supported by the same editor.
- Gig-linked event agreement drafts and immutable issued revisions.
- PDF preview, download, stored versions, and document history.
- Editable email subject/body/recipients, attachment preview, explicit Send, delivery-attempt history.
- Manual payment recording, partial payments, overdue derivation, basic income/expense records, per-currency summaries.
- Labelled dev/staging fixtures; no production mock fallback or fixture leakage.

Excluded:
- E-signature provider, automatic legal acceptance, public signing portal.
- Automatic tax advice, filings, universal legal/tax compliance, structured e-invoice standards.
- Bank reconciliation, payment gateway, live FX conversion, full bookkeeping system.
- Bulk marketing, automatic reminders and unsolicited sends.
- Public hosting of private financial documents or a new multi-tenant platform.

PDF/email generation is a product feature, not a claim of legal or tax compliance. Jurisdiction-specific invoice fields, tax treatments and agreement wording require a separate validation gate before real use. Do not infer jurisdiction from the user's estimated location.

## Evidence and starting-state caveats

**Verified from repository reads / supplied source context:**
- `.planning/ROADMAP.md:217-225` marks Finance Phase 5 not started, dependent on Phase 4, with plans TBD.
- `api/go.mod:5-19` already declares fpdf, decimal, pgx, goose and testcontainers dependencies.
- `apps/dj/project.json:5-10` defines a build target using `apps/dj/scripts/build-prod.mjs`.
- Existing booking PDF implementation at `api/internal/gig/pdf.go` is a booking confirmation, not an agreement/invoice. The inspected handler at `api/internal/gig/handler.go:413` passes literal `DJ Name`; previous chat claims that this was fixed are not reliable evidence.
- Existing settings model includes `DJName`, `ContactInfo`, `InvoicePrefix`, but not a structured billing profile (`api/internal/settings/model.go`).
- Current `main` has uncommitted UI/design changes. Preserve them; do not reset, stash or commit unrelated work automatically.
- `CLAUDE.md:93-94` warns that typecheck has been a no-op. A target saying success is not proof of type safety.

**Suspicion / revalidation required at execution start:**
- Re-read current gig PDF/service/storage/handler and finance page before edits; earlier snippets may differ from HEAD.
- Verify actual email integrations, middleware, migrations and generated Nx targets; absence in earlier searches is not definitive.
- Existing roadmap has conflicting summary/detail statuses. Do not use checkboxes as proof that runtime flows work.

**Open before production use:** DJ billing jurisdiction/tax treatment, approved agreement wording, sender mailbox/domain and SMTP credentials. Development can proceed with explicit test data and a local mail capture service; no real recipient sends.

## Product workflows and invariants

### Invoice
1. Open gig → Create invoice; copy event and recipient details into a draft snapshot.
2. Review legal/billing identity, billing address, event/service date, line items, currency, tax treatment, payment instructions, issue/due dates and notes.
3. Save draft → preview watermarked PDF → explicitly Issue.
4. Issue validates required configuration, assigns an invoice number atomically and freezes financial/party fields. Export references this immutable version, not current gig/settings values.
5. Download or Prepare email; choose promoter/booker/other contact explicitly and review address, subject, body and attachments.
6. Record dated payment(s); derive partial/paid/overdue from invoice balance and due date. Sending an invoice never marks it paid.

Use decimal strings on the wire and decimal arithmetic server-side. Define currency precision and rounding explicitly; test zero- and three-decimal currencies as well as ordinary two-decimal cases. One currency per invoice; never sum different currencies into a falsely labelled EUR total. Separate invoice lifecycle (`draft`, `issued`, `void`) from delivery state and derived payment state. Issued documents cannot be silently edited/deleted or have their numbers reused. Corrections use explicit reversal/credit records referencing the original; jurisdiction rules determine the allowed issued-document correction workflow.

Deposits: issue a deposit invoice, record payment against it, and create a linked balance invoice that deducts the agreed deposit allocation exactly once. Maintain a gig billing allocation model so multiple invoices cannot accidentally double-bill the same charge. Do not count both paid invoice receipts and a gig status transition as income.

### Event agreement
1. Open gig → Create agreement; snapshot DJ and contracting organizer details plus venue/event/date/timezone/set duration/fee/currency.
2. Review editable terms: deposit and balance schedule, travel/accommodation responsibilities, technical/hospitality attachments, cancellation/rescheduling and other approved clauses.
3. Preview a clearly labelled draft; finalize a revision, generating a versioned PDF.
4. Prepare/review/send email to the selected contact; save exact document and email version sent.
5. Manual acceptance recording requires who/when/how and optional uploaded evidence. `sent` is not `accepted` or `signed`; record rejection/supersession separately. No automatic gig confirmation from a send.

Templates are user-editable and versioned. Do not ship model-written clauses as universally enforceable. If approved wording is missing, allow draft/export with a clear draft marker; block claims that the agreement is finalized for operational use.

### Email
- SMTP is a proposed self-hosted default; manual PDF download and email draft copy remain available without SMTP.
- Explicit send confirmation includes To/CC, subject and exact attachments. No send triggered by Save, Issue, Accept or recording payment.
- An outbox record captures content, immutable attachments and idempotency key before delivery.
- Separate `queued`, `sending`, `accepted_by_smtp`, `failed`, `unknown` states. SMTP acceptance is not inbox delivery/read/recipient agreement.
- Lease workers to prevent concurrent sends. Retry known safe pre-acceptance failures; ambiguous timeouts after transmission become `unknown`, not automatic duplicate sends. Explicit resend creates a new attempt.
- Store credentials outside business data/frontend responses; sanitize logs. Restrict sending endpoints to the deployment's authenticated/access-controlled boundary and add CSRF/origin protections as applicable, request limits and rate limits. No open relay.

## Proposed data and API contracts

These are **new designs**, not assertions that these tables/routes exist.

Tables: `billing_profiles`, `invoices`, `invoice_items`, `invoice_payments`, `invoice_adjustments`, `gig_billing_allocations`, `finance_entries`, `gig_agreements`, `agreement_versions`, `document_exports`, `email_outbox`, `email_attempts`. Split into the minimum migrations needed, with foreign keys, uniqueness constraints and indexes. Allocate next numeric goose identifiers only after inspecting the migration directory; never rename deployed migrations.

All issued documents snapshot issuer, customer, gig facts and template version. `document_exports` records document kind/id/version, private object key, checksum, MIME type, size, generation status and timestamp. Garage keys include document ID/version, not just gig ID; regenerating must not overwrite a previously sent revision. Failed/missing storage never returns a fictional saved key. Invoice issue and PDF readiness are distinct states; failed generation is retryable without allocating another invoice number.

Proposed route families under `/api/v1`:
- `/finance/billing-profile`: GET/PUT.
- `/finance/invoices`: list/create; `/{id}` read/update draft; `/{id}/issue`, `/void`, `/adjustments`, `/payments`, `/exports` explicit operations.
- `/finance/entries`: manual non-invoice income/expenses; `/finance/summary`: currency-separated totals.
- `/gigs/{id}/agreements`: create/list; `/agreements/{id}` read/update draft; `/agreements/{id}/finalize`, `/acceptance`, `/exports`.
- `/documents/{id}/download`: authorized stream of a stored export.
- `/emails`: create draft; `/{id}` read/update; `/{id}/send`, `/{id}/attempts`.

Before implementing, publish a concrete contract fixture for each route: method, body, response, validation and status codes. Use snake_case for new finance/agreement/email wire fields, typed TS adapters where needed, `{data: ...}` JSON successes, and the actual repo error convention. Define pagination, decimal-string handling, UTC timestamps plus event timezone, optimistic concurrency, and idempotency at issue/payment/send boundaries. No permissive dual-shape parsing to hide mismatches. Existing endpoints must keep backwards-compatible behavior unless explicitly migrated with their consumers/tests.

## Ordered implementation work packages

For each package: write the listed failing tests first; run them to establish RED; implement the smallest change; run focused tests for GREEN; review spec then code quality. Commit only when authorized, staging exact task files. Do not fabricate runnable code examples before interfaces are inspected.

### 0. Baseline, prerequisites and roadmap handoff
**Files to read:** `AGENTS.md`, `CLAUDE.md`, `.planning/ROADMAP.md`, `.planning/REQUIREMENTS.md` if present, `api/cmd/api/main.go`, `api/internal/gig/{handler,service,pdf,pdf_storage}.go`, `api/internal/settings/{model,service}.go`, `apps/dj/nuxt.config.ts`, `apps/dj/scripts/build-prod.mjs`.
1. Capture branch/status and preserve local UI changes; select an isolated branch/worktree without sweeping in unrelated changes.
2. Map FIN-01–FIN-10 to this plan; list intentional additions for agreements/email.
3. Inspect auth/proxy routes, schema and test targets; verify real typecheck.
4. At execution time update `.planning/ROADMAP.md` Phase 5 to reference this plan and mark in progress, not complete. Add agreement/email scope explicitly; preserve unrelated phase history.
**Gate:** known baseline failures documented, target names verified, no unreviewed writes to live storage/mail.

### 1. Billing identity and wire contracts
**Modify:** `api/internal/settings/model.go` only if integrating profile references; `api/cmd/api/main.go` for later wiring.
**Create:** `api/internal/finance/{model,validation,handler,contract_test}.go`, `apps/dj/app/types/finance.ts`, `apps/dj/app/stores/__tests__/finance.test.ts`.
1. Add contract fixtures for identity/recipient, decimal strings and validation errors.
2. Add separate structured billing profile model and validation; missing configuration must be actionable.
3. Add authorized profile read/write with concurrency tests.
**Gate:** a valid test profile round-trips; missing legal identity cannot issue documents; private billing details do not leak into public EPK output.

### 2. Schema, numbering and draft invoices
**Create:** new numbered files under `api/internal/platform/migrations/`; `api/internal/finance/{repository,service,repository_test,service_test}.go`.
1. Test fresh migration and upgrade on disposable Postgres.
2. Implement draft CRUD and item ordering, snapshots, linked gigs/contacts.
3. Test concurrent issue requests; implement transactional unique number allocation and idempotency.
4. Test frozen issued fields and stale update rejection.
**Gate:** retries allocate one invoice number, parallel issuance cannot duplicate numbers, drafts remain editable, issued records do not change with settings/gig edits.

### 3. Money, deposits, payments and corrections
**Create:** `api/internal/finance/{calculations,payments,calculations_test,payments_test}.go`.
1. Define/test rounding policy, tax-exclusive items and explicit configured tax/exemption labels; no hardcoded country rate.
2. Test deposit/balance allocation, partial payments, adjustments, overpayments and duplicate payment submissions.
3. Define one ledger source per receipt; reconcile legacy gig `payment_status` with invoice-derived values, surfacing conflicts rather than silently rewriting financial history.
4. Add manual expense/non-invoice income entries and currency-separated summaries.
**Gate:** totals reconcile at item/invoice/payment/gig levels; no cross-currency sums or duplicate income.

### 4. Shared PDF exports and invoice rendering
**Create:** `api/internal/document/{model,service,repository,handler,service_test}.go`, `api/internal/finance/{pdf,pdf_test}.go`.
**Inspect/reuse carefully:** `api/internal/gig/pdf.go`, `api/internal/gig/pdf_storage.go`, `api/internal/platform/storage/storage.go`.
1. Test render failure propagation, Unicode names, long/multipage items and totals.
2. Render branded but print-readable invoice PDFs, with draft watermark where appropriate and embedded licensed font assets if needed.
3. Persist immutable exports to private Garage objects; test missing/nil storage and failed writes.
4. Implement authorized preview/download and version history; verify extracted PDF text and real file bytes, not only HTTP 200.
**Gate:** two different document revisions remain retrievable; old PDFs do not change after editing live records; unauthorized access fails.

### 5. Agreement authoring, revisions and PDF
**Create:** `api/internal/gig/{agreement_model,agreement_repository,agreement_service,agreement_handler,agreement_pdf,agreement_test}.go`, `apps/dj/app/types/agreement.ts`.
1. Test gig/party snapshot, optional promoter versus contracting entity, terms/template revision and attachment references.
2. Implement draft save/finalize/revise/supersede transitions and evidence-based manual acceptance.
3. Render agreement PDF and signature spaces without pretending it is signed.
4. Fix the existing booking-confirmation DJ-name/error/storage defects as separate regression-backed tasks; keep booking confirmation distinct from agreement.
**Gate:** agreement authoring works before gig confirmation; sent/accepted/signed are not conflated; superseded revisions remain immutable and identifiable.

### 6. Email drafts, outbox and delivery
**Create:** `api/internal/mailer/{model,repository,service,smtp,worker,service_test,worker_test}.go`.
**Modify:** `api/internal/platform/config/config.go`, `api/cmd/api/main.go`; deployment examples/docs after transport design is verified.
1. Write tests for draft rendering, valid recipients, CRLF/header injection, filename handling, body escaping, attachment ownership and limits.
2. Implement local mail-capture transport first; build MIME from stored immutable export bytes.
3. Add authenticated draft/send/status routes, transactional outbox creation and leased worker.
4. Exercise disabled transport, rejection, crash/restart, concurrent clicks, post-transmission timeout and explicit resend.
**Gate:** no email without explicit confirmation; repeated request does not duplicate send; ambiguous outcome is visible; SMTP acceptance never reads DELIVERED.

### 7. Invoice and agreement UI
**Create:** `apps/dj/app/stores/{finance,agreement,documentEmail}.ts`, `apps/dj/app/components/finance/{InvoiceEditor,InvoiceList,InvoicePaymentPanel,BillingProfileForm}.vue`, `apps/dj/app/components/gig/GigAgreementPanel.vue`, `apps/dj/app/components/documents/{DocumentHistory,DocumentEmailDialog}.vue` and matching tests.
**Modify:** `apps/dj/app/pages/finance.vue`, `apps/dj/app/components/gig/GigFormDialog.vue`; inspect existing gig details before choosing exact insertion points.
1. Build billing setup and invoice draft editor with clear save/issue states.
2. Wire gig Create agreement/Create invoice and independent document history.
3. Build PDF preview/download and accessible email composer with explicit recipient/attachment confirmation.
4. Add payment recording and per-currency summaries; avoid labelling outstanding balance as earned cash.
5. Preserve current Kinetic HUD tokens and unrelated design edits; load `klubhub-design` and Vue skills before UI work.
**Gate:** keyboard/screen-reader accessible validation; failed save/export/send remains retryable and never shows success; reload preserves document/payment state.

### 8. Integration proof, mock policy and handoff
**Create:** E2E specs in the actual discovered E2E project (do not assume `apps/dj-e2e` exists), plus local-only mail-capture integration fixture and contract checks under `tests/bruno` as appropriate.
**Modify at execution:** `docs/API.md`, `docs/CONFIGURATION.md`, `docs/SELF-HOSTING.md`, `.planning/ROADMAP.md`, and relevant requirements/status documents after checking their existence.
1. Run real PostgreSQL + Garage + Go + Nuxt + captured SMTP flow: gig → agreement → PDF → email; invoice → PDF → email → partial/full payment → dashboard.
2. Retrieve captured MIME, assert exact recipient/content/attachment bytes and selected export revision.
3. Simulate database/storage/mail failures and restart; verify honest state and recovery without duplicates.
4. Build production through its real build target and inspect both client/server output for fixture sentinels. Verify labelled mocks in dev/staging cannot send real emails.
5. Add docs for configuration, document backups/restores, numbering, tax/legal limitations, delivery uncertainty and corrections.
6. Two-stage review, then address every applicable CodeRabbit comment and verify CI at the exact PR head before merge. Mark only actually verified scope complete.

## Verification commands and evidence

At execution start discover generated target names with `pnpm nx show projects` and `pnpm nx show project @dev/dj --json`. The required nx-workspace skill was unavailable in this planning session; load the repo-local equivalent if available. Use Nx for workspace tasks. Known build command: `pnpm nx run @dev/dj:build`. Previously used test/lint commands to confirm against discovered targets: `pnpm nx run @dev/dj:test -- --run` and `pnpm nx run @dev/dj:lint`.

If an API Nx target exists, use it; otherwise add a real API test target as a scoped setup task with working directory `api` and command `go test ./... -race -count=1`, then invoke it through Nx. Add/repair a real frontend typecheck target if the existing one only echoes success. Do not claim typecheck/e2e passed from no-op output. Inspect CLI help before adding unfamiliar flags.

Required evidence: test outputs, concurrency/decimal edge cases, migration upgrade results, extracted PDF text and visual inspection, real Garage readback, captured MIME with attachment comparison, UI success/error screenshots, production mock-exclusion check, and exact-head CI. No live promoter/booker email is required or authorized for testing.

## Decisions to resolve without blocking safe draft development

1. Billing jurisdiction and tax status: obtain from DJ before issuing real invoices; research applicable requirements then, not by guessing US/EU rules.
2. Agreement wording: user-supplied/reviewed templates, primary language and acceptance evidence requirements.
3. Sender: SMTP proposed, configurable sender/reply-to; credentials through secure configuration, never committed or requested in chat.
4. Number format: proposed configurable prefix/year/sequence; confirm actual format and correction policy before first issue, then prevent duplicate/reused numbers.
5. Deposit policy and currency: configurable per gig/invoice, no presumed default percentage or EUR conversion.

## Definition of done

- DJ identity and contracting/billing recipient are correct, editable before issue and snapshotted after.
- Invoice and event agreement are separate real documents, generated as readable PDFs and retrievable from Garage.
- Explicitly reviewed email reaches a local capture inbox with the exact selected PDFs; honest delivery states persist through failures/restarts.
- Invoice numbering is concurrency-safe, issued versions immutable, payment/deposit accounting reconciles without duplicate income.
- Finance UI uses persisted records and per-currency summaries; dev/staging demos remain clearly labelled and absent from production output.
- Unit, contract, database, storage, email, real typecheck and E2E gates have actual passing evidence.
- Jurisdiction/template/sender production-use gates are explicitly resolved or shown as blockers, not silently declared complete.

Planning creates only this file. Implementation, roadmap edits, commits, pushes, deployment and external email sending are not performed in this turn.
