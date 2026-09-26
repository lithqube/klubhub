import { defineConfig, devices } from '@playwright/test';
import { nxE2EPreset } from '@nx/playwright/preset';
import { workspaceRoot } from '@nx/devkit';

// For CI, set BASE_URL to a deployed promoter app.
const baseURL = process.env['BASE_URL'] || 'http://localhost:4400';

export default defineConfig({
  ...nxE2EPreset(__filename, { testDir: './src' }),
  use: {
    baseURL,
    trace: 'on-first-retry',
  },
  // Mock mode (no NUXT_PUBLIC_API_BASE): the Nuxt dev server serves /api/v1.
  webServer: {
    command: 'pnpm exec nx run @dev/promoter:serve',
    url: 'http://localhost:4400',
    reuseExistingServer: true,
    cwd: workspaceRoot,
    timeout: 180_000,
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] }, testIgnore: /\.mobile\.spec\.ts$/ },
    { name: 'mobile-chrome', use: { ...devices['Pixel 5'] }, testMatch: /\.mobile\.spec\.ts$/ },
  ],
});
