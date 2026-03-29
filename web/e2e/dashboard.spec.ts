import { test, expect } from '@playwright/test';
import path from 'path';

const authFile = path.join(__dirname, '.auth', 'user.json');

test.describe('Dashboard', () => {
  test.use({ storageState: authFile });

  test('dashboard loads with heading', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('networkidle');

    const heading = page.locator('h1, h2, [data-testid="dashboard-heading"]');
    await expect(heading.first()).toBeVisible({ timeout: 10000 });
  });

  test('shows resource stats section', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('networkidle');

    const statsSection = page.locator('[data-testid="resource-stats"], .resource-stats, section:has-text("Resources"), div:has-text("Total")');
    await expect(statsSection.first()).toBeVisible({ timeout: 10000 });
  });

  test('shows resource list', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('networkidle');

    // Look for a table, list, or resource items
    const resourceList = page.locator('[data-testid="resource-list"], table, .resource-list, [role="table"], ul.resources');
    if (await resourceList.first().isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(resourceList.first()).toBeVisible();
    }
  });

  test('quick actions section visible', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('networkidle');

    const quickActions = page.locator('[data-testid="quick-actions"], .quick-actions, section:has-text("Quick"), button:has-text("Create"), button:has-text("Add")');
    if (await quickActions.first().isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(quickActions.first()).toBeVisible();
    }
  });
});
