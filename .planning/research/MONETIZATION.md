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
| **Tracklist Image Generator** | 1 | **Flagship identity** — "upload file, get image in 60 seconds" IS the product. Must be free to create viral adoption. This is your funnel. |
| **Design System** | 1.5 | UI foundation — no value as paid |
| **Gig Tracker** | 4 | Privacy-sensitive data (fees, contacts, venues). Hub module — restricting it blocks 3 downstream modules. DJs won't trust a tool that locks their gig data behind a paywall. |
| **Finance Tracker** | 5 | **Highly privacy-sensitive** (income, expenses, invoices). DJs will not enter financial data into a system that could paywall their own records. Trust signal. |
| **Release Planner** | 6 | Simple CRUD — not enough value to justify payment. Better as a "stickiness" feature that keeps users in the ecosystem. |

**Why this split matters:** The open-source modules cover the **data-ownership** story. A DJ's tracklists, gig history, finances, and release plans are *their* data. Keeping this free builds the trust needed to sell the SaaS tier.

---

### Tier 2: SaaS Premium (Hosted Cloud — Subscription)

These modules have characteristics that naturally justify SaaS pricing:
- They require **infrastructure you manage** (OAuth token refresh, API rate limits, scheduling reliability)
- They provide **automation value** (set-and-forget publishing, batch operations)
- They involve **external service orchestration** that's painful to self-host
- They benefit from **always-on availability** (24/7 scheduler, real-time monitoring)

| Module | Phase | Why SaaS | SaaS Value Proposition |
|--------|-------|----------|----------------------|
| **Social Media Scheduler** | 2 | Requires Instagram App Review (3+ weeks), OAuth token management, 24/7 scheduler uptime, retry logic, rate limit management. Self-hosters must keep Docker running 24/7 or miss posts. | "Never miss a post. We handle Instagram OAuth, token refresh, and 24/7 scheduling so you don't need a server running." |
| **EPK / Press Kit Builder** | 3 | PDF generation is compute-intensive. **Hosted EPK pages** (shareable public URLs) require a web server, custom domains, SSL — this is explicitly a v3 feature that only makes sense as SaaS. | "One-click professional press kit with a shareable URL. No hosting setup required." |
| **Tour Manager** | 7 | **Collaboration potential** — touring DJs need to share logistics with managers, agents, promoters. Multi-user access to tour data is the natural SaaS upsell. Document storage (contracts, boarding passes) needs reliable hosted storage. | "Share tour logistics with your team. Collaborate on per-stop checklists without managing a server." |
| **Unified Dashboard** | 8 | Aggregation across modules with **real-time monitoring** (upcoming gigs, scheduled posts, release deadlines). Cloud dashboard with mobile access is a natural premium. | "Your DJ career at a glance — from any device, anywhere. Real-time alerts for upcoming deadlines." |

---

## Pricing Model Recommendation

### Model: Freemium Open-Core + Hosted SaaS

```
FREE (Self-Hosted, Open-Source MIT)
├── Tracklist Image Generator (unlimited)
├── Gig Tracker (unlimited)
├── Finance Tracker (unlimited)
├── Release Planner (unlimited)
├── Design System & Full UI
└── Docker Compose deployment

CLOUD FREE TIER (Hosted)
├── Everything in Self-Hosted FREE
├── Social Scheduler: 4 posts/month
├── EPK Builder: 1 press kit (no public URL)
├── 500 MB storage
└── Community support

PRO — $9/month or $89/year
├── Everything in Cloud Free
├── Social Scheduler: unlimited posts, multi-platform (Instagram + TikTok)
├── EPK Builder: unlimited press kits + shareable public URLs with custom branding
├── Tour Manager: full tour planning + team collaboration (invite 3 collaborators)
├── Unified Dashboard: real-time alerts + mobile-optimized view
├── 10 GB storage
├── Priority cover art fetching (pooled API keys, faster lookups)
├── Email support
└── Early access to new features

TEAM — $25/month or $249/year
├── Everything in PRO
├── Multi-user: up to 10 DJs (collective, label, agency)
├── Tour Manager: unlimited collaborators
├── Shared template library across team
├── 50 GB storage
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

### 1. Template Marketplace (v2.0+)

**What:** Community-created tracklist image templates, EPK themes, and invoice designs.

**Model:** 70/30 revenue split (creator gets 70%). Templates priced $2-$10 each.

**Why it works:** The 5 built-in presets are free. Premium templates with unique designs, animations, or branded layouts become the upsell. Creators in the DJ community become evangelists.

**Technical fit:** The `imagePresets` design token architecture already supports arbitrary preset definitions. Extend with a `templates` table and MinIO-stored template assets.

### 2. Priority Cover Art API (SaaS Enhancement)

**What:** Self-hosted users hit free API rate limits (MusicBrainz: 1 req/sec, Discogs: 60/min). SaaS users get pooled API keys with higher throughput.

**Model:** Included in PRO tier. Self-hosted users can optionally buy an API key add-on ($3/mo) for faster cover art resolution.

**Technical fit:** The `CoverArtProvider` interface already abstracts the API layer. Add a `HostedCoverArtProvider` that routes through a centralized proxy with pooled credentials.

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
| **Buffer / Later / Hootsuite** | Social scheduling | $6-$25/mo | DJ-specific (tracklist → post workflow), not generic scheduling |
| **Linktree / Carrd** | Simple landing pages | $9-$19/mo | Full EPK with PDF export, not just a link page |
| **Google Sheets** | Manual gig/finance tracking | Free | Purpose-built workflows (status tracking, auto-income, invoicing) |
| **Gigsalad / Encore** | Gig marketplace | Commission-based | Self-managed gig tracking without commission; DJ owns their data |
| **KUVO / Mixcloud** | Tracklist sharing | Free/paid | Image generation + full career toolkit, not just tracklist hosting |

**KlubHub's moat:** No single tool combines tracklist generation + social scheduling + press kits + gig tracking + finance + touring for DJs. Each competitor covers one slice. KlubHub's integration across all modules is the unique value.

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
