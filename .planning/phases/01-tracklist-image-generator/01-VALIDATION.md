---
phase: 1
slug: tracklist-image-generator
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-14
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (Go)** | `testing` stdlib + testify v1.11.1 + testcontainers-go v0.41.0 |
| **Framework (Nuxt)** | Vitest v4.0.8 (unit), `@playwright/test` v1.36.0 (e2e) |
| **Go config** | `api/` — standard `go test ./...` |
| **Nuxt config** | `apps/dj/vitest.config.ts` |
| **Quick run command** | `cd api && go test ./internal/tracklist/... -run TestParse -count=1 -timeout 30s` |
| **Full suite command** | `cd api && go test ./... -timeout 120s && pnpm nx test dj` |
| **Estimated runtime** | ~30 seconds (unit); ~90s with e2e |

---

## Sampling Rate

- **After every task commit:** Run `cd api && go test ./internal/tracklist/... -timeout 30s`
- **After every plan wave:** Run `cd api && go test ./... -timeout 120s && pnpm nx test dj`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds (unit suite)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 1-01-01 | 01 | 1 | TRKL-02 | unit | `go test ./internal/tracklist/... -run TestParseRekordbox` | ❌ W0 | ⬜ pending |
| 1-01-02 | 01 | 1 | TRKL-03 | unit | `go test ./internal/tracklist/... -run TestTrackFields` | ❌ W0 | ⬜ pending |
| 1-01-03 | 01 | 1 | TRKL-04 | unit | `go test ./internal/tracklist/... -run TestInvalidFormat` | ❌ W0 | ⬜ pending |
| 1-01-04 | 01 | 1 | TRKL-05 | unit | `go test ./internal/tracklist/... -run TestMaxUploadSize` | ❌ W0 | ⬜ pending |
| 1-01-05 | 01 | 1 | TRKL-06 | unit | `go test ./internal/tracklist/... -run TestPartialParse` | ❌ W0 | ⬜ pending |
| 1-01-06 | 01 | 1 | TRKL-07 | unit | `go test ./internal/tracklist/... -run TestZeroTracks` | ❌ W0 | ⬜ pending |
| 1-02-01 | 02 | 1 | TRKL-09 | integration | `go test ./internal/tracklist/... -run TestSaveTracklist` | ❌ W0 | ⬜ pending |
| 1-02-02 | 02 | 1 | TRKL-08 | integration | `go test ./internal/tracklist/... -run TestUpdateTrack` | ❌ W0 | ⬜ pending |
| 1-02-03 | 02 | 1 | TRKL-27 | integration | `go test ./internal/tracklist/... -run TestDeleteCascade` | ❌ W0 | ⬜ pending |
| 1-03-01 | 03 | 2 | TRKL-10 | unit | `go test ./internal/artwork/... -run TestFetchChain` | ❌ W0 | ⬜ pending |
| 1-03-02 | 03 | 2 | TRKL-11 | unit | `go test ./internal/artwork/... -run TestArtworkCache` | ❌ W0 | ⬜ pending |
| 1-04-01 | 04 | 2 | TRKL-17 | e2e | `pnpm nx e2e dj-e2e --grep="screenshot dimensions"` | ❌ W0 | ⬜ pending |
| 1-04-02 | 04 | 2 | TRKL-26 | e2e | `pnpm nx e2e dj-e2e --grep="unicode rendering"` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `api/internal/tracklist/parser_test.go` — stubs covering TRKL-02, 03, 04, 05, 06, 07
- [ ] `api/internal/tracklist/repository_test.go` — testcontainers-go integration tests for TRKL-08, 09, 27
- [ ] `api/internal/artwork/fetcher_test.go` — mock HTTP server tests for TRKL-10, 11
- [ ] `api/internal/platform/migrations/002_tracklists.sql` — tracklists + tracks schema needed by integration tests
- [ ] `api/testdata/rekordbox_sample.txt` — UTF-16 LE TSV with BOM, 5 tracks including CJK titles
- [ ] `apps/dj-e2e/src/screenshot.spec.ts` — Playwright e2e for TRKL-17, TRKL-26 (needs running stack)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Drag-and-drop file upload | TRKL-01 | Browser drag events not reliable in headless | Open http://127.0.0.1:3000/tracklist, drag .txt file onto dropzone, verify parse |
| Live preview image renders in UI | TRKL-16 | Visual check | After upload+save, verify image appears in right panel at 50% resolution |
| "Export Both" downloads two files | TRKL-18 | Download dialog interaction | Click Export Both, verify two files: 1080x1920 + 1080x1080 |
| Cover art placeholder shows "Retry" button | TRKL-12 | API outage simulation required | Disable API keys, upload tracklist, verify placeholder + retry button |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
