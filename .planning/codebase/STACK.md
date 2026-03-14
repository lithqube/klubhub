# Technology Stack

**Analysis Date:** 2026-03-14

## Languages

**Primary:**
- TypeScript 5.9.2 - All application code, server APIs, configuration
- Vue 3.5.13 - Frontend UI components and templates

**Secondary:**
- JavaScript - Build configuration and utilities (ESLint, Prettier)

## Runtime

**Environment:**
- Node.js (version not explicitly pinned; inferred from TypeScript 5.9 compatibility)

**Package Manager:**
- pnpm (lockfileVersion 9.0)
- Lockfile: `pnpm-lock.yaml` - Present

## Frameworks

**Core:**
- Nuxt 4.0.0 - Full-stack Vue framework for server-side rendering and routing
- Vue 3.5.13 - Reactive UI framework
- Vue Router 4.5.0 - Client-side routing
- h3 1.8.2 - Lightweight HTTP handler for Nuxt server endpoints

**Build & Development:**
- Vite 7.0.0 - Frontend build tool
- @nx/vite 22.5.4 - Nx plugin for Vite integration
- @vitejs/plugin-vue 6.0.1 - Vue support for Vite
- Nx 22.5.4 - Monorepo build system with workspace management

**Testing:**
- Vitest 4.0.8 - Unit and integration test runner
- @nx/vitest 22.5.4 - Nx plugin for Vitest integration
- @vitest/coverage-v8 4.0.0 - Code coverage reporting
- @playwright/test 1.36.0 - End-to-end testing framework
- @nx/playwright 22.5.4 - Nx plugin for Playwright integration
- @vue/test-utils 2.4.6 - Vue component testing utilities
- jsdom 27.1.0 - DOM environment for unit tests

**Linting & Formatting:**
- ESLint 9.8.0 - JavaScript/TypeScript linting
- @nx/eslint 22.5.4 - Nx ESLint plugin
- @nx/eslint-plugin 22.5.4 - Custom Nx rules
- @nuxt/eslint-config 1.10.0 - Nuxt-specific ESLint rules
- typescript-eslint 8.40.0 - TypeScript ESLint integration
- eslint-config-prettier 10.0.0 - Prettier integration for ESLint
- eslint-plugin-playwright 1.6.2 - Playwright test linting
- Prettier 3.6.2 - Code formatter with single quote configuration

**TypeScript & Type Checking:**
- typescript 5.9.2 - TypeScript compiler
- @types/node 20.19.9 - Node.js type definitions
- vue-tsc 2.2.8 - Vue component type checking
- @nx/js/typescript - Nx TypeScript plugin for type checking

**Development Tools:**
- @nuxt/devtools 3.0.0 - Nuxt development panel and debugging
- @nuxt/kit 4.0.0 - Nuxt module development toolkit
- @nuxt/ui-templates 1.3.1 - UI component templates
- @swc/core 1.15.5 - Fast JavaScript/TypeScript compiler
- @swc/helpers 0.5.18 - SWC runtime helpers
- @swc-node/register 1.11.1 - SWC Node.js loader
- tslib 2.3.0 - TypeScript runtime library
- @nx/devkit 22.5.4 - Nx development utilities
- @nx/workspace 22.5.4 - Nx workspace management
- @nx/js 22.5.4 - Nx JavaScript/TypeScript support
- @nx/nuxt 22.5.4 - Nx Nuxt plugin
- @nx-go/nx-go 3.3.1 - Nx Go language support (optional)

## Key Dependencies

**Critical:**
- Nuxt 4.0.0 - Core full-stack framework providing SSR, routing, and API integration
- Nx 22.5.4 - Monorepo orchestration and build optimization
- Vue 3.5.13 - Reactive rendering engine for all UI

**Infrastructure:**
- h3 1.8.2 - HTTP server for Nuxt API routes (used in `apps/dj/server/api/`)
- TypeScript 5.9.2 - Ensures type safety across entire codebase

## Configuration

**Environment:**
- Development server runs on `localhost:4200` by default (configurable in `apps/dj/nuxt.config.ts`)
- E2E tests configured to run against `BASE_URL` environment variable or default `http://localhost:4200`
- No `.env` file requirements detected in base configuration

**Build:**
- `nx.json` - Nx monorepo configuration with plugin definitions for TypeScript, ESLint, Vitest, Nuxt, and Playwright
- `tsconfig.base.json` - Base TypeScript configuration with strict mode, ES2022 target
- `apps/dj/nuxt.config.ts` - Nuxt application configuration with devtools enabled, TypeScript typecheck enabled, auto-imports enabled
- `apps/dj/vitest.config.ts` - Vitest configuration with jsdom environment, coverage via v8
- `apps/dj-e2e/playwright.config.ts` - Playwright configuration with Chromium, Firefox, and WebKit browsers
- `eslint.config.mjs` - Flat ESLint configuration with Nx module boundary enforcement
- `.prettierrc` - Prettier configuration with single quotes enabled

**TypeScript Compiler Options:**
- Target: ES2022
- Module: ESNext
- Module Resolution: Bundler
- Strict mode: Enabled
- Declaration maps: Enabled
- No unused locals: Error
- No implicit returns: Error
- No fallthrough switch cases: Error

## Platform Requirements

**Development:**
- Node.js runtime with pnpm package manager
- Modern terminal/shell for Nx commands
- Browsers supporting ES2022 for running applications

**Production:**
- Node.js runtime (inferred from Nuxt 4.0 requirements)
- No specific database or infrastructure dependencies detected
- Deployment via standard Node.js HTTP server

---

*Stack analysis: 2026-03-14*
