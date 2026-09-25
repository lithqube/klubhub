# Phase 5 — DJ Invoices, Event Agreements and Email: progress and evidence

> **Authoritative verification log for Phase 5.** Updated only after running
> real commands and recording their actual output. No claims of green from
> no-op output, unverified test targets, or unproven mock fallbacks.
> Companion plan: `.hermes/plans/2026-09-24_213014-phase-5-invoices-agreements-email.md`.

## Wave 0 — prerequisites and baseline (completed 2026-09-24)

Branch: `feat/phase-5-invoices-agreements-email`
Baseline commit: `a52d62c feat(ui): refine HUD navigation and previews; plan phase 5`

### Real verification targets established

`api/project.json` (new) defines the API test target with cache disabled:

```json
{
  "name": "api",
  "projectType": "application",
  "sourceRoot": "api",
  "targets": {
    "test": {
      "executor": "nx:run-commands",
      "cache": false,
      "options": {
        "command": "go test ./... -race -count=1 -p 1",
        "cwd": "api"
      }
    }
  }
}
```

`-p 1` serializes package compilation. Without it, `internal/platform/config`
test pollution from a previous package's `DATABASE_URL` shadow the
`_FILE`-based test fixture.

`apps/dj/project.json` adds the `typecheck` target (cache disabled):

```json
"typecheck": {
  "executor": "nx:run-commands",
  "cache": false,
  "options": {
    "command": "vue-tsc --noEmit -p .nuxt/tsconfig.json",
    "cwd": "apps/dj",
    "envFile": "{workspaceRoot}/.env.example"
  }
}
```

`vue-tsc --noEmit` against the Nuxt-generated tsconfig sidesteps the
composite-project emit constraint in the workspace `tsconfig.base.json`,
which previously broke `nuxt typecheck`. The Nx `envFile` resolves
`NUXT_PUBLIC_API_BASE` for the proxy-mode build guard. `nuxt prepare`
must run before this target: `NUXT_PUBLIC_API_BASE=http://api:8080 pnpm exec nuxt prepare --cwd apps/dj`.

`apps/dj/app/utils/__tests__/verification-targets.test.ts` (new) asserts
both contracts so a regression in the verification targets is caught in
the Vitest run.

### Verification commands and recorded results

```text
$ pnpm nx show projects
@dev/dj-e2e
site
@dev/dj
api

$ pnpm nx run api:test         # exit 0 — all packages green
ok  github.com/klubhub/dj/api/cmd/api
ok  github.com/klubhub/dj/api/internal/artwork
ok  github.com/klubhub/dj/api/internal/epk
ok  github.com/klubhub/dj/api/internal/gig
ok  github.com/klubhub/dj/api/internal/platform/config
ok  github.com/klubhub/dj/api/internal/platform/crypto
ok  github.com/klubhub/dj/api/internal/platform/http
ok  github.com/klubhub/dj/api/internal/platform/migrations
ok  github.com/klubhub/dj/api/internal/platform/storage
ok  github.com/klubhub/dj/api/internal/ra
ok  github.com/klubhub/dj/api/internal/settings
ok  github.com/klubhub/dj/api/internal/social
ok  github.com/klubhub/dj/api/internal/tracklist
?   github.com/klubhub/dj/api/internal/contact  [no test files]
?   github.com/klubhub/dj/api/internal/platform/db  [no test files]
?   github.com/klubhub/dj/api/internal/platform/log  [no test files]
?   github.com/klubhub/dj/api/internal/venue         [no test files]

$ pnpm nx test @dev/dj         # exit 0 — Vitest
Test Files  18 passed (18)
Tests       196 passed (196)

$ pnpm nx run @dev/dj:typecheck # exit 1 — real signal
error TS ... count: 138 errors
```

The typecheck run is real (not a no-op) and surfaces **138 pre-existing
TypeScript errors** across `apps/dj` components, stores, composables and
tests. These are not Phase 5 regressions; they are dormant latent issues
the previous "typecheck" target masked by silently succeeding. They must
be triaged and addressed in a dedicated cleanup wave **before** wave 1
writes new finance code on top of a moving typechecker.

### Real defects fixed in wave 0

Two Go defects in HEAD that the previous test runs were masking:

1. `api/internal/gig/integration_test.go` — `TestIntegration_LinkTracklist_HandlerRoutes`
   decoded `GET /{id}/detail` as the bare `GigDetailResponse` while the
   handler wraps it in `{data: detail}`. Test now unwraps the envelope.

2. `api/internal/platform/config/config.go` and `config_test.go` — when
   `POSTGRES_PASSWORD_FILE` is set but the inherited shell env still has
   `POSTGRES_PASSWORD=***` (from `.env.example` via `nx` `loadRootEnvFiles`),
   the explicit literal beats the file value and the synthesized DSN
   contained `***`. Fix: the synthesis branch in `Load` now runs after
   `loadFileSecrets`; the test additionally `os.Unsetenv("POSTGRES_PASSWORD")`
   so the file path is actually exercised. The placeholder
   `DATABASE_URL="_pending_synthesis_"` is cleaned up by the second
   `envconfig.Process` pass in both the success and failure branches so
   sibling tests in the same package don't see leaked state.

### Discovery: E2E and API test tooling

```text
$ pnpm nx show project @dev/dj-e2e --json | jq '.targets | keys'
[ "e2e", "e2e-ci", "e2e-ci--merge-reports", "e2e-ci--src/example.spec.ts",
  "e2e-ci--src/screenshot.spec.ts", "lint", "typecheck" ]

$ pnpm nx show project @dev/dj --json | jq '.targets | keys'
[ "build", "build-deps", "build-static", "lint", "serve", "serve-static",
  "test", "test-ci", "test-ci--...", "typecheck", "watch-deps" ]
```

E2E uses Playwright. `pnpm nx e2e @dev/dj-e2e` depends on
`@dev/dj:serve-static` (the production Nuxt build), so a full E2E run
requires a successful `pnpm nx run @dev/dj:build-static` and a reachable
API base. Today the E2E specs (`apps/dj-e2e/src/example.spec.ts`,
`apps/dj-e2e/src/screenshot.spec.ts`) are placeholders — Phase 5 will
add specs covering invoice/agreement/email end-to-end through a real
Go API + captured SMTP transport.

The Bruno API test suite at `tests/bruno/` is the API integration layer.
Wave 1 will add finance/agreement/email contract fixtures there once the
endpoint contracts are finalised.

## Wave 1 — billing identity and wire contracts (next)

**Goal:** Add a separate `billing_profiles` model distinct from public
artist biography, with validation, GET/PUT API, and typed frontend
contract.

**Planned files:**
- `api/internal/finance/{model,validation,handler,repository,service,contract_test}.go` (new)
- `api/internal/platform/migrations/012_billing_profiles.sql` (new; next goose id)
- `apps/dj/app/types/finance.ts` (new)
- `apps/dj/app/stores/__tests__/finance.test.ts` (new)
- `apps/dj/app/server/api/v1/finance/billing-profile.{get,put}.ts` mock passthrough (new, dev-only)
- `api/cmd/api/main.go` wires the new handler under `/api/v1/finance/billing-profile`

**Coverage required:**
- Unit: validation (required fields, ISO 4217 currency, address shape)
- Unit: repository (round-trip via testcontainers Postgres)
- Contract: handler round-trips `{data: profile}` shape via `httptest`
- Frontend: store actions for GET/PUT and contract for snake_case fields

**Gate:** missing legal identity cannot issue documents; private billing
details never leak into public EPK; concurrency-safe update with
`updated_at` token.

## Wave 2–8 — pending

Implementation order is locked in the plan. Each wave ends with the
matching unit + contract tests plus the relevant E2E / Bruno / API
integration coverage before the next wave starts. No wave commits until
the previous wave has been verified by running `pnpm nx run api:test`,
`pnpm nx test @dev/dj`, and `pnpm nx run @dev/dj:typecheck` to actual
exit 0 — no waivers, no skip flags.

## Open before production use

- DJ billing jurisdiction and tax treatment (blocks FIN-10 completion).
- Approved agreement wording (blocks agreement finalisation claims).
- SMTP sender credentials (blocks any real recipient send).
- Mock fixture exclusion must be verified against the production build
  output, not just the dev/staging bundle.

These are listed in the plan and are *not* silently declared complete.

## Definition of done for the whole phase

- Wave 1–8 each green under `pnpm nx run api:test`, `pnpm nx test @dev/dj`,
  and `pnpm nx run @dev/dj:typecheck` with real exit codes.
- Pre-existing 138 typecheck errors triaged and either fixed, removed,
  or explicitly waived with a documented reason.
- Production mock-fixture exclusion verified by inspecting
  `apps/dj/.output/` for fixture strings.
- E2E spec added covering a gig → agreement → PDF → email → payment flow
  against a real API + captured SMTP transport.
- Two-stage review (spec then quality) on every wave's diff.
- Roadmap Phase 5 status updated to complete only after the gates above.
