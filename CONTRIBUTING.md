# Contributing to KlubHub DJ

Thanks for your interest in contributing! KlubHub DJ is an open-source, self-hosted
career-management platform for independent DJs. We welcome bug reports, feature
requests, docs improvements, and pull requests of all sizes.

This guide covers the practical steps. For project intent and module scope see
[`docs/`](./docs/) and the [roadmap](./docs/v1-release-plan.md).

---

## Code of Conduct

By participating you agree to the [Code of Conduct](./CODE_OF_CONDUCT.md).
Be respectful, assume good faith, and help us keep the project welcoming.

---

## Project layout

```
klubhub-dj/
├── apps/
│   ├── dj/        # Nuxt 4 frontend (Vue 3 + TypeScript)
│   └── dj-e2e/    # Playwright end-to-end tests
├── api/           # Go backend (chi router, pgx, goose migrations)
├── docs/          # BRD, NFR, architecture, release plans
├── scripts/       # backup.sh, restore.sh
└── docker-compose.yml
```

The repo is an Nx 22 monorepo with pnpm workspaces. The full conventions live
in [`AGENTS.md`](./AGENTS.md) and the Claude-specific notes in [`CLAUDE.md`](./CLAUDE.md).

---

## Development setup

### Prerequisites

| Tool   | Version |
|--------|---------|
| Node   | 20.x LTS |
| pnpm   | 9.x |
| Go     | 1.26+ |
| Docker | 24+ with Compose v2 |

### First run

```bash
# 1. Install JS dependencies (monorepo + Nuxt app)
pnpm install

# 2. Frontend-only mode (no backend needed — uses Nitro mock handlers)
pnpm nx serve dj
# Open http://localhost:4200

# 3. Full stack (db + storage + api + frontend)
cp .env.example .env
cp garage.toml.example garage.toml
# Edit garage.toml and .env — see README "Garage storage setup"
docker compose up -d
```

See the [README quick-start](./README.md#quick-start) for the three supported
run modes.

---

## Workflow

1. **Fork & branch.** Branch from `main`:
   `git checkout -b feat/<short-description>` or `fix/<short-description>`.
2. **Make your change.** Keep commits focused; write descriptive messages.
3. **Verify before pushing.**
   ```bash
   pnpm nx lint <project>
   pnpm nx typecheck <project>
   pnpm nx run-many -t test
   pnpm nx e2e dj-e2e   # requires full Docker stack
   ```
4. **Open a Pull Request** using the [PR template](./.github/PULL_REQUEST_TEMPLATE.md).
   Link any related issue and fill in the "How to test" section.
5. **Address review feedback** by pushing follow-up commits (no force-pushes
   during review).

---

## Coding conventions

### All languages

- 2-space indentation, LF line endings (see [`.editorconfig`](./.editorconfig))
- Prettier formatting (`pnpm exec prettier --write .`)
- No secrets in source — use `.env` (gitignored) and `.env.example` as the template

### Frontend (`apps/dj`)

- Vue 3 Composition API with `<script setup lang="ts">`
- Pinia stores in **setup-function style** — never Options API
- All `$fetch` calls live in store actions or composables, never inline in components
- Reuse tokens from the **Kinetic HUD** design system in `apps/dj/app/assets/css/styles.css`
- 3-way theme (`dark | system | light`) is the only supported theming

### Backend (`api/`)

- `handler → service → repository` three-layer pattern
- Standard library `net/http` style via [`chi`](https://github.com/go-chi/chi)
- Migrations live in `api/internal/platform/migrations/` (goose) — name them
  `NNN_description.sql` where `NNN` is monotonically increasing
- Structured logging via [`rs/zerolog`](https://github.com/rs/zerolog)
- Configuration via [`kelseyhightower/envconfig`](https://github.com/kelseyhightower/envconfig)

### Nx-specific

Always run tasks through `pnpm nx …` — never invoke `nx`, `go`, `nuxt`, `vitest`,
or `playwright` directly. See [`AGENTS.md`](./AGENTS.md) for the full rationale.

---

## Adding a new module

If you want to add a new feature module (e.g. a `samples` library):

1. **Open an issue first** describing the user story, the data model sketch, and
   which existing modules it depends on.
2. Wait for maintainer sign-off before starting implementation — modules are
   tracked in `docs/v1-release-plan.md` and we keep the roadmap ordered.
3. Frontend: add `apps/dj/app/pages/<module>.vue`, `components/<module>/`,
   `stores/<module>.ts`, and any Nitro mock handlers under
   `apps/dj/server/api/v1/<module>/`.
4. Backend: scaffold `api/internal/<module>/{handler,service,repo}.go`, register
   routes in `api/cmd/api/main.go`, add a migration in `api/internal/platform/migrations/`.
5. Update the mock-vs-backend table in the README and the phase roadmap.

---

## Filing issues

- **Bugs:** use the [bug report template](./.github/ISSUE_TEMPLATE/bug_report.md).
  Include steps to reproduce, expected vs actual, environment (OS, Docker, pnpm,
  Go versions), and logs.
- **Feature requests:** use the [feature request template](./.github/ISSUE_TEMPLATE/feature_request.md).
  Describe the user problem first, then the proposed solution.
- **Security issues:** **do not** open a public issue — see [SECURITY.md](./SECURITY.md).

---

## Pull request checklist

- [ ] Linked to a tracking issue (or explained why one isn't needed)
- [ ] `pnpm nx lint <project>` passes
- [ ] `pnpm nx typecheck <project>` passes
- [ ] `pnpm nx run-many -t test` passes
- [ ] E2E passes (if UI or integration changes) — `pnpm nx e2e dj-e2e`
- [ ] README / docs updated for any user-facing change
- [ ] No new secrets, generated artifacts, or build outputs committed
- [ ] Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
      (e.g. `feat(tracklist): add CSV export`, `fix(epk): handle missing photo`)

---

## Questions?

- General questions: open a GitHub Discussion
- Bugs / features: open an issue
- Security: see [SECURITY.md](./SECURITY.md)

Thanks for helping make KlubHub DJ better. 🛠️
