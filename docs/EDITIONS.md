# Editions and feature flags

KlubHub DJ is one codebase that ships as several editions. Everything
built so far is in the open-source edition. A few capabilities can be
switched on or off with configuration; those are the **switchable
features**. Each has its own default. A feature that must be opted into
(a paid or risky integration) is an **edition feature** and stays off
until someone turns it on. There are none yet.

## Editions

| Edition | Who runs it | Edition features |
|---|---|---|
| **Self-hosted open source** | You, from this repository or the published images | Everything built so far, including Resident Advisor import. |
| **Licensed** | A self-hosted deployment licensed for paid features | None defined yet. Planned: SoundCloud and Spotify integrations. |
| **SaaS** | Hosted by KlubHub | None defined yet. Planned: the hosted features in `docs/v2-saas-migration-plan.md`. |

The flags are a configuration switch. They do not enforce a licence and
they do not check a key. They let you turn a feature off, and would keep
an opt-in integration out of default installs.

## Switchable features

| Feature | Key | Edition | Default | API env var | App env var | What it controls |
|---|---|---|---|---|---|---|
| Resident Advisor import | `raImport` / `ra_import` | Open source | On. Off in the browser demo and the frontend-only mock server, which have no RA endpoints. | `FEATURE_RA_IMPORT` | `NUXT_PUBLIC_FEATURES_RA_IMPORT` | EPK artist import (bio, name, links), gig event import, and the RA link field in the EPK editor |

RA import is part of the open-source build and is only used when you ask for an import (the API never calls Resident Advisor otherwise). When it is turned off with `FEATURE_RA_IMPORT=false`:

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

## Turning a feature on or off

Both sides must agree for a feature to work. The API flag mounts the
routes and the app flag shows the UI.

**Production Compose** (`bash scripts/setup.sh prod`): add the flag to the
mode's `.env`. The compose file passes it to the API and to the embedded
Nuxt server. RA import is on without it; to turn it off:

```bash
FEATURE_RA_IMPORT=false
```

Then recreate the app container (`docker compose ... up -d app`) so it
picks up the new environment. The installer has `--no-ra-import` for the
same thing.

**Development** (`bash scripts/setup.sh dev` plus Nuxt on the host): the API
container follows `FEATURE_RA_IMPORT`; Nuxt on the host is on whenever it
proxies a real API. To hide the UI there, export
`NUXT_PUBLIC_FEATURES_RA_IMPORT=false` in the shell that runs
`pnpm nx serve @dev/dj`.

**Frontend-only mock mode** (`pnpm nx serve @dev/dj` without
`NUXT_PUBLIC_API_BASE`) and the **browser demo**: the mock server has no RA
endpoints, so the panels start hidden. Setting
`NUXT_PUBLIC_FEATURES_RA_IMPORT=true` there only shows UI that cannot work.

To check the API side, call `GET /api/v1/health`. Its `features` map lists
every switchable feature and whether it is on, for example
`"features": {"ra_import": true}`.

## How the mechanism works

| Layer | Where | Notes |
|---|---|---|
| API config | `api/internal/platform/config/config.go` → `Features` | One `bool` field per feature under the `FEATURE_` prefix. RA import is `default:"true"`; an opt-in feature would be `default:"false"`. `Features.Enabled()` gives the snake_case map used by `/health`. |
| API wiring | `api/cmd/api/main.go` → `wireRAImport` | Builds the integration's client and registers its routes only when the flag is on. Handlers take an optional client, and `nil` means the routes are not mounted. |
| App config | `apps/dj/nuxt.config.ts` → `runtimeConfig.public.features` | RA import defaults to on when a real API serves the app, off in the demo and the mock server. Nuxt overrides it at runtime from `NUXT_PUBLIC_FEATURES_<KEY>`. |
| App registry | `apps/dj/app/utils/features.ts` → `FEATURES` | The one list of edition features: key, edition, description, env var names. Values other than `true` or `"true"` count as off, so `NUXT_PUBLIC_FEATURES_RA_IMPORT=false` turns it off. |
| App access | `apps/dj/app/composables/useFeatures.ts` | `useFeatures().isEnabled('raImport')` returns read-only flags. It fails closed outside a Nuxt context. |

### Adding a new switchable feature

1. API: add a field to `config.Features` (`default:"false"` for an opt-in feature, `"true"` for one that is part of the open-source build) and add it
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
