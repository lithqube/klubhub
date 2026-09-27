# Plan: KlubHub Promoter P3 — audience and promotion

**Source plan**: `.claude/plans/promoter-app.plan.md` §5 (P3), adjusted by §12 (D2: SaaS-only public pages) and §13.4 (data classes)
**Branch**: `feat/promoter-p3-audience` (P3.1), further slices get their own branches/PRs
**Complexity**: Large — delivered in slices, one or more PRs each, same style as `.claude/plans/promoter-p2-guests-door.plan.md`

## What D2 changes for P3

Self-hosted Promoter has no public surface. The audience CRM in this plan is admin-only: contacts arrive through the admin UI, a CSV import, or (later) an internal RSVP/follow hook — never a public follow/notify-me form, which is SaaS-only (`publicedge`).

## Scope

| Slice | Delivers |
|---|---|
| P3.1 Audience CRM | Contacts with a consent record (basis, form text, timestamp, IP, optional double-opt-in), status (active/unsubscribed/bounced/complained), sources (rsvp/follow/notify_me/csv/door), CSV import (dedupe by email blind index, shared consent per batch), segments as saved filters (status/source/since-days), contact table with status tabs and counts, CSV export |
| P3.2 Email platform | `MarketingProvider` interface, generic SMTP + Plunk adapter, separate transactional/marketing streams, throttled send queue, RFC 8058 List-Unsubscribe headers, bounce/complaint suppression, click tracking, DNS checker (SPF/DKIM/DMARC) |
| P3.3 Campaign templates & timeline | MJML-filled templates (announce, lineup, on-sale, last tickets, timetable, day-of, thank-you), scheduling, test sends, relative-offset timeline auto-drafting into a calendar |
| P3.4 Social asset generator & publishing | Flyer/lineup/timetable/countdown asset variants at the standard sizes + A3 poster PDF with a UTM QR code; Instagram/Telegram publishing, WhatsApp/FB-Event copy export |
| P3.5 Attribution | `/go/:code` short links with UTM presets, per-promoter tracking links, UTM saved on orders, QR generator, optional Umami/Plausible script, consent-gated Meta Pixel |

Order: P3.1 first — segments and consent records are the data every later slice (campaign targeting, attribution) reads. P3.2–P3.5 are scoped for later PRs; this file is updated as each lands, per the P2 plan's running-log style.

This session (2026-09-28) delivers **P3.1 only**, as the first reviewable slice of P3.

## P3.1 data model (migration 00013)

`api/internal/promoter/migrations/00013_audience.sql`. Both tables: `tenant_id`, ENABLE + FORCE RLS, fail-closed policy, table and column `data_class` comments.

- **`audience_contacts`** (personal): `id`, `tenant_id`, `name_enc`, `email_enc`, `email_bidx` (unique per tenant — dedupe/lookup), `phone_enc`, `status` (active/unsubscribed/bounced/complained), `source` (rsvp/follow/notify_me/csv/door), `consent_basis` (consent/soft_opt_in), `consent_recorded_at`, `consent_ip` (nullable — not every import has one), `consent_form_text`, `double_opt_in_confirmed_at` (nullable), `created_at`, `updated_at`. `CHECK (email_enc IS NOT NULL OR name_enc IS NOT NULL)` — a contact needs at least one identifier.
- **`audience_segments`** (internal — a segment is a saved filter, never personal data itself): `id`, `tenant_id`, `name`, `filter` jsonb (`{status?, source?, since_days?}`), `created_at`, `updated_at`.

## P3.1 API (all through `authz.Engine.Handle`; reused existing Rego actions)

| Route | Action |
|---|---|
| `GET /api/v1/audience/contacts` (status, source, q filters; counts) | audience.read |
| `POST /api/v1/audience/contacts` | audience.write |
| `PUT /api/v1/audience/contacts/{id}` | audience.write |
| `DELETE /api/v1/audience/contacts/{id}` | audience.write |
| `POST /api/v1/audience/contacts/{id}/status` | audience.write |
| `POST /api/v1/audience/contacts/import` (CSV rows, shared consent) | audience.write |
| `GET /api/v1/audience/contacts/export.csv` | audience.export |
| `GET/POST /api/v1/audience/segments`, `PUT/DELETE .../segments/{id}` | audience.read / audience.write |

`audience.read`/`audience.write`/`audience.export` already existed in the Rego policy from an earlier phase's route-coverage groundwork — no policy change needed for P3.1.

## P3.1 decisions (implementation)

- **A contact needs a name or an email**, not both — a door signup often has only a name; a CSV row might have only an email. Enforced both in the Go validator and the DB `CHECK`.
- **CSV import shares one consent record across the whole batch** — the form text and lawful basis describe the list the operator is importing (e.g. "past customers, soft opt-in"), not each row individually. Rows are deduped by the email blind index; a repeat email updates rather than duplicates, matching the P2.1 attendee-import convention.
- **Updating a contact never touches its consent record.** Consent is captured once, at the moment of collection; editing name/email/phone/status later must not silently imply re-consent. Re-consenting (if ever needed) would be a separate, explicit action — not built here.
- **`SinceDays` is computed in Go, not a Postgres `interval` cast** — pgx has no default encoding for `time.Duration` into `interval`, and a hand-rolled string cast is an easy place to get silently wrong. The service computes the cutoff timestamp itself and binds it as a plain parameter.
- **Segment filters are never personal data** — `audience_segments` is `internal` class, not `personal`; a saved filter like `{status: "active", since_days: 30}` names no one.
- **CSV export never includes `consent_ip` or `consent_form_text`** — those are compliance evidence, not marketing data, and exporting them would spread PII further than the export's purpose needs.
- **No public follow/notify-me form** — per the D2 boundary, contacts arrive through the admin UI or CSV only in OSS; the `follow`/`notify_me` sources exist in the schema for parity with SaaS-imported data but nothing in this repo writes them yet.
- **Unsubscribing keeps the contact visible on the "ALL" tab** with an updated status badge, rather than hiding it — an unsubscribed contact is still a real, addressable audience member for filtering/reporting; it just can't receive marketing email once P3.2 lands.

## Frontend

- New page `/audience` (replaces the `PhasePlaceholder` stub): segments panel (create a saved filter, see its live matching count) + contact table with status tabs and counts, search, add-contact dialog (consent fields required), CSV-import dialog (file + shared consent for the batch), CSV export link.
- `useAudienceStore` (Pinia): loads contacts + segments together, explicit per-status count bumps on create (mirrors the Go `Counts` shape rather than a dynamic-key increment), reload-after-mutate for status changes and imports (server is the source of truth for counts), local removal for delete.
- `types/audience.ts` mirrors the Go API types.
- Mock handlers under `apps/promoter/server/api/v1/audience/{contacts,segments}/...` for frontend-only dev mode, per repo convention.
- Dialogs use the shared `Dialog`/`DialogContent`/`DialogTitle`/`DialogDescription` shadcn-vue primitives from `libs/ui` (not a bespoke component); each dialog carries a `DialogDescription` for screen readers.

## Validation

`go build ./...`, `go vet ./...`, all existing migration guards (RLS, data-class, no-ID-images, pgxpool-import) pass unmodified against the new migration and package, `TestEveryRouteHasAPolicyDecision` covers all 11 new routes, full server integration suite with no regression, 14 new `audience` package tests (unit + testcontainers Postgres integration), `go test -race -count=1 -p 1 ./internal/platform/... ./internal/promoter/... ./cmd/promoter/...` clean, `govulncheck` clean. Frontend: lint and typecheck clean for `@dev/promoter`, 8 new Vitest store tests (316/316 total passing across `@dev/promoter`, `@dev/ui`, `@dev/dj`), 4 new Playwright e2e tests for the audience page.

## Status (2026-09-28)

| Slice | State | Notes |
|---|---|---|
| P3.1 Audience CRM | Done | Backend (`audience` package, migration 00013, 11 routes), frontend (`/audience` page, store, mocks), e2e — all green. See decisions above. |
| P3.2 Email platform | Not started | Deferred to a follow-up PR |
| P3.3 Campaign templates & timeline | Not started | Deferred to a follow-up PR |
| P3.4 Social asset generator & publishing | Not started | Deferred to a follow-up PR |
| P3.5 Attribution | Not started | Deferred to a follow-up PR |
