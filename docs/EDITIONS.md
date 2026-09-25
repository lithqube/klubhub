# Editions and feature flags

KlubHub DJ is one codebase that ships as several editions. Most features
are in every edition. The few that are not are called **edition features**.
They are off by default and are switched on with configuration.

## Editions

| Edition | Who runs it | Edition features |
|---|---|---|
| **Self-hosted open source** | You, from this repository or the published images | All off. This is the default for every install and for the public demo. |
| **Licensed** | A self-hosted deployment licensed for the paid features | Switched on per feature by the operator (see below). |
| **SaaS** | Hosted by KlubHub | Switched on by the hosted deployment's configuration. |

The flags are a configuration switch. They do not enforce a licence and
they do not check a key. They keep paid integrations out of default
installs and let a licensed or hosted deployment turn them on.

## Gated features

| Feature | Key | Edition | API env var | App env var | What it controls |
|---|---|---|---|---|---|
| Resident Advisor import | `raImport` / `ra_import` | Licensed, SaaS | `FEATURE_RA_IMPORT` | `NUXT_PUBLIC_FEATURES_RA_IMPORT` | EPK artist import (bio, name, links), gig event import, and the RA link field in the EPK editor |

When RA import is off:

- The API does not create an RA client and does not mount
  `POST /api/v1/epk/import-ra`, `POST /api/v1/gigs/import-ra` or
  `GET /api/v1/gigs/info/{slug}`. Those paths return 404 and the API
  makes no requests to Resident Advisor.
- The UI hides the RA import panels on the EPK and Gigs pages, the RA
  link field, and all RA wording. The RA store's actions do nothing and
  never call the API.
- Data you already have is kept: imported gigs, EPK text, and any RA link
  saved in settings stay as they are. A saved RA link is hidden in the
  editor but not deleted.

## Turning a feature on

Both sides must be on for a feature to work. The API flag mounts the
routes and the app flag shows the UI.

**Production Compose** (`bash scripts/setup.sh prod`): add the flag to the
mode's `.env`. The compose file passes it to the API and to the embedded
Nuxt server.

```bash
FEATURE_RA_IMPORT=true
```

Then recreate the app container (`docker compose ... up -d app`) so it
picks up the new environment.

**Development** (`bash scripts/setup.sh dev` plus Nuxt on the host): set
`FEATURE_RA_IMPORT=true` for the API container and also export
`NUXT_PUBLIC_FEATURES_RA_IMPORT=true` in the shell that runs
`pnpm nx serve @dev/dj`.

**Frontend-only mock mode** (`pnpm nx serve @dev/dj` without
`NUXT_PUBLIC_API_BASE`): the mock server has no RA endpoints, so enabling
the flag only shows the UI.

To check the API side, call `GET /api/v1/health`. Its `features` map lists
every edition feature and whether it is on, for example
`"features": {"ra_import": false}`.

## How the mechanism works

| Layer | Where | Notes |
|---|---|---|
| API config | `api/internal/platform/config/config.go` → `Features` | One `bool` field per feature under the `FEATURE_` prefix, `default:"false"`. `Features.Enabled()` gives the snake_case map used by `/health`. |
| API wiring | `api/cmd/api/main.go` → `wireRAImport` | Builds the integration's client and registers its routes only when the flag is on. Handlers take an optional client, and `nil` means the routes are not mounted. |
| App config | `apps/dj/nuxt.config.ts` → `runtimeConfig.public.features` | Defaults to `false`. Nuxt overrides it at runtime from `NUXT_PUBLIC_FEATURES_<KEY>`. |
| App registry | `apps/dj/app/utils/features.ts` → `FEATURES` | The one list of edition features: key, edition, description, env var names. Values other than `true` or `"true"` count as off. |
| App access | `apps/dj/app/composables/useFeatures.ts` | `useFeatures().isEnabled('raImport')` returns read-only flags. It fails closed outside a Nuxt context. |

### Adding a new edition feature

1. API: add a field to `config.Features` with `default:"false"` and add it
   to `Features.Enabled()`. Gate the construction and route mounting in
   `main.go`, and make sure the disabled path makes no outbound calls.
2. App: add an entry to `FEATURES` in `app/utils/features.ts` and
   `<key>: false` under `runtimeConfig.public.features`. Gate every entry
   point with `useFeatures().isEnabled('<key>')`, including copy,
   placeholders, empty states and toasts.
3. Deployment: forward the env vars in `docker-compose.prod.yml` (default
   `false`) and document them in `.env.example`.
4. Tests: default off, on from env, routes return 404 when off, UI
   renders nothing when off.
5. Docs: add a row to the table above and to
   [`CONFIGURATION.md`](./CONFIGURATION.md).
