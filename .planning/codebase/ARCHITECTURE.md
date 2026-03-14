# Architecture

**Analysis Date:** 2026-03-14

## Pattern Overview

**Overall:** Monorepo with server-side rendered frontend (Nuxt 4 / Vue 3) managed by Nx.

**Key Characteristics:**
- Monorepo-based development using pnpm workspaces and Nx orchestration
- Full-stack application with frontend (Nuxt app) and optional backend server endpoints
- TypeScript-first with strict type checking enabled
- Component-driven UI architecture using Vue 3 Single File Components
- Server-side routing handled by Nuxt file-based routing system
- API endpoints defined in server directory using h3 event handlers

## Layers

**Presentation Layer:**
- Purpose: Vue 3 components and pages rendering the user interface
- Location: `apps/dj/app/`
- Contains: Vue SFC files (.vue), page routing, component definitions
- Depends on: Vue 3, Nuxt runtime, CSS styles
- Used by: Browser clients, end users

**Routing Layer:**
- Purpose: Automatic file-based routing provided by Nuxt
- Location: `apps/dj/app/pages/` and `apps/dj/app/app.vue` (root layout)
- Contains: Page components and layout wrappers
- Depends on: Nuxt routing engine
- Used by: Presentation layer for page navigation

**Server API Layer:**
- Purpose: Backend HTTP endpoints for server-side logic
- Location: `apps/dj/server/api/`
- Contains: H3 event handlers for API routes
- Depends on: h3 framework, request/response handling
- Used by: Frontend components for data fetching

**Build & Tooling Layer:**
- Purpose: Project configuration, task execution, code quality
- Location: Root config files (nx.json, nuxt.config.ts, vitest.config.ts, eslint.config.mjs)
- Contains: Build system setup, linting rules, test configuration
- Depends on: Nx, Vite, Vitest, ESLint, TypeScript
- Used by: Development workflow, CI/CD pipelines

**Assets Layer:**
- Purpose: Static files and global styles
- Location: `apps/dj/app/assets/` and `apps/dj/public/`
- Contains: CSS stylesheets and static resources
- Depends on: None
- Used by: Presentation layer for styling

## Data Flow

**Page Load Flow:**

1. User requests URL from browser
2. Nuxt routing system matches URL to page component in `apps/dj/app/pages/`
3. Layout wrapper (`apps/dj/app/app.vue`) renders with `<nuxt-page/>` outlet
4. Page component renders with UI elements
5. Browser displays rendered HTML with global CSS

**API Request Flow:**

1. Frontend component imports/calls API endpoint
2. Browser makes HTTP request to server endpoint (e.g., `/api/greet`)
3. h3 event handler in `apps/dj/server/api/` processes request
4. Handler extracts parameters and returns JSON response
5. Frontend receives response and updates component state

**Component Composition:**

1. Root layout: `apps/dj/app/app.vue` provides navigation header and routing outlet
2. Pages consume layout: `apps/dj/app/pages/index.vue`, `apps/dj/app/pages/about.vue`
3. Page imports or inline defines components: `apps/dj/app/components/NxWelcome.vue`
4. Components compose HTML, styles, and TypeScript logic

**State Management:**

- No explicit state management library detected
- Component-local state managed via Vue's `<script setup>` composition API
- Props passed from parent to child components: `NxWelcome` accepts `title` prop
- No global stores or centralized state container in current codebase

## Key Abstractions

**Nuxt App Instance:**
- Purpose: SSR framework providing routing, rendering, and server endpoints
- Examples: `apps/dj/nuxt.config.ts` defines Nuxt configuration
- Pattern: Configuration-driven setup with Nuxt composables (e.g., `defineNuxtConfig`)

**Vue Components (SFC):**
- Purpose: Reusable UI building blocks with integrated logic and styles
- Examples: `apps/dj/app/components/NxWelcome.vue`, `apps/dj/app/pages/*.vue`
- Pattern: Single File Components with `<script setup>`, `<template>`, `<style scoped>`

**H3 Event Handlers:**
- Purpose: Type-safe backend route handlers
- Examples: `apps/dj/server/api/greet.ts`
- Pattern: `defineEventHandler((event) => {})` wrapping request/response logic

**Nx Project Configuration:**
- Purpose: Define build targets, dependencies, and task configuration
- Examples: Inferred from plugin rules (Nuxt, Playwright, Vitest plugins)
- Pattern: Plugin-driven task inference via `nx.json`

## Entry Points

**Development Server:**
- Location: `apps/dj/nuxt.config.ts`
- Triggers: `pnpm nx serve dj` or `pnpm nx run @dev/dj:serve`
- Responsibilities: Loads Nuxt app, starts dev server on localhost:4200, enables HMR

**Production Build:**
- Location: `apps/dj/nuxt.config.ts` (build output)
- Triggers: `pnpm nx build dj`
- Responsibilities: Bundles app with Vite, outputs to `dist/`

**Browser Entry:**
- Location: `apps/dj/app/app.vue`
- Triggers: User navigates to application URL
- Responsibilities: Renders root layout with navigation header and routing outlet

**E2E Test Entry:**
- Location: `apps/dj-e2e/playwright.config.ts`
- Triggers: `pnpm nx e2e dj-e2e`
- Responsibilities: Starts test server, runs Playwright tests against localhost:4200

## Error Handling

**Strategy:** Synchronous request handling with implicit error propagation.

**Patterns:**
- Server handlers use h3 utilities (`getQuery`, `defineEventHandler`) which throw on invalid input
- Frontend components render without explicit error boundaries
- No try-catch blocks detected in current sample code
- Nuxt provides default error page for unhandled runtime errors

## Cross-Cutting Concerns

**Logging:** No explicit logging framework detected; uses browser console and server stdout

**Validation:** Query parameter extraction via h3's `getQuery()` utility; no schema validation library

**Authentication:** Not implemented; no auth middleware or providers configured

**Type Safety:** Enforced via TypeScript strict mode (`strict: true` in tsconfig.base.json), `noUnusedLocals`, `noImplicitReturns`

**Module Boundaries:** Nx enforces module boundaries via `@nx/enforce-module-boundaries` ESLint rule; currently allows all tags to depend on all tags

---

*Architecture analysis: 2026-03-14*
