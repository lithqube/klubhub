# Plan: KlubHub Promoter P1 — events, venues, lineup, export pack

**Source plan**: `.claude/plans/promoter-app.plan.md` §5 (P1), adjusted by §12 (D2: SaaS-only public pages)
**Complexity**: Large — delivered in slices, one commit (or more) each

## Scope

| Slice | Delivers |
|---|---|
| P1.1 Venues | Venues with rooms (stage templates), IANA timezone, capacity, curfew, tech notes; address, geo and contact details sealed (envelope) |
| P1.2 Events | Event CRUD; status lifecycle draft → scheduled/published → cancelled/postponed; visibility public/unlisted/private; `publish_at` embargo; location mode venue / city only / secret with a reveal time; doors/start/end in the event timezone; genres, min age, cost text, external ticket link |
| P1.3 Lineup & timetable | Event stages (copied from venue rooms or free-form), lineup entries (display name + profile URL, billing order, b2b groups), set times validated server-side: inside the event window, no overlap per stage except b2b, changeover buffer, stage curfew |
| P1.4 Export pack (OSS replacement for hosted pages, D2) | API: JSON-LD `MusicEvent` (Google required + recommended fields) and ICS (stable UID, SEQUENCE). Client: static HTML page, embed snippet, 1200×630 OG image (canvas), copy-paste fields for RA / Facebook / DICE, all as one ZIP |
| P1.5 Collective profile | Org bio, website, socials, accent colour; used by the export pack |
| P1.6 RA import *(later)* | Import event / venue / lineup by RA URL |
| P1.7 Flyers *(later)* | Garage (S3) in the promoter stack; flyer upload; flyer in export pack and OG image |

## Data model (migration 00006)

- `venues` (internal): name, city, country, timezone, capacity, curfew (local time), tech notes; `address_enc`, `geo_enc`, `contact_*_enc` sealed.
- `venue_rooms` (internal): per-venue stage templates.
- `events` (public class; internal fields named so): lifecycle, visibility, location mode, times (timestamptz), genres, links.
- `event_stages` (public): per-event stages with optional curfew and changeover minutes.
- `lineup_entries` (public): display name (reviewed exception: artist stage name), profile URL, billing order, b2b group, stage, set start/end.
- `door_pins.event_id` gains its foreign key to `events`.

All tables: tenant_id, ENABLE + FORCE RLS, fail-closed policy, data_class comment (guards enforce).

## API (all through authz.Engine.Handle)

| Route | Action |
|---|---|
| `GET/POST /api/v1/venues`, `GET/PUT/DELETE /api/v1/venues/{venueID}` | event.read / event.write |
| `GET/POST /api/v1/events`, `GET/PUT /api/v1/events/{eventID}` | event.read / event.write |
| `POST /api/v1/events/{eventID}/status` | event.write |
| `PUT /api/v1/events/{eventID}/stages`, `PUT /api/v1/events/{eventID}/lineup` | event.write |
| `GET /api/v1/events/{eventID}/export/jsonld`, `.../export/ics` | event.read |
| `GET/PUT /api/v1/org/profile` | org.read / org.update |

Events emitted: `event.created`, `event.updated`, `event.status_changed`, `lineup.updated` (IDs only).

## Frontend

Nav gains VENUES. Pages: `/events` (upcoming / drafts / past), `/events/new`, `/events/[id]` (Details · Lineup & timetable · Export), `/venues`, `/settings` (collective profile). Stores: `useVenueStore`, `useEventStore`. Timetable: per-stage columns, drag-and-drop reordering, curfew line, server validation messages inline.

## Validation

`go test -race` (domain + integration + route coverage + guards) · Vitest · vue-tsc · Playwright e2e · built-in browser check of the timetable and export.

## Risks

| Risk | Mitigation |
|---|---|
| Timezones (DST, venue vs event tz) | Store UTC timestamptz + IANA tz; render with Intl in the event tz; DST tests |
| Secret locations leaking via exports | Export pack honours location mode and reveal time; JSON-LD uses city only until reveal |
| Timetable edge cases (b2b, overnight sets) | Server-side validation with table tests; UI shows server errors |
| Export pack in OSS drifts from SaaS hosted pages | Same JSON-LD/ICS endpoints feed both |

## Status (2026-09-26)

| Slice | State | Commits |
|---|---|---|
| P1.1 Venues | Done — list, editor, rooms, sealed address/geo/contacts, MFA-gated audited reveal, archive blocked while in use, tech-notes contact hint | 7879d5d |
| P1.2 Events | Done — list (upcoming/drafts/past), create/edit, status lifecycle, embargo, location modes | c9766b0 |
| P1.3 Lineup & timetable | Done — grid + list, drag and keyboard, conflicts saved but block publish/export, device drafts | c9766b0 |
| P1.4 Export pack | Done — withheld panel, OG image, RA/Facebook/DICE/generic fields, static page (sandboxed preview), embed, JSON-LD, ICS, store-only ZIP with `-embargoed` suffix | 17e4059 |
| P1.5 Collective profile | Done — migration 00007, `PUT /api/v1/org/profile` (audited), accent + links in exports | 48e5d2b |
| Dashboard / `/now` | Done — attention queue (conflict > untimed > go-live/reveal > drafts > security), next-event preflight, nav counter; list API returns issue counts | 7879d5d |
| P1.6 RA import | Later | — |
| P1.7 Flyers (Garage) | Later | — |

Verification: Go `-race` (unit + testcontainers Postgres) for promoter and platform; 46 unit tests, 16 Playwright e2e (desktop + mobile), typecheck and lint clean; every screen checked in the browser at desktop and 375 px.
