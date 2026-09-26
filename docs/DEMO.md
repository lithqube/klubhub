# Browser-only demo

A demo of KlubHub DJ runs at **https://klubhub.io/demo/**. It is the real Nuxt
app built as a static single-page app. An in-browser API replaces the Go
backend, so there is no server, no account, and no network traffic beyond the
static files. Everything a visitor changes is saved in their own browser
(`localStorage`). The **Reset demo** button in the banner restores the sample
data.

All sample data is fictional. The DJ is "Sam Example", the venues are "Club
Alpha" to "Zeta Garden", email addresses use `example.*` domains, and tax IDs
are all zeros.

## How it works

| Piece | Where | What it does |
|---|---|---|
| Build flag | `apps/dj/nuxt.config.ts` | `NUXT_DEMO=1` sets `ssr: false` and `app.baseURL: '/demo/'` (override with `NUXT_APP_BASE_URL`). It adds `<meta name="robots" content="noindex">` and `runtimeConfig.public.demo = true`, and prerenders `/`, `/tracklist`, `/social`, `/epk`, `/gigs` and `/finance` as SPA shells (plus `200.html`/`404.html`). It uses an empty `serverDir`, so no Nitro routes, dev mocks or Playwright plugin are bundled, skips the production `NUXT_PUBLIC_API_BASE` guard and the proxy, and builds into `.nuxt-demo`/`.output-demo`. Edition features stay at their defaults (RA import off). |
| Plugin | `apps/dj/app/plugins/00.demo.client.ts` | Runs first (`enforce: 'pre'`) and does nothing unless `runtimeConfig.public.demo` is set. In the demo it lazy-loads `app/demo` and installs the fetch shim, so normal builds never download the demo code. |
| Fetch shim | `apps/dj/app/demo/fetch.ts` | Wraps `globalThis.fetch`. Same-origin requests to `<baseURL>api/v1/*` are answered in-process. Everything else (assets, fonts) passes through. The stores' `$fetch` calls go through it unchanged. |
| Router and handlers | `apps/dj/app/demo/router.ts`, `handlers/*.ts` | Method + path patterns with one module per domain: settings, tracklists, social, epk, gigs (with venues and contacts), finance. Status codes and error bodies match the Go API: `{ "error": "..." }` in general, and `{ error, message, problems? }` for finance (see [`INVOICING.md`](./INVOICING.md)). |
| Finance rules | `apps/dj/shared/finance-mock/` | The invoicing contract (tax suggestion, numbering at issue, issue checks, credit notes, corrections, payments, balances) as pure code. The Nitro dev mocks under `apps/dj/server/api/v1/finance/` and the demo both use it, so there is one copy of the rules. |
| Persistence | `apps/dj/app/demo/state.ts` | One JSON document under the `klubhub-demo:v1` key. It is seeded on the first visit. If storage is blocked or full, the demo keeps working in memory. Bump the key's version when the state shape changes incompatibly. |
| Images | `apps/dj/app/utils/apiAssetUrl.ts` | `<img>` tags that point at the API (post images, storage proxy) go through `apiAssetUrl()`. The demo registers a resolver that returns a local data URL. Outside the demo, the path is returned unchanged. |
| Banner | `apps/dj/app/components/TheDemoBanner.vue` | Reads "DEMO · Your changes stay in this browser" and holds the two-step **Reset demo** button and links to klubhub.io and GitHub. |

## What is simulated

| Area | In the demo |
|---|---|
| Tracklists | Uploads are parsed in the browser. The parser reads rekordbox XML, tab- or comma-separated exports with a header row, and plain `Artist - Title` lines. **Use a sample file** loads a bundled export. Artwork lookups return generated covers, with no calls to Spotify, Discogs or MusicBrainz. Exported images are generated SVG cards, not the Playwright PNG render. |
| Social | A fictional Instagram account, `@sam.example`, is connected. Scheduled posts become "published" once their time passes, and **Retry** publishes a failed post at once. Nothing is sent to Instagram. |
| EPK | Content autosaves. Photos are stored as data URLs, capped at 750 KB each and about 3 MB in total. PDF export produces a simple text PDF made in the browser. The full app renders a designed PDF on the server. |
| Gigs | Full CRUD, links to venues, contacts and tracklists, and filters. The booking PDF is a generated text PDF. **Export iCal** downloads an `.ics` file, because a live feed URL needs a server. |
| Finance | Every invoicing rule from the shared core applies: drafts from gigs, EU reverse charge, US invoices with withholding, issuing, payments, credit notes and corrections. The "Earnings" panel shows the same empty state as a production build. |
| Not included | Resident Advisor import (licensed feature, off), email sending, and multi-user access. |

## Build and preview locally

```sh
# Build into dist/demo (base URL /demo/)
pnpm nx run @dev/dj:build-demo

# Serve it under /demo/ like GitHub Pages does
mkdir -p /tmp/klubhub-pages && rm -rf /tmp/klubhub-pages/demo && cp -R dist/demo /tmp/klubhub-pages/demo
python3 -m http.server 8090 --directory /tmp/klubhub-pages --bind 127.0.0.1
# open http://127.0.0.1:8090/demo/
```

To serve from a server root instead, build with
`NUXT_APP_BASE_URL=/ pnpm nx run @dev/dj:build-demo` and serve `dist/demo`
directly. `DEMO_OUT_DIR` changes the output folder.

A normal dev server (`pnpm nx serve @dev/dj`) and production builds are
unaffected: without `NUXT_DEMO` the plugin is a no-op and the demo code is
never loaded.

## Tests

```sh
pnpm nx test @dev/dj                      # includes tests/demo-*.test.ts and tests/finance-mocks.test.ts
pnpm exec playwright install chromium     # once
pnpm nx run @dev/dj:test-demo             # browser smoke test (apps/dj/tests-demo/smoke.test.mjs)
```

The smoke test serves `dist/demo` under `/demo/` from a local static server
and visits every page. It fails on any console error or on any request that
leaves the local server. It also creates a draft invoice from a played gig and
checks that **Reset demo** restores the seed data. It builds the demo first if
`dist/demo` is missing (`DEMO_REBUILD=1` forces a rebuild).

## Publishing

`.github/workflows/pages.yml` has a `demo` job. It installs the pnpm
workspace, runs the demo unit tests, and builds the demo. The site job then
copies the result to `dist/site/demo/` before uploading the Pages artifact.
The workflow runs on changes to `apps/dj/**` so the demo tracks the app. See
[`github-pages.md`](./github-pages.md).

GitHub Pages serves only the site's root `404.html`, so an unknown path under
`/demo/` does not fall back to the app. The six app pages each have their own
`index.html`, so direct links to them work.

## Updating the seed data

The seed lives in `apps/dj/app/demo/seed/`: `tracklists.ts`, `gigs.ts` and
`content.ts` (settings, EPK, social). Finance gigs, customers and invoices
come from `apps/dj/shared/finance-mock/seed.ts`. Dates are offsets from the
visitor's "today", so upcoming gigs stay upcoming.

Rules for seed data:

- Keep it obviously fictional. Do not use real people, venues, festivals,
  clubs, labels, companies, brands or street addresses. Use `example.com` or
  `example.test` style domains and zero-padded IDs. A unit test checks that
  every email address uses an example domain.
- Returning visitors keep their stored copy. If they must see new seed data,
  bump `STORAGE_KEY` in `state.ts` (for example to `klubhub-demo:v2`).
- Run `pnpm nx test @dev/dj` and the smoke test afterwards.

When the app calls a new endpoint, add a handler to the matching module in
`apps/dj/app/demo/handlers/`. Unknown `/api/v1` routes answer 404 in the demo.
