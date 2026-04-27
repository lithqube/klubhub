# Codebase Structure

**Analysis Date:** 2026-04-27

## Directory Layout

```
klubhub-dj/
├── apps/
│   ├── dj/                              # Nuxt 4 frontend application
│   │   ├── app/                         # Frontend application code
│   │   │   ├── pages/                   # File-based routing pages
│   │   │   │   ├── index.vue            # Dashboard (/)
│   │   │   │   ├── tracklist.vue        # Tracklist generator (/tracklist)
│   │   │   │   ├── social.vue           # Social scheduler (/social)
│   │   │   │   ├── epk.vue              # EPK builder (/epk)
│   │   │   │   ├── gigs.vue             # Gig tracker (/gigs) — mock UI
│   │   │   │   └── finance.vue          # Finance tracker (/finance) — mock UI
│   │   │   ├── components/
│   │   │   │   ├── TheNav.vue           # Desktop sidebar (220px, lg:flex)
│   │   │   │   ├── TheMobileHeader.vue  # Mobile brand bar + 3-way theme (lg:hidden)
│   │   │   │   ├── TheBottomNav.vue     # Mobile bottom nav 5 items (lg:hidden)
│   │   │   │   ├── TheStatusBar.vue     # Desktop status bar (API latency, storage, sync)
│   │   │   │   ├── TrackcardPreview.vue # Tracklist image preview orchestrator
│   │   │   │   ├── trackcard/           # TrackcardPreview sub-components (6 files)
│   │   │   │   ├── tracklist/           # Feature components
│   │   │   │   │   ├── TracklistUploadZone.vue
│   │   │   │   │   ├── TracklistEditor.vue
│   │   │   │   │   ├── TracklistCustomizer.vue
│   │   │   │   │   ├── TracklistExporter.vue
│   │   │   │   │   └── TracklistHistory.vue
│   │   │   │   ├── social/              # Social scheduler components
│   │   │   │   │   ├── SocialPageHeader.vue
│   │   │   │   │   ├── SocialTabs.vue
│   │   │   │   │   ├── SocialPostCompose.vue
│   │   │   │   │   ├── SocialQueueGrid.vue
│   │   │   │   │   ├── SocialPostCard.vue
│   │   │   │   │   ├── SocialPostCardFailed.vue
│   │   │   │   │   ├── SocialCalendarView.vue
│   │   │   │   │   └── SocialConnectionBanner.vue
│   │   │   │   ├── epk/                 # EPK section components
│   │   │   │   │   ├── EpkBioSection.vue
│   │   │   │   │   ├── EpkPhotosSection.vue
│   │   │   │   │   ├── EpkTechRiderSection.vue
│   │   │   │   │   ├── EpkStagePlotSection.vue
│   │   │   │   │   ├── EpkGigHighlightsSection.vue
│   │   │   │   │   ├── EpkPressQuotesSection.vue
│   │   │   │   │   ├── EpkSocialLinksSection.vue
│   │   │   │   │   ├── EpkContactInfoSection.vue
│   │   │   │   │   └── EpkExportPanel.vue
│   │   │   │   └── ui/                  # shadcn-vue base components
│   │   │   ├── composables/
│   │   │   │   ├── useTheme.ts          # 3-way theme (dark|system|light)
│   │   │   │   ├── useTracklist.ts      # Tracklist API calls
│   │   │   │   ├── useEpkAutosave.ts    # Debounced EPK autosave
│   │   │   │   └── useSocialPostForm.ts # Social post form state
│   │   │   ├── stores/
│   │   │   │   ├── tracklist.ts         # useTracklistStore
│   │   │   │   ├── social.ts            # useSocialStore
│   │   │   │   ├── epk.ts               # useEpkStore
│   │   │   │   ├── settings.ts          # useSettingsStore
│   │   │   │   ├── ui.ts                # useUiStore
│   │   │   │   └── __tests__/           # Store unit tests
│   │   │   ├── types/
│   │   │   │   ├── tracklist.ts         # Tracklist, Track, ParseWarning
│   │   │   │   ├── social.ts            # SocialAccount, ScheduledPost, PostStatus
│   │   │   │   └── epk.ts               # EPKContent, EPKExport, PressQuote
│   │   │   ├── utils/
│   │   │   │   └── design-tokens.ts     # Image generation PRESETS (5 color schemes)
│   │   │   └── assets/css/
│   │   │       └── styles.css           # Tailwind @theme tokens + Kinetic HUD @layer components
│   │   ├── server/
│   │   │   ├── api/
│   │   │   │   ├── v1/                  # Mock data handlers (no NUXT_PUBLIC_API_BASE)
│   │   │   │   │   ├── settings.get.ts
│   │   │   │   │   ├── tracklists/
│   │   │   │   │   │   ├── index.get.ts
│   │   │   │   │   │   └── [id].get.ts
│   │   │   │   │   ├── social/
│   │   │   │   │   │   ├── posts.get.ts
│   │   │   │   │   │   ├── posts.post.ts
│   │   │   │   │   │   └── accounts.get.ts
│   │   │   │   │   └── epk/
│   │   │   │   │       ├── content.get.ts
│   │   │   │   │       └── content.put.ts
│   │   │   │   ├── greet.ts
│   │   │   │   └── screenshot/tracklist/[id].get.ts
│   │   │   └── plugins/
│   │   │       └── playwright.ts        # Playwright singleton (graceful fallback)
│   │   ├── public/                      # Static assets + variable fonts
│   │   ├── nuxt.config.ts               # Nuxt config
│   │   ├── vitest.config.ts
│   │   ├── components.json              # shadcn-vue config
│   │   └── tsconfig*.json
│   │
│   └── dj-e2e/                          # Playwright E2E tests
│       └── src/                         # Screenshot pipeline tests
│
├── api/                                 # Go backend
│   ├── cmd/api/
│   │   └── main.go                      # Entry point (chi router, worker goroutine, SIGTERM)
│   ├── internal/
│   │   ├── config/                      # Environment variable loading
│   │   ├── db/                          # pgxpool connection + embedded migrations
│   │   ├── storage/                     # MinIO client wrapper
│   │   ├── crypto/                      # Token encryption/decryption
│   │   ├── health/                      # GET /api/v1/health
│   │   ├── settings/                    # user_settings CRUD
│   │   ├── tracklist/                   # Tracklist feature (parser, repo, service, handler)
│   │   ├── social/                      # Social scheduler (repo, service, handler, worker, instagram client)
│   │   └── epk/                         # EPK builder (repo, service, handler, PDF generator)
│   └── migrations/                      # goose SQL files (001–004)
│
├── docs/                                # BRD v2, NFR, architecture docs
├── scripts/
│   ├── backup.sh
│   └── restore.sh
├── .planning/                           # GSD planning documents
│   ├── STATE.md                         # Current phase + decisions log
│   ├── ROADMAP.md                       # All phases with status
│   ├── PROJECT.md
│   ├── REQUIREMENTS.md
│   ├── codebase/                        # Architecture, stack, structure, conventions docs
│   ├── phases/                          # Per-phase PLAN + SUMMARY files
│   └── research/                        # Pre-project research docs
├── docker-compose.yml
├── .env.example
├── nx.json
├── package.json
├── pnpm-workspace.yaml
├── tsconfig.base.json
└── README.md
```

## Key File Locations

### Entry Points

| File | Purpose |
|---|---|
| `apps/dj/app/app.vue` | Root layout: `hud-bg` shell, sidebar, mobile header, error boundary |
| `apps/dj/app/pages/index.vue` | Dashboard page (`/`) — hero gig card, social queue, tracklists, finance |
| `apps/dj/nuxt.config.ts` | Nuxt config; proxy conditional on `NUXT_PUBLIC_API_BASE` |
| `api/cmd/api/main.go` | Go API entry point |

### Design System

| File | Purpose |
|---|---|
| `apps/dj/app/assets/css/styles.css` | All Tailwind `@theme` tokens + Kinetic HUD `@layer components` classes |
| `apps/dj/app/utils/design-tokens.ts` | Image-generation PRESETS (color schemes for TrackcardPreview) |
| `apps/dj/app/composables/useTheme.ts` | 3-way theme mode with localStorage persistence |

### State

| File | Purpose |
|---|---|
| `apps/dj/app/stores/tracklist.ts` | Upload state, parsed tracks, past tracklists |
| `apps/dj/app/stores/social.ts` | Posts, account, compose panel — API calls live here |
| `apps/dj/app/stores/epk.ts` | EPK content, photos, exports, save status |
| `apps/dj/app/stores/settings.ts` | DJ name, logo, preset, field visibility |
| `apps/dj/app/stores/ui.ts` | Step (upload/edit), loading flags, error strings |

### Mock Data (dev only)

| File | Endpoint |
|---|---|
| `apps/dj/server/api/v1/settings.get.ts` | `GET /api/v1/settings` |
| `apps/dj/server/api/v1/tracklists/index.get.ts` | `GET /api/v1/tracklists` |
| `apps/dj/server/api/v1/tracklists/[id].get.ts` | `GET /api/v1/tracklists/:id` |
| `apps/dj/server/api/v1/social/posts.get.ts` | `GET /api/v1/social/posts` |
| `apps/dj/server/api/v1/social/accounts.get.ts` | `GET /api/v1/social/accounts` |
| `apps/dj/server/api/v1/epk/content.get.ts` | `GET /api/v1/epk/content` |
| `apps/dj/server/api/v1/epk/content.put.ts` | `PUT /api/v1/epk/content` |

## Naming Conventions

**Files:**
- Page components: lowercase (e.g., `tracklist.vue`, `index.vue`)
- Feature components: PascalCase (e.g., `TracklistUploadZone.vue`, `SocialPostCard.vue`)
- Singleton app components: `The*` prefix (e.g., `TheNav.vue`, `TheMobileHeader.vue`)
- Composables: `use*` camelCase (e.g., `useTheme.ts`, `useEpkAutosave.ts`)
- Stores: `use*Store` (e.g., `useTracklistStore`, `useSocialStore`)
- API server routes: `[name].[method].ts` or `[name].ts` (e.g., `posts.get.ts`, `settings.get.ts`)
- Dynamic routes: `[param].get.ts` (e.g., `[id].get.ts`)

**Directories:**
- Feature components grouped under their feature name (e.g., `components/tracklist/`, `components/social/`, `components/epk/`)
- Sub-components of a complex component grouped in their parent's name folder (e.g., `trackcard/`)

## Where to Add New Code

**New page (route):**
- `apps/dj/app/pages/[feature].vue`

**New feature components:**
- `apps/dj/app/components/[feature]/[ComponentName].vue`

**New Pinia store:**
- `apps/dj/app/stores/[feature].ts` (setup function style, not options API)

**New mock API handler:**
- `apps/dj/server/api/v1/[feature]/[action].[method].ts`

**New Go feature:**
- `api/internal/[feature]/` — model.go, repository.go, service.go, handler.go
- Add migration: `api/migrations/00N_[feature].sql`

**New design tokens / utility classes:**
- `apps/dj/app/assets/css/styles.css` — inside `@layer components {}`

---

*Structure analysis: 2026-04-27*
