# Codebase Concerns

**Analysis Date:** 2026-03-14

## Tech Debt

**Incomplete Application Skeleton:**
- Issue: The Nuxt 3 application is a minimal scaffold with only template components and no substantial business logic implementation
- Files: `apps/dj/app/app.vue`, `apps/dj/app/pages/index.vue`, `apps/dj/app/components/NxWelcome.vue` (881 lines), `apps/dj/server/api/greet.ts` (10 lines)
- Impact: No actual KlubHub DJ features are implemented yet despite comprehensive ARCHITECTURE.md, BRD, and NFR documentation. All planned functionality (Spotify integration, image generation, social media posting, gig management) is missing
- Fix approach: Follow ARCHITECTURE.md guidance to implement backend Go services and frontend components incrementally across planned phases

**Monolithic NxWelcome Component:**
- Issue: Single component file at `apps/dj/app/components/NxWelcome.vue` is 881 lines with hardcoded Nx branding, learning links, and command documentation
- Files: `apps/dj/app/components/NxWelcome.vue`
- Impact: This template component should not ship to production; blocks legitimate home page development. Requires complete replacement with actual KlubHub UI
- Fix approach: Remove NxWelcome.vue; implement feature-specific components as per design system in later phases

**Minimal Server API Surface:**
- Issue: Only one trivial API endpoint exists: `apps/dj/server/api/greet.ts` which echoes a query parameter
- Files: `apps/dj/server/api/greet.ts`
- Impact: No API infrastructure for core features. Lacks error handling, validation, authentication middleware (planned for v3), and integration with backend services
- Fix approach: Implement proper API layer with Go backend integration, error handling, and middleware as per ARCHITECTURE.md

## Known Bugs

**E2E Test Assumption Mismatch:**
- Symptoms: E2E test expects `<h1>` containing "Welcome" text; actual home page contains only NxWelcome component template
- Files: `apps/dj-e2e/src/example.spec.ts` (line 7)
- Trigger: Running Playwright E2E tests will fail because the test assertion (`expect(await page.locator('h1').innerText()).toContain('Welcome')`) assumes specific DOM structure from the scaffold that doesn't match actual rendered content
- Workaround: Update test once real home page is implemented

## Security Considerations

**No Authentication/Authorization Implemented:**
- Risk: API endpoint `apps/dj/server/api/greet.ts` accepts and echoes user input without validation. Current v1 design (per NFR v1.0) explicitly has "no authentication" but this must be addressed before multi-user support (planned for v3)
- Files: `apps/dj/server/api/greet.ts`, all planned API endpoints
- Current mitigation: Documentation (ARCHITECTURE.md, NFR v1.0) explicitly declares single-user scope. No multi-user features implemented
- Recommendations:
  - Implement input validation on all API endpoints before production
  - Plan authentication/authorization layer for v3 multi-user support per ARCHITECTURE.md §10 "v3 Multi-User Guardrails"
  - Add CORS and CSRF protection in Nuxt middleware

**Missing Input Validation:**
- Risk: `getQuery()` in `apps/dj/server/api/greet.ts` accepts arbitrary input; real endpoints (image generation, Spotify parsing) will need schema validation
- Files: `apps/dj/server/api/greet.ts`
- Current mitigation: Only trivial implementation exists; not exposed to user data yet
- Recommendations:
  - Adopt schema validation library (zod, valibot) for all input validation
  - Validate file uploads (image size, format) per NFR-207 (image generation requirements)
  - Validate user-supplied metadata for data injection attacks

**No Rate Limiting on API:**
- Risk: Per NFR-102, external API retry logic is implemented in design but no local rate limiting exists to prevent abuse of image generation or cover art APIs
- Files: All planned API endpoints; currently only `apps/dj/server/api/greet.ts` stub exists
- Current mitigation: Single-user scope limits practical attack surface
- Recommendations:
  - Implement rate limiting per user (IP-based for v1, user-based for v3)
  - Add request throttling for external API calls to prevent quota exhaustion

**No HTTPS/TLS Configuration:**
- Risk: `nuxt.config.ts` specifies `host: 'localhost'` and port 4200 with no TLS; v3 multi-user will require encrypted transport
- Files: `apps/dj/nuxt.config.ts` (lines 8-11)
- Current mitigation: Single-user localhost-only deployment in v1
- Recommendations:
  - Document TLS requirements for production deployment
  - Plan reverse proxy (nginx) configuration for v3 multi-user release

## Performance Bottlenecks

**No Caching Strategy:**
- Problem: Application has no caching layer for cover art, Spotify metadata, or image generation results
- Files: All planned API endpoints; currently minimal
- Cause: Early-stage scaffold; comprehensive caching architecture not yet implemented despite being in ARCHITECTURE.md
- Improvement path:
  - Implement Redis caching for external API responses (cover art, Spotify metadata) per ARCHITECTURE.md data flow section
  - Cache generated images in MinIO with consistent hash-based keys
  - Add ETags and Cache-Control headers to reduce bandwidth

**Single-Instance Database Without Connection Pooling:**
- Problem: Nuxt/Nitro server has no database connection pooling; will create new connection per request
- Files: No database integration exists yet; will affect all future database-backed APIs
- Cause: Bootstrap phase; database layer not yet implemented
- Improvement path:
  - Implement connection pooling (pgbouncer or application-level with pgx) before NFR-103 multi-user constraints apply
  - Plan for connection limits per NFR-300 scalability requirements

**No Content Compression:**
- Problem: Nuxt dev server in `nuxt.config.ts` does not enable gzip/brotli compression
- Files: `apps/dj/nuxt.config.ts` (no compression config)
- Cause: Scaffold template; compression deferred to production deployment
- Improvement path:
  - Enable compression in Nitro middleware for API responses
  - Configure Nginx/reverse proxy compression for frontend assets

## Fragile Areas

**Component Import Auto-Detection Without Barrel Files:**
- Files: `apps/dj/app/components/` (single component NxWelcome.vue), `apps/dj/nuxt.config.ts` (line 19: `autoImport: true`)
- Why fragile: Nuxt auto-imports all components from `app/components/`, making them globally available. As component library grows, naming collisions and implicit dependencies become harder to track. Currently masked by single large component
- Safe modification:
  - When adding new components, establish naming conventions (e.g., `Feature{Name}.vue`, `Base{Name}.vue`)
  - Create explicit barrel files (`components/index.ts`) to document public component API
  - Periodically audit unused imported components
- Test coverage: No component unit tests exist; only E2E scaffold test

**Hardcoded Development Server Configuration:**
- Files: `apps/dj/nuxt.config.ts` (lines 8-11)
- Why fragile: Server config hardcoded to `localhost:4200`. Docker Compose integration or CI/CD will break if port conflicts exist or hostname needs to change
- Safe modification:
  - Extract host/port to environment variables with defaults
  - Use `process.env.HOST` and `process.env.PORT` with fallbacks
- Test coverage: No environment configuration tests

**TypeScript Path Alias Complexity:**
- Files: `tsconfig.base.json` (no custom paths defined), `apps/dj/nuxt.config.ts` (lines 22-24: Vite nx-tsconfig-paths plugin), `apps/dj/vitest.config.ts` (no path alias config)
- Why fragile: Nx TypeScript path plugin assumes `.nuxt/tsconfig.json` extends `../../../tsconfig.base.json` (line 15 comment: "Nuxt copies this string as-is"). Relative path fragile to directory restructuring
- Safe modification:
  - Consider using absolute path aliases (@app, @server, @components) documented in project README
  - Test path aliases in isolated test suite before refactoring directory structure
- Test coverage: No path resolution tests

**Vue 3 Setup Script Language Attribute Handling:**
- Files: `apps/dj/app/app.vue` (line 1: `<script setup lang="ts">`), `apps/dj/app/components/NxWelcome.vue` (line 1 same pattern)
- Why fragile: All components use `<script setup>` with TypeScript. Changes to Vue compiler config or Vite plugin order can silently break template type checking
- Safe modification:
  - Validate `vue-tsc` type checking before shipping: `vue-tsc --noEmit` in CI
  - Document that all `.vue` files require `lang="ts"` in CONVENTIONS.md (not yet written)
- Test coverage: No standalone Vue component tests (vitest with @vue/test-utils)

## Scaling Limits

**Single-Node MinIO Object Storage Without Replication:**
- Current capacity: Per NFR-104, single-drive erasure-coding-disabled mode
- Limit: Single disk failure = data loss. No cross-zone or cross-node redundancy
- Scaling path:
  - For v1 (single-user): Named Docker volume is acceptable; document backup strategy
  - For v3 (multi-user): Migrate to multi-node MinIO cluster with replication per ARCHITECTURE.md §9 "Deployment Architecture"
  - Implement backup/restore scripts per NFR-107

**Database without Replication or Standby:**
- Current capacity: Single PostgreSQL 16 instance; default durability (fsync=on, synchronous_commit=on)
- Limit: Per NFR-105, unclean shutdowns lose in-progress operations. No HA failover
- Scaling path:
  - For v1: Adequate for single-user; WAL replay handles crashes
  - For v3: Implement streaming replication with hot standby per ARCHITECTURE.md multi-user guardrails
  - Add pgBackRest or pg_dump automation for compliance with NFR-107 backup requirements

**Memory Consumption Without Monitoring:**
- Current capacity: Baseline "8-16 GB RAM"; no memory limits configured in docker-compose.yml
- Limit: Image generation (especially large batches) can exhaust heap; no metrics to detect
- Scaling path:
  - Add memory profiling to image generation pipeline
  - Configure Docker memory limits and OOM handlers
  - Implement APM/monitoring for v3 multi-user per ARCHITECTURE.md cross-cutting concerns

## Dependencies at Risk

**Nuxt 4.0 with Ongoing Plugin Migration:**
- Risk: `package.json` pins `"nuxt": "^4.0.0"` (line 36). Nuxt 4 has ongoing breaking changes in edge releases; auto-module-discovery may change
- Impact: Auto-imports (`nuxt.config.ts` line 19) and plugin discovery may break between patch versions
- Migration plan:
  - Pin to specific minor version `nuxt: "^4.0.x"` after v1 release stabilizes
  - Test upgrade path from 4.0 to 4.1+ before committing to minor auto-upgrades
  - Monitor Nuxt 4 migration guide for auto-import deprecations

**Vue 3.5.13 with Potential Minor Breaking Changes:**
- Risk: `"vue": "^3.5.13"` may introduce non-breaking but behavioral changes in patch versions
- Impact: Component rendering or reactivity edge cases may shift
- Migration plan:
  - Pin to stable 3.5.x; defer major upgrades to v2 roadmap
  - Ensure test coverage (currently minimal: only E2E scaffold test) to catch behavior regressions

**Vite 7.0 with SWC/Transpilation Defaults:**
- Risk: `"vite": "^7.0.0"` introduced SWC as default transpiler; configuration may differ from previous Esbuild
- Impact: Build performance may vary; type-checking behavior may diverge
- Migration plan:
  - Test build performance before production release
  - Verify TypeScript type checking aligns between `tsc` and Vite's SWC transpilation

**Testing Framework Fragmentation:**
- Risk: Using both Vitest (unit/integration) and Playwright (E2E) with separate configs that may diverge
- Files: `apps/dj/vitest.config.ts`, `apps/dj-e2e/playwright.config.ts`
- Impact: Inconsistent test environment setup; difficult to share fixtures between unit and E2E tests
- Migration plan:
  - Document test strategy in TESTING.md (not yet written)
  - Create shared test fixtures in `/tests/fixtures/` before test suite grows

## Missing Critical Features

**No Actual Application Code:**
- Problem: Entire KlubHub DJ business logic is missing (Spotify parsing, image generation, gig management, social posting)
- Blocks: Every planned feature from ARCHITECTURE.md, BRD, NFR documentation

**No Error Boundary or Global Error Handler:**
- Problem: Frontend has no error boundary component; uncaught exceptions will crash page
- Files: `apps/dj/app/app.vue` (no Nuxt error boundary or layout error fallback)
- Blocks: Proper error handling for failed API requests, image generation errors, social post failures

**No Environment Configuration:**
- Problem: Hardcoded `localhost:4200` in `apps/dj/nuxt.config.ts`; no `.env` file handling for database credentials, API keys, MinIO endpoints
- Files: `apps/dj/nuxt.config.ts` (no env var references)
- Blocks: Multi-environment deployment (dev, staging, production)

**No Logging or Monitoring:**
- Problem: No structured logging setup; no APM or error tracking (Sentry, etc.)
- Files: No log setup anywhere in codebase
- Blocks: Debugging production issues; monitoring NFR-101 external API failures

## Test Coverage Gaps

**No Unit Tests for Server API:**
- What's not tested: `apps/dj/server/api/greet.ts` has no unit tests
- Files: `apps/dj/server/api/greet.ts` (10 lines)
- Risk: Even trivial API changes could introduce bugs; parameter validation not tested
- Priority: High (should establish pattern before adding real endpoints)

**No Vue Component Unit Tests:**
- What's not tested: `apps/dj/app/components/NxWelcome.vue` (881 lines) has zero unit tests
- Files: `apps/dj/app/app.vue`, `apps/dj/app/pages/index.vue`, `apps/dj/app/components/NxWelcome.vue`
- Risk: Template logic, prop validation, styling changes can regress undetected
- Priority: High (currently only covered by E2E scaffold test which is brittle)

**Insufficient E2E Test Coverage:**
- What's not tested: Only one scaffold test (`apps/dj-e2e/src/example.spec.ts` 8 lines) checking for "Welcome" text; doesn't test actual functionality
- Files: `apps/dj-e2e/src/example.spec.ts`
- Risk: No coverage of user workflows (e.g., parsing, image generation, social posting); no navigation testing
- Priority: Medium (E2E tests should cover feature flows once implemented)

**No Accessibility Testing:**
- What's not tested: ARIA labels, keyboard navigation, contrast ratios (esp. critical per NFR-207 Unicode/accessibility requirements)
- Files: All Vue components
- Risk: Violates WCAG guidelines; impacts DJ users with accessibility needs
- Priority: High (plan a11y testing before v1 release per NFR-207)

**No Integration Test Layer:**
- What's not tested: API ↔ database, Nuxt ↔ Nitro server integration, external API mocking
- Files: No integration test files
- Risk: Component tests pass but system-level behavior broken
- Priority: Medium (establish pattern for NFR-102 retry logic, NFR-103 durability tests)

---

*Concerns audit: 2026-03-14*
