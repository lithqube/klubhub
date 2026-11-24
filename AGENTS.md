# KlubHub — agent guidelines

KlubHub is an open-source, self-hosted career platform for DJs (KlubHub DJ) and promoters (KlubHub Promoter), a project by EUN Records (Berlin). This file is the one source of rules for every coding agent; CLAUDE.md imports it.

## Products

**KlubHub DJ** is an open-source, self-hosted career management platform for independent DJs. It includes:

- Tracklist image generator (flagship feature)
- Social media scheduler (Instagram)
- EPK / Press Kit Builder (PDF export)
- Gig tracker (complete) and Rider Templates (v1.2.0)
- Finance tracker: invoicing, payments, agreements and email shipped in v1.1.0; v1.2.0 adds EN 16931 e-invoicing (Factur-X / XRechnung export validated before it ships, archive of issued documents, send by email with attachments), receipts on expenses, and German legal notes; the income/expense ledger shipped too, so Phase 5 is complete

**Current state:** Phases 0–5 plus 0.5, 1.5, 1.5.5, 4.5, 4.9 (RA import), 4.10 (Availability & Outreach) and 9 are complete — 13 of 18. v1.0.0, v1.0.1, v1.1.0 and v1.2.0 are released; 4.10 and the installer update/backup/rollback work are unreleased on `main`. Next: Phase 6 (Release Planner) → 7 (Tour Manager) → 8 (Unified Dashboard); 4.8 (Spotify) and 5.5 (US invoicing) are deferred. See `.planning/ROADMAP.md` for per-phase status.

**KlubHub Promoter** (merged to `main`, plan `.claude/plans/promoter-app.plan.md`): `apps/promoter` (Nuxt, port 4400), `api/cmd/promoter` + `api/internal/promoter/*`, shared UI in `libs/ui` (Nuxt layer, `#kui` alias). P0 (foundations: RLS tenancy, envelope encryption, embedded OPA, local/Zitadel identity, NATS outbox), P1 (events, venues, lineup, export pack), P2 (guest lists, attendee import, offline door, retention, sealed ban list) and P3.1 (audience CRM) are done. P3.2–P3.5 (email platform, campaigns, social assets, attribution) are next — see `.claude/plans/promoter-p3-audience-promotion.plan.md`; 4.10 shipped the front half of P4 (Offers). No versioned Promoter release exists yet. `.claude/plans/soundcloud-integration.plan.md` and `.claude/plans/spotify-integration.plan.md` hold related licensed-feature research. ADRs in `docs/adr/`.

## Architecture

```
Browser → Nuxt 4 (SSR/SPA) → Go API (chi) → PostgreSQL + Garage (S3 API)
         ↑ proxy only when NUXT_PUBLIC_API_BASE is set
         ↑ mock handlers at apps/dj/server/api/v1/ when unset
```

| Layer | Stack |
|---|---|
| **Frontend:** | Nuxt 4 / Vue 3 / TypeScript / Pinia / Tailwind CSS v4 / shadcn-vue / "Kinetic HUD" design system |
| **Backend:** | Go 1.26 / chi router / pgx v5 / goose migrations / go-pdf/fpdf |

| Mode | What runs |
|---|---|
| **Dev mode:** | `pnpm nx serve @dev/dj` runs frontend with mock data (no backend needed) |
| **Development stack:** | `bash scripts/setup.sh dev` starts db, storage, and a locally built API. Nuxt runs on the host at port 4200; the container callback uses `host.docker.internal:4200`. |
| **Production stack:** | `bash scripts/setup.sh prod` uses three image-only services: db, storage, and one supervised Go + Nuxt/Node + Playwright app. Go exposes UI and API on port 8080; internal Nuxt port 3000 is not published. `SERVE_FRONTEND=true` is production-only. |

Use the [canonical setup guide](./docs/SELF-HOSTING.md), not manual secret/bootstrap commands. Mode-specific `.local/dev` and `.local/prod` state and separate Compose projects prevent credential/data mixing. Existing `klubhub-dj` volumes require explicit reuse or migration. NEVER use `down -v` unless data is disposable.

Local fixes are not automatically in GHCR: publish a new version and verify package access. Both images pull anonymously from GHCR; pushing a `vX.Y.Z` git tag publishes `:vX.Y.Z` for both (see `docs/container-images.md`). The app has no general user authentication; TLS and CORS are not access control.

## Commands

```bash
# Start frontend with mock data (dev mode)
pnpm nx serve @dev/dj

# Start development backend (run local Nuxt per SELF-HOSTING.md)
bash scripts/setup.sh dev

# Run tests
pnpm nx run-many -t test
pnpm nx e2e dj-e2e

# Real Nuxt typecheck (baseline failures tracked in .hermes/plans/phase-5-progress.md)
pnpm nx typecheck @dev/dj

# Go unit + disposable-Postgres integration tests, race detector, no Nx cache
pnpm nx run api:test

# Linting
pnpm nx lint @dev/dj
```

<!-- nx configuration start-->
<!-- Leave the start & end comments to automatically receive updates. -->

### Working with Nx

- For navigating/exploring the workspace, invoke the `nx-workspace` skill first - it has patterns for querying projects, targets, and dependencies
- When running tasks (for example build, lint, test, e2e, etc.), always prefer running the task through `nx` (i.e. `nx run`, `nx run-many`, `nx affected`) instead of using the underlying tooling directly
- Prefix nx commands with the workspace's package manager (e.g., `pnpm nx build`, `npm exec nx test`) - avoids using globally installed CLI
- You have access to the Nx MCP server and its tools, use them to help the user
- For Nx plugin best practices, check `node_modules/@nx/<plugin>/PLUGIN.md`. Not all plugins have this file - proceed without it if unavailable.
- NEVER guess CLI flags - always check nx_docs or `--help` first when unsure
- For scaffolding tasks (creating apps, libs, project structure, setup), ALWAYS invoke the `nx-generate` skill FIRST before exploring or calling MCP tools

| nx_docs | When |
|---|---|
| USE for | advanced config options, unfamiliar flags, migration guides, plugin configuration, edge cases |
| DON'T USE for | basic generator syntax (`nx g @nx/react:app`), standard commands, things you already know |

- The `nx-generate` skill handles generator discovery internally - don't call nx_docs just to look up generator syntax

<!-- nx configuration end-->

## Where things live

| What | Where |
|---|---|
| Mock API handlers (frontend-only dev) | `apps/dj/server/api/v1/` |
| Kinetic HUD design tokens | `apps/dj/app/assets/css/styles.css` (shared system in `libs/ui`) |
| Roadmap and phase state | `.planning/ROADMAP.md`, `.planning/STATE.md` |
| ui-sdk (`apps/dj` only) | `@ui-sdk/vue` and `@ui-sdk/core` are installed in `apps/dj` only, so their docs live under `apps/dj/node_modules`. |

## Conventions

| Area | Rule |
|---|---|
| State | All `$fetch` calls live in Pinia store actions or composables — NEVER directly in `<script setup>` |
| State | Use `storeToRefs()` for reactive destructuring to preserve reactivity |
| State | Stores use setup function (Composition API) style — not options API |
| State | Frontend uses Pinia stores for all API calls |
| Theme | 3-way theme mode: `dark \| system \| light` |
| Theme | `useTheme` composable exports `mode` (user choice) and `theme` (resolved DOM value) |
| Theme | `data-theme` attribute on `<html>` drives CSS overrides |
| Theme | LocalStorage persistence via `klubhub-theme` key |
| API | Go backend uses handler → service → repository three-layer pattern |
| Naming | Pages: lowercase `tracklist.vue`, `social.vue` |
| Naming | Components: PascalCase `TracklistUploadZone.vue` |
| Naming | Singleton shell: `The*` prefix `TheNav.vue`, `TheBottomNav.vue` |
| Naming | Composables: `use*` camelCase `useTheme.ts` |
| Naming | Stores: `use*Store` `useTracklistStore` |
| Design | No design guesswork — use the Kinetic HUD design tokens in `apps/dj/app/assets/css/styles.css` |
| ui-sdk | Before using ui-sdk, follow the `ui-sdk` skill if it's installed, and read `apps/dj/node_modules/@ui-sdk/vue/docs/llms.txt`. |
| ui-sdk | To build a new component, also search `apps/dj/node_modules/@ui-sdk/vue/docs/core.md` by heading, and read `rendering.md` next to it. |

### Promoter security invariants (enforced by tests; do not bypass)

- Promoter DB access only via `tenantdb.WithTenant/Run`; NEVER import `pgxpool` in `internal/promoter/**`.
- Every new table: `tenant_id`, `ENABLE` + `FORCE` RLS, fail-closed policy, and a `data_class` comment; personal/financial columns are `*_enc` / `*_bidx` BYTEA.
- Every route goes through `authz.Engine.Handle` with an action known to the Rego policy.
- Events carry identifiers only (`events.New`); publish via the outbox in the same transaction.
- Client code patterns from bridge-latam are reimplemented, NEVER copied.

## Boundaries

| | Rule |
|---|---|
| ALWAYS | Use `pnpm nx` — NEVER call nx/go/nuxt directly; always through pnpm nx |
| ALWAYS | Run lint/typecheck after any code changes: `pnpm nx lint` and `pnpm nx typecheck` |
| ALWAYS | Test before commit: `pnpm nx run-many -t test` for unit tests, `pnpm nx e2e` for E2E |
| NEVER | Commit secrets — keep `.env`, generated Garage configuration, and `.local/` private. |

---

_Last updated: 2026-11-24_

<!-- super-board:begin v3.1.2 (managed; edits inside are overwritten on upgrade) -->
## Super Board

Board: Backlog · Ready · Building · QA · Review · Blocked · Done. Labels route: `qa` skips Building · `bug`, `feature`, none → built first.

| When | Use | NEVER |
|---|---|---|
| Set up / repair the board | `/super-board onboard` | DON'T hand-edit config mid-run |
| Check tickets before a run | `/super-board lint` | NEVER run with AC-less tickets |
| Drain the board | `/super-board run` | NEVER build, test or merge from the orchestrator |
| Board state / halt | `/super-board status` · `/super-board stop` | |
| Build one ticket | `/super-build` | NEVER build outside the card's worktree |
| QA a branch, or a live URL alone | `/super-qa` · `/super-qa <url>` | NEVER mark QA pass without evidence |
| Review a PR, merge | `/super-review` | NEVER `gh pr merge` direct — merge gate only |
| File bugs from Sentry, PostHog, PRs | `/super-collect` | DON'T file without a verifier pass |
| UI polish | `/ui-refine-loop` (human runs it) | board NEVER runs it |
| Diagram / explainer page | `/visual` | |
| Commit, pull, push this branch | `/git-sync` | NEVER force push or rebase shared history |

| Rule | Value |
|---|---|
| Config | `.claude/super-board/configs/<slug>.json` |
| Isolation | 1 card · 1 worktree · 1 branch |
| Merge | auto for normal changes · money, auth, destructive schema (DROP/TRUNCATE/RENAME) → human · PR > 400 changed lines → human |
| Migrations | additive → robot migrates allowed DBs only · live DB ALWAYS → 🙋 needs you |
| Blocked card | block-template comment · last line `blocked-by:` · 🙋 = needs you |
| Secrets | NEVER read `.env`; key names via `.claude/bin/super-board-env-check.sh` |

## Writing (super-board)

| Thing | Format → `.claude/skills/super-board/references/writing-standard.md` |
|---|---|
| Commit, PR title | `<emoji> [type] scope: subject` + short bullets · ✨ feat 🐛 fix 🔧 chore ♻️ refactor 🧪 test 📝 docs |
| PR body | blocks: status · Problem · Solution · AC + proof · history · Before\|After · Risk — `super-board-pr-body.sh` |
| Ticket | Problem · Context · Fix · AC · Risk · Blocked by → `docs/agents/issue-tracker.md` |
| Comment | `[role] [label] status` · Did · ✅ Done · ❌ Not done · Next · ≤ 8 lines |
| PR author notes | file + critical inline review comments: Purpose · What changed · Why it matters → `pr-author-notes.md` |

- NEVER chain steps with arrows. One step per line, lettered under Where.
- NEVER link screenshots. Embed a raw URL pinned to a sha.
- DON'T repeat file lists in status comments. DON'T write "Not verified" or "Next" in a PR body.
<!-- super-board:end -->
