# Nx tasks — KlubHub

This is a pnpm + Nx monorepo. The product is KlubHub DJ, with future products (Promoter, Label) landing as additional Nx projects in the same workspace.

## Project names (THE most common mistake)

| Project | Full name | What it is |
|---|---|---|
| KlubHub DJ Nuxt site | **`@dev/dj`** | The static site + Nuxt dev server (apps/dj/app, apps/site) |
| KlubHub DJ Go API | **`dj-api`** | The Go backend (apps/dj/api) |
| KlubHub site | `@site` | The KlubHub.io GitHub Pages static site (legacy alias of `@dev/dj` for the GitHub Pages build) |

Use `@dev/dj` for the Nuxt site, not `dj`. The shorter form silently resolves to the wrong project or fails to resolve.

Use `dj-api` (NOT `@dev/dj-api`) for the Go API — it's a different project.

## Targets

### `@dev/dj` targets

```
pnpm nx run @dev/dj:build        # Production Nuxt build → apps/dj/app/.output
pnpm nx run @dev/dj:serve        # Nuxt dev server → http://localhost:4200
pnpm nx run @dev/dj:test         # node --test on apps/site/site.test.mjs
pnpm nx run @dev/dj:lint         # ESLint on apps/site/public + apps/dj/app
pnpm nx run @dev/dj:build-site   # Static site build for GitHub Pages → apps/site/dist
```

### `dj-api` targets

```
pnpm nx run dj-api:serve     # Go run api/cmd/api/main.go → :8080
pnpm nx run dj-api:test      # cd api && go test ./...
pnpm nx run dj-api:build     # Go build → apps/dj/api/bin/api
pnpm nx run dj-api:lint      # go vet ./...
```

### Repo-level

```
pnpm nx run-many -t test                    # All tests
pnpm nx run-many -t lint                    # All lints
pnpm nx affected -t build --base=origin/main  # Only changed
```

## NX_DAEMON=false

The repo's dev workflow requires `NX_DAEMON=false` (or `--skip-nx-cache` on individual commands). Reason: Nx's daemon caches Go-bridge state that doesn't reflect Compose hot-reload correctly. Without disabling, Go test results get stale by 30-60 seconds.

Set in shell before running Nx commands:

```bash
export NX_DAEMON=false
pnpm nx run dj-api:test
```

## Common pitfalls

| Symptom | Cause | Fix |
|---|---|---|
| `nx run dj:serve` errors with "project not found" | Used `dj` instead of `@dev/dj` | Use `@dev/dj:serve` |
| `nx run dj-api:test` works but Go binary is stale | Nx daemon cached the old build | `NX_DAEMON=false pnpm nx run dj-api:test` |
| `nx run @dev/dj:test` reports 0 tests | Test glob isn't picking up | Check `apps/site/site.test.mjs` exists; run `pnpm nx run @dev/dj:test --verbose` |
| `nx run-many -t test` runs the wrong subset | Affected graph is stale | `pnpm nx reset` then re-run |
