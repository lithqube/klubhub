# CONTEXT — Phase 04.10: Availability & Outreach (Booking Requests)

Spec: `04.10-SPEC.md` (same directory). Branch: `feat/availability-outreach`.

## Locked decisions (user-confirmed)

1. **Wormhole / public tunnel from a laptop: DROPPED.** Conflicts with the SECURITY.md threat model, sealed PII, the offline door app, and the edition strategy (PII would transit a third-party relay).
2. **EmailService is an independent shared service** — extracted from `finance` into `api/internal/platform/email` (+ `api/internal/platform/docstore` for the document-pointer layer), usable by finance, marketing, outreach, and future consumers. Both binaries (`api/cmd/api`, `api/cmd/promoter`) import it.
3. **City-name matching confirmed** — no lat/lng on venues/contacts/talent contacts; radius matching deferred until coordinates exist.
4. **Both directions in scope now** (not deferred to Cloud): DJ→promoter (DJ app) and promoter→DJ (promoter app). The cross-tenant opt-in marketplace/directory stays deferred (Cloud slice).
5. **EPK PDF attach on the DJ side** — the DJ attaches their own EPK export to a booking request; templates are customizable per side.

## Assumptions (planned, flag if wrong)

- **A1 (SPEC §14 Q1):** promoter-side DJ source is a **new `talent_contacts` table** (sealed PII + blind index, source-tracked, symmetric to the DJ's `contacts`), NOT the consent-based `audience_contacts`. Rationale: audience contacts are the promoter's marketing audience (fans) — a different purpose and lawful basis; adding city/genre/role to them blurs purpose limitation. If the user picks audience reuse instead, plan 04.10-03 tasks 1–2 change (schema + service), tasks 3–5 mostly stand.
- **A2 (SPEC §14 Q2):** promoter sender identity is **instance-level** `PLUNK_FROM_*` in v1 (correct for self-hosted single-org). Per-org sender identity is future Cloud work.
- **A3:** promoter-side offers carry **no attachment** in v1 (event details in the body). EPK attach is DJ-side only.
- **A4:** authz needs **no rego change** — `booking.read`/`booking.write` already exist in `authz.rego` and are granted to owner/admin/booker.

## Scope fences (non-goals, SPEC §1)

No cross-tenant marketplace, no public DJ directory, no geo-radius matching, no bulk/campaign sends, no auto-attach, no auto-send, no in-app messaging, no contracts/e-signature (rest of promoter P4), no per-org sender identity.

## Requirement index

| Prefix | Side | Plans |
|---|---|---|
| EMAIL-01..04 | shared | 04.10-01 |
| AVAIL-01..03, OUTR-01..07 | DJ app (backend) | 04.10-02 |
| OUTR-08 | DJ app (frontend) | 04.10-04 |
| TALENT-01..03, OSLOT-01..02, OFFER-01..07 | promoter app (backend) | 04.10-03 |
| OFFER-08 | promoter app (frontend) | 04.10-05 |

(Following the 4.8/4.9 precedent, inserted-phase requirements live in the plan frontmatter + this file, not in `.planning/REQUIREMENTS.md`.)

## Plan / wave map

```
wave 1:  04.10-01  shared email platform extraction (platform/email + platform/docstore)
wave 2:  04.10-02  DJ→promoter backend   ┐ both depend on 04.10-01, independent of each other
         04.10-03  promoter→DJ backend   ┘
wave 3:  04.10-04  DJ frontend          (depends on 04.10-02)
         04.10-05  promoter frontend    (depends on 04.10-03)
```

## Key repo facts the plans rely on

- DJ migrations: `api/internal/platform/migrations/NNN_*.sql` (goose, embedded), latest `032_email_reply_to.sql` → new start **033**.
- Promoter migrations: `api/internal/promoter/migrations/NNNNN_*.sql`, latest `00013_audience.sql` → new start **00014**. Every new table: `tenant_id` + `ENABLE`/`FORCE ROW LEVEL SECURITY` + `tenant_isolation` policy + `GRANT` + `data_class` comments. `rls_guard_test.go` + `data_class_guard_test.go` scan all SQL automatically — new tables are covered by running those tests.
- Email primitives today: `api/internal/finance/email_model.go` (EmailKind/EmailStatus/EmailMessage/CreateEmailRequest/EmailAttachment/AttachmentSource/EmailSender/SMTP/PlunkConfig/EmailService/StartWorker/EmailRepository), `plunk_sender.go` (PlunkSender), `document_model.go` (Document/DocumentService), `document_repository.go`, `storage_adapter.go`. `finance/attachment*.go` are RECEIPTS (ledger entry files) — a different concept, NOT part of this extraction.
- `documents` table (015): `owner_type` CHECK includes `'epk'`; `kind` CHECK is `('general','invoice_pdf','einvoice_xml','validation_report')` (031) → 034 adds `'epk_pdf'`. Unique current index: `documents_owner_kind_current_idx (owner_type, owner_id, kind) WHERE is_current`.
- `email_messages` (017) exists only in the DJ DB. The promoter DB runs only `promoter/migrations` → 00014 creates a tenant-scoped mirror.
- Sealing pattern (audience): `envelope.Keyring` → `keys.Current(ctx, tx, tenant)` → `envelope.SealString(dek, tenant, Field{Table, Column, RowID}, val)` + `dek.BlindIndex(table, col, envelope.NormalizeEmail(email))`.
- Promoter routing: `promoter/server/server.go` `Deps` struct + `d.<Module>.Mount(r, d.Authz, reg, onDeny)`; handlers register via `e.Handle(r, reg, authz.Route{Method, Pattern, Action, ResourceType}, fn, onDeny)`.
- DJ routing: `apphttp.NewRouter(...)` in `api/internal/platform/http/router.go`, `r.Mount("/api/v1/<x>", handler)`; handlers expose relative chi paths.
- Plunk config (DJ): `platform/config/config.go:117-124` (`PLUNK_BASE_URL`, `PLUNK_PROJECT_ID`, `PLUNK_API_KEY(_FILE)`, `PLUNK_FROM_EMAIL`, `PLUNK_FROM_NAME`). Promoter config: `api/internal/promoter/config/config.go` — no Plunk yet.
- `tenantdb`: `WithTenant(ctx, pool, tenant, fn)` sets `app.tenant_id`; `TenantFromContext(ctx)`; `Tenants(ctx)` lists tenants (for the per-tenant email worker — RLS is FORCE'd, so an unscoped poll sees nothing).
- Frontend: DJ `apps/dj/app/{pages,components,stores,composables}`, dev mocks `apps/dj/server/api/v1/`. Promoter `apps/promoter/app/{pages,components,stores,composables}`, placeholder `pages/bookings.vue` (P4).
