# Monetization Research: Open-Core SaaS Model for KlubHub DJ

**Researched:** 2026-03-20
**Status:** Proposal — not yet decided
**Source:** Analysis of docs/ (BRD v2.0, NFR v1.0, ARCHITECTURE.md) cross-referenced with .planning/ (ROADMAP, REQUIREMENTS, research/)

---

## Executive Summary

KlubHub DJ's architecture — modular monolith with interface-first module boundaries, v3 multi-user guardrails, and clean phase independence — is **already structured for an open-core SaaS split**. The key insight: modules that require infrastructure, external API management, or reliability guarantees are natural SaaS candidates, while privacy-sensitive data modules and the flagship identity feature should remain open-source to build trust and community.

**Recommended model:** Open-Core with a hosted SaaS tier.

---

## The Split: What Stays Open-Source vs. What Goes SaaS

### Tier 1: Open-Source (MIT License — Free Forever)

These modules form the **core identity** of KlubHub DJ and must remain free to:
- Maintain the "self-hosted, own your data" promise (the core value proposition per BRD)
- Build community and contributor base
- Keep the flagship feature as the marketing funnel

| Module | Phase | Why Open-Source |
|--------|-------|-----------------|
| **Infrastructure** | 0 | Foundation — breaking this out would kill self-hosted story |
| **Tracklist Image Generator** | 1 | **Flagship identity** — "upload file, get image in 60 seconds" IS the product. Must be free to create viral adoption. Includes text export (1001Tracklists/Mixcloud/SoundCloud). |
| **Design System** | 1.5 | UI foundation — no value as paid |
| **Social Media Scheduler (Instagram)** | 2 | Instagram scheduling completes the tracklist→post workflow. Paywalling the last step of the flagship workflow frustrates the core user at peak value. Multi-platform (TikTok, Twitter/X) is SaaS PRO. |
| **Gig Tracker** | 4 | Privacy-sensitive data (fees, contacts, venues). Hub module — restricting it blocks 3 downstream modules. Now includes venue/contact DB, iCal feed, and booking confirmation PDF. |
| **Contacts & Venue Database** | 4 | User owns their professional network. Table-stakes for any booking tool. |
| **Rider Templates** | 4.5 | User owns their technical requirements. Completes the gig advancing workflow. |
| **Finance Tracker** | 5 | **Highly privacy-sensitive** (income, expenses, invoices). DJs will not enter financial data into a system that could paywall their own records. Trust signal. |
| **Release Planner** | 6 | Simple CRUD — not enough value to justify payment. Better as a "stickiness" feature that keeps users in the ecosystem. |
| **Tour Manager** | 7 | Basic tour planning (named gig groups, per-stop logistics, budget aggregate) is user data. Collaboration is SaaS TEAM. |
| **Unified Dashboard + Career Analytics** | 8 | Aggregation of data the user already owns. Analytics derived from their own gig history — no external APIs. |

**Why this split matters:** The open-source modules cover the **data-ownership** story. A DJ's tracklists, gig history, contacts, finances, releases, and tours are *their* data. Keeping all of this free builds the trust needed to sell the SaaS tier.

---

### Tier 2: SaaS Premium (Hosted Cloud — Subscription)

These features have characteristics that naturally justify SaaS pricing:
- They require **infrastructure you manage** (multi-platform OAuth, rate limit orchestration, CDN/SSL/domain routing)
- They provide **automation value beyond a single platform** (cross-platform publishing, AI generation)
- They involve **collaboration** that requires always-on hosted infrastructure
- They require **hosted web serving** that self-hosters cannot trivially replicate

| Feature | Why SaaS | SaaS Value Proposition |
|---------|----------|----------------------|
| **Multi-platform Social Posting** (TikTok, Twitter/X, Facebook) | Requires centralized API key management + rate limit orchestration across platforms that is impractical to self-host. Instagram stays OSS; multi-platform is the upgrade. | "Post to TikTok, Twitter, and Facebook automatically alongside Instagram — from the same tracklist export." |
| **AI Caption Generation** | Requires hosted Claude API key management; cost-per-use model is incompatible with self-hosted. | "Let AI write your post caption from your tracklist data. One click, three variants." |
| **Hosted EPK Page** (`klubhub.dj/epk/name`) | Requires web hosting, SSL, CDN, domain routing — fundamentally not self-hostable without DevOps expertise. PDF is always free; the public URL is the SaaS value. | "Share your press kit as a link, not an attachment. `klubhub.dj/epk/your-name` — always up to date." |
| **Collaborative Tour Planning** | Real-time shared editing (invite manager/agent/promoter) requires WebSocket service + hosted always-on infrastructure. | "Share tour logistics with your team. Managers update per-stop details without accessing your full account." |
| **Custom EPK Domain** (TEAM tier) | DNS verification + automatic SSL requires hosted infrastructure and domain management. | "Your press kit on your own domain: `epk.yourname.com`." |

---

## Pricing Model Recommendation

### Model: Freemium Open-Core + Hosted SaaS

```
FREE (Self-Hosted, Open-Source MIT)
├── Tracklist Image Generator (unlimited) + text export (1001Tracklists, Mixcloud, SoundCloud)
├── Social Scheduler: Instagram (unlimited posts)
├── EPK Builder: unlimited press kits as PDF
├── Gig Tracker (unlimited) + Contacts & Venue DB + iCal feed + booking confirmation PDF
├── Rider Templates (unlimited)
├── Finance Tracker (unlimited, PDF invoices)
├── Release Planner (unlimited)
├── Tour Manager (basic: groups, logistics, budget)
├── Unified Dashboard + Career Analytics
├── Design System & Full UI
└── Docker Compose deployment

CLOUD FREE TIER (Hosted)
├── Everything in Self-Hosted FREE
├── Multi-platform social: 4 cross-platform posts/month (TikTok, Twitter/X, Facebook)
├── EPK Builder: 1 hosted press kit page (no custom domain)
├── 500 MB cloud storage
└── Community support

PRO — $9/month or $89/year
├── Everything in Cloud Free
├── Multi-platform social: unlimited posts (Instagram + TikTok + Twitter/X + Facebook)
├── AI caption generation from tracklist data (unlimited)
├── Hosted EPK page: unlimited press kits at klubhub.dj/epk/name with custom branding
├── Priority cover art fetching (pooled API keys, faster lookups)
├── 10 GB cloud storage
├── Email support
└── Early access to new features

TEAM — $25/month or $249/year
├── Everything in PRO
├── Multi-user: up to 10 DJs (collective, label, agency)
├── Collaborative tour planning (invite managers/agents, real-time shared checklists)
├── Custom domain for hosted EPK page (epk.yourname.com)
├── Shared template library across team
├── 50 GB cloud storage
├── Team analytics (aggregate stats across members)
├── Priority support
└── Custom branding removal
```

### Why These Price Points

| Tier | Comparable Market | Reference |
|------|-------------------|-----------|
| **$9/mo PRO** | Buffer ($6/mo 1 channel), Later ($25/mo), Linktree Pro ($9/mo), Canva Pro ($13/mo) | Undercuts social schedulers while bundling more DJ-specific value |
| **$25/mo TEAM** | Buffer Team ($12/user/mo), Notion Team ($10/user/mo) | Competitive for small collectives (2-10 DJs); per-seat pricing avoided to keep it simple |

**Key insight from BRD:** The project explicitly targets DJs in markets where "SaaS tools are expensive or unavailable." The free self-hosted tier respects this. The $9/mo tier captures DJs who value convenience over ops work.

---

## Revenue Streams Beyond Subscriptions

### 0. AI Caption Generation (PRO — included, not add-on)

**What:** When scheduling a social post after tracklist export, Claude API generates 3 caption variants based on tracklist data (venue, event name, track labels, genre signals). One-click "Use This" fills the caption field.

**Model:** Included in PRO tier. Estimated cost: ~$0.001/generation at Claude Haiku pricing — negligible at SaaS scale.

**Why SaaS-only:** Requires a hosted API key. Self-hosters can optionally add their own `CLAUDE_API_KEY` to `.env` for local generation (OSS-friendly escape hatch), but the managed experience is PRO.

### 1. Template Marketplace (v2.0+)

**What:** Community-created tracklist image templates, EPK themes, and invoice designs.

**Model:** 70/30 revenue split (creator gets 70%). Templates priced $2-$10 each.

**Why it works:** The 5 built-in presets are free. Premium templates with unique designs, animations, or branded layouts become the upsell. Creators in the DJ community become evangelists.

**Technical fit:** The `imagePresets` design token architecture already supports arbitrary preset definitions. Extend with a `templates` table and MinIO-stored template assets.

### 2. Priority Cover Art API (SaaS Enhancement)

**What:** Self-hosted users hit free API rate limits (MusicBrainz: 1 req/sec, Discogs: 60/min). SaaS users get pooled API keys with higher throughput.

**Model:** Included in PRO tier. Self-hosted users can optionally buy an API key add-on ($3/mo) for faster cover art resolution.

**Technical fit:** The `CoverArtProvider` interface already abstracts the API layer. Add a `HostedCoverArtProvider` that routes through a centralized proxy with pooled credentials.

### 2.5. Bandsintown Sync (OSS — growth driver, not revenue)

**What:** When a gig is confirmed and `BANDSINTOWN_API_KEY` is configured, event is pushed to the DJ's Bandsintown artist page automatically (Phase 4.8).

**Model:** Free Bandsintown API, ships as OSS. Not a revenue stream — a **growth driver** that makes KlubHub DJ indispensable to DJs who promote on Bandsintown, and a discovery vector when DJs mention KlubHub in their Bandsintown event descriptions.

### 3. Hosted EPK Pages (SaaS-Only Feature)

**What:** A shareable public URL for your press kit (e.g., `klubhub.dj/epk/your-name`). Custom domains supported in TEAM tier.

**Model:** PRO tier includes 1 hosted EPK page. TEAM tier includes custom domains.

**Why SaaS-only:** Requires web hosting, SSL, CDN, custom domain management — fundamentally not self-hostable without DevOps knowledge. This is the cleanest SaaS-exclusive feature.

**BRD alignment:** "Hosted EPK web pages with shareable links" is explicitly listed as a v3.0+ feature in PROJECT.md.

### 4. White-Label / Agency License (TEAM+)

**What:** Agencies managing multiple DJ artists can use KlubHub as their backend tool with custom branding.

**Model:** $99/month for 25+ users, white-label branding, API access.

**Why:** DJ agencies (talent management companies) need exactly this toolset but centralized. The v3 multi-user guardrails (`user_id` columns, MinIO path prefixing) make this technically straightforward.

---

## Architecture Alignment: Why This Works

The existing architecture is **already designed for this split** without knowing it:

### 1. Module Boundaries Enable Feature Gating

Each Go module (tracklist, social, epk, gig, finance, release, tour) is a self-contained package with its own `handler.go`, `service.go`, `repository.go`. The `RouteRegistrar` pattern means:

```go
// SaaS version: register all modules
router.Mount("/api/v1/social", social.NewHandler(deps))
router.Mount("/api/v1/tour", tour.NewHandler(deps))

// Open-source version: skip premium modules
// (or gate behind license check middleware)
```

**No code changes needed to the open-source modules.** Premium modules are additive.

### 2. Interface-First Design Enables Clean Boundaries

The `GigReader`, `CoverArtProvider`, `SocialMediaPublisher`, `PDFGenerator` interfaces mean:
- Open-source modules depend on **interfaces**, not implementations
- SaaS can provide enhanced implementations (pooled API keys, hosted rendering)
- No circular dependencies

### 3. v3 Multi-User Guardrails Enable Multi-Tenant SaaS

Already built into v1:
- `user_id` columns on all tables (nullable in v1, required in SaaS)
- MinIO path prefixing per user
- Repository methods accept `context.Context` (inject tenant ID)
- Rate-limiting middleware (disabled in v1, enabled in SaaS)

### 4. Docker Compose → Kubernetes Migration Path

Self-hosted: `docker compose up -d` (4 services)
SaaS: Kubernetes with:
- Shared PostgreSQL (with row-level security per tenant)
- Shared MinIO (with bucket-per-tenant policy)
- Horizontal API scaling (stateless Go binary)
- Centralized scheduler service (extracted from in-process goroutine)

The architecture doc already defines this migration path in §9 "Deployment Architecture."

---

## Competitive Landscape

| Competitor | What They Offer | Price | KlubHub Advantage |
|-----------|-----------------|-------|-------------------|
| **Canva** | Generic image templates | $13/mo | DJ-specific templates, automatic track parsing, cover art fetching |
| **Buffer / Later / Hootsuite** | Social scheduling | $6-$25/mo | DJ-specific (tracklist → post workflow), not generic scheduling. Instagram OSS = free. |
| **Linktree / Carrd** | Simple landing pages | $9-$19/mo | Full EPK with PDF export (OSS) + hosted page (PRO); not just a link page |
| **Google Sheets** | Manual gig/finance tracking | Free | Purpose-built workflows: venue DB, iCal, booking PDF, status tracking, auto-income, invoicing |
| **Optune** | DJ agency booking CRM | €19-79/mo | OSS + self-hosted; venue/contact DB, iCal, rider templates included free |
| **Stagent** | Artist agency booking | €39-99/mo | Full OSS equivalent at no cost; agency collaboration at $25/mo vs €99+/mo |
| **Gigwell** | Agency/venue booking + analytics | $39-319/mo | OSS core tools free forever; SaaS PRO at fraction of cost |
| **Gigsalad / Encore** | Gig marketplace | Commission-based | Self-managed gig tracking without commission; DJ owns their data |
| **KUVO / Mixcloud** | Tracklist sharing | Free/paid | Image generation + text export + full career toolkit, not just tracklist hosting |

**KlubHub's moat:** No single tool combines tracklist generation + text export + Instagram scheduling + EPK + gig tracking with venue DB + finance + rider templates + tour management for DJs as a self-hosted OSS tool. Each competitor covers one slice at $19-$99+/mo. KlubHub covers all of it free (self-hosted) or at $9/mo (cloud PRO).

---

## Implementation Roadmap for Monetization

### Phase M1: Open-Source Foundation (Current — Phases 0-1.5)
- Ship the free, self-hosted tracklist generator
- Build community and collect feedback
- Establish brand and GitHub presence
- **Goal:** 500+ GitHub stars, 100+ self-hosted users

### Phase M2: Cloud Hosting Beta (After Phase 2)
- Deploy hosted version (same codebase, Kubernetes instead of Docker Compose)
- Free tier: tracklist generator + gig tracker + 4 social posts/month
- Collect usage data and iterate on pricing
- **Goal:** 50+ cloud beta users, validate willingness to pay

### Phase M3: Pro Launch (After Phase 4)
- Launch PRO tier ($9/mo) with social scheduler + EPK + tour manager
- Template marketplace (community-created premium templates)
- **Goal:** 200+ paying customers, $1,800+ MRR

### Phase M4: Team & Agency (After Phase 8)
- Launch TEAM tier ($25/mo) with multi-user + collaboration
- White-label option for agencies ($99/mo)
- **Goal:** 50+ team accounts, $3,000+ MRR

---

## Risk Analysis

| Risk | Impact | Mitigation |
|------|--------|------------|
| Open-source users never convert to paid | Lost SaaS revenue | Free tier is the marketing funnel; SaaS adds infrastructure value (uptime, OAuth management) that self-hosters genuinely can't replicate easily |
| Competitor forks the repo and hosts it | Direct competition | MIT allows this. Mitigate by: (1) moving fast on features, (2) building community loyalty, (3) SaaS-only features that require hosted infra (EPK pages, pooled API keys) |
| Instagram API changes break social scheduler | Core SaaS feature down | Abstract behind `SocialMediaPublisher` interface; multi-platform support (TikTok, Twitter) reduces single-platform dependency |
| Self-hosted users demand premium features for free | Community friction | Clear communication: "core data tools are forever free; premium = infrastructure + automation + collaboration." The split follows a natural value boundary. |
| Price too low to sustain development | Unsustainable business | Start at $9/mo, raise to $12-15/mo once product-market fit confirmed. Template marketplace provides additional revenue without price increase. |

---

## Summary: The Golden Rule

> **Open-source what DJs own (their data, their creative output).**
> **Monetize what DJs rent (infrastructure, automation, collaboration, hosted services).**

This maps cleanly to:
- **Own:** Tracklists, gig history, finances, releases, generated images → Free
- **Rent:** 24/7 scheduling, OAuth management, hosted EPK pages, team collaboration, pooled API keys → Paid

---

*Monetization research for: KlubHub DJ — Open-Core SaaS Strategy*
*Researched: 2026-03-20*
