import { fileURLToPath } from 'node:url'
import { defineNuxtConfig } from 'nuxt/config'

// KlubHub Promoter — admin UI for promoters, collectives and organisers.
//
// Same frontend contract as KlubHub DJ:
//   - Dev without NUXT_PUBLIC_API_BASE: /api/v1/* is served by the mock
//     handlers under server/api/v1/ (frontend-only loop).
//   - Production: /api/v1/* is proxied to the Go promoter API; a build
//     without NUXT_PUBLIC_API_BASE is refused.
//
// Self-hosted Promoter is a private, admin-only tool (plan D2): public
// event pages, RSVP and submission links are a SaaS surface.
if (process.env.NODE_ENV === 'production' && !process.env.NUXT_PUBLIC_API_BASE) {
  throw new Error(
    'Refusing to build a production Nuxt image without NUXT_PUBLIC_API_BASE. ' +
      'Set it to the Go promoter API base (e.g. http://127.0.0.1:8081).',
  )
}

const isProd = process.env.NODE_ENV === 'production'

export default defineNuxtConfig({
  // Kinetic HUD design system (absolute file path; see apps/dj/nuxt.config.ts).
  extends: [fileURLToPath(new URL('../../libs/ui/nuxt.config.ts', import.meta.url))],
  workspaceDir: '../../',
  modules: ['@pinia/nuxt', 'nuxt-security', 'nuxt-nats'],
  devtools: { enabled: true },
  devServer: {
    host: 'localhost',
    // 4200 is KlubHub DJ, 4300 the static site preview.
    port: 4400,
  },
  typescript: {
    typeCheck: false,
    tsConfig: {
      extends: '../../../tsconfig.base.json',
      // The base is tuned for Nx composite libraries (declaration emit). An
      // app is only type-checked, never emitted, and declaration emit makes
      // inferred types from pnpm-nested packages unnameable (TS2742).
      compilerOptions: {
        composite: false,
        declarationMap: false,
        emitDeclarationOnly: false,
        noEmit: true,
      },
    },
  },
  css: ['~/assets/css/styles.css'],

  // Security headers and rate limits (pattern from OpenSchild's config).
  // CSP keeps nuxt-security's defaults: per-request nonce on inline scripts
  // (the theme bootstrap), 'unsafe-inline' only on style-src for Vite/Tailwind.
  security: {
    headers: {
      xFrameOptions: isProd ? 'DENY' : 'SAMEORIGIN',
      xContentTypeOptions: 'nosniff',
      referrerPolicy: 'strict-origin-when-cross-origin',
      xXSSProtection: '0',
    },
    rateLimiter: isProd ? { tokensPerInterval: 150, interval: 300000, headers: true } : false,
  },

  // nuxt-nats (the Nuxt server only consumes; the Go API owns publishing
  // through its outbox). Servers and credentials come from NUXT_NATS_*
  // env vars at runtime. The module's health endpoint reports JetStream
  // account stats, so it stays off; health is aggregated by the Go API.
  nats: {
    servers: ['nats://localhost:4222'],
    health: { enabled: false },
  },

  routeRules: {
    // Auth routes (added with platform/auth): tighter limit per client.
    '/api/v1/auth/**': {
      security: { rateLimiter: isProd ? { tokensPerInterval: 10, interval: 60000 } : false },
    },
    ...(process.env.NUXT_PUBLIC_API_BASE
      ? { '/api/v1/**': { proxy: process.env.NUXT_PUBLIC_API_BASE + '/api/v1/**' } }
      : {}),
  },
  nitro: process.env.NUXT_PUBLIC_API_BASE
    ? {
        devProxy: {
          '/api/v1': {
            // devProxy strips the matched prefix; the target must carry it.
            target: process.env.NUXT_PUBLIC_API_BASE + '/api/v1',
            changeOrigin: true,
          },
        },
      }
    : {},
  runtimeConfig: {
    // Server-only. Nothing secret-bearing may go under `public`.
    public: {},
  },
})
