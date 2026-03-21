# KlubHub DJ — v2.0 SaaS Migration Plan

**Version:** 1.0
**Date:** 2026-03-20
**Status:** Draft — not started; depends on v1.0 release
**Companion docs:** `ARCHITECTURE.md`, `v1-release-plan.md`, `.planning/research/MONETIZATION.md`

---

## Overview

v2 transforms KlubHub DJ from a single-user self-hosted tool into a multi-tenant SaaS platform while keeping the open-source core intact. The strategy is **open-core**: data-ownership modules (tracklist, gig, finance, release, tour manager, unified dashboard, and Instagram social scheduling) stay MIT-licensed and free forever. Infrastructure/automation features that require centralized hosting (multi-platform social posting, hosted EPK pages, real-time collaboration) become premium SaaS features.

**Golden rule:** Own your data free, rent infrastructure paid.

**Business model:** Freemium with PRO ($9/mo) and TEAM ($25/mo) tiers. See `.planning/research/MONETIZATION.md` for full pricing analysis.

---

## Repository Strategy

### Separate Repo, Shared Core

```
klubhub-dj (existing, MIT, public)
├── Go modules (tracklist, gig, finance, release, etc.)
├── Nuxt 4 frontend
├── Docker Compose deployment
└── All v1 features — continues to receive updates

klubhub-dj-cloud (new, private)
├── go.mod: require github.com/user/klubhub-dj v1.x
├── Auth service
├── Billing service (Stripe)
├── Scheduler service (extracted)
├── EPK hosting service
├── WebSocket service
├── API Gateway
├── Kubernetes manifests
├── Terraform/Pulumi IaC
└── Admin dashboard
```

**Why separate repo, not a fork?**
- Open-source repo stays clean — no SaaS code leaks into MIT codebase
- SaaS repo imports open-source as a Go module — gets all updates automatically
- Feature flags in frontend control SaaS-only UI without forking Nuxt app
- Contributors to open-source never see billing/auth code

---

## SaaS Milestones

### M1: Authentication & Multi-Tenancy

**Goal:** Add user authentication and convert single-user tables to multi-tenant.

| Task | Details |
|------|---------|
| Auth service | JWT-based authentication; OIDC-compatible (Google, GitHub, email/password) |
| User registration | Sign up, email verification, password reset |
| `user_id` enforcement | Make all nullable `user_id` columns NOT NULL; backfill existing data |
| Row-Level Security | PostgreSQL RLS policies on all tables — tenant isolation at DB level |
| MinIO isolation | Bucket-per-tenant or prefix-per-tenant with IAM policies |
| Tenant middleware | Extract tenant ID from JWT, inject into `context.Context` for all handlers |
| Session management | Refresh tokens with rotation; secure cookie + Bearer token support |
| API key support | Allow API access via long-lived API keys (for integrations) |

**Architecture change:** New `auth` service in front of existing Go API. All routes require authentication except `/api/v1/health` and `/api/v1/auth/*`.

**Migration path:** Existing v1 self-hosted users upgrading to cloud get a "Claim Your Data" flow that associates their existing data with a new user account.

### M2: Feature Gating & Billing

**Goal:** Stripe-powered subscription management with tier-based feature access.

| Task | Details |
|------|---------|
| Stripe integration | Subscription lifecycle: create, update, cancel, reactivate |
| Tier middleware | Check `subscription.tier` before premium route handlers |
| Free tier limits | Instagram: unlimited posts (OSS). Multi-platform (TikTok/Twitter/Facebook): 4 posts/month. 1 hosted EPK (no custom domain). 500MB storage. |
| PRO tier ($9/mo) | Multi-platform: unlimited posts. AI captions: unlimited. Hosted EPK + public URL with branding. 10GB storage. |
| TEAM tier ($25/mo) | Multi-user (up to 10). Collaborative tour planning. Custom EPK domain. 50GB storage. |
| Usage metering | Track posts scheduled, storage used, EPKs created per billing period |
| Billing portal | Stripe Customer Portal: self-service plan changes, invoices, payment methods |
| Webhook handling | Stripe webhooks for payment success/failure, subscription changes |
| Grace period | 7-day grace period on failed payments before downgrade |
| Trial | 14-day free trial of PRO tier (no credit card required) |

**Pricing reference:** See `.planning/research/MONETIZATION.md` — competitive analysis against Buffer ($6/mo), Linktree ($9/mo), Canva ($13/mo).

### M3: Social Scheduler Extraction

**Goal:** Extract the in-process Go scheduler goroutine into a standalone, reliable service.

| Task | Details |
|------|---------|
| Scheduler service | Standalone Go binary; runs independently of API |
| Job queue | Redis-backed (Asynq library) — replaces DB polling |
| 24/7 reliability | Service restarts don't lose scheduled posts; jobs survive process crashes |
| Pooled OAuth | Centralized Instagram token management across tenants |
| Rate limiting | Per-tenant and global rate limits to respect Instagram API quotas |
| Multi-platform posting | TikTok, Twitter/X, and Facebook publisher implementations — SaaS PRO feature |
| AI caption generation | Claude API integration for caption suggestions from tracklist data — SaaS PRO |
| Retry improvements | Dead-letter queue for permanently failed posts; admin visibility |
| Monitoring | Scheduler health endpoint; Prometheus metrics for queue depth, publish latency |

**Why extract?** The scheduler extraction enables two things: (1) **reliable 24/7 uptime** for self-hosters who can't keep Docker running continuously — Instagram scheduling is OSS and self-hosted users benefit from this too; and (2) **multi-platform posting** (TikTok, Twitter/X, Facebook) that requires centralized API key management and rate limit orchestration across tenants that self-hosters cannot replicate easily. Instagram scheduling itself remains OSS — the extracted service adds reliability and platform breadth.

### M4: Hosted EPK Pages

**Goal:** Shareable public URLs for press kits — the cleanest SaaS-only feature.

| Task | Details |
|------|---------|
| EPK hosting service | Server-rendered public pages at `klubhub.dj/epk/{username}` |
| CDN integration | Static assets served via CDN (Cloudflare or CloudFront) |
| Custom domains (TEAM) | DNS verification + automatic SSL via Let's Encrypt |
| Template engine | EPK page templates with design system integration |
| SEO optimization | Server-rendered HTML with structured data (JSON-LD for MusicGroup) |
| Analytics | Page view tracking per EPK (visible to EPK owner) |
| Access control | Public by default; optional password protection |

**Why SaaS-only?** Hosting public web pages requires web servers, SSL, CDN, domain management — fundamentally not self-hostable without DevOps expertise. This is explicitly a v3.0+ feature per `PROJECT.md`.

### M5: Collaboration & Real-Time

**Goal:** Multi-user collaboration for touring DJs, collectives, and agencies.

| Task | Details |
|------|---------|
| WebSocket service | Real-time updates for shared resources |
| Tour collaboration | Invite managers, agents, promoters to view/edit tour data |
| Permission model | Owner, Editor, Viewer roles per resource |
| Activity feed | "Sarah updated the Frankfurt stop" — real-time activity stream |
| Shared templates | Team-wide tracklist image templates and EPK themes |
| Notification service | Email + in-app notifications for updates, mentions, deadlines |
| Presence indicators | Show who is currently viewing/editing a resource |

**Target users:** DJ collectives (2-10 members), talent agencies, booking agents who manage multiple artists.

### M6: API Gateway & Horizontal Scaling

**Goal:** Scale the API layer horizontally for growing user base.

| Task | Details |
|------|---------|
| API Gateway | Traefik or Kong — routing, rate limiting, SSL termination |
| Stateless API | Extract all in-process state to PostgreSQL/Redis |
| Horizontal autoscaling | Kubernetes HPA based on CPU/request rate |
| Connection pooling | PgBouncer between API pods and PostgreSQL |
| CDN for assets | Generated images and uploads served via CDN |
| API versioning | `/api/v1/` and `/api/v2/` — breaking changes go to v2 |
| Request tracing | Correlation IDs propagated through all services |

### M7: Observability

**Goal:** Full-stack monitoring, logging, tracing, and alerting.

| Task | Details |
|------|---------|
| Log aggregation | Structured JSON logs → Loki (or CloudWatch) |
| Distributed tracing | OpenTelemetry SDK → Tempo/Jaeger |
| Metrics | Prometheus → Grafana dashboards (API latency, DB queries, queue depth) |
| Error tracking | Sentry for Go and Nuxt — error grouping, stack traces, release tracking |
| Uptime monitoring | External health checks with PagerDuty/Opsgenie alerting |
| SLO definitions | 99.9% API uptime, <500ms p95 latency, <2s image generation p99 |
| Cost monitoring | AWS/GCP cost alerts per service |

### M8: Infrastructure & Kubernetes

**Goal:** Production-grade cloud infrastructure with IaC.

| Task | Details |
|------|---------|
| Kubernetes manifests | Helm charts for all services |
| PostgreSQL | Managed service (RDS/Cloud SQL) with read replicas, automated backups |
| Object storage | S3 for SaaS (MinIO retained for self-hosted Docker Compose) |
| Redis cluster | Sessions, caching, job queue — managed Redis (ElastiCache/Memorystore) |
| CI/CD pipeline | GitHub Actions → build → test → deploy staging → promote to prod |
| Infrastructure as Code | Terraform (or Pulumi) for all cloud resources |
| Zero-downtime deploys | Rolling updates with readiness probes |
| Database migrations | goose with zero-downtime strategy (expand-contract pattern) |
| Secrets management | Kubernetes Secrets + external secrets operator (Vault/AWS SM) |
| Multi-region prep | Architecture supports multi-region; initial deploy is single-region |

### M9: Admin Dashboard & Marketplace

**Goal:** Internal tools for platform management and community-driven content.

| Task | Details |
|------|---------|
| Admin panel | User management, subscription overview, usage metrics |
| Template marketplace | Community-created tracklist templates, EPK themes, invoice designs |
| Revenue sharing | 70/30 split (creator gets 70%); templates priced $2-$10 |
| Content moderation | Review queue for marketplace submissions |
| Analytics dashboard | DAU/MAU, feature usage, conversion funnel, churn analysis |
| Support integration | Ticketing system integration (Intercom/Zendesk) |
| Tenant impersonation | Admin can view app as any user (for debugging, with audit log) |
| White-label API | Agency tier ($99/mo): custom branding, 25+ users, API access |

---

## Architecture Comparison: v1 vs v2

### v1: Self-Hosted (Docker Compose)

```
docker compose up -d
├── frontend     (Nuxt 4 SSR)
├── api          (Go monolith — all modules + scheduler)
├── db           (PostgreSQL 16)
└── storage      (MinIO)
```

- Single Go binary, no external job queue
- No auth, single user
- All modules in one process
- `127.0.0.1` binding

### v2: SaaS (Kubernetes)

```
Kubernetes Cluster
├── Ingress (Traefik/Kong)
│   ├── /api/v1/*    → API Service (Go, replicas: 2-10)
│   ├── /api/v1/auth → Auth Service (Go, replicas: 2)
│   ├── /ws/*        → WebSocket Service (Go, replicas: 2)
│   └── /epk/*       → EPK Hosting Service (Nuxt/Go)
│
├── Frontend (Nuxt 4, CDN-backed, replicas: 2)
│
├── Scheduler Service (Go, singleton with leader election)
│
├── Data Layer
│   ├── PostgreSQL (managed, RLS, read replicas)
│   ├── Redis (sessions, cache, job queue)
│   └── S3 (object storage, CDN-fronted)
│
├── Observability
│   ├── Prometheus + Grafana
│   ├── Loki (logs)
│   ├── Tempo (traces)
│   └── Sentry (errors)
│
└── Admin
    ├── Admin Dashboard (internal)
    └── Stripe Webhooks
```

### Key Differences

| Aspect | v1 | v2 |
|--------|----|----|
| Auth | None | JWT + OIDC |
| Tenancy | Single user | Multi-tenant with RLS |
| Scheduler | In-process goroutine | Standalone service + Redis queue |
| Storage | MinIO | S3 (MinIO for self-hosted) |
| Database | Single PostgreSQL | Managed + read replicas + RLS |
| Scaling | Vertical only | Horizontal autoscaling |
| Deployment | Docker Compose | Kubernetes + Helm + Terraform |
| Monitoring | Health endpoint | Full observability stack |
| Billing | None | Stripe subscriptions |

---

## Migration Strategy

### Go Module Import Pattern

The SaaS repo imports the open-source modules as a Go dependency:

```go
// klubhub-dj-cloud/cmd/api/main.go
import (
    "github.com/user/klubhub-dj/internal/tracklist"
    "github.com/user/klubhub-dj/internal/gig"
    // ... all open-source modules

    "github.com/user/klubhub-dj-cloud/internal/auth"
    "github.com/user/klubhub-dj-cloud/internal/billing"
    "github.com/user/klubhub-dj-cloud/internal/scheduler"
)
```

### Feature Flags in Frontend

```typescript
// Nuxt plugin: detects SaaS vs self-hosted
export default defineNuxtPlugin(() => {
  const isSaaS = useRuntimeConfig().public.mode === 'saas'
  const tier = useTier() // 'free' | 'pro' | 'team'

  return {
    provide: {
      features: {
        // Instagram scheduling is always enabled (OSS — never gated)
        instagramScheduler: true,
        // Multi-platform requires SaaS PRO (centralized API key management)
        multiPlatformSocial: isSaaS && tier !== 'free',
        // AI captions require hosted Claude API key
        aiCaptions: isSaaS && tier !== 'free',
        // Hosted EPK page requires web serving infrastructure
        hostedEPK: isSaaS && tier !== 'free',
        // Collaboration requires WebSocket + real-time infrastructure
        collaboration: isSaaS && tier === 'team',
        // Custom EPK domain requires DNS + SSL management
        customEPKDomain: isSaaS && tier === 'team',
      }
    }
  }
})
```

### Database Migration (v1 → v2)

1. **Add `users` and `subscriptions` tables**
2. **Backfill `user_id`** — create a default user for existing data, set all rows
3. **Add NOT NULL constraint** on `user_id` columns
4. **Enable RLS** — `CREATE POLICY tenant_isolation ON {table} USING (user_id = current_setting('app.user_id')::uuid)`
5. **Add indexes** for `user_id` on all tables (multi-tenant query performance)

All migrations use the expand-contract pattern for zero downtime.

---

## Revenue Projections

Based on `.planning/research/MONETIZATION.md`:

| Phase | Target | Revenue |
|-------|--------|---------|
| M1-M2 (Auth + Billing) | 50 cloud beta users, validate willingness to pay | $0 (free beta) |
| M3-M4 (Scheduler + EPK) | 200 paying PRO customers | ~$1,800/mo MRR |
| M5-M6 (Collab + Scale) | 300 PRO + 50 TEAM | ~$3,950/mo MRR |
| M7-M9 (Observability + Marketplace) | 500 PRO + 100 TEAM + marketplace | ~$7,000/mo MRR |

Additional revenue: Template marketplace (70/30 split), Priority Cover Art API ($3/mo add-on), White-Label Agency ($99/mo).

---

## Risk Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| v1 delays push back v2 | Lost SaaS revenue window | v1 phases can ship incrementally; SaaS work can start on M1 (auth) after Phase 4 |
| Open-source users fork and host | Direct competition | Move fast on features; SaaS-only features require hosted infra (EPK pages, pooled OAuth) |
| Instagram API changes | Core SaaS feature breaks | `SocialMediaPublisher` interface abstracts platform; multi-platform reduces dependency |
| Scaling costs exceed revenue | Unsustainable infrastructure | Start single-region; use spot instances for non-critical workloads; right-size from day 1 |
| Self-hosted users demand SaaS features for free | Community friction | Clear messaging: "Core data tools free forever; premium = infrastructure + automation + collaboration" |

---

## Timeline Guidance

**Note:** No specific date commitments — timeline depends on team size and velocity.

| Milestone | Dependencies | Relative Order |
|-----------|-------------|----------------|
| v1.0 Release | Phases 0-9 complete | First |
| M1: Auth + Multi-Tenancy | v1.0 shipped | Start after v1.0 |
| M2: Billing | M1 complete | After M1 |
| M3: Scheduler Extraction | M1 complete | Can parallel with M2 |
| M4: Hosted EPK | M2 complete (needs billing) | After M2 |
| M5: Collaboration | M1 complete | Can parallel with M3-M4 |
| M6: Horizontal Scaling | M1-M5 deployed, traffic data available | After initial SaaS launch |
| M7: Observability | Deploy alongside M3+ | Incremental, parallel with all |
| M8: Kubernetes | Can start alongside M1 | Infrastructure-parallel |
| M9: Admin + Marketplace | M2 complete, meaningful user base | After PRO tier has users |

**Parallelization opportunities:**
- M3 (Scheduler) and M5 (Collaboration) can run in parallel after M1
- M7 (Observability) is incremental and runs alongside all milestones
- M8 (Kubernetes) infrastructure work can start early, in parallel with M1

---

*v2 SaaS Migration Plan for: KlubHub DJ*
*Created: 2026-03-20*
