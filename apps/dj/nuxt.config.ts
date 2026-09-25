import tailwindcss from '@tailwindcss/vite';
import { defineNuxtConfig } from 'nuxt/config';

// https://nuxt.com/docs/api/configuration/nuxt-config

// Plan D — frontend mock exclusion.
//
// Behavior:
//   - Dev / staging:    /api/v1/* is served by the in-tree mock handlers
//                        under apps/dj/server/api/v1/. This is the fast
//                        frontend-only loop described in the README.
//   - Production:       /api/v1/* is served exclusively by the Nitro
//                        proxy in front of the Go API. The mock
//                        handlers are physically excluded from the
//                        built bundle.
//
// Production builds are launched by scripts/build-prod.mjs, which stages the
// mocks before Nuxt starts. Hooks/config evaluation are too late for route
// discovery. This guard remains as defense in depth for direct Nuxt builds.
// Browser-only demo build (docs/DEMO.md): `NUXT_DEMO=1 nuxi generate`. A
// static SPA with an in-browser API (app/demo) — no Go API, no Nitro server
// routes, no proxy. NUXT_APP_BASE_URL overrides the /demo/ base locally.
const isDemo = process.env.NUXT_DEMO === '1' || process.env.NUXT_DEMO === 'true'
const demoBaseURL = process.env.NUXT_APP_BASE_URL || '/demo/'
// App pages prerendered as SPA shells so deep links work on static hosting
// (/render/* is the screenshot target and stays out of the demo).
const DEMO_ROUTES = ['/', '/tracklist', '/social', '/epk', '/gigs', '/finance']

if (process.env.NODE_ENV === 'production' && !isDemo) {
  if (!process.env.NUXT_PUBLIC_API_BASE) {
    throw new Error(
      'Refusing to build a production Nuxt image without NUXT_PUBLIC_API_BASE. ' +
        'Set the env var to the Go API base (e.g. http://api:8080) so the ' +
        'Nitro proxy routes /api/v1/* to the real backend.',
    )
  }

}

export default defineNuxtConfig({
  workspaceDir: '../../',
  modules: ['@pinia/nuxt', 'shadcn-nuxt'],
  shadcn: {
    prefix: '',
    componentDir: './app/components/ui',
  },
  devtools: { enabled: true },
  devServer: {
    host: 'localhost',
    port: 4200,
  },
  typescript: {
    // Type checking handled separately via `pnpm nx typecheck @dev/dj`
    // Keeping typeCheck off here avoids pre-existing errors blocking the build
    typeCheck: false,
    tsConfig: {
      extends: '../../../tsconfig.base.json', // Nuxt copies this string as-is to the `./.nuxt/tsconfig.json`, therefore it needs to be relative to that directory
      // The finance mock routes are type-checked through the generated
      // nitro route types; their in-memory store is a "-" prefixed
      // (non-route) module and its rules live in shared/finance-mock (also
      // used by the browser demo), so the composite project lists both.
      include: ['../server/api/v1/finance/-mockDb.ts', '../shared/**/*.ts'],
    },
  },
  imports: {
    autoImport: true,
  },
  css: ['~/assets/css/styles.css'],
  vite: {
    plugins: [tailwindcss()],
    server: {
      // The dockerized Go API calls back to the host Nuxt dev server for
      // tracklist screenshots via NUXT_INTERNAL_URL
      // (http://host.docker.internal:4200). Vite's host check answers 403
      // to unknown Host headers, so that one name must be allowed.
      allowedHosts: ['host.docker.internal'],
    },
  },

  // Proxy /api/v1/* to the Go backend in production (and dev when
  // NUXT_PUBLIC_API_BASE is set). Without it, dev Nitro serves the
  // mock handlers from server/api/v1/.
  routeRules: process.env.NUXT_PUBLIC_API_BASE && !isDemo
    ? {
        '/api/v1/**': {
          proxy: process.env.NUXT_PUBLIC_API_BASE + '/api/v1/**',
        },
      }
    : {},
  nitro: isDemo
    ? {
        prerender: { crawlLinks: false, routes: DEMO_ROUTES },
        output: { dir: '.output-demo' },
      }
    : process.env.NUXT_PUBLIC_API_BASE
    ? {
        devProxy: {
          '/api/v1': {
            // devProxy strips the matched '/api/v1' prefix before
            // forwarding, so the target must carry it or the Go API
            // receives '/gigs' instead of '/api/v1/gigs' and 404s.
            target: process.env.NUXT_PUBLIC_API_BASE + '/api/v1',
            changeOrigin: true,
          },
        },
      }
    : {},
  runtimeConfig: {
    public: {
      // Plan B.5: removed icalSecret from public runtime config — anything
      // here ships in the client bundle. Calendar/PDF authentication is
      // now exclusively server-side via process.env.ICAL_SECRET read by
      // the Nitro handlers under apps/dj/server/api/v1/gigs/.
      // Plan B.9: no other secret-bearing fields are allowed here.

      // Edition features (licensed / SaaS). All off by default so the
      // self-hosted open-source build and the public demo never show them.
      // Override per key at runtime, e.g. NUXT_PUBLIC_FEATURES_RA_IMPORT=true.
      // Registry: app/utils/features.ts · docs/EDITIONS.md
      features: {
        raImport: false,
      },
      // Browser-only demo build: app/plugins/00.demo.client.ts serves the
      // API in the browser. Only the NUXT_DEMO build sets this.
      demo: isDemo,
    },
  },
  ...(isDemo
    ? {
        ssr: false,
        // Separate build dirs so a demo build never clobbers .nuxt/.output.
        buildDir: '.nuxt-demo',
        // No server in the demo: an empty server dir keeps the dev mocks,
        // the screenshot route and the Playwright plugin out of the build.
        serverDir: '.demo-no-server',
        app: {
          baseURL: demoBaseURL,
          head: {
            meta: [{ name: 'robots', content: 'noindex' }],
            link: [{ rel: 'icon', href: `${demoBaseURL}favicon.ico` }],
          },
        },
      }
    : {}),
}) as any;
