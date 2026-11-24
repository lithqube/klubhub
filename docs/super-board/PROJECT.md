# KlubHub — project brief for board workers

KlubHub is an open-source, self-hosted career platform for DJs and promoters, a project by EUN Records (Berlin). Rules live in [AGENTS.md](../../AGENTS.md); this page is the map.

## Products

| Product | App | API | Status |
|---|---|---|---|
| KlubHub DJ | `apps/dj` (Nuxt 4, port 4200) | `api/cmd/api` + `api/internal/*` | v1.2.0 released; phases 6–8 left (`.planning/ROADMAP.md`) |
| KlubHub Promoter | `apps/promoter` (Nuxt 4, port 4400) | `api/cmd/promoter` + `api/internal/promoter/*` | P0–P2 + P3.1 done, never released (`.claude/plans/promoter-app.plan.md`) |
| Landing site | `apps/site` | — | deployed by `.github/workflows/pages.yml` from `main` |
| Shared UI | `libs/ui` (Nuxt layer, `#kui` alias) | — | Kinetic HUD tokens and shadcn-vue primitives |

## Stack

Nx monorepo with pnpm. Frontend: Nuxt 4, Vue 3.5, Pinia, Tailwind CSS v4. Backend: Go 1.26, chi, pgx v5, goose migrations embedded and run at API startup. Storage: PostgreSQL and Garage (S3 API). Promoter adds RLS tenancy, envelope encryption, embedded OPA and a NATS outbox.

## Checks (what CI runs)

```bash
pnpm nx run-many -t lint -p @dev/dj @dev/dj-e2e @dev/promoter @dev/promoter-e2e
pnpm nx typecheck @dev/dj
pnpm nx typecheck @dev/promoter
pnpm nx run-many -t test -p @dev/dj @dev/promoter @dev/ui
pnpm nx run api:test
```

E2E: `pnpm nx e2e @dev/dj-e2e`, `pnpm nx e2e @dev/promoter-e2e`.

## Release

| Item | Where |
|---|---|
| Images | `ghcr.io/lithqube/klubhub-dj-api` (arm64), `ghcr.io/lithqube/klubhub-promoter-api` (amd64 + arm64) |
| Tags | `sha-<commit>` from `main`; `vX.Y.Z` from a pushed `v*` git tag ([container images](../container-images.md)) |
| Installer | `scripts/install.sh` (`--dj`, `--promoter`, `--release`) |
| Notes | `CHANGELOG.md`, `docs/release-notes/` |
| Editions | [EDITIONS.md](../EDITIONS.md): OSS = everything built; SoundCloud/Spotify planned as licensed; hosted public pages are SaaS |

## Human gates

Merges into `main` are yours. Finance and e-invoicing (`api/internal/finance`, `api/internal/einvoice`), Promoter authz/identity, and migrations always wait for a person. Outreach sending stays off until an operator sets `OUTREACH_PROVIDER_PERMITS_OUTREACH=true` ([OUTREACH-PROVIDER.md](../OUTREACH-PROVIDER.md)).
