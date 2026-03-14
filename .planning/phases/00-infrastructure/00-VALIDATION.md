---
phase: 0
slug: infrastructure
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-14
---

# Phase 0 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (Go stdlib) + vitest (Nuxt frontend) |
| **Config file** | `api/` — standard `go test ./...`; `apps/dj/vitest.config.ts` |
| **Quick run command** | `cd api && go test ./... -short` |
| **Full suite command** | `cd api && go test ./... && pnpm nx test dj` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd api && go test ./... -short`
- **After every plan wave:** Run `cd api && go test ./... && docker compose up -d && curl -sf http://localhost:8080/api/v1/health`
- **Before `/gsd:verify-work`:** Full suite must be green + `docker compose up -d` end-to-end passes
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 0-01-01 | 01 | 1 | INFRA-01 | integration | `docker compose up -d && curl -sf http://localhost:3000` | ❌ W0 | ⬜ pending |
| 0-01-02 | 01 | 1 | INFRA-02 | integration | `docker compose port frontend 3000 \| grep 127.0.0.1` | ❌ W0 | ⬜ pending |
| 0-01-03 | 01 | 1 | INFRA-03 | unit | `cd api && go test ./internal/platform/db/...` | ❌ W0 | ⬜ pending |
| 0-01-04 | 01 | 1 | INFRA-04 | integration | `curl -sf http://localhost:8080/api/v1/health \| jq '.status'` | ❌ W0 | ⬜ pending |
| 0-01-05 | 01 | 1 | INFRA-05 | unit | `cd api && go test ./internal/platform/config/...` | ❌ W0 | ⬜ pending |
| 0-01-06 | 01 | 2 | INFRA-06 | integration | `curl -sf http://localhost:3000/api/v1/health \| jq '.status'` | ❌ W0 | ⬜ pending |
| 0-01-07 | 01 | 1 | INFRA-07 | unit | `cd api && go test ./internal/platform/db/migrations/...` | ❌ W0 | ⬜ pending |
| 0-02-01 | 02 | 2 | INFRA-08 | manual | `bash scripts/backup.sh && bash scripts/restore.sh` | ❌ W0 | ⬜ pending |
| 0-02-02 | 02 | 1 | INFRA-09 | unit | `cd api && go test ./internal/platform/crypto/...` | ❌ W0 | ⬜ pending |
| 0-02-03 | 02 | 1 | INFRA-10 | unit | `cd api && go test ./internal/platform/db/...` | ❌ W0 | ⬜ pending |
| 0-02-04 | 02 | 2 | INFRA-11 | unit | `cd api && go test ./internal/platform/middleware/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `api/internal/platform/db/db_test.go` — migration runner + connection pool tests (INFRA-03, INFRA-10)
- [ ] `api/internal/platform/config/config_test.go` — env var parsing + graceful degradation (INFRA-05)
- [ ] `api/internal/platform/crypto/crypto_test.go` — AES-256-GCM encrypt/decrypt round-trip (INFRA-09)
- [ ] `api/internal/platform/middleware/middleware_test.go` — optimistic concurrency 409 response (INFRA-11)
- [ ] Test for health endpoint JSON shape (INFRA-04)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `docker compose up -d` single-command deployment | INFRA-01 | Requires Docker daemon; cannot run in CI unit test | Run `docker compose up -d` in fresh clone; verify all 4 services show `healthy` in `docker compose ps` |
| 127.0.0.1 binding default | INFRA-02 | Requires live Docker network inspection | Run `docker compose port frontend 3000`; confirm output starts with `127.0.0.1` |
| Nuxt proxies /api/v1/* to Go | INFRA-06 | Requires both services running | Request `http://localhost:3000/api/v1/health`; confirm Go health JSON response (not Nuxt 404) |
| backup.sh + restore.sh round-trip | INFRA-08 | Requires running PostgreSQL + MinIO + mc CLI | Run backup.sh, wipe volumes, run restore.sh, verify data intact |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
