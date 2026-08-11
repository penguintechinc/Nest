import { defineConfig, devices } from '@playwright/test';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  // Keep all artifacts (traces, screenshots, HTML report) out of the repo tree —
  // a report written in-tree is how web/playwright-report/ got committed before.
  outputDir: '/tmp/playwright-nest/results',
  reporter: [
    ['html', { open: 'never', outputFolder: '/tmp/playwright-nest/report' }],
    ['list'],
  ],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  // Build the SPA and serve it with vite preview before the suite runs.
  // Not web/src/server.js: that Express BFF's deps (express et al.) live in the ROOT
  // package, and it serves ./dist relative to cwd — neither holds when run from web/.
  webServer: {
    command: 'npm run build && npm run preview -- --port 3000 --strictPort',
    url: 'http://localhost:3000',
    timeout: 120_000,
    reuseExistingServer: !process.env.CI,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
