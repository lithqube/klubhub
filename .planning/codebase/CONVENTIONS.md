# Coding Conventions

**Analysis Date:** 2026-03-14

## Naming Patterns

**Files:**
- TypeScript API routes: `[name].ts` (e.g., `greet.ts` in `apps/dj/server/api/`)
- Vue components: PascalCase (e.g., `NxWelcome.vue`, `app.vue` for root)
- Pages: lowercase (e.g., `index.vue`, `about.vue` in `apps/dj/app/pages/`)
- Configuration files: descriptive names (e.g., `nuxt.config.ts`, `vitest.config.ts`)

**Functions:**
- Event handlers: camelCase with descriptive names (e.g., `defineEventHandler` from h3)
- Vue methods/functions: camelCase (e.g., `defineProps`)
- API endpoints: follow Nuxt file-based routing conventions

**Variables:**
- Query parameters/options: camelCase (e.g., `projectName`, `baseURL`)
- Constants: camelCase (e.g., `workspaceDir`, `devServer`)
- Short variable names allowed in scopes: single letters (e.g., `q` for query in `apps/dj/server/api/greet.ts`)

**Types:**
- Generic prop types: PascalCase (e.g., `string` primitives in `defineProps<{title: string}>()`)
- TypeScript strict mode enabled - all types must be explicit

## Code Style

**Formatting:**
- Prettier configured with `singleQuote: true`
- 2-space indentation (enforced via `.editorconfig`)
- UTF-8 charset
- Final newline required
- Trailing whitespace trimmed
- No max line length for markdown files

**Linting:**
- ESLint with Nx plugin configuration in `eslint.config.mjs`
- Base configs: `@nx/eslint-plugin` with TypeScript and JavaScript support
- Module boundary enforcement enabled via `@nx/enforce-module-boundaries`
- Files ignored: `dist`, `out-tsc`, `vitest.config.*.timestamp*`, `test-output`

**Linting Rules:**
```javascript
// From eslint.config.mjs
- '@nx/enforce-module-boundaries': enforces buildable lib dependency constraints
- Module tags allow all dependencies (wildcard '*' tags used)
```

## Import Organization

**Order:**
1. External packages (e.g., `import { defineConfig } from 'vite'`)
2. Nx/framework utilities (e.g., `import { nxViteTsPaths } from '@nx/vite/plugins/...`)
3. Vue/component imports (e.g., `import vue from '@vitejs/plugin-vue'`)
4. Local/relative imports (when used)

**Path Aliases:**
- `@dev/source` - Custom condition in `tsconfig.base.json` for monorepo
- Module resolution: `bundler` mode in strict TypeScript configuration
- Workspace: Uses pnpm workspace with Nx monorepo setup

## Error Handling

**Patterns:**
- No explicit error handling shown in example files
- API handlers use h3's `defineEventHandler` which handles execution context
- No try-catch blocks in current samples - framework handles exceptions
- Query parameter fallback pattern: `const projectName = q.name || 'World'` (null-coalescing style)

## Logging

**Framework:** Not detected in current codebase

**Patterns:**
- No structured logging framework present
- Console methods not found in examined files
- Consider implementing when backend API calls are added

## Comments

**When to Comment:**
- Inline comments used minimally
- Configuration files include links to external docs (e.g., Nuxt, Playwright docs)
- Vue components lack JSDoc but are self-documenting via template structure

**JSDoc/TSDoc:**
- Not currently used in codebase
- Could be added for exported functions and types in future

## Function Design

**Size:** Small, focused functions (API handlers typically 3-10 lines)

**Parameters:**
- Use object destructuring for multiple parameters
- Event handlers receive framework context (e.g., `event` from h3)
- Props use TypeScript generics: `defineProps<{ title: string }>()`

**Return Values:**
- API handlers return plain objects (converted to JSON by framework)
- Components return template structures (Vue)
- No explicit return statements in component scripts

## Module Design

**Exports:**
- API routes: `export default` the handler function
- Components: implicit export via Vue SFC syntax (`<script setup>`)
- Configuration: `export default` the config object

**Barrel Files:**
- Not used in current structure
- Each file is self-contained in its directory

## Vue-Specific Conventions

**Script Setup Syntax:**
- All examined Vue components use `<script setup lang="ts">` (modern pattern)
- No explicit component registration needed
- Props defined inline with TypeScript generics

**Template Structure:**
- Scoped styles with `<style scoped>` to prevent CSS leakage
- ID-based selectors for major sections (e.g., `#welcome`, `#hero`, `#commands`)
- Class-based styling for reusable patterns (e.g., `.shadow`, `.rounded`, `.button-pill`)

**Component Organization:**
- Root component: `app.vue` with `<NuxtLink>` navigation
- Page components: in `app/pages/` directory (auto-routed by Nuxt)
- Reusable components: in `app/components/` directory

## Strict TypeScript Settings

**Enforced:**
- `strict: true` - Full type safety
- `noImplicitReturns: true` - All code paths must return
- `noUnusedLocals: true` - No unused variables allowed
- `noImplicitOverride: true` - Override keyword required
- `noFallthroughCasesInSwitch: true` - Switch cases must break/return
- `isolatedModules: true` - Modules can be transpiled independently
- `target: es2022` - Modern JavaScript features available

---

*Convention analysis: 2026-03-14*
