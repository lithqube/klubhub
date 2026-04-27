# Architecture

**Analysis Date:** 2026-04-27

## Pattern Overview

**Overall:** Full-stack monorepo — Nuxt 4 / Vue 3 frontend (Nx workspace, pnpm) + Go API backend + PostgreSQL + MinIO. Frontend proxies `/api/v1/*` to Go API when `NUXT_PUBLIC_API_BASE` is set; falls back to Nitro mock handlers in dev mode without the env var.

**Key Characteristics:**
- Monorepo: pnpm workspaces + Nx orchestration
- Frontend: Nuxt 4 SSR + Vue 3 Composition API + Pinia state management
- Backend: Go with chi router, three-layer architecture (handler → service → repository)
- Design system: Tailwind CSS v4 + "Kinetic HUD" custom component classes
- All API data flows through Pinia stores — components never call `$fetch` directly (except stores and composables)

## Layers

### Frontend Layers

**Presentation Layer (Pages)**
- Location: `apps/dj/app/pages/`
- Pure orchestrators: import stores via `storeToRefs()`, pass data to feature components
- Never call `$fetch` directly — all API interaction through stores
- Routes: `/` (Dashboard), `/tracklist`, `/social`, `/epk`, `/gigs`, `/finance`

**Feature Component Layer**
- Location: `apps/dj/app/components/[feature]/`
- Receive data as props, emit events upward
- Access stores only when needed for deep integration (e.g., auto-save composables)

**Shell Components (Singleton)**
- `TheNav.vue` — Desktop sidebar (220px, hidden on mobile)
- `TheMobileHeader.vue` — Mobile brand bar + 3-way theme toggle (hidden on desktop)
- `TheBottomNav.vue` — Mobile bottom navigation (hidden on desktop)
- `TheStatusBar.vue` — Desktop status bar (API latency, storage status)

**State Layer (Pinia Stores)**
- Location: `apps/dj/app/stores/`
- All stores use setup function (Composition API) style — not options API
- `storeToRefs()` required for reactive destructuring in components
- Store actions make all `$fetch` API calls

**Composable Layer**
- Location: `apps/dj/app/composables/`
- `useTheme` — Global theme state (Nuxt `useState` for SSR compatibility)
- `useTracklist` — Raw API functions (not a store — stateless wrappers around `$fetch`)
- `useEpkAutosave` — Debounced watcher that calls `epkStore.updateContent()`
- `useSocialPostForm` — Local form state for compose panel

**Nitro Server Layer**
- Location: `apps/dj/server/`
- `server/plugins/playwright.ts` — Playwright browser singleton for screenshot generation
- `server/api/screenshot/` — Screenshot endpoint (proxies to Go API for full generation)
- `server/api/v1/` — Mock data handlers (active when `NUXT_PUBLIC_API_BASE` is unset)

### Backend Layers (Go API)

**Handler Layer** (`internal/[feature]/handler.go`)
- Validates HTTP requests using h3-style input parsing
- Calls service interface (`serviceIface`) — never calls repo directly
- Converts domain errors to HTTP status codes

**Service Layer** (`internal/[feature]/service.go`)
- Business logic, authorization checks, cross-feature orchestration
- Calls repository interface (`repoIface`) — decoupled from pgx
- Also calls `storageIface` for MinIO operations
- Unit-tested with hand-written mock implementations (no testcontainers, no pgxmock)

**Repository Layer** (`internal/[feature]/repository.go`)
- All SQL queries (pgx v5)
- Returns domain models, not raw rows
- Accepts `pgxpool.Pool` — connection management handled at startup

**Worker** (`internal/social/worker.go`)
- Goroutine started in `main.go` alongside the HTTP server
- Uses `signal.NotifyContext` for graceful SIGTERM shutdown
- Thin interfaces (`workerRepoIface`, `instagramIface`, `storageIface`) for full-mock unit tests

## Data Flow

### Frontend → API (with backend)

```
User action
  → Vue component emits / calls store action
  → Pinia store action ($fetch '/api/v1/...')
  → Nuxt Nitro (proxies to NUXT_PUBLIC_API_BASE + /api/v1/...)
  → Go chi router
  → Handler → Service → Repository → PostgreSQL
  → JSON response
  → Pinia store updates reactive state
  → Vue component re-renders
```

### Frontend → Mock (no backend)

```
User action
  → Pinia store action ($fetch '/api/v1/...')
  → Nuxt Nitro (no proxy rule active)
  → Nitro server/api/v1/[handler].ts
  → Returns mock data JSON
  → Pinia store updates reactive state
  → Vue component re-renders
```

### Theme Flow

```
User clicks theme toggle (TheNav / TheMobileHeader)
  → setTheme(mode: 'dark' | 'system' | 'light')
  → localStorage.setItem('klubhub-theme', mode)
  → resolveMode() → 'dark' | 'light'
  → document.documentElement.setAttribute('data-theme', resolved)
  → CSS [data-theme="light"] overrides activate / deactivate
  → useHead computed htmlAttrs keeps data-theme in sync on navigation
```

### Image Generation Flow

```
TracklistExporter.vue → generateImage(tracklistId, format)
  → POST /api/v1/tracklists/:id/generate-image
  → Go API: renders TrackcardPreview HTML via Playwright screenshot endpoint
  → Screenshot saved to MinIO
  → Returns signed URL
  → TracklistExporter shows preview image + download/schedule buttons
```

## API Contract

All API routes are under `/api/v1/`. In production, Nuxt proxies to the Go backend. In dev without `NUXT_PUBLIC_API_BASE`, Nitro mock handlers serve identical response shapes.

| Route | Method | Feature |
|---|---|---|
| `/api/v1/health` | GET | Infrastructure health |
| `/api/v1/settings` | GET, PUT | User settings |
| `/api/v1/tracklists` | GET, POST | Tracklist list + upload |
| `/api/v1/tracklists/:id` | GET, DELETE | Tracklist detail + delete |
| `/api/v1/tracklists/:id/tracks/:tid` | PUT | Track inline edit |
| `/api/v1/tracklists/:id/tracks/:tid/artwork` | PUT | Manual artwork upload |
| `/api/v1/tracklists/:id/generate-image` | POST | Screenshot generation |
| `/api/v1/social/accounts` | GET, DELETE | Instagram account |
| `/api/v1/social/posts` | GET, POST | Post list + create |
| `/api/v1/social/posts/:id` | PUT, DELETE | Post edit + delete |
| `/api/v1/social/posts/:id/retry` | POST | Manual retry |
| `/api/v1/epk/content` | GET, PUT | EPK content upsert |
| `/api/v1/epk/photos` | POST | Photo upload |
| `/api/v1/epk/photos/:path` | DELETE | Photo delete |
| `/api/v1/epk/export` | POST | PDF generation |
| `/api/v1/epk/exports` | GET | Export history |
| `/api/v1/epk/exports/:id` | DELETE | Export delete |

## Key Design Decisions

**Proxy conditional on env var:** `nuxt.config.ts` `routeRules` and `nitro.devProxy` only activate when `NUXT_PUBLIC_API_BASE` is set. Without it, Nitro serves mock handlers — no hanging requests in dev.

**Mock data layer:** `apps/dj/server/api/v1/` provides full mock responses for every API route used by the frontend stores. Enables complete frontend development without any backend running.

**Pinia as the API boundary:** All `$fetch` calls live in store actions or composable functions — never in page or component `<script setup>`. Components only read reactive state from `storeToRefs()`.

**Three-layer Go backend:** Handler → Service → Repository. Handler calls `serviceIface`, Service calls `repoIface`. Enables full unit testing without testcontainers — just hand-written mock structs implementing the interfaces.

**Singleton EPK row:** EPK content is a single DB row per user, enforced with `CREATE UNIQUE INDEX ON epk_content ((true))`. No ID-based lookups needed — all EPK operations are upserts.

**Playwright as Nitro plugin singleton:** Browser instance is created once at startup, shared across all screenshot requests. Graceful error catch if browsers aren't installed — warning logged, `_playwright` set to `null`.

## Error Handling

**Frontend:**
- Store actions catch `$fetch` errors and set `uiStore.uploadError` or `uiStore.pastTracklistsError` strings
- `NuxtErrorBoundary` in `app.vue` catches unhandled component errors → shows HUD-styled error panel
- Toaster component shows toast notifications for transient errors

**Backend:**
- Handler layer maps domain errors to HTTP status codes (400/404/409/500)
- Service layer returns typed errors; handler uses `errors.Is()` / `errors.As()` for classification
- Worker swallows `RateLimitError` (returns nil to stop tick loop) — logs but doesn't crash

## Cross-Cutting Concerns

**Authentication:** Not implemented in v1 OSS (single-user self-hosted). OAuth tokens for Instagram stored encrypted in PostgreSQL using `TOKEN_ENCRYPTION_KEY`.

**Logging:** Go API uses structured JSON logging. Nuxt uses browser console. No log aggregation in v1.

**Validation:** Go API validates all inputs at handler layer. Frontend validates file types and sizes client-side before upload.

**Type Safety:** TypeScript strict mode throughout. Go is statically typed. Pinia stores use explicit TypeScript generics on all `ref<T>()` declarations.

---

*Architecture analysis: 2026-04-27*
