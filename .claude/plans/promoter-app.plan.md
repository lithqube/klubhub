# Plan: KlubHub Promoter — second product in the KlubHub family

**Source**: free-form request + 8-agent research sweep (2026-09-25)
**Selected Milestone**: Product definition + phased roadmap (P0 → P7); first implementation target = P0 Foundations
**Complexity**: Large (new product, shared-platform extraction, first auth/multi-user model in the repo)

## Summary

KlubHub Promoter is an open-source (MIT) Nuxt 4 + Go app for promoters, collectives, club-night organisers and small open-air/festival crews. It lands as `apps/promoter` on a shared **Kinetic HUD** UI layer extracted from `apps/dj`. It runs as a second Go binary inside the existing `api/` module, reusing `internal/platform` and the venue, contact, email, PDF and storage code. It ships as its own image, `klubhub-promoter-api`.

The basic product is:
- **events** and a **shareable landing page**
- **RSVP and free tickets**
- **guest lists** with per-DJ quotas
- an **offline door app**
- **email and social promotion**

A **booking exchange** connects it to KlubHub DJ: an offer sent by a promoter becomes a gig in the DJ app, and the DJ's guest list, advance details, contract, invoice and settlement move between the two apps. Paid ticketing comes later, on a pretix-shaped schema designed in from day one.

---

## 1. Requirements restatement

1. **New product `promoter`** in the monorepo. Target users are promoters, collectives, club-night crews, open-air and mini-festival organisers, and venue bookers.
2. **Same design system and components** as DJ (Kinetic HUD tokens, shadcn-vue primitives, shell). Share them; don't fork them.
3. **Basic features.** The user named guest management, guest list, email promotion, and social sharing with an event landing page. Research added:
   - an events and lineup core
   - a collective profile/hub
   - door check-in
   - an audience CRM with consent
   - attribution (UTM, tracking links)
4. **Shared backend with DJ**, in two senses:
   - the same code and platform
   - data exchanged between a promoter and a DJ (offers → gigs, guest lists, advance details, contracts, invoices, EPK, set times)
5. **Research-backed.** The benchmarks are RA, Luma, Partiful, Eventbrite, DICE, Shotgun, Posh, Xceed, Fatsoma, Skiddle, Backline and GuestAccess (door/guest list), Prism, Gigwell, Stagent, ABOSS, Lennd and Festival Pro (operations), and pretix, Hi.Events and alf.io (open-source ticketing).

---

## 2. Research digest (what the market says)

### 2.1 Features every benchmark converges on (the MVP backbone)

| Area | Converged pattern | Main references |
|---|---|---|
| Event page | Cover/flyer and accent theme. Custom slug. Auto-generated OG image. "Presented by" collective. Lineup linked to artist profiles. Map. Add to calendar. A "N going" strip that can be switched off. | Luma themes/OG, RA event fields |
| Visibility | Public / private-link / scheduled go-live ("embargo"). Secret location revealed at a set time or after approval. | RA `visibility`, `embargoDate`, `secretVenueReveal`; Luma hide-location |
| Registration | No account, name + email first. Per-ticket-type capacity, sale window, hidden/unlock-code types, approval-required, waitlist. Custom questions (text, options, IG handle, terms). | Luma tickets/questions |
| Tiers/waves | The next tier auto-opens when the previous one sells out. Only the current tier is shown. Modelled as a `dependency` on the previous tier. | Shotgun progressive release, DICE, RA `Ticket.dependency` |
| Guest list | Types (artist, promoter comp, industry/press, VIP, reduced-entry, crew). **Per-submitter allocation with a secret link** (quota, +N, deadline, approval, revoke). Standing lists copied into each new event. | Backline, RA guestlist, GuestlistOnline |
| Door | **Offline-first** PWA with name search and QR. Partial arrivals ("2 of 3 in"). One-tap undo. Log of device, staff member and time. Live in/out occupancy. Door-staff PIN role with no export. Conflicts flagged, never silently merged. | Backline, Shotgun Scan, RA Scan, Luma check-in, GuestAccess |
| Reporting | Arrivals, no-shows and +1s used, per list and per submitter. Check-in curve. "List back" to the artist. | Backline, tourmanager.info |
| Audience | Followers of the collective page. "Just announced" / "on sale" notifications. Newsletters to followers. Attendee CRM with marketing opt-in. | RA follower mail, Luma Calendars, Fatsoma |
| Attribution | UTM stored on every registration. Tracking link per promoter or rep. Promo codes. Views-vs-registrations chart. | Luma insights/referrals, Shotgun promoters, Posh |
| Integrations | Signed webhooks (Luma: HMAC-SHA256, `t=…,v1=…`). CSV import/export. | Luma webhooks |

### 2.2 Nightlife-specific features (why a generic event tool isn't enough)

- **DJ guest-list quota as a booking term.** Typically 2–4 names for support acts and more for headliners, with a deadline the day before. "Every name is a ticket not sold."
- **Pricing driven by sell-outs**, not dates.
- **Artist-graph marketing.** Tagging a DJ notifies that DJ's followers nearby (RA, Shotgun).
- **Reduced-entry lists with a cutoff** (e.g. "€10 before 01:00"), plus perk tags (drink tokens, green room, AAA).
- **Secret or undisclosed location** for warehouse and open-air events.
- **German BFH ruling:** techno club nights count as concerts, so they take the reduced 7% VAT rate.

### 2.3 Promotion playbook (drives the campaign feature)

- **Club night:** T-8w teaser → T-7w announce + early bird (notify list gets a 1h head start) → weekly reveals → T-4w full lineup → T-1w timetable → day-of info → T+2d thank-you/photos.
- **Open air / mini festival:** T-6mo save-the-date → lineup phases one per month → T-6w info → T-3w timetable → T+1w aftermovie/survey.
- **Implementation:** a **campaign is a template of relative offsets** (`T-42d`) plus triggers (`tier_sold_out`, `lineup_published`). The scheduler auto-drafts posts and emails from it.

### 2.4 Hard platform constraints found

| Constraint | Consequence |
|---|---|
| **Instagram Graph API** | Publishes feed, carousel, reel and story media, 100 posts per 24h. Media must be at a **public URL**. **No story stickers** (link, countdown). Each self-hoster needs their own Meta app. Feed images must be 4:5–1.91:1, so generate **1080×1350**. |
| **Facebook Events** | Can't be created by API (partner-only, onboarding paused). Generate copy-paste fields instead. |
| **WhatsApp** | Channels have no API. Broadcast templates cost per message. MVP is a "copy to channel" export. |
| **Telegram** | Free Bot API for channel posts. Easiest automated broadcast to support. |
| **Email deliverability** | Gmail/Microsoft enforce SPF, DKIM, DMARC and RFC 8058 one-click unsubscribe (550 rejections since Nov 2025). Don't send bulk mail from a VPS IP; use a relay (SES about $0.10 per 1k, Resend, Postmark broadcast stream, Brevo). Double opt-in is effectively mandatory in DE. Past buyers can be mailed under the soft opt-in rule only about similar events. |
| **Open-source ticketing licences** | pretix, Hi.Events, pretalx, Mobilizon and Gancio are AGPL; alf.io is GPL. **We are MIT, so we can reimplement ideas and integrate over APIs but must not copy their code.** pretixSCAN is Apache-2.0. |
| **Paid ticketing** | Pulls in PCI (hosted checkout gives SAQ-A), SCA, refunds and disputes. EU VAT is due where the event takes place. Card surcharges are banned in the EU, so a "service fee" must be a fee line, not a surcharge. |

### 2.5 Reference data model (pretix-shaped, reimplemented)

- `Organizer → Event → (SubEvent)`
- `Item/Variation` (ticket types, add-ons) with a **Quota** as a separate many-to-many capacity bucket. An item is sellable only when all of its quotas have room, which covers tier caps and total venue capacity at once.
- `CartPosition` (reservation with a time limit)
- `Voucher` (code, max uses, price mode, `show_hidden_items`, blocks quota)
- `Order → OrderPosition` (one ticket each, with a `secret`), plus payment/refund ledgers
- `CheckinList` (rules) with `Checkin` rows (entry/exit, device, gate, **client nonce** for idempotency, failure reason)
- **A free RSVP is a €0 order**, so paid tickets later require no schema rewrite.
- **Optional signed tickets:** Ed25519 over a protobuf payload plus a revocation list, for fully offline scanning.

---

## 3. Codebase facts that shape the plan

- **No shared lib exists.** Tokens are in `apps/dj/app/assets/css/styles.css` (`@theme` L47-111, light overrides L1048-1226). Primitives are in `apps/dj/app/components/ui/*`. The shell is `apps/dj/app/app.vue` plus `TheNav`, `TheBottomNav`, `TheMobileHeader` and `TheStatusBar`. `apps/site/build.mjs` reads DJ's CSS directly.
- `pnpm-workspace.yaml` only globs `apps/*`. The Nx inference plugins would pick up `apps/promoter` automatically.
- The **README already reserves this path.** README.md L24 says Promoter lands as `apps/<product>`, and `docker-compose.prod.yml` L69-72 names the image `klubhub-promoter-api`.
- **Go:** one module, `api/` (`github.com/klubhub/dj/api`), with domains under `api/internal/*`. Go's `internal/` rule means any promoter code that wants `internal/platform/*` must live **inside the `api/` module**.
- **No auth; single tenant.** Rows are singletons (`epk_content`, `billing_profiles`). The only secret gates are the `ICAL_SECRET` checks in `api/internal/gig/handler.go:55-80`. **Promoter can't ship without users, orgs and roles**, because door staff, team roles and public submission links all need them.
- **Seams already on the DJ side:**
  - `gigs` has inline promoter fields, `gig_status` and `payment_status`
  - `gig_contacts(role='promoter')` and `gig_venues`
  - `contacts` has party/VAT fields
  - `agreement_instances` has `required_signers ['dj','client']`
  - `invoices.customer_contact_id`
  - `email_messages` queue plus the Plunk sender
  - EPK
  - RA import (`api/internal/ra`)
  - `calendar.ics`
- **Packaging:** `apps/dj/Dockerfile` is arm64-only. `apps/dj/scripts/runtime.mjs` supervises Go on :8080 and Nitro on :3000, and `api/internal/platform/http/frontend.go` proxies anything that isn't an API route to Nuxt.
- **Conflict to resolve:** `docs/v2-saas-migration-plan.md` treats public web pages as **SaaS-only**, because self-hosters struggle to expose them. The promoter app's core value is a public event page plus RSVP. See Decision D2.

---

## 4. Architecture

```
                   ┌──────────────── libs/ui (Nuxt layer: Kinetic HUD tokens, fonts, ui/*, shell, useTheme) ───────────────┐
                   │                                                                                                        │
apps/dj (Nuxt) ────┘                                                               apps/promoter (Nuxt) ────────────────────┘
   │  /api/v1/**                                                                     │  /api/v1/** (admin, authed)
   ▼                                                                                 │  /e/:slug, /c/:org, /l/:token, /door  (public / PWA)
api/cmd/api  (DJ binary)                                                             ▼
   └─ internal/{gig,epk,finance,...}                                     api/cmd/promoter (Promoter binary)
                  ╲                                                          └─ internal/promoter/{org,event,lineup,guestlist,
                   ╲___ internal/platform/{config,db,http,storage,crypto,           door,rsvp,audience,campaign,booking,...}
                        migrations,auth*,email*,pdf*,render*}  ← shared (* = new/extracted)
                                         │
                           PostgreSQL (separate database per product) + Garage (bucket per product)

   DJ instance  ⇄  Booking Exchange (signed JSON over HTTPS, token links)  ⇄  Promoter instance
```

### 4.1 Frontend: `libs/ui` as a **Nuxt layer**
- Move into the layer:
  - the `@theme` and generic HUD classes, plus the light overrides
  - fonts
  - `components/ui/*`, `lib/utils.ts`
  - `useTheme`, `useSidebar`
  - `utils/money.ts`, `utils/dialogFocus.ts`
  - a **generic shell**: `TheNav`, `TheBottomNav`, `TheMobileHeader` and `TheStatusBar`, with nav items, brand and status items passed as props or slots
- DJ-specific classes (`.gig-*`, `.epk-*`, `.spost*`, `.tx-*`) stay in `apps/dj`.
- `apps/dj/nuxt.config.ts` adds `extends: ['../../libs/ui']`. `apps/site/build.mjs` switches to reading the layer's CSS.
- Add `libs/*` to `pnpm-workspace.yaml`. Name the package `@dev/ui`.

### 4.2 Backend: second binary in the same Go module
- `api/cmd/promoter/main.go` provides wiring only. Domains live under `api/internal/promoter/*`.
- Shared platform additions:
  - `internal/platform/auth`: users, sessions, orgs, memberships, roles, magic-link and password login, door PINs, share tokens
  - `internal/platform/email`: the Plunk sender moved out of `finance/`, plus a generic SMTP adapter and a bounce-webhook interface
  - `internal/platform/pdf`: shared fpdf font setup
  - `internal/platform/render`: Playwright screenshot of Nuxt `/render/*` routes, generalised from tracklist
- Venue and contact code is shared **as code**, not as tables. The promoter binary has its own `venues`/`contacts` in its own database.
- Separate goose migration set: `api/internal/platform/migrations/promoter/`.
- Image `klubhub-promoter-api` comes from `apps/promoter/Dockerfile`, cloned from the DJ Dockerfile and runtime supervisor.

### 4.3 DJ ⇄ Promoter integration: the **Booking Exchange** (no shared DB)
The DJ and the promoter are usually **different people on different instances**, so the integration is a small document protocol, not shared tables. It works the same way when both apps run on one host.

- The promoter creates a **Booking** (offer). The exchange issues a **booking link**: an opaque token URL plus a per-booking HMAC secret.
- The DJ app gets a new action, **"Import booking from link"**. It creates or links a `gig` and stores the remote booking URL and secret.
- Documents are versioned JSON:
  - `offer` (fee, deal type, set slot, deposit)
  - `contract` (reuses the DJ agreement engine; `client` signer = promoter)
  - `advance` (travel, hotel, pickup, contacts)
  - `guestlist_allocation` (quota, +N, deadline)
  - `set_time`
  - `invoice` (DJ → promoter payable)
  - `settlement`
  - `epk_ref`
- Transport: pull (`GET /x/v1/bookings/{token}`) plus optional push webhooks signed Luma-style (`t=…,v1=HMAC-SHA256`). If an instance isn't publicly reachable, the fallback is **manual link exchange** where each side polls the other.
- Fallback for DJs not using KlubHub: every document also works as a **no-login web form** (ABOSS/Backline pattern). DJs submit guest names and advance details through the link.

---

## 5. Feature scope by phase

**MVP = P0–P3 (D3).** These cover every "basic feature" the user named. P4 is the DJ integration that makes the pair of apps unique. Where a feature below mentions a public route (`/e`, `/c`, `/l`, `/go`, `/u`), read it through §12: in OSS it becomes the export/import equivalent, and the hosted route is SaaS.

### P0 — Foundations (shared platform) · Size L
1. **Extract `libs/ui` Nuxt layer.** DJ app visual parity is proven by existing Vitest suites, the Playwright screenshot spec, and a before/after check in the built-in browser.
2. **Scaffold `apps/promoter`** (package `@dev/promoter`, dev port **4400** (4300 is the site preview)) and `apps/promoter-e2e`. Add the promoter shell nav (Dashboard · Events · Guests · Door · Audience · Campaigns · Bookings · Settings), mock-handler mode under `apps/promoter/server/api/v1/`, and the `NUXT_PUBLIC_API_BASE` proxy guard copied from the DJ config.
3. **Promoter binary.** Add `api/cmd/promoter`, wire the shared `platform/*`, and give it its own DB, migrations and bucket. Add `api:test` coverage.
4. **`platform/auth`** (first auth in the repo):
   - users with argon2id passwords and optional magic link
   - HttpOnly SameSite=Lax session cookies with CSRF protection
   - `organizations`, `memberships`, and roles `owner|admin|booker|finance|marketing|door`, enforced by chi middleware
   - share tokens (random 32 bytes, hashed at rest, scoped, expiring, revocable)
   - per-event door PINs
   - a `publicedge` package boundary (not mounted in the OSS binary; contract-tested) — see §12
5. **Packaging:**
   - `apps/promoter/Dockerfile`
   - runtime supervisor
   - `scripts/setup.sh promoter-dev|promoter-prod` (or a `--product` flag) and a compose service
   - a CI build in `containers-ci.yml`
   - a `docs/SELF-HOSTING.md` section: the instance stays private; door access over LAN / VPN / Tailscale

### P1 — Events, collective hub and landing page · Size L
- **Organization (collective) profile:** name, slug, logo, cover, bio, socials, accent colour, and a **hub page `/c/:slug`** with upcoming events, series tags, a follow form and an embeddable widget.
- **Venues:** name, address, geo, **IANA timezone**, capacity, rooms/stages, tech spec, curfew, contacts. Import from RA venue data.
- **Events:**
  - title, slug, dates (start, end, doors), venue, stages
  - status (draft/scheduled/published/cancelled/postponed)
  - visibility (public/unlisted/private) and `publish_at` embargo
  - **location reveal** (hidden → city only → full address at a time or after approval)
  - minimum age, genres, description (markdown), free-text `cost`
  - flyer front/back, co-promoters
  - series or recurring dates (SubEvent-ready)
- **Lineup:**
  - artist entries with a linked profile (KlubHub DJ EPK URL, RA URL) **and** raw text
  - billing order, b2b flag
  - **timetable** per stage with changeover buffer and curfew line; drag-and-drop
- **Public landing page `/e/:slug`**, SSR:
  - theme accent
  - OG and Twitter meta with a **dynamic OG image** (1200×630, cache-busted on lineup or status change)
  - JSON-LD `MusicEvent`/`Festival` (name, startDate with TZ, location, offers, performer `sameAs`, eventStatus, doorTime)
  - ICS and Google/Outlook add-to-calendar links (stable UID, SEQUENCE)
  - map
  - "N going" strip that can be switched off
  - sitemap
  - external ticket link (RA, DICE, pretix) as a first-class field
- **Import from RA** (event, venue, lineup), reusing `api/internal/ra`, and **export** in RA-compatible fields.

### P2 — RSVP, guest list and door · Size L
- **RSVP / free tickets** on the pretix-shaped schema:
  - `ticket_types` (capacity, sale window, hidden + unlock code, approval_required, max per order, `depends_on` for waves), `quotas`, `orders`, `order_positions` (secret), `vouchers`
  - waitlist
  - registration questions (text, options, IG handle, terms)
  - no-account checkout with email confirmation, .ics and a QR ticket page
  - reminders 1 day and 1 hour before
- **Guest lists:**
  - types: artist / promoter / comp / industry / VIP / reduced / crew
  - `entry_terms`: free, reduced price, **cutoff time**, perk tags, allowed doors
  - **allocations per submitter with a secret submission link**: quota, +N per guest, deadline, approval, revoke
  - **standing lists** auto-copied into each new event
  - CSV import (RA, Shotgun, DICE formats)
- **Guest table** (Luma pattern):
  - status tabs with counts (Going / Pending / Waitlist / Invited / Declined / Checked-in)
  - search, ticket-type and list filters
  - bulk status updates by pasting emails
  - "add directly as Going"
  - CSV export
- **Door PWA `/door`:**
  - PIN login
  - **downloads the full list for offline use** into IndexedDB, with a local check-in queue and a sync badge
  - search from the first letter, tolerant of accents and typos
  - QR scan in Express and Standard modes
  - partial arrivals, one-tap undo toast
  - "PAST CUTOFF" state
  - big in/out occupancy counter with walk-up button
  - on-the-spot adds (need a manager PIN)
  - quiet ban-list warning
  - conflicts flagged on sync
  - check-ins idempotent by **client nonce**
  - forced dark theme with large touch targets
- **Post-event report:** attendance by list and submitter, no-show rate, +1s used, check-in curve, "list back" export for artists.
- **Privacy:**
  - name-only by default; contact details opt-in per list
  - **retention job** (default: purge guest personal data 30 days after the event, keeping anonymised counts)
  - ban list is org-scoped, needs a reason and an expiry, and is never shared between orgs
  - no ID images

### P3 — Audience and promotion · Size L
- **Audience CRM:**
  - contacts with a consent record (timestamp, IP, form text shown, lawful basis `consent|soft_opt_in`, `double_opt_in_confirmed_at`)
  - status (active/unsubscribed/bounced/complained)
  - sources: RSVP, follow, notify-me, CSV, door signup
  - **followers** of the collective
  - **segments** as saved filters (attended X / any in the last 12 months, follows, city, genre, RSVP'd but not attended, clicked last campaign)
- **Email:**
  - platform email with **generic SMTP + Plunk + optional SES/Resend/Postmark bounce webhooks**
  - separate transactional and marketing streams
  - throttled queue (reuse `email_messages`)
  - **RFC 8058** `List-Unsubscribe` + `List-Unsubscribe-Post` headers and a footer link
  - bounce and complaint suppression
  - click tracking via redirect with UTM appended; open pixel as a soft metric only
  - **DNS checker in the UI** (SPF, DKIM, DMARC for the From domain)
- **Campaign templates** (MJML blocks filled from event data): announce, lineup, on-sale, last tickets, timetable, day-of, thank-you. Scheduling and test sends.
- **Campaign timeline:**
  - club-night and open-air templates of relative offsets and triggers (`T-42d`, `on tier_sold_out`, `on lineup_published`)
  - auto-drafts emails and social posts into a calendar view for review
- **Social asset generator** (reuses the render pipeline): flyer variants, save-the-date, lineup card, lineup carousel, timetable per stage, ticket-status badges (on sale, % gone, sold out, last tickets), countdowns, info/FAQ carousel, thank-you. Each comes in **1080×1350 · 1080×1920 · 1080×1080 · 1200×630**, plus an A3 poster PDF with a QR code carrying `utm_medium=poster`.
- **Publishing:**
  - Instagram via the social publishing domain, extracted to shared code (reusing DJ's worker/retry)
  - Telegram channel bot
  - "copy for WhatsApp / FB Event" exports
  - story sticker reminders, since stickers can't go through the API
- **Attribution:**
  - short links `/go/:code` with UTM presets per channel
  - **tracking link per promoter or rep**, with an attributed RSVP/sales report
  - UTM saved on each order
  - QR generator
  - optional Umami (fits Postgres) or Plausible script
  - Meta Pixel **off by default and consent-gated**

### P4 — Bookings and the DJ bridge · Size L
- **Holds:** 1st/2nd/3rd with expiry and a challenge clock, shown in the calendar with conflict warnings.
- **Offers/deal memos:**
  - deal types: flat, guarantee vs %, door split
  - deposit, buyouts, radius, expiry
  - status (draft/sent/accepted/declined/expired)
- **Contracts and riders:**
  - reuse the DJ agreement engine as shared code
  - rider line items with an approved or substituted mark per item
- **Advancing:**
  - groups: schedule / travel / hotel / ground / contacts / guest list / compliance
  - each group has a traffic-light status that turns "ready" automatically when complete
  - no-login external link
  - day-sheet PDF
- **Booking Exchange v0** (§4.3), plus DJ-app changes:
  - "Import booking link" on the gig
  - guest-list allocation shown on the gig ("2/4 · closes Fri 18:00") with name submission
  - set-time sync
  - invoice sent as a payable
  - read-only settlement view
- **Artist payables:**
  - deposit and balance, invoices received
  - flags for **withholding tax** (DE §50a 15.825%, exempt ≤ €250; BE, FR, ES tables, versioned and dated) and **reverse charge**
- **EPK import by URL:** bio, photos and socials flow into the lineup and the marketing assets.

### P5 — Finance & Accounting · Size XL
**Goal:** complete financial records for each event and for the organisation, ready to hand to an accountant. This is **not** a bookkeeping system: the app produces clean, immutable records and exports, and the accountant does the books and the filing.

**Dependencies:**
- DJ Phase 5 (invoices, VAT, payments, agreements) must finish first. Promoter finance reuses that code rather than forking it.
- **Org-scoping of shared finance code.** `billing_profiles` is currently a singleton (migration `011`) and the finance tables have no owner. The shared code must accept an `organization_id` or owner scope, and DJ keeps a single implicit owner. Land this as a separate, DJ-regression-tested PR before P5 feature work.
- P4 payables and P2 door data feed into this phase.

#### P5.1 Budget and settlement (event level)
- Budget lines: category, fixed / variable per head / revenue-dependent, budgeted vs **actual (auto-filled from P5.3 expenses and P5.2 bills)**, vendor.
- Revenue lines: tickets, bar split, sponsorship, cloakroom, tables.
- **Scenarios at 70/85/100% of capacity** and break-even (tickets and price).
- **Settlement sheet:**
  - gross → fees and taxes → net
  - expenses
  - per-artist walkout (applies the deal, deducts deposit and withholding)
  - co-promoter splits
  - sign-off
  - PDF, pushed to the DJ via the exchange
- Artist fees are hidden from the `marketing` and `door` roles.

#### P5.2 Outgoing invoices (the promoter bills others)
- Invoice to: **sponsors** (sponsorship packages), **venues** (bar-split claims, promoter fees), **co-promoters** (settlement balances), **table / private bookings**, and **other promoters** (hosting fees, equipment hire).
- Reuses the shared invoice engine: concurrency-safe numbering per org, VAT treatment (domestic / reverse charge / exempt), credit notes, deposits and balance invoices, immutable versioned PDFs, payment recording, email send via `email_messages`.
- Invoices link to an **event** (and optionally a budget revenue line), so settlement and P&L pick them up automatically.
- Customers are org `contacts` with party and VAT fields (mirrors DJ migration `020`).

#### P5.3 Bills and expenses (money going out)
- **Supplier bills (payables):** sound and light, security, venue hire, print, transport, hotels, insurance, royalties (GEMA/PRS/SACEM), ticketing fees.
  - Fields: supplier contact, bill number, issue and due dates, net / VAT / gross in minor units, currency, VAT treatment (incl. reverse charge for foreign suppliers), event and budget-line allocation (splitting across events allowed), status `draft → approved → scheduled → paid → void`.
  - **Receipt or invoice upload** (PDF or photo to Garage via the `documents` table), with the original kept immutable.
  - **Approval flow:** `booker` or `marketing` submit, `finance` or `owner` approve; amounts over a threshold need `owner`.
  - Due-date list and overdue reminders.
- **Artist invoices received** (from P4) are the same payable type, with the withholding and reverse-charge flags.
- **Expenses / reimbursements:** crew out-of-pocket costs (taxi, supplies) with a receipt photo from mobile, marked "to reimburse", then paid.
- **Categories:** a fixed chart shared with P5.1 budget categories so "actual" fills itself. Each category has an optional mapping to an accountant account code (see P5.8).

#### P5.4 Door cash and card reconciliation
- **Float:** the cash float per door or bar point is recorded before doors open (denomination count optional).
- **Close-out sheet:** end-of-night count (cash by denomination), card terminal totals (SumUp / Zettle / Stripe Terminal as manually entered totals first), comps and guest-list count from P2 door data, and walk-up and door-sale counts.
- **Expected vs counted:** expected cash = float + door cash sales − cash payouts (e.g. artist cash buyouts, crew). Variance is shown and needs a note and a second signature.
- Cash payouts at the door (artist buyouts, crew) are recorded as expenses with a signature or photo.
- Close-out PDF, locked after sign-off, which feeds settlement revenue.

#### P5.5 Ticketing payout reconciliation
- **Import settlement / sales reports** from external ticketing: RA, DICE, Shotgun, Eventbrite, pretix, Hi.Events, Luma (CSV first, API where available).
- Normalise to: gross sales, buyer fees, platform fees, payment fees, refunds / chargebacks, VAT, net payout, payout date and reference.
- Match each **payout** to the event and to the **amount received in the bank** (manually marked "received", with date and reference). Any mismatch is flagged.
- Ticket revenue per tier flows into settlement (P5.1) and P&L (P5.6). Once native paid ticketing ships (P6), its ledgers feed the same model.

#### P5.6 Organisation P&L and cash flow
- **P&L** per event, per series (e.g. "Sunday Open Air 2026"), per period (month / quarter / year) and per venue, **by currency (no cross-currency sums without an explicit, dated FX rate**, the same rule as DJ Phase 5).
- Budget vs actual per event and rolled up across events.
- **Cash-flow forecast:** upcoming artist deposits and balances, supplier bills due, and expected ticket payouts (from P5.5 schedules) versus the current bank balance entered manually. Warns when a date goes negative.
- Co-promoter view: a statement of shared-event balances per partner, with an outstanding amount and a "settle up" invoice or bill.
- Dashboard tiles: this month's net, outstanding receivables, bills due in the next 14 days, cash variance alerts.

#### P5.7 VAT and withholding summary (preparation, not filing)
- A report per VAT period (monthly / quarterly, set per org) covering:
  - **Output VAT:** tickets (per event country and rate; reduced cultural rate where chosen), sponsorship, other invoices.
  - **Input VAT:** supplier bills and expenses with a valid invoice.
  - **Reverse-charge** amounts (foreign artists and suppliers), shown separately.
  - **Foreign-event VAT:** events outside the home country are listed apart (VAT is due where the event takes place; may need foreign registration or a local ticketing partner).
- **Withholding tax register:** per foreign artist fee, the amount withheld, rate, country, treaty-certificate reference, date paid to the tax office, and the certificate issued to the artist (e.g. DE §50a).
- All rate tables are versioned and dated, with a "verify with your accountant" notice. **No tax filing and no ELSTER / MTD submission.**

#### P5.8 Accountant exports
- **Generic CSV / XLSX** export: invoices, bills, expenses, payments, payouts and door close-outs for a period, with document links.
- **DATEV export (DE)** first: a booking-batch CSV in DATEV format with account mapping (SKR03 / SKR04 selectable) and receipt PDFs bundled as a ZIP.
- **Document bundle:** all invoices, bills and receipts for a period as a ZIP with an index CSV, named consistently.
- **Immutability for audits (GoBD-oriented):** issued and approved records are locked, corrections go through credit notes or reversals, and edits are logged in an audit trail.
- **Later:** direct API sync to Lexoffice, sevDesk, Xero and QuickBooks through a small `AccountingExporter` interface, the same adapter pattern as payments and marketing.

#### Explicitly out of scope
Double-entry general ledger and chart-of-accounts bookkeeping; automatic bank feeds (PSD2 aggregators such as GoCardless Bank Account Data are a possible later add-on); payroll and crew employment contracts; tax filing; full multi-currency accounting with live FX.

#### P5 build order
1. Shared finance org-scoping (DJ-regression-tested)
2. P5.3 bills and expenses
3. P5.1 budget and settlement
4. P5.2 outgoing invoices
5. P5.4 door cash
6. P5.5 ticketing payouts
7. P5.6 P&L and cash flow
8. P5.7 VAT and withholding summary
9. P5.8 exports

### P6 — Paid ticketing · Size XL
- A `PaymentProvider` interface (`CreateCheckout`, `HandleWebhook`, `Refund`, `Capabilities`) with payment and refund ledgers and idempotency keys.
- **Default: the promoter's own Stripe restricted key.** Optional Stripe Connect with direct charges and an application fee (never destination charges). Mollie for iDEAL and Bancontact. Hosted checkout only, so PCI stays at SAQ-A and SCA is handled by the provider.
- Reservations with a time limit (`cart_positions`) prevent overselling. Tiers and waves, promo codes, per-order limits, add-ons.
- **TaxRule per event**, taken from the venue country, with a reduced or standard rate. The rate is snapshotted on each position and fee, reusing the Phase-5 finance VAT code. Service fee is an `order_fee`.
- Refunds, transfers with **secret reissue**, and a waitlist with a claim window.
- On dispute: flag the order and revoke unscanned tickets.
- Optional **Ed25519 signed tickets** plus a revocation list for offline-only doors. QR shown only from T-24h.
- An "external ticketing" connector that reads pretix / Hi.Events orders and check-ins into our stats (API only; no AGPL code).

### P7 — Open-air / mini-festival pack · Size L
Permits and compliance tracker with a document vault; noise log; curfew enforcement on the timetable; accreditation (pass types × zones × quotas × approval, badge/wristband export); crew and volunteer shifts; production build/show/break schedule; multi-day and camping info; weather and evacuation contingency docs.

### Deliberately out of scope for now
Resale marketplace; SMS; public "anyone can earn" affiliate payouts; Meta Conversions API and managed ads; table/bottle floor plans; cashless and Tap to Pay; ActivityPub federation (a possible later fit via Gancio/Mobilizon interop); a native mobile app (the PWA covers the door).

---

## 6. Patterns to mirror

| Category | Source | Pattern |
|---|---|---|
| Naming (FE) | `apps/dj/app/pages/*.vue`, `apps/dj/app/stores/gig.ts` | Lowercase pages, `use*Store` setup-style Pinia stores, `$fetch` only inside store actions, `storeToRefs` |
| Mocks | `apps/dj/server/api/v1/finance/-mockDb.ts`, `apps/dj/server/api/v1/gigs/*` | Nitro file-routed mock handlers sharing one in-memory DB module; stripped from prod by `apps/dj/scripts/build-prod.mjs` |
| API proxy | `apps/dj/nuxt.config.ts` L19-28 | Prod build throws without `NUXT_PUBLIC_API_BASE`; `routeRules` + `devProxy` for `/api/v1/**` |
| Layering (Go) | `api/internal/gig/{handler,service,repository}.go` | Handler with `ServiceIface` → service with repo interfaces → pgxpool repository; soft delete |
| Router | `api/internal/platform/http/router.go` L55-105 | Mount per domain under `/api/v1/*`; `ServeFrontend` NotFound → Nuxt proxy |
| Secret gates | `api/internal/gig/handler.go` L55-80 | Constant-time compare, the starting point for hashed share tokens |
| Money/tax | `api/internal/finance/*`, `apps/dj/app/utils/money.ts` | Integer minor units, VAT snapshot per line, immutable issued documents |
| Email | `api/internal/finance/plunk_sender.go`, migration `017 email_messages` | Queue table + sender adapter, extracted to platform |
| PDF / render | `api/internal/gig/pdf.go`, `apps/dj/server/plugins/playwright.ts`, `render/tracklist/[id].vue` | fpdf for documents; Playwright screenshots of bare `/render/*` routes for images |
| Go tests | `api/internal/gig/integration_test.go` L43-120 | testcontainers `postgres:16-alpine` in `TestMain`, skipped under `-short`; `contract_test.go` |
| FE tests | `apps/dj/vitest.config.ts`, `app/stores/__tests__/` | jsdom Vitest, colocated `__tests__` |
| E2E | `apps/dj-e2e/playwright.config.ts` | nxE2EPreset, `serve-static` web server |

---

## 7. Files to change (P0 only; later phases get their own plan files)

| File | Action | Why |
|---|---|---|
| `pnpm-workspace.yaml` | UPDATE | Add `libs/*` |
| `libs/ui/{package.json,nuxt.config.ts,app/assets/css/kinetic.css,app/components/ui/*,app/components/shell/*,app/composables/{useTheme,useSidebar}.ts,app/lib/utils.ts,app/utils/{money,dialogFocus}.ts,public/fonts/*}` | CREATE (moved) | Shared Nuxt layer |
| `apps/dj/nuxt.config.ts`, `apps/dj/app/app.vue`, `apps/dj/app/assets/css/styles.css`, `apps/dj/components.json` | UPDATE | `extends` the layer; keep only DJ-specific classes; shell via props or slots |
| `apps/site/build.mjs`, `apps/site/project.json` | UPDATE | Read tokens from `libs/ui` |
| `apps/promoter/**`, `apps/promoter-e2e/**` | CREATE | New app and e2e project |
| `tsconfig.json` | UPDATE | Add project references |
| `api/cmd/promoter/main.go` | CREATE | Promoter binary wiring |
| `api/internal/platform/auth/**` | CREATE | Users, sessions, orgs, roles, share tokens, PINs, middleware |
| `api/internal/platform/email/**` | CREATE (extract from `finance/plunk_sender.go`) | Shared sender + SMTP adapter |
| `api/internal/platform/migrations/promoter/*.sql` | CREATE | Promoter schema (auth + org first) |
| `api/internal/platform/config/config.go` | UPDATE | Product-aware config (DB name, bucket, public base URL) |
| `api/project.json` | UPDATE | Build/test targets for both binaries |
| `apps/promoter/Dockerfile`, `apps/promoter/scripts/runtime.mjs` | CREATE | Image and supervisor |
| `docker-compose*.yml`, `scripts/stack_setup.py`, `.github/workflows/containers-ci.yml` | UPDATE | Promoter service, setup mode, CI image |
| `docs/SELF-HOSTING.md`, `README.md`, `CLAUDE.md`, `DESIGN.md` | UPDATE | Promoter docs, public-route exposure, shared layer |

## 8. Tasks (P0)

1. **Extract the UI layer.** Move the generic tokens and components into `libs/ui` and make `apps/dj` extend it.
   - Mirror: Nuxt layers; keep the `klubhub-theme` localStorage key.
   - Validate: `pnpm nx lint @dev/dj && pnpm nx typecheck @dev/dj && pnpm nx test @dev/dj && pnpm nx e2e dj-e2e`, plus a visual check in the built-in browser.
2. **Update the site build.** Point `apps/site` at the layer's CSS.
   - Validate: `pnpm nx build site`
3. **Scaffold `apps/promoter`** with a shell, empty pages and the mock mode.
   - Mirror: `apps/dj` config and mocks.
   - Validate: `pnpm nx serve @dev/promoter` (port 4400), then `pnpm nx lint @dev/promoter && pnpm nx typecheck @dev/promoter`.
4. **Add `apps/promoter-e2e`** with a smoke spec.
   - Validate: `pnpm nx e2e promoter-e2e`
5. **Build `platform/auth`.** Write tests first: password hashing, sessions, CSRF, role middleware, share-token hashing and expiry, PIN rate limiting.
   - Mirror: gig handler/service/repository layering and the integration-test harness.
   - Validate: `pnpm nx run api:test`
6. **Add the promoter binary.** Wire up config, DB, migrations and storage, `/api/v1/health`, and the auth routes.
   - Validate: `pnpm nx run api:test` plus a local `go build` through Nx.
7. **Extract the email platform.** Move the Plunk sender into it and add an SMTP adapter. The DJ finance email flow must not regress.
   - Validate: the finance integration tests.
8. **Packaging.** Add the Dockerfile, runtime, compose service, setup mode and CI job.
   - Validate: a `bash scripts/setup.sh` promoter dev mode boots, and health is green.
9. **Docs.** Update SELF-HOSTING (private instance, door access over LAN / VPN), README (OSS vs SaaS surface) and CLAUDE.md.

## 9. Validation

```bash
pnpm nx lint @dev/dj @dev/promoter
pnpm nx typecheck @dev/dj
pnpm nx typecheck @dev/promoter
pnpm nx run-many -t test
pnpm nx run api:test
pnpm nx e2e dj-e2e
pnpm nx e2e promoter-e2e
pnpm nx build site
```

## 10. Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Layer extraction regresses the DJ UI (CSS order, `@theme` scoping in Tailwind v4, shadcn aliases) | M | H | Move in one PR with screenshot-spec and built-in-browser before/after checks; keep DJ-only classes in the app |
| **First auth in the repo:** security bugs, and scope creep into the DJ app | H | H | Isolated `platform/auth` package, TDD, `security-reviewer` pass; DJ stays auth-less until explicitly decided |
| Self-hosted value is reduced without public pages (RSVP, submission links, native unsubscribe) (D2) | H | M | Export pack, external ticketing import, provider-hosted email lists, file-based Booking Exchange; `publicedge` boundary keeps the SaaS upsell clean |
| Door staff can't reach a private instance from the venue | M | H | Offline-first door PWA with pre-doors download; documented LAN / Tailscale access; sync after the night if needed |
| Email deliverability (Gmail/Microsoft 550 rejections) | H | M | Relay-only for marketing, DNS checker, RFC 8058, double opt-in, suppression lists |
| GDPR: guest names, ban lists, marketing consent | M | H | Minimal data, consent ledger, retention purge job, org-scoped bans with expiry, no ID images, DPA/ROPA notes in docs |
| Offline door sync conflicts and double entries | M | M | Client nonce idempotency, conflict flagging, pre-doors download, field test at a real event |
| Instagram API limits (no stickers, public media URL, per-instance Meta app) | H | L | Manual fallbacks, public Garage URL doc, reuse the existing DJ social worker |
| AGPL contamination from pretix / Hi.Events code | L | H | Concepts only; integrate over APIs; code-review rule |
| Booking Exchange across instances not reachable from each other (NAT, home servers) | M | M | Pull-based design with manual link paste; same-host mode; defer push |
| Paid ticketing compliance (VAT, refunds, disputes, SCA) | M | H | Deferred to P6; promoter-owned merchant account; hosted checkout; reuse finance VAT |
| Withholding and VAT tables drift from the law | M | M | Versioned, dated tables; "verify with advisor" UI copy; flags rather than auto-filing |
| Org-scoping the shared finance code breaks DJ invoicing (singleton `billing_profiles`, numbering) | M | H | Separate PR before P5; DJ keeps an implicit single owner; run the existing finance integration tests + migration tests unchanged |
| Accounting scope creeps into a full bookkeeping system | H | M | Hard out-of-scope list in P5; the product is "records + exports for the accountant"; ledger-like behaviour stays limited to payments and payouts |
| DATEV format or account mapping is wrong, so accountants reject exports | M | M | Validate against the DATEV format spec and sample imports; configurable SKR03/SKR04 mapping; generic CSV is always available as a fallback |
| Door cash handling disputes (theft suspicion, counting errors) | M | M | Two-person sign-off on variance; locked close-out PDF; audit trail with user, device and time |
| Receipts contain personal data; retention rules clash between GDPR (delete) and tax law (keep 8–10 years) | M | M | Financial documents follow tax retention (e.g. DE 8 years for receipts, 10 for books from 2025) and are excluded from the guest-data purge job; documented in the ROPA notes |
| arm64-only image limits self-hosters | M | M | Add amd64 to the promoter (and DJ) build matrix in P0 |
| Scope: this is a 7-phase product | H | M | Ship P0–P2 as the first usable release (events + landing + RSVP + guest list + door); P3 immediately after |

## 11. Decisions (answered 2026-09-25)

- **D1 Backend topology → second binary in the same `api/` module.** `api/cmd/promoter`, reusing `internal/platform`, with its own DB, bucket and `klubhub-promoter-api` image.
- **D2 Public pages → SaaS-only.** Self-hosted Promoter is an **admin-only, private** tool, like DJ today. Hosted public pages are a SaaS feature. See §12 for the consequences.
- **D3 MVP cut → P0 through P3.** The first release covers foundations, events, guest list and door, plus audience and promotion.
- **D4 DJ-app auth → Promoter only.** DJ stays single-user. Only the promoter binary uses `platform/auth`.
- **D5 Zero knowledge → Tiered.** Public data stays plaintext. Personal and financial data use per-org envelope encryption: the server can decrypt only during processing, and deleting the org destroys its key. The most sensitive items are **end-to-end sealed** in the browser, so the operator can never read them. See §13.
- **D6 Identity → Zitadel for SaaS only.** Self-host gets a simpler **built-in single-org login**. Both paths sit behind one `IdentityProvider` interface and produce the same `Principal`, so authorisation (OPA) and tenancy (RLS) are identical.
- **D7 Secrets → Infisical optional, env fallback.** Infisical is used for dev, CI and our own deploys. Self-hosters can use plain env or `*_FILE` secrets. Root-of-trust keys stay offline.
- **D8 Messaging → NATS JetStream in P0.** Go uses a transactional outbox → NATS (`nats.go`) with nsc JWT/NKey credentials per service and tenant-scoped subjects. **The Nuxt server uses the user-maintained [`nuxt-nats`](https://github.com/lithqube/nuxt-nats) module** (MIT, `~/dev/nuxt-nats`, beta 0.1.0-beta.3+).

## 12. What D2 (SaaS-only public pages) changes

The public surface is built as a **separate "public edge" module** (`api/internal/promoter/publicedge` plus Nuxt `apps/promoter/app/pages/(public)/*`). It is **not mounted by the self-hosted binary.** The SaaS repo (`klubhub-promoter-cloud`, per `docs/v2-saas-migration-plan.md`) imports the OSS module and mounts it. Clear boundary: **admin API = OSS/self-host; public edge = SaaS**, with the same data model in both.

| Feature | Self-hosted (OSS) | SaaS |
|---|---|---|
| Event page | **Export pack:** static HTML/ZIP page (SSR-rendered once), embed snippet for the collective's own site, OG images, JSON-LD block, `.ics`, copy-paste fields for RA / FB / DICE | Hosted `/e/:slug` with a live OG image |
| Collective hub | Export / embed widget (static JSON + JS) | Hosted `/c/:slug` with follow form |
| RSVP / free tickets | **External ticket link** (RA, DICE, pretix, Luma). CSV/API import of attendees into the guest table | Native RSVP checkout |
| Guest-list submission by DJs / promoters | **Booking Exchange by file:** allocation sent via the DJ app as a signed JSON file or email. Names come back via CSV/paste or a DJ-app export that the promoter imports | Secret submission links `/l/:token` |
| Door app | **Unchanged.** Door staff are authenticated users (PIN) of the private instance, reached over LAN / VPN / Tailscale. Offline-first matters even more here | Same |
| Email marketing | **The provider owns opt-in and unsubscribe.** Self-host sends through a provider with hosted list management (Brevo / Plunk / Resend audiences) so RFC 8058 unsubscribe and double-opt-in confirm URLs are **provider-hosted**. Signup forms are provider-embedded | Native `/u/:token` unsubscribe and opt-in pages |
| Social publishing | Unchanged (outbound only). Instagram still needs media at a public URL, so use a Garage public-read bucket/presigned URL or upload via the provider (documented) | Same |
| Attribution | UTM on outbound links; short links and QR point to the **external** ticket page | Native `/go/:code` short links |

**Plan edits this implies:**
- **P0:** auth stays; "public routes + rate limiting" becomes "a `publicedge` package boundary with contract tests, not mounted in OSS". The Caddy/Tunnel exposure guide is dropped. The self-host docs state that the instance must stay private.
- **P1:** the landing page is built once as an SSR render target (reusing the Playwright/Nitro `/render` pattern) that feeds both the **export pack** (OSS) and the SaaS route.
- **P2:** the RSVP schema (pretix-shaped) is still built, because imported external attendees and SaaS RSVP land in the same `orders/order_positions`. The self-host UI shows "external ticketing + import" instead of a checkout.
- **P3:** add a `MarketingProvider` interface (lists, contacts, consent sync, campaign send) with a Brevo adapter first (hosted forms, DOI and unsubscribe; free tier). Plain SMTP remains transactional-only.
- **P4:** the Booking Exchange gets **file/email transport as the default**. HTTP pull/push only happens where both sides can reach each other, or via SaaS.

## Acceptance (P0)
- [ ] DJ app renders identically on the shared layer; all DJ tests and e2e pass
- [ ] `apps/promoter` serves in mock mode with the shared shell and HUD theme (dark, light, system)
- [ ] Promoter binary boots with its own DB, bucket and migrations; auth, org and role tests pass under `-race`
- [ ] `klubhub-promoter-api` image builds in CI; setup script brings up a promoter dev stack
- [ ] Docs updated (self-hosting private instance, OSS vs SaaS surface, shared layer)
- [ ] Patterns mirrored, not reinvented

## 13. Security architecture (from D5–D8)

**References and IP rule:**
- **Copy freely** (the user's own IP): OpenSchild (`~/dev/OpenSchild`), mttl (`~/Mettal/mttl`, excluding `docs/reference/cto-ip/`) and nuxt-nats (`~/dev/nuxt-nats`).
- **Patterns only, never code** (client IP): bridge-latam (`~/Consulting/bridgeltm/bridge-latam-workspace`) and anything copied from it, including mttl's `docs/reference/cto-ip/`.
- None of the references has Go code, so the Go pieces below are written fresh.

### 13.1 Identity (D6)
- **`platform/auth.IdentityProvider`** has two implementations. Both produce the same `Principal{Sub, OrgID, Roles[], AMR[], AuthTime}`.
  - **`local` (self-host):** a single org, users created by an owner invite, argon2id passwords, **TOTP required for `owner|admin|finance`**, WebAuthn later. Sessions are server-side (a hashed session ID in an HttpOnly `__Host-` cookie, SameSite=Lax, CSRF via Origin check plus a double-submit token).
  - **`zitadel` (SaaS):**
    - The Nuxt server is the OIDC client and holds the tokens: nuxt-auth-utils, OIDC code + PKCE, tokens kept in a sealed cookie and **never exposed to the browser**.
    - The access token (JWT) is sent to the Go API as a Bearer token. Go validates it against JWKS (cached, with cooldown) and checks `iss`, `aud` (a map per route prefix, **failing closed when the audience is missing**), `exp` and `nbf`.
    - Roles come from `urn:zitadel:iam:org:project:roles`. **One Zitadel org = one collective = the `tenant_id`**, used as the Postgres RLS key and the NATS subject token.
    - Step-up: `prompt=login` plus `amr` including MFA and `auth_time` ≤ 15 min for destructive or finance actions.
    - Logout goes through `end_session` with `id_token_hint`. Redirects are sanitised.
    - Idempotent bootstrap script modelled on mttl `infrastructure/scripts/zitadel-bootstrap.sh`. The Zitadel adapter code lives in the OSS repo (MIT) but is only wired in by SaaS config.
    - Gotcha: fetch JWKS internally with the correct Host header (mttl `zitadelFetch.ts`).
- **Door devices:** a manager-provisioned device key plus a per-event PIN gives a **short-lived, event-scoped `door` session**. It can't export and can't reach other events.

### 13.2 Authorisation: OPA embedded in Go
- Policies live in `api/internal/platform/authz/policies/*.rego` (`package klubhub.promoter.authz`, `rego.v1`), are embedded with `go:embed`, and are compiled at boot through `github.com/open-policy-agent/opa/v1/rego`. There is **no sidecar, so there is no "OPA unreachable" failure mode.** Signed bundles are optional later.
- Input: `{principal:{sub, org_id, roles, amr, auth_time}, action, resource:{type, id, org_id, event_id, owner_sub, class}, context:{route, method, now, device_id}}`.
- Output: `{allow, deny_reason}`. **`default allow := false`, with no fallback for unlisted routes** (fixing the bridge bug).
- Every chi route declares its `action` and `resource` through a small registry. A test fails if any mounted route is missing from the registry or has no policy.
- `opa test` and a Go table test of the matrix run in CI. Denials are audit-logged with `deny_reason`.
- Examples: artist fees are hidden from the `marketing`/`door` roles; `door` sessions are limited to their own `event_id`; finance approval above a threshold needs `owner`.

### 13.3 Tenant isolation: Postgres RLS that fails closed
- Every tenant table has `tenant_id uuid not null`, `ENABLE` **and `FORCE`** ROW LEVEL SECURITY, and one policy `USING/WITH CHECK (tenant_id = current_setting('app.tenant_id', false)::uuid)`. With the `false` flag, a missing setting raises an error.
- Roles: the app connects as `klubhub_app` (not the owner, no BYPASSRLS). Migrations run as `klubhub_owner`. **There is no pool-level default tenant** (fixing the bridge pitfall).
- `db.WithTenant(ctx, pool, tenantID, func(pgx.Tx) error)` runs `BEGIN; SELECT set_config('app.tenant_id',$1,true)` and is the **only** way repositories get a transaction.
- A CI guard parses migrations and fails if any table with a `tenant_id` column lacks FORCE plus a policy. An integration test proves cross-tenant reads return nothing and a missing setting errors.
- Self-host runs the same code path with exactly one tenant.

### 13.4 Data classes and encryption (D5 tiered)
| Class | Examples | Protection | Who can read |
|---|---|---|---|
| `public` | Event title, lineup, venue city, published times | Plaintext | Anyone (export pack / SaaS pages) |
| `internal` | Budgets, timetables, templates, settings | Plaintext + RLS | Org members per OPA |
| `personal` | Guest names/emails/phones, audience contacts, submitter details, check-in rows | **Field-level envelope encryption** with a per-org DEK. HMAC-SHA256 **blind index** for exact lookup (email, normalised name tokens) | Server only while processing; org members per OPA |
| `financial` | Bills, invoices, payouts, cash counts, bank refs, withholding certificates | Envelope encryption (as above); issued PDFs encrypted in Garage with the DEK | As above |
| `sealed` (E2EE) | **Ban list, private notes on guests/artists, document vault (contracts, IDs, riders with personal data), deal memos/artist fees in negotiation, third-party API credentials** | Encrypted **in the browser** with the org's sealed key. The server stores ciphertext only and has no decryption path | Only members (and provisioned door devices) that hold the key |

- **Server-side envelope encryption:**
  - AES-256-GCM (Go `crypto/cipher`). AAD = `tenant_id‖table‖column‖row_id`.
  - One DEK per org, wrapped by the **deployment KEK**. The KEK comes from Infisical, a file or env and is **never stored in the DB**. Wrapped DEKs live in an RLS-forced `tenant_keys` table.
  - Key IDs are versioned for rotation. Deleting an org destroys its DEK, so its data is **crypto-shredded** (this meets GDPR Art. 17 even in backups).
  - A CI guard requires every column in a `personal`/`financial` table to be annotated with its class, and fails on unannotated ones.
  - Honest limit (as in bridge ADR-0009): root on a *running* host can read data while it is processed. This is documented in SECURITY.md and the UI.
- **Sealed tier (E2EE):**
  - Each member has an X25519 identity keypair generated in the browser. The private key is protected by an Argon2id key derived from a passphrase (passkey PRF later). The **org sealed key** is wrapped to each member's public key.
  - A **recovery kit is mandatory** when the org is created. Removing a member triggers rotation and re-wrap. Door devices get the sealed key wrapped to the device key when a manager provisions them, so the ban-list check works offline.
  - Crypto: WebCrypto (AES-GCM, X25519 where available), `@noble/curves` and `@noble/hashes` as fallback (OpenSchild `STACK.md` choice). The client-decrypt pattern follows mttl `card-decrypt.ts` (user IP).
  - PQC (hybrid X25519 + ML-KEM) goes in the algorithm-agility slot later.
- **Other at-rest protection:**
  - Backups: pg_dump + WAL, **age-encrypted to an offline recipient** (mttl `setup-backup-cron.sh`).
  - Garage objects: `personal`/`financial` files are encrypted by the app, `sealed` files by the browser.
  - LUKS is recommended in the SaaS runbook.
- **Operator view (SaaS):** we can see public data, internal metadata (counts, timestamps, sizes) and ciphertext. We see personal/financial plaintext only inside running jobs, and sealed data never. Pseudonymised audit/analytics use HMAC IDs and truncated IPs.

### 13.5 Secrets (D7)
- Infisical is used for dev (compose on its own port), CI (`Infisical/secrets-action`, GitHub-OIDC machine identity) and prod (agent rendering an env file with a Universal-Auth machine identity).
  - Wrappers `scripts/with-secrets.sh` and `secrets-seed.sh` are copied from OpenSchild/mttl and fall back to `.env`.
- The app itself only reads env or `*_FILE` values, never an SDK.
- **Offline custody (not in Infisical):** the deployment KEK backup, the Zitadel master key (SaaS), the NATS operator seed and the age backup identity.
- Boot asserts: the session secret is ≥ 32 bytes and the KEK is present and the right length. Otherwise the process refuses to start (OpenSchild `assert-session-password.ts`).

### 13.6 Messaging (D8)
- **Go (`nats.go`), transactional outbox:** an `outbox` table written in the same transaction as the domain change; a relay using LISTEN/NOTIFY plus `FOR UPDATE SKIP LOCKED` publishes with `Nats-Msg-Id` = row ID. **Payloads carry IDs only, no PII.**
- **Nuxt server (`nuxt-nats`):**
  - `useEphemeralConsumer()` powers **SSE endpoints** for live UI: door occupancy and check-ins across devices, guest-list submissions arriving, campaign send progress, render-job completion.
    - The subject filter is always built from the **session's tenant** (`tenant.{session.tenantId}.…`), never from client input.
  - Its `/api/_nats/health` endpoint is included in the app health check.
  - Graceful drain on SIGTERM works inside the existing `runtime.mjs` supervisor.
  - Typed subjects come through `NatsEvents` augmentation generated from a shared event-schema package, so Go and TS share one catalogue (JSON Schema → Go structs + TS types).
  - `useKV()` is available for short-lived UI state (e.g. door device heartbeat/presence).
  - The Nuxt server **does not publish domain events**; writes go through the Go API, which owns the outbox.
- **Subjects:** `tenant.{tenant_id}.{aggregate}.{verb}` (e.g. `tenant.X.event.published`, `tenant.X.guest.checked_in`, `tenant.X.campaign.send.requested`), plus `audit.{class}.{name}`, `system.*` and `dlq.*`.
- **Streams:** `PROMOTER_EVENTS`, `PROMOTER_JOBS` (work queue) and `AUDIT` (drained into an append-only `audit_log`). Dead letters use nuxt-nats `defineDeadLetterConsumer()` on the Nuxt side and advisories on the Go side.
- **Credentials:** nsc JWT/NKey per service with least-privilege permissions:
  - `promoter-api`: publish `tenant.*.>` and `audit.>`
  - `promoter-worker`: pull from JOBS
  - `outbox-relay`
  - `promoter-bff` (nuxt-nats): subscribe-only on the ephemeral-consumer API for `tenant.*.>` UI subjects; no publish on domain subjects
  - A rotation script. In SaaS, per-tenant subject ACLs come via auth callout later.
- **Consumers:** idempotent pull consumers named `{stream}_{svc}_{purpose}`. They drive campaign sends, reminders, retention purges, render jobs and door-sync fan-out.

### 13.7 Hardening
- **Nuxt:** `nuxt-security` CSP with a per-request nonce, HSTS, frame-deny, nosniff, strict referrer policy, Permissions-Policy.
- **Rate limits** on auth, PIN and export routes, as in OpenSchild's config (avoiding the nitropack `'POST /path'` key gotcha).
- **Go:** security headers, request size limits (already present), and PIN attempt lockout backed by Redis or Postgres.
- **Audit log:** append-only, capturing actor type, tenant, action, resource, decision and `deny_reason`. IPs are HMAC'd or truncated.
- **Supply chain:** gitleaks in CI and lefthook, Trivy image scan, SBOM (syft), cosign signing of `klubhub-promoter-api`, pinned image digests (no `:latest`), Dependabot.
- **Self-host guide:** the instance stays private (D2). Door access goes over LAN, VPN or Tailscale.

### 13.8 P0 task list (supersedes §8 tasks 5–8)
1. `libs/ui` Nuxt layer (unchanged).
2. `apps/promoter` + e2e scaffold, with `nuxt-security`, `nuxt-auth-utils` and `nuxt-nats` installed.
3. `platform/db`: `WithTenant`, app and owner roles, the RLS migration template, and the RLS CI guard with integration tests.
4. `platform/crypto`: KEK loader, DEK wrap/unwrap, field encryption with AAD, blind index, crypto-shred, and class annotations with a CI guard. Test-first.
5. `platform/authz`: embedded OPA, route registry, policies plus `opa test`, and a coverage test.
6. `platform/auth`: the `IdentityProvider` interface; the `local` provider (argon2id, TOTP, sessions, CSRF, invites); the `zitadel` provider (JWKS/audience/claims, not wired in OSS); door device and PIN sessions.
7. `platform/events`: outbox table, relay, `nats.go` client with nsc credentials, stream bootstrap, and a shared event-schema catalogue (Go + TS for nuxt-nats typed subjects).
8. Promoter binary wiring: health, auth routes, org bootstrap.
9. Infra: compose with db, garage, nats (JetStream, nsc creds script), the app and optional Infisical; a SaaS overlay with Zitadel and Login V2; setup script; secrets wrappers.
10. CI: gitleaks, `opa test`, RLS and crypto guards, Trivy, SBOM, cosign.
11. Docs: SECURITY.md (threat model and operator-visibility statement) plus ADRs under `docs/adr/` for D5–D8.
12. Sealed-tier (E2EE) browser key management lands with the first sealed feature (P2 ban list), with the ADR written in P0.
