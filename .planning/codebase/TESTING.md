# Testing Patterns

**Analysis Date:** 2026-03-14

## Test Framework

**Runner:**
- Vitest 4.0.8 (configured in `apps/dj/vitest.config.ts`)
- Environment: jsdom (for DOM/component testing)
- Watch mode: disabled by default
- Global test utilities enabled

**Assertion Library:**
- Built into Vitest (compatible with Jest-style assertions)
- Playwright test assertions via `@playwright/test` (for E2E)

**Run Commands:**
```bash
pnpm exec nx run @dev/dj:test          # Run unit tests
pnpm exec nx run @dev/dj-e2e:e2e       # Run E2E tests with Playwright
nx test @dev/dj                         # Alternative: test via Nx
nx test @dev/dj --watch               # Watch mode (if enabled)
```

## Test File Organization

**Location:**
- Unit/component tests: co-located with source (same directory as code)
- Placement pattern: `{src,tests}/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}`
- E2E tests: `apps/dj-e2e/src/` directory (separate app)

**Naming:**
- `.test.ts` or `.spec.ts` suffixes (both supported)
- Example: `/Users/admin/dev/klubhub-dj/apps/dj-e2e/src/example.spec.ts`
- TypeScript files only (no JavaScript test files currently)

**Structure:**
```
apps/dj-e2e/
├── src/
│   └── example.spec.ts          # Test files
├── playwright.config.ts         # E2E config
└── tsconfig.json
```

## Test Structure

**Suite Organization:**
```typescript
// From apps/dj-e2e/src/example.spec.ts
import { test, expect } from '@playwright/test';

test('has title', async ({ page }) => {
  await page.goto('/');
  expect(await page.locator('h1').innerText()).toContain('Welcome');
});
```

**Patterns:**
- Single `test()` function per assertion (simple structure)
- Descriptive test names explaining what's being tested
- Arrow functions with async/await for async operations
- Page fixture provided by Playwright automatically
- Locator-based element selection: `page.locator(selector)`

## Mocking

**Framework:** Vitest has built-in mocking via `vi` module (not shown in current examples)

**What to Mock:**
- External API calls (not implemented yet)
- Nuxt auto-imports (handled by framework)
- File system operations (when needed)

**What NOT to Mock:**
- Vue component rendering (use real components)
- Router navigation (test real routing)
- Basic DOM operations

## E2E Testing with Playwright

**Configuration:**
- File: `apps/dj-e2e/playwright.config.ts`
- Base URL: `http://localhost:4200` (development server)
- Trace collection: enabled on first retry (`trace: 'on-first-retry'`)
- Dev server: auto-started with `pnpm exec nx run @dev/dj:serve-static`
- Server reuse: enabled (`reuseExistingServer: true`)

**Browser Coverage:**
```typescript
projects: [
  { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  { name: 'firefox', use: { ...devices['Desktop Firefox'] } },
  { name: 'webkit', use: { ...devices['Desktop Safari'] } },
]
// Mobile and branded browsers available but commented out
```

**Test Pattern:**
```typescript
test('description', async ({ page }) => {
  await page.goto('/');                          // Navigate
  expect(await page.locator('selector')).toContain('text');  // Assert
});
```

## Fixtures and Factories

**Test Data:**
- Not currently used - tests use live application state
- No factory or fixture patterns implemented yet

**Location:**
- When needed, place in `tests/fixtures/` directory
- Consider implementing for complex test scenarios

## Coverage

**Requirements:** Not enforced currently

**View Coverage:**
```bash
# Coverage configured but not shown in run commands
# To enable: pnpm exec nx run @dev/dj:test --coverage
# Reports directory: apps/dj/test-output/vitest/coverage
# Provider: v8
```

**Configuration:**
```typescript
// From apps/dj/vitest.config.ts
coverage: {
  reportsDirectory: './test-output/vitest/coverage',
  provider: 'v8' as const,
}
```

## Unit Test Setup (Vitest)

**Configuration:**
- File: `apps/dj/vitest.config.ts`
- Root: app directory (from `import.meta.dirname`)
- Cache dir: `../../node_modules/.vite/apps/dj`
- Vue plugin enabled for component testing
- JSX source: Vue
- JSON module resolution enabled

**TypeScript Support:**
- File: `apps/dj/tsconfig.spec.json`
- Includes vitest globals in scope (no import needed)
- JSX preservation with Vue source
- Extends base tsconfig for consistency

**Test Include Patterns:**
```typescript
include: [
  '{src,tests}/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'
]
```

## Test Types

**Unit Tests:**
- Scope: Individual functions and components
- Approach: Not yet implemented (structure is in place)
- Environment: jsdom with Vue Test Utils support
- Use case: Testing API endpoints, component logic

**Integration Tests:**
- Scope: Multiple components or API + data interactions
- Approach: Not yet implemented
- Would use Vitest with real component mounting

**E2E Tests:**
- Framework: Playwright 1.36.0
- Scope: Full user workflows across the application
- Example implemented: `apps/dj-e2e/src/example.spec.ts`
- Approach: Browser-based testing with real server
- Currently tests: Page navigation and element visibility

## Common Patterns

**Async Testing:**
- E2E tests use `async/await` with Playwright page methods
- `await page.goto()` waits for navigation
- `await page.locator().innerText()` waits for element

**Error Testing:**
- Not yet implemented
- Suggestion: Test error boundaries and fallback states when added

**Nuxt-Specific Testing:**
- Auto-import system: test components without explicit imports
- Router: tests use real routing via page.goto()
- Composables: can be tested with `useAsyncData`, `useFetch` when used

## Reporter Configuration

**Default Reporter:**
```typescript
reporters: ['default']  // From vitest.config.ts
```

Output goes to console, can be extended with:
- 'verbose' for detailed output
- 'dot' for minimal output
- 'html' for HTML reports

## CI/CD Integration

**Via Nx:**
- Tests run through Nx task system
- Integrated with workspace linting and building
- GitHub Actions compatible (via Playwright)
- Coverage reports stored in `test-output/vitest/coverage`

---

*Testing analysis: 2026-03-14*
