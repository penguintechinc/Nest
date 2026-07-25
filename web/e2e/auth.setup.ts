import { test as setup } from '@playwright/test';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const authFile = path.join(__dirname, '.auth', 'user.json');

setup('authenticate', async ({ page }) => {
  await page.goto('/login');

  // Fill tenant field (id="tenant")
  await page.fill('#tenant', 'nest');

  // Interactive login is email + password; API keys are never entered here.
  await page.fill('#email', 'user@example.com');
  await page.fill('#password', 'test-password-12345');

  // Click submit button
  await page.click('button[type="submit"]');

  // Wait for redirect to dashboard
  await page.waitForURL('**/dashboard', { timeout: 15000 });

  await page.context().storageState({ path: authFile });
});
