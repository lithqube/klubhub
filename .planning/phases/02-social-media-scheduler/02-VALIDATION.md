---
phase: 2
slug: social-media-scheduler
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-21
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (backend)** | `go test` + testcontainers-go (existing) |
| **Framework (frontend)** | Vitest (existing, `apps/dj/vitest.config.ts`) |
| **Config file (frontend)** | `apps/dj/vitest.config.ts` |
| **Quick run (backend)** | `pnpm nx test api -- -run TestSocial -count=1 ./internal/social/...` |
| **Full suite (backend)** | `pnpm nx test api` |
| **Quick run (frontend)** | `pnpm nx test dj -- --testPathPattern social` |
| **Full suite (frontend)** | `pnpm nx test dj` |
| **Estimated runtime** | ~60 seconds (backend integration), ~10 seconds (frontend) |

---

## Sampling Rate

- **After every task commit:** Run the test package for the task's files (`go test ./internal/social/...` for backend tasks; `pnpm nx test dj -- --testPathPattern social` for frontend tasks)
- **After every plan wave:** Run `pnpm nx run-many -t test -p dj api`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** ~60 seconds (backend), ~10 seconds (frontend)

---

## Per-Task Verification Map

| Task ID | Req | Test Type | Automated Command | File Exists | Status |
|---------|-----|-----------|-------------------|-------------|--------|
| 2-oauth-url | SOCL-01 | unit | `pnpm nx test api -- -run TestOAuthURL ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-oauth-callback | SOCL-01 | unit (mocked HTTP) | `pnpm nx test api -- -run TestOAuthCallback ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-token-refresh-proactive | SOCL-02 | unit | `pnpm nx test api -- -run TestTokenRefreshProactive ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-token-refresh-failure | SOCL-02 | unit | `pnpm nx test api -- -run TestTokenRefreshFailure ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-schedule-utc | SOCL-03 | unit | `pnpm nx test api -- -run TestScheduleUTCConversion ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-worker-publish-due | SOCL-04 | integration (testcontainers) | `pnpm nx test api -- -run TestWorkerPublishDue ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-publish-feed | SOCL-05 | unit (mocked HTTP) | `pnpm nx test api -- -run TestPublishFeedPost ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-publish-story | SOCL-06 | unit (mocked HTTP) | `pnpm nx test api -- -run TestPublishStory ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-presigned-url | SOCL-07 | unit | `pnpm nx test api -- -run TestPresignedURL ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-image-validation | SOCL-08 | unit | `pnpm nx test api -- -run TestImageValidation ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-social-store | SOCL-09 | unit (Vitest) | `pnpm nx test dj -- --testPathPattern social.test` | ❌ Wave 0 | ⬜ pending |
| 2-post-status-enum | SOCL-10 | unit | `pnpm nx test api -- -run TestPostStatusEnum ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-backoff-schedule | SOCL-11 | unit | `pnpm nx test api -- -run TestBackoffSchedule ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-retry-exhaustion | SOCL-11 | unit | `pnpm nx test api -- -run TestRetryExhaustion ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-manual-retry | SOCL-12 | unit | `pnpm nx test api -- -run TestManualRetry ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-rate-limit | SOCL-13 | unit (mocked HTTP) | `pnpm nx test api -- -run TestRateLimit429 ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-edit-blocking | SOCL-14 | unit | `pnpm nx test api -- -run TestEditBlocking ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-disconnect-cascade | SOCL-15 | unit | `pnpm nx test api -- -run TestDisconnectCascade ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-duplicate-prevention | SOCL-16 | unit (mocked HTTP) | `pnpm nx test api -- -run TestDuplicatePrevention ./internal/social/...` | ❌ Wave 0 | ⬜ pending |
| 2-prefill-from-image | SOCL-17 | unit (Vitest) | `pnpm nx test dj -- --testPathPattern socialPage.test` | ❌ Wave 0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `api/internal/platform/migrations/003_social.sql` — required before any social service tests can run
- [ ] `api/internal/social/handler_test.go` — HTTP handler tests for all routes
- [ ] `api/internal/social/service_test.go` — business logic unit tests (scheduling, retry, disconnect cascade)
- [ ] `api/internal/social/worker_test.go` — worker tick, backoff calculation, token refresh trigger
- [ ] `api/internal/social/instagram_test.go` — mocked HTTP Instagram API client (container create, publish, status check)
- [ ] `apps/dj/app/stores/__tests__/social.test.ts` — useSocialStore unit tests (load, create, retry, prefill from imageId)
- [ ] `apps/dj/app/components/social/__tests__/SocialPostCompose.test.ts` — caption auto-gen, char counter, timezone combobox

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Instagram OAuth round-trip in browser | SOCL-01 | Requires live Meta OAuth redirect + browser interaction | Click "Connect Instagram", complete OAuth, verify account shown as connected |
| Token auto-refresh warning UI | SOCL-02 | Requires time manipulation + live token state | Simulate near-expiry token, verify yellow warning banner appears in nav |
| Post published on Instagram at scheduled time | SOCL-04/05 | Requires live Instagram API and time passage | Schedule post 5 min in future, wait, verify post appears on Instagram |
| Stories App Review behavior | SOCL-06 | Unclear if dev-mode bypasses App Review | Attempt story publish in development mode; document result |
| Queue/calendar view rendering | SOCL-09 | Visual layout requires human inspection | View queue with posts in all 5 statuses; verify grouping and icons |
| "Schedule Post" CTA on export page | SOCL-17 | Requires full page interaction | Export tracklist image, click CTA, verify compose panel opens with image pre-filled |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s (backend), < 10s (frontend)
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
