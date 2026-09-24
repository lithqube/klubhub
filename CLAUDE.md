# KlubHub DJ — Claude AI Assistant Guidelines

This file provides Claude-specific instructions for working with the KlubHub DJ codebase.

---

## Project Context

**KlubHub DJ** is an open-source, self-hosted career management platform for independent DJs. It includes:
- Tracklist image generator (flagship feature)
- Social media scheduler (Instagram)
- EPK / Press Kit Builder (PDF export)
- Gig tracker, Finance tracker (mock UI, Phase 4/5 wiring pending)

**Current state:** Phases 0-3 and 1.5, 1.5.5 complete. Phase 4 (Gig Tracker) is next.

---

## Architecture Summary

```
Browser → Nuxt 4 (SSR/SPA) → Go API (chi) → PostgreSQL + Garage (S3 API)
         ↑ proxy only when NUXT_PUBLIC_API_BASE is set
         ↑ mock handlers at apps/dj/server/api/v1/ when unset
```

**Frontend:** Nuxt 4 / Vue 3 / TypeScript / Pinia / Tailwind CSS v4 / shadcn-vue / "Kinetic HUD" design system

**Backend:** Go 1.26 / chi router / pgx v5 / goose migrations / go-pdf/fpdf

**Dev mode:** `pnpm nx serve @dev/dj` runs frontend with mock data (no backend needed)

**Development stack:** `bash scripts/setup.sh dev` starts db, storage, and a locally built API. Nuxt runs on the host at port 4200; the container callback uses `host.docker.internal:4200`.

**Production stack:** `bash scripts/setup.sh prod` uses three image-only services: db, storage, and one supervised Go + Nuxt/Node + Playwright app. Go exposes UI and API on port 8080; internal Nuxt port 3000 is not published. `SERVE_FRONTEND=true` is production-only.

Use the [canonical setup guide](./docs/SELF-HOSTING.md), not manual secret/bootstrap commands. Mode-specific `.local/dev` and `.local/prod` state and separate Compose projects prevent credential/data mixing. Existing `klubhub-dj` volumes require explicit reuse or migration. Never use `down -v` unless data is disposable.

Local fixes are not automatically in GHCR: publish a new version and verify package access. The default v1.0.0 package returned 403 to anonymous requests. The app has no general user authentication; TLS and CORS are not access control.

---

## Key Conventions

### State Management
- All `$fetch` calls live in Pinia store actions or composables — never directly in `<script setup>`
- Use `storeToRefs()` for reactive destructuring to preserve reactivity
- Stores use setup function (Composition API) style — not options API

### Theme System
- 3-way theme mode: `dark | system | light`
- `useTheme` composable exports `mode` (user choice) and `theme` (resolved DOM value)
- `data-theme` attribute on `<html>` drives CSS overrides
- LocalStorage persistence via `klubhub-theme` key

### API Layer
- Go backend uses handler → service → repository three-layer pattern
- Frontend uses Pinia stores for all API calls
- Mock handlers at `apps/dj/server/api/v1/` for frontend-only development

### Naming
- Pages: lowercase `tracklist.vue`, `social.vue`
- Components: PascalCase `TracklistUploadZone.vue`
- Singleton shell: `The*` prefix `TheNav.vue`, `TheBottomNav.vue`
- Composables: `use*` camelCase `useTheme.ts`
- Stores: `use*Store` `useTracklistStore`

---

## Critical Rules

1. **Never commit secrets** — keep `.env`, generated Garage configuration, and `.local/` private.
2. **Use `pnpm nx`** — never call nx/go/nuxt directly; always through pnpm nx
3. **Run lint/typecheck** — after any code changes, run `pnpm nx lint` and `pnpm nx typecheck`
4. **Test before commit** — run `pnpm nx run-many -t test` for unit tests, `pnpm nx e2e` for E2E
5. **No design guesswork** — use the Kinetic HUD design tokens in `apps/dj/app/assets/css/styles.css`

---

## Useful Commands

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

---

*Last updated: 2026-04-27*