---
title: Phase 4.5 — Rider Templates CONTEXT
version: 1.0
---

# Phase 4.5: Rider Templates — CONTEXT

## User intent

A DJ who books 4–6 gigs a month writes the same tech requirements every time — "2× CDJ-3000", "DJM-A9 or better", "monitor wedges ×2", "XLR line out". They write it once for each gig because the EPK tech rider text lives on the EPK singleton row (`epk_content.tech_rider`), not per gig. When they confirm a gig and want to send the rider to the promoter, they paste it into an email or re-type it.

The user wants a **template + per-gig copy** workflow:

1. **Templates** — reusable named sets of four rider sections (technical, hospitality, backline, other notes). Edit once.
2. **Attach on advance** — when a gig transitions to `advanced`, pick a template → copy is attached to the gig (snapshot, not a live link).
3. **Override freely** — edit the per-gig copy without touching the source template.
4. **Export PDF** — per-gig rider as a one-page PDF (DJ name, venue, date, four sections), using the same Garage storage path and fpdf renderer style as EPK.

This is a manual workflow on the OSS edition. There is no "send to promoter" outbound email — that's reserved for SaaS PRO (matches the open-core monetization decision in `MONETIZATION.md`).

## Requirements (REQUIREMENTS.md §Rider Templates)

| ID | Statement |
|----|-----------|
| RIDER-01 | Create / edit / delete named rider templates with four sections (technical / hospitality / backline / other notes), each free-text |
| RIDER-02 | When a gig transitions to `advanced`, user can select a template to attach; attachment creates a **per-gig copy**, not a live link |
| RIDER-03 | Override any section of the per-gig copy without modifying the source template |
| RIDER-04 | Export per-gig rider as PDF (existing PDFGenerator interface); PDF includes DJ name, venue, date, all four sections |
| RIDER-05 | All rider template + per-gig rider data stored in PostgreSQL with **soft deletion** |

## Domain model

Two tables, both with `deleted_at` for soft-delete (RIDER-05):

```
rider_templates                          rider_attachments
─────────────────                        ─────────────────
id                UUID PK                id                UUID PK
name              TEXT NOT NULL          gig_id            UUID NOT NULL  → gigs.id
technical         TEXT NOT NULL ''       template_id       UUID NULL      → rider_templates.id
hospitality       TEXT NOT NULL ''       technical         TEXT NOT NULL ''
backline          TEXT NOT NULL ''       hospitality       TEXT NOT NULL ''
other_notes       TEXT NOT NULL ''       backline          TEXT NOT NULL ''
created_at        TIMESTAMPTZ NOT NULL   other_notes       TEXT NOT NULL ''
updated_at        TIMESTAMPTZ NOT NULL   created_at        TIMESTAMPTZ NOT NULL
deleted_at        TIMESTAMPTZ NULL       updated_at        TIMESTAMPTZ NOT NULL
                                        deleted_at        TIMESTAMPTZ NULL
```

Two soft-delete patterns exist in the repo today:
- `gigs.deleted_at` with `SoftDelete()` repo method
- `epk_content` rows are hard-deleted only on `Restore` (no soft-delete column)

The `gig.deleted_at` pattern is the canonical soft-delete; the rider tables use it.

`template_id` is `NULL`-able — a per-gig copy may exist without a source template (a gig advanced with no template picked; the rider was filled out manually). This matches RIDER-02's "user can select" — selection is optional.

One attachment per gig is enforced with a partial unique index:
```sql
CREATE UNIQUE INDEX rider_attachments_one_per_gig
    ON rider_attachments (gig_id) WHERE deleted_at IS NULL;
```
The `rider/` package follows the **partial unique index — boolean column pattern** from `references/finance-module-pattern.md` (Postgres rejects subqueries in predicate).

## API surface (mount point: `/api/v1/rider/*`)

```
GET    /api/v1/rider/templates                    → 200 {data:[{...}]}     RIDER-01 list
POST   /api/v1/rider/templates                    → 201 {data:{...}}       RIDER-01 create
GET    /api/v1/rider/templates/{id}               → 200 {data:{...}}       RIDER-01 read
PUT    /api/v1/rider/templates/{id}               → 200 {data:{...}}       RIDER-01 update
DELETE /api/v1/rider/templates/{id}               → 204                     RIDER-01 soft-delete

GET    /api/v1/rider/attachments/by-gig/{gigId}   → 200 {data:{...} | null}
POST   /api/v1/rider/attachments                  → 201 {data:{...}}       RIDER-02 create from template OR blanks
PUT    /api/v1/rider/attachments/{id}             → 200 {data:{...}}       RIDER-03 override
DELETE /api/v1/rider/attachments/{id}             → 204                     RIDER-05 soft-delete
POST   /api/v1/rider/attachments/{id}/pdf         → 200 {data:{id, downloadUrl, createdAt}}  RIDER-04
```

The PDF endpoint mirrors `POST /api/v1/epk/export` shape: returns `{id, downloadUrl, createdAt}` — the same `ExportResult` envelope.

## Gig → advanced hook

There is no automatic copy on transition. RIDER-02 says "user can select a template to attach"; the *user* clicks "Attach rider" on the gig form and picks a template from a dropdown. The repo hook on status change is not needed — the UI flow is explicit.

Why explicit and not automatic:
- The user may not want a rider on every advanced gig (e.g. low-key local show with a known promoter)
- Automatic copy on every transition would surprise the user with empty templates
- An explicit "attach rider" step is fluent (per the user's "clean and fluent UI/UX" requirement) — the user picks the moment

The `GigFormDialog.vue` gains a "RIDER" subsection visible when status is `advanced`. A sub-dialog `AttachRiderDialog.vue` lets the user pick a template (preview-pane) or "Start from blank". The selection calls `POST /api/v1/rider/attachments` with `{gigId, templateId?: uuid}`, which copies the template fields into a new attachment row.

## Frontend UX — clean and fluent

Following EPK patterns:
- Pinia store `useRiderStore` (setup style, `storeToRefs` for read, direct for write inside composables — matches EPK convention)
- One page `/rider` with two tabs:
  - **TEMPLATES** — list + create + edit + delete
  - **ATTACHMENTS** — list of per-gig attachments grouped by upcoming/recent gigs
- Templates list uses HUD list rows (`.glass` rows + `.data-frag` chips) — same as EPK `TracklistHistory.vue` per Phase 1.5.5
- Editor: 4 stacked textareas with quick-insert chip groups per section (mirrors `EpkTechRiderSection.vue`'s `RIDER_CHIPS`)
- Quick-insert chips come from a constant map per section (`TECH_CHIPS`, `HOSPITALITY_CHIPS`, `BACKLINE_CHIPS`, `OTHER_CHIPS`)
- PDF export button uses `.btn-hud.gradient-cta` (same as EPK export)
- The gig form's "RIDER" subsection shows a card with: status (none / from template X / custom), last exported PDF, "OPEN EDITOR" / "EXPORT PDF" / "DETACH" buttons

Status colors:
- No attachment → muted, "NO RIDER ATTACHED"
- Attached from template → accent green, "FROM [TEMPLATE NAME]"
- Overridden → amber, "CUSTOM (FROM [TEMPLATE NAME])" — detected by `template_id != null AND updated_at > created_at`

## Nav integration

- `TheNav.vue`: insert "RIDER" between "GIGS" and "FINANCE" (matches GIGS → FINANCE → EPK flow currently)
- `TheBottomNav.vue`: same insertion
- Route `/rider` in Nuxt pages

## Page-to-page flow

1. User edits a template on `/rider` → autosaves (matches EPK `useEpkAutosave` pattern)
2. User opens a gig form on `/gigs`, advances it → "Attach rider" panel appears
3. User picks template (preview pane shows fields), confirms → attachment row created
4. User clicks "OPEN EDITOR" → routes to `/rider?gig=<id>` (matches the `?gig=<id>` deep-link pattern in `gigs.vue`)
5. User overrides any section → autosaves (separate autosave queue keyed by attachment id)
6. User clicks "EXPORT PDF" → POST `/attachments/{id}/pdf` → download link
7. User copies the link into an email to the promoter (manual send, OSS edition)

## PDF design — one-page A4

Mirrors `epk/pdf.go`'s structure:
- Header: DJ name (uppercase) + accent rule
- Venue line: `<Venue Name> — <City, Country>` and date `<Long Date>`
- Four sections: TECHNICAL / HOSPITALITY / BACKLINE / OTHER NOTES (only render non-empty)
- Section headings in dark ink, accent rule under each (same pattern as EPK sections)
- Footer: "Generated by KlubHub DJ" + accent-color hairline

Filename: `rider-<gig-id>.pdf` (Garage key: `rider/exports/<attachment-id>.pdf`).

## Out of scope (intentionally)

- "Send to promoter" outbound email — manual copy of download link in OSS; SaaS PRO will use Plunk outbox pattern (mirrors `finance/email/`).
- Per-currency pricing fields — not a rider concern.
- Multi-language templates — single-locale free-text; PDF uses cp1252 (matches EPK's existing limitation).
- Template categories / folders — flat list; can grow later.
- Rider PDF version diff / history — single row per attachment is enough for OSS.
- Section visibility toggles per-gig (vs EPK) — all four sections always render if non-empty.

## Files to be created / modified

**Backend (new):**
- `api/internal/rider/model.go` — types + interfaces
- `api/internal/rider/repository.go` — Postgres impl
- `api/internal/rider/service.go` — business logic, PDF orchestration
- `api/internal/rider/handler.go` — chi routes
- `api/internal/rider/mux.go` — composes handler under `/api/v1/rider/*`
- `api/internal/rider/pdf.go` — fpdf renderer (per-gig copy)
- `api/internal/rider/repository_test.go` — contract tests with `fakeRiderRepo`
- `api/internal/rider/service_test.go` — service tests with fakes
- `api/internal/rider/handler_test.go` — handler tests via httptest.Server
- `api/internal/platform/migrations/026_rider_templates.sql`

**Backend (modified):**
- `api/cmd/api/main.go` — wire `rider.NewMux` into the router

**Frontend (new):**
- `apps/dj/app/types/rider.ts`
- `apps/dj/app/stores/rider.ts`
- `apps/dj/app/composables/useRiderAutosave.ts`
- `apps/dj/app/pages/rider.vue` (top-level page)
- `apps/dj/app/components/rider/RiderPageHeader.vue`
- `apps/dj/app/components/rider/RiderTemplateList.vue`
- `apps/dj/app/components/rider/RiderTemplateEditor.vue`
- `apps/dj/app/components/rider/RiderTemplateDialog.vue` (create/edit)
- `apps/dj/app/components/rider/RiderSectionField.vue` (one reusable section: label + chips + textarea)
- `apps/dj/app/components/rider/RiderAttachmentList.vue`
- `apps/dj/app/components/rider/RiderAttachmentEditor.vue` (per-gig copy editor)
- `apps/dj/app/components/rider/AttachRiderDialog.vue` (used inside `GigFormDialog`)
- `apps/dj/app/components/rider/__tests__/*.test.ts`
- `apps/dj/app/stores/__tests__/rider.test.ts`
- `apps/dj/app/server/api/v1/rider/templates/index.get.ts` (mock for FE-only dev)

**Frontend (modified):**
- `apps/dj/app/components/TheNav.vue` — insert Rider nav item
- `apps/dj/app/components/TheBottomNav.vue` — same
- `apps/dj/app/components/gig/GigFormDialog.vue` — add Rider attachment card
- `apps/dj/nuxt.config.ts` — no change needed (proxy routes to API as before)

## Dependencies (already in tree)

- `codeberg.org/go-pdf/fpdf` — same renderer EPK uses
- `github.com/go-chi/chi/v5` — router (same as EPK)
- `github.com/jackc/pgx/v5/pgxpool` — DB pool
- `github.com/google/uuid` — IDs
- Pinia + `@nuxt/ui` + Tailwind — frontend stack (no new deps)

## Testing strategy

- **Repo contract tests**: `fakeRiderRepo` + httptest server (no testcontainers needed; same pattern as EPK)
- **Service unit tests**: fakes for `repo`, `storage`, `settings`
- **Handler tests**: route-by-route with fakes
- **Frontend store tests**: same `vi.mock` reactive() pattern as `EpkTechRiderSection.test.ts`
- **Component tests**: HUD section field renders, chip insertion, autosave debounce

## Plan breakdown (5 plans, matching GSD pattern)

1. `04.5-01-PLAN.md` — data layer: migration 026, `rider_model.go`, `rider_repository.go` (with `deleted_at` soft-delete), contract tests
2. `04.5-02-PLAN.md` — service + PDF: `rider_service.go`, `rider_pdf.go`, template copy semantics, attachment CRUD, unit tests
3. `04.5-03-PLAN.md` — HTTP layer: `rider_handler.go`, `rider_mux.go`, full `/api/v1/rider/*` surface, contract tests via httptest
4. `04.5-04-PLAN.md` — backend wiring: `cmd/api/main.go` mounts rider mux; mock handlers at `apps/dj/server/api/v1/rider/*` for FE-only dev; E2E sanity check
5. `04.5-05-PLAN.md` — frontend: `/rider` page, templates CRUD, attachment editor, autosave composable, nav insertion, GigFormDialog rider card, store tests + component tests

Each plan ends with a verification loop (commands shown in the PLAN files).