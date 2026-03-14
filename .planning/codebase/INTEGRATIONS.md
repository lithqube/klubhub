# External Integrations

**Analysis Date:** 2026-03-14

## APIs & External Services

**HTTP Requests:**
- Standard fetch API via h3 HTTP handler
  - SDK/Client: h3 (built-in to Nuxt)
  - Pattern: Defined in `apps/dj/server/api/` directory
  - Example: `apps/dj/server/api/greet.ts` uses h3's `defineEventHandler` and `getQuery`

**No Third-Party API SDKs Detected:**
- No Stripe, AWS, Google Cloud, Azure, Supabase, or other major cloud service integrations currently configured
- No REST/GraphQL client libraries (axios, got, node-fetch) in dependencies

## Data Storage

**Databases:**
- Not detected - No ORM or database client dependencies found (no Prisma, TypeORM, Sequelize, Mongoose, etc.)
- Current scope appears to be a frontend-driven application without persistent backend storage

**File Storage:**
- Local filesystem only - No S3, GCS, or cloud storage integration detected

**Caching:**
- None - No Redis, Memcached, or caching layer configured

## Authentication & Identity

**Auth Provider:**
- Not configured - No authentication framework integrated
- No JWT libraries, OAuth providers, or identity management detected in dependencies

**Implementation:**
- No current authentication implementation in codebase
- Ready for future auth integration via Nuxt middleware or h3 handlers

## Monitoring & Observability

**Error Tracking:**
- Not configured - No Sentry, Rollbar, or error tracking service integrated

**Logs:**
- Console logging only (no structured logging frameworks detected)
- Development approach via Nuxt devtools (`@nuxt/devtools 3.0.0`)

## CI/CD & Deployment

**Hosting:**
- Not specified - Ready for deployment to Node.js compatible platforms
- Supports standard Node.js HTTP hosting (Vercel, Netlify, traditional servers, etc.)

**CI Pipeline:**
- Not detected - No GitHub Actions, GitLab CI, Jenkins, or other CI/CD configuration files present
- Nx supports CI integration but not currently configured

## Environment Configuration

**Required env vars:**
- `BASE_URL` (optional) - For E2E testing, defaults to `http://localhost:4200`
- No other environment variables required for baseline operation

**Secrets location:**
- Not applicable - No external services requiring credentials
- Ready to support `.env` files via dotenv (dotenv commented out in Playwright config at `apps/dj-e2e/playwright.config.ts` line 12)

## Webhooks & Callbacks

**Incoming:**
- Not implemented - No webhook endpoints detected

**Outgoing:**
- Not implemented - No outbound webhook calls configured

## Development Integration Points

**Nuxt Devtools:**
- `@nuxt/devtools 3.0.0` enabled in `apps/dj/nuxt.config.ts`
- Provides real-time component inspection and debugging in development

**Nx Workspace Plugins:**
- ESLint plugin (`@nx/eslint`) for code quality
- Vitest plugin (`@nx/vitest`) for unit testing
- Playwright plugin (`@nx/playwright`) for E2E testing
- Nuxt plugin (`@nx/nuxt`) for framework integration
- TypeScript plugin (`@nx/js/typescript`) for type checking

## Third-Party Build Tools

**Vite Build System:**
- No Vite plugins for external services currently configured beyond Vue support
- Extensible via `vite.plugins` in Nuxt config

**Testing Infrastructure:**
- Unit tests: Vitest with jsdom environment (`apps/dj/vitest.config.ts`)
- E2E tests: Playwright with Chromium, Firefox, WebKit browsers (`apps/dj-e2e/playwright.config.ts`)
- No Cypress, TestCafé, or other E2E frameworks

---

*Integration audit: 2026-03-14*
