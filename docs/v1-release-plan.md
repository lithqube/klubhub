# KlubHub DJ — v1.0 Open-Source Production Release Plan

**Version:** 1.0
**Date:** 2026-03-20
**Status:** Draft
**Target:** v1.0.0 — first public open-source release

---

## Overview

KlubHub DJ v1.0 is the complete, self-hosted, open-source DJ career toolkit. It ships as a single `docker compose up -d` command and includes all 8 feature modules plus infrastructure. The release targets DJs who want to own their data and manage their career without fragmented SaaS tools.

**License:** MIT — free forever for self-hosted use.

---

## What Ships in v1.0

### Feature Modules

| # | Module | Phase | Description |
|---|--------|-------|-------------|
| 1 | Infrastructure | 0 | Four-service Docker Compose stack, health endpoint, migrations, backup/restore |
| 2 | Tracklist Image Generator | 1 | Upload DJ history → professional tracklist image in <60s |
| 3 | Design System | 1.5 | Tailwind CSS + shadcn-vue + Cyberpunk HUD design language |
| 4 | Social Media Scheduler | 2 | Instagram OAuth, timezone-aware scheduling, retry with backoff |
| 5 | EPK / Press Kit Builder | 3 | Artist bio, press photos, tech rider, PDF export |
| 6 | Gig Tracker | 4 | Gig CRUD, status/payment workflows, calendar/list views |
| 7 | Finance Tracker | 5 | Income/expense logging, multi-currency summaries, PDF invoices |
| 8 | Release Planner | 6 | Release CRUD, deadline tracking, promo checklists |
| 9 | Tour Manager | 7 | Named tour groups, per-stop logistics, tour budget |
| 10 | Unified Dashboard | 8 | Aggregated career overview across all modules |

### Tech Stack

- **Backend:** Go modular monolith (chi router, goose migrations, gg/imaging for image gen)
- **Frontend:** Nuxt 4 (Vue 3, TypeScript strict, Tailwind CSS, shadcn-vue, Pinia)
- **Database:** PostgreSQL 16
- **Object Storage:** MinIO (S3-compatible)
- **Deployment:** Docker Compose (4 services)

---

## Phase 9: Production Hardening

This phase prepares the codebase for public release after all feature phases (0-8) are complete.

### 9a. Documentation & Onboarding

| Deliverable | Description |
|-------------|-------------|
| `README.md` | Rewrite with hero screenshot, 3-step quick start, feature grid, tech stack badges, contributing link |
| `docs/SELF-HOSTING.md` | Full deployment guide: system requirements (2GB RAM, 10GB disk), env var reference, HTTPS reverse proxy setup (Caddy/nginx examples), backup schedule recommendation |
| `docs/CONFIGURATION.md` | Every env var documented with type, default, example, and which module uses it |
| `docs/CONTRIBUTING.md` | Dev environment setup, code style (Go + Vue), PR process, module architecture overview, testing guide |
| `CHANGELOG.md` | v1.0.0 release notes covering all modules |
| `LICENSE` | MIT license |
| `docs/API.md` | REST API reference for all endpoints (auto-generated from chi routes or hand-written) |

### 9b. Security Hardening

| Task | Details |
|------|---------|
| Input validation audit | All API endpoints validate input types, lengths, and formats; reject unexpected fields |
| SQL injection prevention | Verify all queries use parameterized statements (no string concatenation) |
| Path traversal check | MinIO keys and file paths cannot escape tenant boundaries |
| Rate limiting | Configurable rate limiter on all endpoints (default: 100 req/min per IP) |
| CORS hardening | Default: same-origin only; configurable via `CORS_ORIGINS` env var |
| Service binding | Verify all Docker services bind `127.0.0.1` (not `0.0.0.0`) |
| Token encryption | Verify AES-256-GCM encryption for Instagram OAuth tokens using `TOKEN_ENCRYPTION_KEY` |
| Dependency audit | `go mod tidy` + `govulncheck` + `pnpm audit` — fix all high/critical CVEs |
| `SECURITY.md` | Vulnerability reporting process (email, expected response time, responsible disclosure) |

### 9c. Reliability & Operations

| Task | Details |
|------|---------|
| Graceful shutdown | Go API drains in-flight requests on SIGTERM (30s timeout) |
| Structured logging | JSON log output with configurable level (`LOG_LEVEL` env var: debug/info/warn/error) |
| Docker health checks | All 4 services have `HEALTHCHECK` directives in Dockerfile/compose |
| Resource limits | Memory and CPU limits in `docker-compose.yml` (configurable) |
| Backup cron example | Document recommended cron schedule for `scripts/backup.sh` |
| Scheduler recovery | Social scheduler picks up missed/pending posts after API restart |
| Migration safety | All migrations are idempotent; down migrations tested |

### 9d. Testing & CI

| Task | Details |
|------|---------|
| GitHub Actions CI | Lint → Test → Build → E2E pipeline on every push/PR to main |
| Go test coverage | Minimum 70% coverage on service and repository layers |
| Frontend E2E | Playwright tests for all critical user flows (upload, edit, export, schedule, gig CRUD) |
| Docker build test | CI verifies `docker compose build` and `docker compose up` succeed from clean state |
| Migration rollback | CI tests both up and down migrations |
| Linting | `golangci-lint` for Go, `eslint` + `vue-tsc --noEmit` for frontend |

### 9e. Release Engineering

| Task | Details |
|------|---------|
| Versioning | Semantic versioning starting at v1.0.0 |
| Docker images | Multi-arch (amd64 + arm64) images published to ghcr.io on tag |
| GitHub Release | Pre-built images, release notes, upgrade instructions |
| Upgrade path | `docker compose pull && docker compose up -d` — zero-downtime for stateless API |
| `.env.example` | Complete template with all variables, defaults, and comments |
| Docker Compose profiles | `prod` profile (default), `dev` profile (hot-reload, debug logging) |

---

## Release Checklist

### Pre-Release

```
Code Quality:
- [ ] All phases 0-8 complete and verified per success criteria
- [ ] All E2E tests pass on clean Docker Compose stack
- [ ] Go test coverage ≥70% on service/repository layers
- [ ] `vue-tsc --noEmit` passes with zero errors
- [ ] `golangci-lint run` passes with zero errors
- [ ] `govulncheck ./...` reports no known vulnerabilities
- [ ] `pnpm audit` reports no high/critical vulnerabilities

Security:
- [ ] Input validation audit complete
- [ ] All DB queries use parameterized statements
- [ ] Rate limiting configured and tested
- [ ] CORS defaults to same-origin
- [ ] All Docker services bind 127.0.0.1
- [ ] Token encryption verified with test encrypt/decrypt cycle
- [ ] SECURITY.md written

Documentation:
- [ ] README.md rewritten with hero screenshot
- [ ] SELF-HOSTING.md complete with reverse proxy examples
- [ ] CONFIGURATION.md documents all env vars
- [ ] CONTRIBUTING.md written
- [ ] API.md documents all REST endpoints
- [ ] CHANGELOG.md v1.0.0 written

Operations:
- [ ] Graceful shutdown tested (kill -SIGTERM)
- [ ] Structured JSON logging verified
- [ ] Docker health checks working for all services
- [ ] Backup/restore scripts tested with real data
- [ ] Scheduler recovery tested (restart API mid-schedule)

CI/CD:
- [ ] GitHub Actions pipeline green on main
- [ ] Docker multi-arch build succeeds (amd64 + arm64)
- [ ] Docker Compose starts from clean state in CI
```

### Release Day

```
- [ ] Create git tag v1.0.0 on main
- [ ] GitHub Actions builds and pushes Docker images to ghcr.io
- [ ] Create GitHub Release with:
  - Release notes from CHANGELOG.md
  - Link to SELF-HOSTING.md
  - Link to Docker images
- [ ] Verify `docker compose pull && docker compose up -d` works from fresh machine
- [ ] Update README badges (CI status, license, Docker pulls)
- [ ] Announce on relevant channels (Reddit r/DJs, Twitter, Discord)
```

### Post-Release

```
- [ ] Monitor GitHub Issues for first-week bugs
- [ ] Prepare v1.0.1 patch for any critical issues
- [ ] Begin community engagement (respond to issues, accept PRs)
- [ ] Track GitHub stars and Docker pull metrics
```

---

## v1 Architecture Summary

```
User (Browser)
    │
    ▼
Nuxt 4 Frontend (SSR + SPA)
    │ /api/v1/* proxy
    ▼
Go API (Modular Monolith)
├── tracklist module
├── social module (+ in-process scheduler)
├── epk module
├── gig module
├── finance module
├── release module
├── tour module
├── dashboard module (aggregator)
├── platform packages (config, db, storage, log, crypto)
└── embedded goose migrations
    │           │
    ▼           ▼
PostgreSQL   MinIO
   16        (S3)
```

**Key architectural properties:**
- Single Go binary — no external job queue, no Redis, no message broker
- All modules communicate through Go interfaces, never shared DB queries
- `user_id` columns on all tables (nullable in v1, required in v2 SaaS)
- MinIO path prefixing per user (no-op in v1, enforced in v2)
- `context.Context` threaded through all handlers (tenant injection ready)

---

## v1 Constraints (What v1 Does NOT Include)

| Excluded | Reason |
|----------|--------|
| Authentication / multi-user | Single-user self-hosted; v2 SaaS adds this |
| Billing / subscriptions | No SaaS tier in v1 |
| Kubernetes / cloud deployment | Docker Compose only |
| TikTok / Twitter posting | Instagram only in v1 |
| AI-generated backgrounds | v2 enhancement |
| Real-time collaboration | v2 SaaS feature |
| Hosted EPK pages | Requires web hosting — v2 SaaS |
| Template marketplace | v2 SaaS feature |
| Mobile app | Web-first; mobile deferred |

These constraints keep v1 focused, simple to deploy, and achievable. The modular architecture ensures all of the above can be added in v2 without rewriting v1.

---

*v1 Release Plan for: KlubHub DJ*
*Created: 2026-03-20*
