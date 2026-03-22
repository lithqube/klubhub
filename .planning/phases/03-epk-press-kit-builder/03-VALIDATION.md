---
phase: 3
slug: epk-press-kit-builder
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-22
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (Go)** | go test (built-in) + testify v1.11.1 |
| **Framework (Vue)** | Vitest (jsdom) — `apps/dj/vitest.config.ts` |
| **Config file** | `apps/dj/vitest.config.ts` — no changes needed |
| **Quick run (Go)** | `cd api && go test ./internal/epk/... -count=1` |
| **Quick run (Vue)** | `pnpm nx test dj --testPathPattern=epk` |
| **Full suite (Go)** | `cd api && go test ./... -count=1` |
| **Full suite (Vue)** | `pnpm nx test dj` |
| **Estimated runtime** | ~30 seconds (Go unit), ~20 seconds (Vitest) |

---

## Sampling Rate

- **After every task commit:** Run `cd api && go test ./internal/epk/... -count=1` and `pnpm nx test dj --testPathPattern=epk`
- **After every plan wave:** Run `cd api && go test ./... -count=1` and `pnpm nx test dj`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** ~50 seconds (combined Go + Vue quick run)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 03-W0-01 | Wave0 | 0 | EPK-01 | unit (Vue) | `pnpm nx test dj --testPathPattern=EpkBioSection` | ❌ W0 | ⬜ pending |
| 03-W0-02 | Wave0 | 0 | EPK-02 | unit (Go) | `cd api && go test ./internal/epk/... -run TestUploadPhoto -count=1` | ❌ W0 | ⬜ pending |
| 03-W0-03 | Wave0 | 0 | EPK-03 | unit (Go) | `cd api && go test ./internal/epk/... -run TestUpsertContent -count=1` | ❌ W0 | ⬜ pending |
| 03-W0-04 | Wave0 | 0 | EPK-04 | unit (Go) | `cd api && go test ./internal/epk/... -run TestSocialLinksSync -count=1` | ❌ W0 | ⬜ pending |
| 03-W0-05 | Wave0 | 0 | EPK-05 | unit (Go) | `cd api && go test ./internal/epk/... -run TestRepository -count=1` | ❌ W0 | ⬜ pending |
| 03-W0-06 | Wave0 | 0 | EPK-06 | unit (Go) | `cd api && go test ./internal/epk/... -run TestGeneratePDF -count=1` | ❌ W0 | ⬜ pending |
| 03-W0-07 | Wave0 | 0 | EPK-07 | unit (Go) | `cd api && go test ./internal/epk/... -run TestSectionVisibility -count=1` | ❌ W0 | ⬜ pending |
| 03-W0-08 | Wave0 | 0 | EPK-08 | unit (Go) | `cd api && go test ./internal/epk/... -run TestExportHistory -count=1` | ❌ W0 | ⬜ pending |
| 03-W0-09 | Wave0 | 0 | EPK-09 | unit (Go) | `cd api && go test ./internal/epk/... -run TestRepository -count=1` | ❌ W0 | ⬜ pending |
| 03-W0-10 | Wave0 | 0 | EPK-10 | unit (Vue) | `pnpm nx test dj --testPathPattern=EpkGigHighlightsSection` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `api/internal/epk/service_test.go` — mockRepo + mockStorage stubs; covers EPK-02 through EPK-09
- [ ] `api/internal/epk/repository_test.go` — covers EPK-05, EPK-08, EPK-09 (may use testcontainers like social repo tests)
- [ ] `api/internal/epk/handler_test.go` — HTTP handler routing tests
- [ ] `apps/dj/app/components/epk/__tests__/EpkBioSection.test.ts` — covers EPK-01
- [ ] `apps/dj/app/components/epk/__tests__/EpkPhotosSection.test.ts` — covers EPK-02
- [ ] `apps/dj/app/components/epk/__tests__/EpkExportPanel.test.ts` — covers EPK-06, EPK-07, EPK-08
- [ ] `apps/dj/app/components/epk/__tests__/EpkGigHighlightsSection.test.ts` — covers EPK-10 stub

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| PDF renders logo and artist colors correctly | EPK-06 | Visual fidelity — fpdf output requires visual inspection | Generate PDF, open in viewer, confirm logo and brand colors appear |
| Press photos appear in correct grid layout in PDF | EPK-02 | Layout/visual — pixel placement hard to unit test | Generate PDF with 4+ photos, verify grid placement in viewer |
| Presigned MinIO download URL opens file in browser | EPK-08 | Requires live MinIO + browser interaction | Click download on export history item, confirm file opens |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
