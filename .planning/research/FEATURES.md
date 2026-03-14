# Feature Research

**Domain:** DJ career management platform — self-hosted, open-source, single-user
**Researched:** 2026-03-14
**Confidence:** MEDIUM (WebSearch unavailable; analysis based on BRD/Addendum source documents plus training-data knowledge of competitor landscape: Buffer, Later, Hootsuite, Canva, EPK.io, Sonicbids, Indiemono, spreadsheet workflows, Rekordbox/Serato export behaviors)

---

## Competitive Landscape (Training Data — LOW confidence for current details)

| Tool | What It Does | Why DJs Use It | Gaps |
|------|-------------|----------------|------|
| **Buffer / Later / Hootsuite** | Social scheduling | Post tracklist images | Not music-aware; no tracklist parsing; paid |
| **Canva** | Image design | Tracklist visuals manually | Manual typing; no DJ file import; not specialized |
| **EPK.io / Sonicbids** | Press kit hosting | Send to promoters | Subscription cost; cloud lock-in; generic |
| **Notion / Airtable** | Gig and release tracking | Flexible spreadsheet-like | No specialization; no automation; no image gen |
| **Spreadsheets (Excel/Sheets)** | Finance + gig tracking | Universal fallback | No visualization; no automation; no invoicing |
| **Rekordbox (export only)** | Tracklist history export | Source of truth for sets | No visual output; no social pipeline |

**Key gap KlubHub DJ fills:** No single tool chains tracklist file → visual → social post → gig record → invoice. Each transition is manual today.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Features that DJs will expect from any career toolkit. Missing these = product feels broken or incomplete before they even discover the differentiators.

#### Module 1a — Tracklist Image Generator

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Upload DJ history file (drag-and-drop) | Every comparable tool (Canva, image editors) accepts drag-and-drop; feels 2010 without it | LOW | File picker fallback required too |
| Auto-detect Rekordbox, Serato, Traktor formats | These are the top 3 DJ software platforms; DJs expect their specific software to be supported | MEDIUM | Parser interface pattern handles this cleanly |
| Display parsed tracks for review before generating | DJs will not trust a "black box" that generates without confirmation | LOW | Essential trust-builder; per-track warnings critical |
| Inline edit track metadata after parsing | Serato/Rekordbox history often has truncated or incorrect metadata; edits are essential not optional | LOW | FR-003a in Addendum |
| Cover art fetched automatically | Any tracklist image tool that requires manual image upload for each track will be abandoned immediately | HIGH | Fallback chain (Spotify → Discogs → MusicBrainz) adds complexity |
| Live preview before full export | Without preview, export feels risky; DJs will generate dozens of times with trial-and-error | MEDIUM | 50% resolution preview per A5.1 is correct approach |
| Export PNG and JPEG | Minimum viable download; PNG for quality, JPEG for smaller file size | LOW | Both sizes (Story + Square) expected |
| Persist last-used settings | DJs generate images every week; re-configuring from scratch each time is a dealbreaker | LOW | Simple user_settings row in DB |
| Logo/watermark placement | DJs brand their visuals; this is universally expected in social-content tools | LOW | Corner position control is sufficient for v1 |

#### Module 1b — Social Media Scheduler

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Schedule posts for future time (timezone-aware) | Any scheduler that ignores timezone is broken for DJs who travel internationally | LOW | Per-post timezone override is the right model |
| Queue / calendar view of scheduled posts | Without visibility into the schedule, users double-post or forget what's queued | MEDIUM | List view minimum; calendar preferred |
| Post status tracking (scheduled / published / failed) | Users need to know if a post failed; silent failure is a trust-destroying bug | LOW | Status enum + dashboard surfacing |
| Retry on failure with notification | Instagram API fails transiently; silent failure = post never goes out | MEDIUM | 3x exponential backoff + permanently_failed state |
| Edit a post before it publishes | DJs often schedule then realize caption needs adjustment | LOW | Block edits only when status = publishing |

#### Module 1c — EPK / Press Kit Builder

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Bio (short + long) with basic rich text | An EPK without a formatted bio is not an EPK | LOW | Bold, italic, lists, links — no custom fonts needed |
| Press photo upload and management | Promoters expect hi-res press photos from an EPK | LOW | Up to 20 photos, 10MB each per A8.2 |
| Tech rider entry | Every serious DJ sends a tech rider; missing this = not a serious EPK tool | LOW | Text entry; stage plot image upload |
| Export as PDF | An EPK that can't be emailed as an attachment is not usable in the real world | HIGH | Pure Go PDF (gofpdf) — layout flexibility is the hard part |
| Section include/exclude control | Different promoters want different things; one-size PDF fails for all | LOW | Checkbox per section |

#### Module 1d — Gig Tracker

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Create/edit/delete gig entries | Basic CRUD is the floor; anything less is a notes app, not a tracker | LOW | Standard REST endpoints |
| Gig status workflow | DJs track inquiry → confirmed → played; "confirmed" with no lifecycle is useless | LOW | Enum with defined transitions per A9.1 |
| Payment status (independent of gig status) | A played gig can be unpaid; conflating statuses creates confusion | LOW | Separate enum; this design decision is correct |
| Calendar and list views | Calendar is how DJs think about gigs; list is how they search/filter | MEDIUM | Both views expected by anyone who has used a gig tracker |
| Filter by date, status, fee | Without filters, tracker becomes unusable at 50+ gigs | LOW | Standard filtering |

#### Module 1e — Finance Tracker

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Log income and expenses manually | Any finance tracker without manual entry is broken | LOW | Core CRUD |
| Category breakdown (gig fee, travel, etc.) | Without categories, summaries are meaningless | LOW | Enum categories |
| Monthly/yearly summary views | DJs need to report income to accountants/tax authorities; summaries are minimum viable | LOW | Aggregate queries |
| PDF invoice generation | Many promoters require invoices; no invoice = no payment in some markets | MEDIUM | gofpdf; invoice numbering convention per A10.3 |
| Multi-currency display (no conversion) | DJs work internationally; GBP fee + EUR travel + USD streaming are all normal | LOW | Group by currency is correct; auto-conversion is the anti-feature |

#### Module 1f — Release Planner

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Create/track releases with status | Any release tool without a status workflow is a glorified notes list | LOW | Standard CRUD + enum |
| Deadline tracking per release | Mastering deadlines, artwork deadlines, release date — missing any = release delays | LOW | release_deadlines table |
| Promo checklist per release | The release promo workflow is highly repetitive; DJs and labels run the same checklist every time | LOW | Default template + per-release customization per A11.2 |

#### Module 1g — Tour Manager

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Group gigs into a named tour | Without grouping, tour management is just individual gig entries | LOW | tours + tour_stops tables |
| Per-stop logistics (travel, accommodation) | This is why you'd use a tour manager vs. a gig tracker | MEDIUM | JSONB fields; flexible per-stop structure |
| Tour budget aggregate | Knowing tour profitability requires aggregating across stops | LOW | Sum queries grouped by currency |

#### Cross-Cutting

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Unified dashboard | Without a home screen that surfaces upcoming gigs + scheduled posts + recent tracklists, the app feels like 7 disconnected tools | MEDIUM | Aggregate query across modules |
| Single docker-compose up deployment | Stated as core value proposition; self-hosters expect "one command" | MEDIUM | Already decided; 4 services |
| Data backup/restore scripts | Self-hosters will ask "how do I backup?" immediately; no answer = trust failure | LOW | scripts/backup.sh + restore.sh |
| Soft delete + trash view | Accidental deletes happen; no recovery = user complaint on day one | LOW | deleted_at pattern; restore endpoint |

---

### Differentiators (Competitive Advantage for Open-Source Self-Hosted)

These features are why DJs choose KlubHub DJ over the fragmented SaaS stack. Not all are unique in isolation — their combination and the open-source/self-hosted nature create the actual moat.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **DJ file → image in under 60 seconds, no typing** | The entire existing workflow (screenshot → Canva → manual typing) takes 15-30 minutes. Eliminating this is the flagship differentiator | HIGH | Auto-detect + parse + cover art fetch pipeline is the core IP |
| **Cover art mosaic background from set artwork** | Canva has no concept of "the tracks I played"; this visual is only possible with actual tracklist data | MEDIUM | Automatic grid from the set's cover art; genuinely novel for a free tool |
| **3 format parsers (Rekordbox, Serato, Traktor)** | Tools that only support one format lose 2/3 of the addressable market immediately | HIGH | Modular parser interface; each format has quirks |
| **Tracklist-to-social post handoff (one click)** | The cross-module CTA after export is differentiating because no tool chains these two actions | LOW | Post-export CTA per A7.1 |
| **Gig-linked tracklists and images** | DJs want their gig history and tracklist history to be one record, not separate tools | LOW | FK relationship; cross-module linking |
| **Auto income entry on gig payment** | Eliminates a manual step that every independent DJ currently does by hand in a spreadsheet | LOW | Finance auto-link per A10.1 |
| **Zero external data dependency (truly self-hosted)** | No cloud lock-in, no SaaS fees, no account required — this is the entire value proposition for privacy-conscious DJs and those in markets where SaaS tools are expensive | MEDIUM | Docker Compose + MinIO + PostgreSQL; everything local |
| **Gig → invoice in one workflow** | DJs who invoice promoters currently re-type gig details into a Word template or Stripe/PayPal. Auto-population from gig data removes that | MEDIUM | PDF invoice from gig data per FR-055 |
| **EPK auto-populated from live gig data** | Static Canva/Word EPKs get stale; an EPK that pulls from the gig tracker's confirmed shows is always current | LOW | Import from Gigs button per A8.4 |
| **Open source MIT license** | DJs and developers can inspect, contribute, and extend; no vendor risk | LOW | License decision — drives community contributions |
| **Non-Latin / Unicode tracklist support** | Japanese, Korean, Cyrillic artist names are common in electronic music; tools that garble these are non-starters for global DJs | MEDIUM | Noto Sans bundled fonts; glyph fallback per A2.7 |
| **Retry + "Download Image" fallback for failed posts** | When Instagram fails, DJs can still post manually; most schedulers just show an error and leave the user stuck | LOW | permanently_failed state + Download Image button per A6.7 |
| **Track range selector for large tracklists** | 4-hour sets have 100+ tracks; tools that truncate without warning lose trust | LOW | max_tracks_displayed + range selector per A2.6 |

---

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem reasonable or will be requested by users but should be deliberately excluded in v1. Each is either a significant complexity trap, out of scope, or a distraction from the core value.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Multi-user / authentication** | Agencies and collectives want a shared instance | Adds auth layer, session management, role-based permissions, data isolation, invitation flows — this is a 3-month project on its own; delays everything else | Plan for v3.0+; single-user design keeps v1 deployment trivially simple |
| **Automatic currency conversion (live rates)** | DJs want to see "total income this year in EUR" | Requires external rate API; rates fluctuate; creates false precision; tax treatment of conversions is jurisdiction-specific | Group by currency; let users decide conversion; document this as intentional |
| **Tax calculation / VAT** | DJs want to know what they owe | Tax law varies by country, entity type, and income type; implementing incorrectly creates liability; "tracking tool, not accounting software" is the right boundary | Export CSV/PDF data; "take this to your accountant" is the correct v1 answer |
| **AI-generated backgrounds** | Looks impressive in demos | Requires GPU or paid API (OpenAI DALL-E, Stability AI); increases Docker image size dramatically; adds cost and latency; real value is "my cover art mosaic", not generic AI art | Deferred to v2.0; cover art mosaic is the genuine differentiator |
| **External platform integrations (RA, Beatport, Bandcamp) in v1** | DJs want gig data auto-imported from Resident Advisor | API stability varies; RA has no official public API; scraping is fragile; manual entry is 100% reliable and ships faster | Integration adapter interfaces defined from day one; actual integrations in v2.0 |
| **Real-time collaboration / sharing** | Users want to share EPKs via link | Requires auth model, public URL routing, access control, rate limiting on public endpoints — adds substantial complexity | Export PDF and share the file; hosted web EPK deferred to v3.0+ |
| **Full design editor (custom fonts, drag-and-drop)** | Designers want full control over tracklist image layout | Drag-and-drop canvas editors (like Fabric.js or Konva.js) are complex to implement, complex to maintain, and create a very wide bug surface; 3-5 strong presets ship faster and satisfy 80% of users | Presets + color customization in v1; full editor in v3.0+ |
| **TikTok posting in v1** | TikTok is large; DJs post there | TikTok's API for non-video (image) posts is restricted; API review process is different from Instagram; splits social testing complexity | v1.x follow-up after Instagram is stable |
| **Hashtag automation / AI caption generation** | DJs want captions written for them | Requires LLM API (cost, dependency); genre/label extraction for hashtag suggestions is a feature, not the tool; manual captions with hashtag fields is sufficient | Genre-based hashtag suggestions (from track label metadata) are a v1.x enhancement |
| **Contract management / e-signatures** | Booking contracts are part of gig workflow | DocuSign-equivalent is a separate legal tech product; e-signature APIs (HelloSign, Adobe Sign) have compliance requirements; out of scope | Document upload to tour_stops covers storing signed contracts; creation/signing is out of scope |
| **Mobile app** | DJs check schedules on phones | Separate native app is a full separate project; web app should be responsive enough for viewing on mobile | Responsive web-first; native app deferred to v3.0+ |
| **Beatport/Spotify artist analytics** | DJs want streaming stats in one place | Read-only analytics dashboards require API partnerships (Spotify for Artists API is invite-only); these are already good in their native apps | Deferred to v2.0 integrations roadmap |
| **Stripe/PayPal payment processing** | Promoters could pay through the app | PCI compliance, payment processor agreements, fraud risk — this is a payments company problem, not a DJ toolkit problem | Track payment status manually; link to external payment tools |

---

## Feature Dependencies

```
[Cover Art Fetch]
    └──required by──> [Tracklist Image Generation]
                          └──required by──> [Social Media Scheduler (attach image)]
                                                └──required by──> [Post-Export CTA]

[Gig Tracker]
    └──required by──> [Finance Tracker (gig fee auto-link)]
    └──required by──> [Tour Manager (tour stops require linked gigs)]
    └──required by──> [EPK "Import from Gigs" button]
    └──required by──> [Invoice generation (gig data populates invoice)]

[User Settings (DJ name, logo, colors)]
    └──required by──> [Tracklist Image Generation (logo overlay, colors)]
    └──required by──> [EPK PDF (branding applied to PDF)]
    └──required by──> [Invoice PDF (DJ name, bank details)]

[PostgreSQL + MinIO infrastructure]
    └──required by──> ALL modules

[Tracklist]
    └──enhances──> [Gig Tracker (link tracklist to gig)]
    └──enhances──> [Social Scheduler (attach tracklist image to post)]

[Finance Tracker]
    └──enhances──> [Gig Tracker (payment status auto-creates income entry)]
    └──enhances──> [Tour Manager (tour budget uses gig fees + expense entries)]

[Multi-user auth] ──conflicts──> [Single-user no-auth design in v1]
[Tax calculation] ──conflicts──> [Explicit "tracking tool not accounting software" scope boundary]
[Live currency rates] ──conflicts──> [Multi-currency group-by-currency approach]
```

### Dependency Notes

- **Cover Art Fetch requires external API keys (Spotify, Discogs):** MusicBrainz is keyless but rate-limited; the system degrades gracefully if Spotify/Discogs keys are missing. Cover art is not a hard blocker for image generation — placeholder artwork is used instead.
- **Social Scheduler requires Instagram OAuth:** This is the only module with a mandatory external service setup. The module should disable gracefully (503 responses) when not configured, per A14.4. This is the highest-risk dependency in the whole platform.
- **Gig Tracker is a hub:** Finance Tracker, Tour Manager, and EPK (via Import from Gigs) all depend on the Gig Tracker data. Phase ordering (1d before 1e and 1g) is mandatory, not optional.
- **User Settings is a singleton dependency:** Any module that uses the DJ's logo, name, or brand colors must read from user_settings. This table must exist before any PDF or image generation.
- **Finance auto-link depends on Gig payment_status transition:** The trigger is the payment_status column transition to `paid`. Finance Tracker cannot meaningfully auto-link until Gig Tracker ships.

---

## MVP Definition

### Launch With (v1, Phase 1a — Core Flagship)

The tracklist image generator is the single must-ship feature that establishes the product's identity and attracts early adopters. Everything else builds on this.

- [x] File upload (drag-and-drop + file picker) — without this the product literally cannot start
- [x] Auto-detect and parse Rekordbox, Serato, Traktor — without 3 formats, 2/3 of users are excluded
- [x] Parsed tracklist review with inline editing — trust layer; DJs must see and correct what was parsed
- [x] Cover art fetch with fallback chain + placeholder — visual quality is the product's core promise
- [x] Per-track cover art override — essential for tracks with wrong or missing art
- [x] 3–5 design presets + color customization — differentiation from "just export the data"
- [x] Story (1080×1920) and Square (1080×1080) output — these two formats cover Instagram Story + Feed
- [x] Cover art mosaic background — this is the flagship visual differentiator
- [x] Logo/watermark upload with corner control — DJ branding; expected immediately
- [x] Live preview (50% resolution) — required to feel professional and trustworthy
- [x] PNG + JPEG export — table stakes download formats
- [x] Persist last-used settings — reduces friction on repeat use; one DB row
- [x] Track range selector for large sets (100+ tracks) — can't ship without handling real-world set sizes
- [x] Unicode / non-Latin rendering (Noto Sans) — electronic music has global artist names; this is launch-blocking

### Add After Validation (v1.x)

- [ ] TikTok posting — after Instagram integration is stable and proven
- [ ] Text-only tracklist export (1001tracklists format) — requested frequently; low complexity
- [ ] Additional output formats (Twitter/X banner, A4 poster) — after Story/Square patterns are established
- [ ] CSV/JSON data export for all modules — power-user feature; add when users ask for data portability
- [ ] Calendar sync (iCal export, Google Calendar) — gig tracker enhancement; low complexity

### Future Consideration (v2+)

- [ ] AI-generated backgrounds — requires GPU/paid API; defer until cover art mosaic is validated
- [ ] External platform integrations (RA, Beatport, Bandcamp) — adapter interfaces already planned; implement after v1 modules are stable
- [ ] Hosted EPK web pages with shareable links — requires auth model to be useful; deferred to v3+
- [ ] Full drag-and-drop design editor — complex; 3–5 presets are sufficient for v1 and v2
- [ ] Multi-user support — v3.0+ per the roadmap; adds auth layer, role permissions, data isolation
- [ ] Analytics dashboard (gig stats, income trends) — useful but not MVP; data exists in DB after v1 ships
- [ ] Contact / network CRM (promoters, labels, agents) — inline promoter fields on gigs are sufficient for v1; full CRM is a separate product domain

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| DJ file parsing (all 3 formats) | HIGH | HIGH | P1 |
| Cover art fetch + mosaic | HIGH | HIGH | P1 |
| Tracklist image generation (presets, sizes) | HIGH | HIGH | P1 |
| Live preview | HIGH | MEDIUM | P1 |
| Persist user settings | HIGH | LOW | P1 |
| Logo/watermark overlay | MEDIUM | LOW | P1 |
| Unicode rendering | HIGH | MEDIUM | P1 |
| Social post scheduling (Instagram) | HIGH | HIGH | P1 |
| Gig CRUD + status workflow | HIGH | LOW | P1 |
| Calendar + list views (gigs) | HIGH | MEDIUM | P1 |
| Finance income/expense logging | HIGH | LOW | P1 |
| PDF invoice generation | HIGH | MEDIUM | P1 |
| EPK bio + photos + PDF export | HIGH | HIGH | P1 |
| Tour manager (group gigs) | MEDIUM | MEDIUM | P2 |
| Release planner + promo checklist | MEDIUM | LOW | P2 |
| Cover art mosaic background | HIGH | MEDIUM | P1 |
| Track range selector | MEDIUM | LOW | P1 |
| Auto income entry on gig payment | MEDIUM | LOW | P2 |
| Post-export "Schedule Post" CTA | HIGH | LOW | P1 |
| Backup/restore scripts | MEDIUM | LOW | P1 |
| Unified dashboard | HIGH | MEDIUM | P1 |
| Data export CSV/JSON | LOW | LOW | P3 |
| AI backgrounds | LOW | HIGH | P3 |
| External platform integrations | MEDIUM | HIGH | P3 |
| Multi-user auth | HIGH | HIGH | P3 |
| Full design editor | MEDIUM | HIGH | P3 |

**Priority key:**
- P1: Must have for launch (v1 phases)
- P2: Should have, adds value once P1 is working
- P3: Nice to have, future consideration

---

## Competitor Feature Analysis

| Feature | Buffer/Later | Canva | EPK.io / Sonicbids | Spreadsheets | KlubHub DJ |
|---------|-------------|-------|-------------------|-------------|------------|
| DJ file parsing (Rekordbox/Serato/Traktor) | No | No | No | No | Yes — flagship |
| Tracklist image generation | No | Manual | No | No | Yes — auto from file |
| Cover art mosaic background | No | No | No | No | Yes — unique |
| Social scheduling | Yes | Yes (limited) | No | No | Yes — music-aware |
| EPK / press kit | No | Templates only | Yes | No | Yes — linked to gigs |
| Gig tracking | No | No | Basic | DIY | Yes — full workflow |
| Finance tracking | No | No | No | DIY | Yes — with invoicing |
| Release planning | No | No | No | DIY | Yes — with deadlines |
| Tour management | No | No | No | DIY | Yes — with budgets |
| Self-hosted / no SaaS fees | No | No | No | Sort of | Yes — core promise |
| Open source | No | No | No | N/A | Yes — MIT license |
| Multi-platform social posting | Yes | Limited | No | No | Instagram v1 only |
| AI features | Some | Yes | No | No | No (v2 roadmap) |
| Cost | Paid SaaS | Freemium | Paid SaaS | Free | Free, self-hosted |

**Verdict:** No single competitor covers the DJ-specific file → image → social → gig → finance → EPK workflow. KlubHub DJ's differentiation is the combination and the open-source self-hosted nature, not any single feature in isolation.

---

## Sources

- `docs/KlubHub_DJ_BRD_v2.0_Platform.md` — primary feature specification; all 7 modules defined
- `docs/KlubHub_DJ_BRD_v2.0_Addendum.md` — clarified requirements, edge cases, data model amendments
- `docs/KlubHub_DJ_NFR_v1.0.md` — non-functional constraints that bound feature scope
- `.planning/PROJECT.md` — project context, out-of-scope decisions, key architectural choices
- Training-data knowledge of: Buffer, Later, Hootsuite, Canva, EPK.io, Sonicbids, Rekordbox/Serato/Traktor export formats, Instagram Graph API behavior — LOW confidence for current product details; HIGH confidence for general capability categories

---

*Feature research for: KlubHub DJ — DJ career management platform (self-hosted, open-source)*
*Researched: 2026-03-14*
