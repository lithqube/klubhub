# Codebase Structure

**Analysis Date:** 2026-03-14

## Directory Layout

```
klubhub-dj/
├── apps/                          # Nx workspace applications
│   ├── dj/                        # Main Nuxt application
│   │   ├── app/                   # Frontend application code
│   │   │   ├── pages/             # File-based routing pages
│   │   │   ├── components/        # Reusable Vue components
│   │   │   ├── assets/            # Global stylesheets
│   │   │   └── app.vue            # Root layout component
│   │   ├── server/                # Backend server routes
│   │   │   └── api/               # API endpoint handlers
│   │   ├── public/                # Static assets (favicon, etc)
│   │   ├── nuxt.config.ts         # Nuxt framework configuration
│   │   ├── vitest.config.ts       # Unit test configuration
│   │   ├── eslint.config.mjs      # ESLint rules for app
│   │   ├── tsconfig.json          # TypeScript references config
│   │   ├── tsconfig.app.json      # Application-specific TypeScript config
│   │   └── tsconfig.spec.json     # Test-specific TypeScript config
│   │
│   └── dj-e2e/                    # Playwright end-to-end tests
│       ├── src/                   # E2E test specifications
│       └── playwright.config.ts   # Playwright configuration
│
├── docs/                          # Project documentation
├── .planning/                     # GSD planning documents
│   └── codebase/                  # Codebase analysis documents
│
├── node_modules/                  # Installed dependencies (pnpm)
├── .nx/                           # Nx workspace metadata
├── .vscode/                       # VS Code workspace settings
│
├── package.json                   # Workspace root dependencies
├── pnpm-workspace.yaml            # pnpm workspace configuration
├── pnpm-lock.yaml                 # pnpm lock file
├── tsconfig.base.json             # Base TypeScript configuration
├── tsconfig.json                  # Root TypeScript references
├── nx.json                        # Nx workspace configuration
├── eslint.config.mjs              # Root ESLint configuration
├── vitest.workspace.ts            # Vitest workspace configuration
├── .editorconfig                  # Editor formatting rules
├── .prettierrc                    # Prettier formatting config
└── README.md                      # Workspace documentation
```

## Directory Purposes

**apps/dj:**
- Purpose: Full-stack Nuxt application serving as the main DJ platform
- Contains: Frontend pages, components, server API routes, configuration files
- Key files: `app/app.vue`, `nuxt.config.ts`, `app/pages/*`

**apps/dj/app:**
- Purpose: Frontend application code consumed by Nuxt
- Contains: Vue components, pages, global assets, layouts
- Key files: `app.vue` (root layout), `pages/index.vue`, `pages/about.vue`

**apps/dj/app/pages:**
- Purpose: File-based routing directory - each .vue file becomes a route
- Contains: Page components that match URL paths
- Key files: `index.vue` (root path `/`), `about.vue` (path `/about`)

**apps/dj/app/components:**
- Purpose: Reusable Vue components used across pages
- Contains: Standalone component definitions
- Key files: `NxWelcome.vue` (welcome/onboarding component)

**apps/dj/app/assets:**
- Purpose: Global application assets and stylesheets
- Contains: CSS files imported globally via `nuxt.config.ts`
- Key files: `css/styles.css`

**apps/dj/server:**
- Purpose: Backend server logic and API endpoints
- Contains: H3 event handlers for HTTP endpoints
- Key files: All files in `server/api/` become `/api/*` routes

**apps/dj/server/api:**
- Purpose: RESTful API endpoint definitions
- Contains: Individual route handlers using h3
- Key files: `greet.ts` (example endpoint accepting query parameters)

**apps/dj/public:**
- Purpose: Static files served at root URL
- Contains: Favicon, robots.txt, other public assets
- Key files: `favicon.ico`

**apps/dj-e2e:**
- Purpose: End-to-end testing suite using Playwright
- Contains: Integration tests for full application flows
- Key files: `src/example.spec.ts`

## Key File Locations

**Entry Points:**
- `apps/dj/app/app.vue`: Root layout component with navigation header and `<nuxt-page/>` outlet
- `apps/dj/nuxt.config.ts`: Nuxt framework initialization and configuration
- `apps/dj/app/pages/index.vue`: Home page (route: `/`)

**Configuration:**
- `nx.json`: Nx workspace task configuration and plugin setup
- `tsconfig.base.json`: Base TypeScript compiler options (strict mode, ES2022 target)
- `package.json`: Workspace root dependencies and metadata
- `pnpm-workspace.yaml`: pnpm workspace package declaration
- `vitest.workspace.ts`: Vitest test runner workspace configuration

**Core Logic:**
- `apps/dj/server/api/greet.ts`: Example API endpoint demonstrating request handling
- `apps/dj/app/components/NxWelcome.vue`: Reusable welcome component with prop-based customization
- `apps/dj/app/pages/about.vue`: Simple about page example

**Testing:**
- `apps/dj-e2e/src/example.spec.ts`: Playwright test verifying page title contains "Welcome"
- `apps/dj/vitest.config.ts`: Unit test configuration for Vitest
- `apps/dj-e2e/playwright.config.ts`: E2E test configuration with automatic server startup

## Naming Conventions

**Files:**
- Page components: Lowercase with hyphens for multi-word names, `.vue` extension (e.g., `index.vue`, `about.vue`)
- Reusable components: PascalCase for component definition (e.g., `NxWelcome.vue`)
- API routes: Lowercase with hyphens, `.ts` extension (e.g., `greet.ts` maps to `/api/greet`)
- Configuration: camelCase with `.config.` prefix (e.g., `nuxt.config.ts`, `eslint.config.mjs`)
- Tests: Pattern `*.spec.ts` or `*.test.ts`

**Directories:**
- Lowercase, single words preferred (e.g., `app`, `server`, `api`, `pages`, `components`)
- Multi-word directories: Lowercase with hyphens (e.g., `dj-e2e`)

**TypeScript:**
- Functions: camelCase (e.g., `defineEventHandler`, `getQuery`)
- Types/Interfaces: PascalCase (e.g., `Event`)
- Constants: UPPER_SNAKE_CASE or camelCase based on visibility
- Props interfaces: Convention not yet established in codebase

## Where to Add New Code

**New Frontend Feature/Page:**
- Primary code: `apps/dj/app/pages/[feature-name].vue`
- Reusable components: `apps/dj/app/components/[FeatureName].vue`
- Styles: Scoped within component `<style scoped>` or global in `apps/dj/app/assets/css/`
- Tests: Co-located as `pages/[feature-name].spec.ts`

**New API Endpoint:**
- Implementation: `apps/dj/server/api/[route-name].ts`
- Pattern: Wrap handler with `defineEventHandler()` and use h3 utilities (`getQuery`, `getBody`, etc)
- Route path: Derived from file path (e.g., `api/users/get.ts` becomes `POST /api/users/get`)

**New Component Library:**
- Use Nx generator: `pnpm nx g @nx/vue:lib [library-name]`
- Output: New directory under workspace root with own package.json and tsconfig
- Import path: Use TypeScript path aliases defined in `tsconfig.base.json`

**Utilities & Helpers:**
- Shared utils: Create new lib via Nx generator or add to `apps/dj/` if app-specific
- Server utilities: Place in `apps/dj/server/` as needed (not yet structured)

## Special Directories

**apps/dj/.nuxt:**
- Purpose: Generated Nuxt build artifacts and type declarations
- Generated: Yes (auto-generated by Nuxt on build/dev)
- Committed: No (in .gitignore)
- Contains: Cached outputs, TypeScript definitions for auto-imports

**apps/dj/dist:**
- Purpose: Production build output
- Generated: Yes (created by `pnpm nx build dj`)
- Committed: No (in .gitignore)
- Contains: Bundled JavaScript, CSS, and static assets

**apps/dj-e2e/test-output:**
- Purpose: Test execution results and coverage reports
- Generated: Yes (created by test runner)
- Committed: No (in .gitignore)

**node_modules:**
- Purpose: Installed npm dependencies
- Generated: Yes (created by pnpm install)
- Committed: No (in .gitignore)

---

*Structure analysis: 2026-03-14*
