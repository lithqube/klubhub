# Technology Stack

**Analysis Date:** 2026-04-27

## Languages

| Language | Usage |
|---|---|
| TypeScript 5.9 | All frontend code, Nuxt server routes, configuration |
| Vue 3.5 | Frontend UI components and templates |
| Go 1.22+ | Backend API server, worker, PDF generation |
| SQL | PostgreSQL migrations (goose) |

## Frontend

### Framework
- **Nuxt 4.3** — Full-stack Vue framework (SSR, file-based routing, Nitro server)
- **Vue 3.5** — Reactive UI framework
- **Vite 7** — Build tool (via `@tailwindcss/vite`)

### UI & Styling
- **Tailwind CSS v4** — Utility CSS via `@tailwindcss/vite` Vite plugin (not a Nuxt module); all tokens in `@theme {}` CSS block — no `tailwind.config.ts`
- **shadcn-vue** — Base component library (`apps/dj/components.json`); installed in `app/components/ui/`
- **lucide-vue-next** — Icon library
- **"Kinetic HUD" design system** — Custom CSS classes in `assets/css/styles.css` (`@layer components {}`): glass panels, HUD cards, accent bars, data-frag chips, badge system, progress bars, bar charts, upload zones, HUD buttons/inputs

### State Management
- **Pinia** — All global state via setup-function stores (not options API)
  - `useTracklistStore` — tracklist, tracks, upload state, past tracklists
  - `useSocialStore` — posts, account, compose panel
  - `useEpkStore` — EPK content, photo paths, exports, save status
  - `useSettingsStore` — DJ name, logo, preset, visible fields, bg mode
  - `useUiStore` — step state, loading flags, error strings

### Composables
- `useTheme` — 3-way theme mode (`dark | system | light`); OS preference listener; `initTheme()` with localStorage; exports `mode`, `theme`, `setTheme`, `toggleTheme`
- `useTracklist` — API composable (`uploadTracklist`, `listTracklists`, `getTracklist`, `updateTrack`, `deleteTracklist`, `generateImage`)
- `useEpkAutosave` — debounced EPK content auto-save watcher
- `useSocialPostForm` — Form state and validation for social post compose

### Testing (Frontend)
- **Vitest 4** — Unit/integration test runner
- **@vue/test-utils 2** — Vue component testing
- **jsdom** — DOM environment for unit tests
- **@playwright/test** — E2E testing

## Backend

### Language & Runtime
- **Go 1.22+** — API server and publishing worker

### Framework & Libraries
- **go-chi/chi v5** — HTTP router
- **jackc/pgx v5** — PostgreSQL driver (pgxpool for connection pooling)
- **pressly/goose v3** — Database migrations (embedded in binary via `//go:embed`)
- **minio/minio-go v7** — Object storage client
- **go-pdf/fpdf** — PDF generation for EPK exports
- **golang.org/x/image** — Image processing

### Architecture Pattern
- **Handler → Service → Repository** three-layer pattern
- Thin interface stack: `handler → serviceIface`, `service → repoIface` for independent mock testing without testcontainers
- Worker goroutine with `signal.NotifyContext` for graceful SIGTERM shutdown

### External APIs
- **Spotify Web API** — Primary cover art source
- **Discogs API** — Cover art fallback
- **MusicBrainz / CAA** — Cover art final fallback
- **Instagram Graph API** — Social post publishing (two-step publish flow)
- **Bandsintown API** (optional, Phase 4.8) — Outbound event sync

## Infrastructure

### Containerization
- **Docker Compose** — Four-service stack: `frontend` (Nuxt), `api` (Go), `db` (PostgreSQL 16), `storage` (MinIO)
- Dev overlay uses `profiles: [prod-only]` to exclude api/frontend from local dev startup

### Database
- **PostgreSQL 16** — Primary datastore
- **goose** — Migration tool, single-file `.sql` convention with `-- +goose Up` / `-- +goose Down` sections
- Migrations: 001 (settings), 002 (tracklists+tracks), 003 (social), 004 (epk)

### Object Storage
- **MinIO** — S3-compatible; stores cover art, tracklist images, EPK photos, exported PDFs

### Screenshot / Image Generation
- **Playwright (playwright-core)** — Headless Chromium for tracklist image screenshots
- Nitro plugin singleton (`server/plugins/playwright.ts`); graceful warn when browsers not installed

## Monorepo

- **Nx 22** — Task orchestration, build caching, affected computation
- **pnpm 9** — Package manager with workspace support
- `pnpm-workspace.yaml` lists all workspace packages

## Configuration

| File | Purpose |
|---|---|
| `apps/dj/nuxt.config.ts` | Nuxt config; Vite/Tailwind plugin; proxy conditional on `NUXT_PUBLIC_API_BASE` |
| `apps/dj/vitest.config.ts` | Vitest with jsdom, `~` and `@` aliases, coverage via v8 |
| `apps/dj-e2e/playwright.config.ts` | E2E config with Chromium, Firefox, WebKit |
| `nx.json` | Nx plugin definitions (TypeScript, ESLint, Vitest, Nuxt, Playwright) |
| `tsconfig.base.json` | Strict TS, ES2022, bundler module resolution |
| `eslint.config.mjs` | Flat ESLint config with Nx module boundary enforcement |

## TypeScript Compiler Options

- Target: ES2022
- Module: ESNext / Bundler resolution
- Strict mode: enabled
- `noUnusedLocals`, `noImplicitReturns`, `noFallthroughCasesInSwitch`: errors

---

*Stack analysis: 2026-04-27*
