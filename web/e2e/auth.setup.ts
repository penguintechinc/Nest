import { test as setup } from '@playwright/test';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const authFile = path.join(__dirname, '.auth', 'user.json');

setup('authenticate', async ({ page }) => {
  await page.goto('/login');

  // Fill tenant field (id="tenant")
  await page.fill('#tenant', 'nest');

  // Fill API token field (id="token")
  await page.fill('#token', 'test-token-12345');

  // Click submit button
  await page.click('button[type="submit"]');

  // Wait for redirect to dashboard
  await page.waitForURL('**/dashboard', { timeout: 15000 });

  await page.context().storageState({ path: authFile });
});
